package store

import (
	"context"
	"database/sql"
	"errors"
)

// Feedback is one suggestion or bug report: a thread between the profile that
// sent it and the admins.
type Feedback struct {
	ID        string
	UserID    string
	Kind      string // idea | bug
	Status    string // open | closed
	Page      string
	CreatedAt int64
	UpdatedAt int64
	// Preview is the first message, for lists.
	Preview string
	// Unread is true when the other side wrote since this side last looked.
	Unread   bool
	Messages int
}

type FeedbackMessage struct {
	ID        int64
	FromAdmin bool
	Author    string
	Body      string
	At        int64
}

const feedbackSchema = `
CREATE TABLE IF NOT EXISTS feedback (
  id TEXT PRIMARY KEY,
  user_id TEXT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
  kind TEXT NOT NULL, status TEXT NOT NULL DEFAULT 'open', page TEXT NOT NULL DEFAULT '',
  created_at INTEGER NOT NULL, updated_at INTEGER NOT NULL,
  -- The last message id each side has seen.
  user_seen INTEGER NOT NULL DEFAULT 0, admin_seen INTEGER NOT NULL DEFAULT 0
);
CREATE INDEX IF NOT EXISTS feedback_user ON feedback(user_id, updated_at);
CREATE TABLE IF NOT EXISTS feedback_messages (
  id INTEGER PRIMARY KEY AUTOINCREMENT,
  feedback_id TEXT NOT NULL REFERENCES feedback(id) ON DELETE CASCADE,
  from_admin INTEGER NOT NULL, author TEXT NOT NULL, body TEXT NOT NULL, at INTEGER NOT NULL
);
CREATE INDEX IF NOT EXISTS feedback_messages_thread ON feedback_messages(feedback_id, id);
`

// feedbackColumns reads a thread for either side: it is unread when the other
// side wrote a message after the last one this side has seen.
const feedbackColumns = `f.id, f.user_id, f.kind, f.status, f.page, f.created_at, f.updated_at,
	COALESCE((SELECT body FROM feedback_messages WHERE feedback_id = f.id ORDER BY id LIMIT 1), ''),
	EXISTS (SELECT 1 FROM feedback_messages m WHERE m.feedback_id = f.id AND m.from_admin = ? AND m.id > (CASE WHEN ? THEN f.admin_seen ELSE f.user_seen END)),
	(SELECT COUNT(*) FROM feedback_messages WHERE feedback_id = f.id)`

func scanFeedback(row interface{ Scan(...any) error }) (Feedback, error) {
	var f Feedback
	err := row.Scan(&f.ID, &f.UserID, &f.Kind, &f.Status, &f.Page, &f.CreatedAt, &f.UpdatedAt, &f.Preview, &f.Unread, &f.Messages)
	return f, err
}

// CreateFeedback opens a thread with its first message.
func (s *Store) CreateFeedback(ctx context.Context, id, userID, kind, page, body string) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	n := now()
	if _, err := tx.ExecContext(ctx, `INSERT INTO feedback (id, user_id, kind, page, created_at, updated_at) VALUES (?, ?, ?, ?, ?, ?)`, id, userID, kind, page, n, n); err != nil {
		return err
	}
	res, err := tx.ExecContext(ctx, `INSERT INTO feedback_messages (feedback_id, from_admin, author, body, at) VALUES (?, 0, ?, ?, ?)`, id, userID, body, n)
	if err != nil {
		return err
	}
	msg, _ := res.LastInsertId()
	if _, err := tx.ExecContext(ctx, `UPDATE feedback SET user_seen = ? WHERE id = ?`, msg, id); err != nil {
		return err
	}
	return tx.Commit()
}

// OpenFeedbackCount is how many open threads a profile has, to keep one
// profile from flooding the admins.
func (s *Store) OpenFeedbackCount(ctx context.Context, userID string) (int, error) {
	var n int
	err := s.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM feedback WHERE user_id = ? AND status = 'open'`, userID).Scan(&n)
	return n, err
}

// UserFeedback lists a profile's own threads, newest activity first.
func (s *Store) UserFeedback(ctx context.Context, userID string) ([]Feedback, error) {
	return s.listFeedback(ctx, `SELECT `+feedbackColumns+` FROM feedback f WHERE f.user_id = ? ORDER BY f.updated_at DESC LIMIT 100`, true, false, userID)
}

// AllFeedback lists every thread for the admins, filtered on status unless
// that is empty.
func (s *Store) AllFeedback(ctx context.Context, status string, limit int) ([]Feedback, error) {
	return s.listFeedback(ctx, `SELECT `+feedbackColumns+` FROM feedback f WHERE ? = '' OR f.status = ? ORDER BY f.updated_at DESC LIMIT ?`, false, true, status, status, limit)
}

func (s *Store) listFeedback(ctx context.Context, query string, otherIsAdmin, asAdmin bool, args ...any) ([]Feedback, error) {
	rows, err := s.db.QueryContext(ctx, query, append([]any{otherIsAdmin, asAdmin}, args...)...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Feedback{}
	for rows.Next() {
		f, err := scanFeedback(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, f)
	}
	return out, rows.Err()
}

// UnreadFeedback counts threads with news for the admins.
func (s *Store) UnreadFeedback(ctx context.Context) (int, error) {
	var n int
	err := s.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM feedback f WHERE EXISTS (SELECT 1 FROM feedback_messages m WHERE m.feedback_id = f.id AND m.from_admin = 0 AND m.id > f.admin_seen)`).Scan(&n)
	return n, err
}

// FeedbackThread loads one thread and its messages and marks it seen by the
// side asking. userID limits it to that profile's threads; admins pass "".
func (s *Store) FeedbackThread(ctx context.Context, id, userID string, asAdmin bool) (Feedback, []FeedbackMessage, error) {
	f, err := scanFeedback(s.db.QueryRowContext(ctx, `SELECT `+feedbackColumns+` FROM feedback f WHERE f.id = ? AND (? = '' OR f.user_id = ?)`, !asAdmin, asAdmin, id, userID, userID))
	if errors.Is(err, sql.ErrNoRows) {
		return f, nil, ErrNotFound
	} else if err != nil {
		return f, nil, err
	}
	rows, err := s.db.QueryContext(ctx, `SELECT id, from_admin, author, body, at FROM feedback_messages WHERE feedback_id = ? ORDER BY id`, id)
	if err != nil {
		return f, nil, err
	}
	defer rows.Close()
	msgs := []FeedbackMessage{}
	for rows.Next() {
		var m FeedbackMessage
		if err := rows.Scan(&m.ID, &m.FromAdmin, &m.Author, &m.Body, &m.At); err != nil {
			return f, nil, err
		}
		msgs = append(msgs, m)
	}
	if err := rows.Err(); err != nil {
		return f, nil, err
	}
	seen := "user_seen"
	if asAdmin {
		seen = "admin_seen"
	}
	if len(msgs) > 0 {
		_, err = s.db.ExecContext(ctx, `UPDATE feedback SET `+seen+` = MAX(`+seen+`, ?) WHERE id = ?`, msgs[len(msgs)-1].ID, id)
	}
	return f, msgs, err
}

// AddFeedbackMessage adds a reply. A reply from the user reopens a closed
// thread, so nothing they write goes unseen.
func (s *Store) AddFeedbackMessage(ctx context.Context, id, author string, fromAdmin bool, body string) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	var exists int
	if err := tx.QueryRowContext(ctx, `SELECT COUNT(*) FROM feedback WHERE id = ?`, id).Scan(&exists); err != nil {
		return err
	} else if exists == 0 {
		return ErrNotFound
	}
	n := now()
	res, err := tx.ExecContext(ctx, `INSERT INTO feedback_messages (feedback_id, from_admin, author, body, at) VALUES (?, ?, ?, ?, ?)`, id, fromAdmin, author, body, n)
	if err != nil {
		return err
	}
	msg, _ := res.LastInsertId()
	// Writing means having seen everything before it.
	update := `UPDATE feedback SET updated_at = ?, user_seen = ?, status = 'open' WHERE id = ?`
	if fromAdmin {
		update = `UPDATE feedback SET updated_at = ?, admin_seen = ? WHERE id = ?`
	}
	if err := affected(tx.ExecContext(ctx, update, n, msg, id)); err != nil {
		return err
	}
	return tx.Commit()
}

func (s *Store) SetFeedbackStatus(ctx context.Context, id, status string) error {
	return affected(s.db.ExecContext(ctx, `UPDATE feedback SET status = ? WHERE id = ?`, status, id))
}
