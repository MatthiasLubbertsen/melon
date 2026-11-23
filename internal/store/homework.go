package store

import (
	"context"
	"strings"
)

// Homework is a note someone wrote on one of their lessons. It hangs on the
// lesson's Zermelo appointment instance, which stays the same when Zermelo
// moves the lesson, so the homework moves along with it.
//
// The lesson_* columns remember what the lesson last looked like, so homework
// can be listed (and found again when its lesson drops out) without asking
// Zermelo for every week.
type Homework struct {
	ID        string
	AccountID string
	Instance  string
	Text      string
	Done      bool
	CreatedAt int64
	UpdatedAt int64

	LessonStart    int64
	LessonEnd      int64
	LessonTitle    string
	LessonSubjects []string
	// LessonStatus is the lesson's status when Melon last saw it, or "gone"
	// when it disappeared from a week Melon looked at.
	LessonStatus string
}

const homeworkSchema = `
CREATE TABLE IF NOT EXISTS homework (
  id TEXT PRIMARY KEY,
  account_id TEXT NOT NULL REFERENCES accounts(id) ON DELETE CASCADE,
  instance TEXT NOT NULL,
  text TEXT NOT NULL,
  done INTEGER NOT NULL DEFAULT 0,
  lesson_start INTEGER NOT NULL, lesson_end INTEGER NOT NULL,
  lesson_title TEXT NOT NULL DEFAULT '', lesson_subjects TEXT NOT NULL DEFAULT '',
  lesson_status TEXT NOT NULL DEFAULT 'scheduled',
  created_at INTEGER NOT NULL, updated_at INTEGER NOT NULL
);
CREATE INDEX IF NOT EXISTS homework_account ON homework(account_id, lesson_start);
CREATE INDEX IF NOT EXISTS homework_instance ON homework(account_id, instance);
`

const homeworkColumns = `id, account_id, instance, text, done, lesson_start, lesson_end, lesson_title, lesson_subjects, lesson_status, created_at, updated_at`

func scanHomework(row interface{ Scan(...any) error }) (Homework, error) {
	var h Homework
	var subjects string
	err := row.Scan(&h.ID, &h.AccountID, &h.Instance, &h.Text, &h.Done, &h.LessonStart, &h.LessonEnd, &h.LessonTitle, &subjects, &h.LessonStatus, &h.CreatedAt, &h.UpdatedAt)
	h.LessonSubjects = split(subjects)
	return h, err
}

func (s *Store) homeworkRows(ctx context.Context, query string, args ...any) ([]Homework, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT `+homeworkColumns+` FROM homework `+query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Homework{}
	for rows.Next() {
		h, err := scanHomework(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, h)
	}
	return out, rows.Err()
}

// HomeworkList is every piece of homework on an account, by lesson time.
func (s *Store) HomeworkList(ctx context.Context, accountID string) ([]Homework, error) {
	return s.homeworkRows(ctx, `WHERE account_id = ? ORDER BY lesson_start, created_at`, accountID)
}

// HomeworkFor is the homework on the given lessons, keyed by instance.
func (s *Store) HomeworkFor(ctx context.Context, accountID string, instances []string) (map[string][]Homework, error) {
	out := map[string][]Homework{}
	if len(instances) == 0 {
		return out, nil
	}
	args := []any{accountID}
	for _, in := range instances {
		args = append(args, in)
	}
	list, err := s.homeworkRows(ctx, `WHERE account_id = ? AND instance IN (?`+strings.Repeat(",?", len(instances)-1)+`) ORDER BY created_at`, args...)
	if err != nil {
		return nil, err
	}
	for _, h := range list {
		out[h.Instance] = append(out[h.Instance], h)
	}
	return out, nil
}

// HomeworkBetween is the homework whose lesson was last seen in [from, to).
func (s *Store) HomeworkBetween(ctx context.Context, accountID string, from, to int64) ([]Homework, error) {
	return s.homeworkRows(ctx, `WHERE account_id = ? AND lesson_start >= ? AND lesson_start < ?`, accountID, from, to)
}

func (s *Store) Homework(ctx context.Context, accountID, id string) (Homework, error) {
	h, err := scanHomework(s.db.QueryRowContext(ctx, `SELECT `+homeworkColumns+` FROM homework WHERE account_id = ? AND id = ?`, accountID, id))
	return h, one(err)
}

func (s *Store) CreateHomework(ctx context.Context, h Homework) error {
	t := now()
	_, err := s.db.ExecContext(ctx, `INSERT INTO homework (`+homeworkColumns+`) VALUES (?,?,?,?,?,?,?,?,?,?,?,?)`,
		h.ID, h.AccountID, h.Instance, h.Text, h.Done, h.LessonStart, h.LessonEnd, h.LessonTitle, strings.Join(h.LessonSubjects, ","), h.LessonStatus, t, t)
	return err
}

// UpdateHomework saves the text, done flag and lesson of an existing entry.
func (s *Store) UpdateHomework(ctx context.Context, h Homework) error {
	return affected(s.db.ExecContext(ctx, `UPDATE homework SET instance = ?, text = ?, done = ?, lesson_start = ?, lesson_end = ?, lesson_title = ?, lesson_subjects = ?, lesson_status = ?, updated_at = ? WHERE account_id = ? AND id = ?`,
		h.Instance, h.Text, h.Done, h.LessonStart, h.LessonEnd, h.LessonTitle, strings.Join(h.LessonSubjects, ","), h.LessonStatus, now(), h.AccountID, h.ID))
}

// SeenLesson records what a lesson with homework looks like now. It does not
// touch updated_at: nobody changed the homework itself.
func (s *Store) SeenLesson(ctx context.Context, accountID, instance string, start, end int64, title string, subjects []string, status string) error {
	_, err := s.db.ExecContext(ctx, `UPDATE homework SET lesson_start = ?, lesson_end = ?, lesson_title = ?, lesson_subjects = ?, lesson_status = ? WHERE account_id = ? AND instance = ?`,
		start, end, title, strings.Join(subjects, ","), status, accountID, instance)
	return err
}

// LessonGone marks homework whose lesson is no longer in the schedule.
func (s *Store) LessonGone(ctx context.Context, accountID, id string) error {
	_, err := s.db.ExecContext(ctx, `UPDATE homework SET lesson_status = 'gone' WHERE account_id = ? AND id = ?`, accountID, id)
	return err
}

func (s *Store) DeleteHomework(ctx context.Context, accountID, id string) error {
	return affected(s.db.ExecContext(ctx, `DELETE FROM homework WHERE account_id = ? AND id = ?`, accountID, id))
}
