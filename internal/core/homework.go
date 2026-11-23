package core

import (
	"context"
	"errors"
	"net/http"
	"slices"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/matthiaslubbertsen/melon/internal/secret"
	"github.com/matthiaslubbertsen/melon/internal/store"
)

// MaxHomeworkLength caps one piece of homework, in characters.
const MaxHomeworkLength = 5000

// Homework is a note (Markdown) someone wrote on one of their own lessons.
// It hangs on the lesson's instance, so it follows the lesson when Zermelo
// moves it.
type Homework struct {
	ID        string    `json:"id"`
	Text      string    `json:"text"`
	Done      bool      `json:"done"`
	CreatedAt time.Time `json:"createdAt"`
	UpdatedAt time.Time `json:"updatedAt"`
	// Lesson is the lesson as Melon last saw it. Only lists outside a
	// schedule (GET .../homework) include it.
	Lesson *HomeworkLesson `json:"lesson,omitempty"`
}

// HomeworkLesson is what Melon remembers of the lesson a piece of homework is
// on. Status is the lesson's status, or "gone" when it disappeared from the
// schedule.
type HomeworkLesson struct {
	Instance string    `json:"instance"`
	Date     string    `json:"date"`
	Start    time.Time `json:"start"`
	End      time.Time `json:"end"`
	Title    string    `json:"title"`
	Subjects []string  `json:"subjects"`
	Status   string    `json:"status"`
}

func (s *Service) toHomework(h store.Homework, withLesson bool) Homework {
	out := Homework{ID: h.ID, Text: h.Text, Done: h.Done, CreatedAt: time.Unix(h.CreatedAt, 0).In(s.Loc), UpdatedAt: time.Unix(h.UpdatedAt, 0).In(s.Loc)}
	if withLesson {
		start := time.Unix(h.LessonStart, 0).In(s.Loc)
		out.Lesson = &HomeworkLesson{
			Instance: h.Instance, Date: start.Format(time.DateOnly), Start: start, End: time.Unix(h.LessonEnd, 0).In(s.Loc),
			Title: h.LessonTitle, Subjects: h.LessonSubjects, Status: h.LessonStatus,
		}
	}
	return out
}

// homeworkAccount loads an account that keeps homework. A parent's account
// is a child's schedule seen by the parent: parents have no homework.
func (s *Service) homeworkAccount(ctx context.Context, p Principal, accountID string) error {
	a, _, err := s.account(ctx, p, accountID)
	if err != nil {
		return err
	}
	if a.Role == "parent" {
		return Errf(http.StatusForbidden, "forbidden", "Parents have no homework in Melon.")
	}
	return nil
}

func cleanHomework(text string) (string, error) {
	text = strings.TrimSpace(strings.ReplaceAll(text, "\r\n", "\n"))
	if text == "" {
		return "", Errf(http.StatusBadRequest, "invalid_request", "Homework needs some text.")
	}
	if utf8.RuneCountInString(text) > MaxHomeworkLength {
		return "", Errf(http.StatusBadRequest, "invalid_request", "Homework can be at most %d characters.", MaxHomeworkLength)
	}
	return text, nil
}

func subjectCodes(l Lesson) []string {
	out := make([]string, 0, len(l.Subjects))
	for _, sub := range l.Subjects {
		out = append(out, strings.ToLower(sub.Code))
	}
	return out
}

// attachHomework puts the account's homework on its own lessons, and updates
// what Melon remembers of those lessons: where they are now, and whether they
// dropped out or vanished from the weeks just fetched.
func (s *Service) attachHomework(ctx context.Context, accountID string, out *Schedule, from, end time.Time) error {
	instances := make([]string, 0, len(out.Lessons))
	for _, l := range out.Lessons {
		instances = append(instances, l.Instance)
	}
	byInstance, err := s.Store.HomeworkFor(ctx, accountID, instances)
	if err != nil {
		return err
	}
	seen := map[string]bool{}
	for i := range out.Lessons {
		l := &out.Lessons[i]
		list := byInstance[l.Instance]
		l.Homework = make([]Homework, 0, len(list))
		for _, h := range list {
			l.Homework = append(l.Homework, s.toHomework(h, false))
		}
		if len(list) == 0 || seen[l.Instance] {
			continue
		}
		seen[l.Instance] = true
		h := list[0]
		subs := subjectCodes(*l)
		if h.LessonStart != l.Start.Unix() || h.LessonEnd != l.End.Unix() || h.LessonStatus != l.Status || h.LessonTitle != l.Title || !slices.Equal(h.LessonSubjects, subs) {
			if err := s.Store.SeenLesson(ctx, accountID, l.Instance, l.Start.Unix(), l.End.Unix(), l.Title, subs, l.Status); err != nil {
				return err
			}
		}
	}
	// Homework whose lesson was in these weeks but is not anymore lost its lesson.
	if out.Stale {
		return nil
	}
	inRange, err := s.Store.HomeworkBetween(ctx, accountID, from.Unix(), end.Unix())
	if err != nil {
		return err
	}
	for _, h := range inRange {
		if !seen[h.Instance] && h.LessonStatus != "gone" {
			if err := s.Store.LessonGone(ctx, accountID, h.ID); err != nil {
				return err
			}
		}
	}
	return nil
}

// HomeworkList is all homework on an account, by lesson time.
func (s *Service) HomeworkList(ctx context.Context, p Principal, accountID string) ([]Homework, error) {
	if err := s.homeworkAccount(ctx, p, accountID); err != nil {
		return nil, err
	}
	list, err := s.Store.HomeworkList(ctx, accountID)
	if err != nil {
		return nil, err
	}
	out := make([]Homework, 0, len(list))
	for _, h := range list {
		out = append(out, s.toHomework(h, true))
	}
	return out, nil
}

// findLesson looks up one of the account's own lessons on a date, by lesson
// id or instance.
func (s *Service) findLesson(ctx context.Context, p Principal, accountID, ref, date string) (Lesson, error) {
	day, err := time.ParseInLocation(time.DateOnly, date, s.Loc)
	if err != nil {
		return Lesson{}, Errf(http.StatusBadRequest, "invalid_request", "'date' must be the lesson's date, like 2026-11-12.")
	}
	sched, err := s.Schedule(ctx, p, accountID, ScheduleQuery{From: day, To: day})
	if err != nil {
		return Lesson{}, err
	}
	for _, l := range sched.Lessons {
		if l.ID == ref || l.Instance == ref {
			return l, nil
		}
	}
	return Lesson{}, Errf(http.StatusNotFound, "lesson_not_found", "There is no lesson %q on %s.", ref, date)
}

// nextLesson finds the first lesson after the given time that shares a
// subject with it and still takes place, within four weeks.
func (s *Service) nextLesson(ctx context.Context, p Principal, accountID string, after time.Time, subjects []string, skip string) (Lesson, error) {
	if len(subjects) == 0 {
		return Lesson{}, Errf(http.StatusNotFound, "lesson_not_found", "This homework has no subject to look for.")
	}
	from := after.In(s.Loc)
	if today := s.Today(); from.Before(today) {
		from = today
	}
	day := time.Date(from.Year(), from.Month(), from.Day(), 0, 0, 0, 0, s.Loc)
	sched, err := s.Schedule(ctx, p, accountID, ScheduleQuery{From: day, To: day.AddDate(0, 0, 27)})
	if err != nil {
		return Lesson{}, err
	}
	for _, l := range sched.Lessons {
		if l.Status == "cancelled" || l.Instance == skip || !l.Start.After(after) {
			continue
		}
		for _, c := range subjectCodes(l) {
			if slices.Contains(subjects, c) {
				return l, nil
			}
		}
	}
	return Lesson{}, Errf(http.StatusNotFound, "lesson_not_found", "There is no next lesson for this subject in the coming four weeks.")
}

func onLesson(h *store.Homework, l Lesson) {
	h.Instance = l.Instance
	h.LessonStart, h.LessonEnd = l.Start.Unix(), l.End.Unix()
	h.LessonTitle, h.LessonSubjects, h.LessonStatus = l.Title, subjectCodes(l), l.Status
}

// AddHomework puts a piece of homework on a lesson. lesson is the lesson's id
// or instance, date the day it is on.
func (s *Service) AddHomework(ctx context.Context, p Principal, accountID, lesson, date, text string) (Homework, error) {
	if err := s.homeworkAccount(ctx, p, accountID); err != nil {
		return Homework{}, err
	}
	text, err := cleanHomework(text)
	if err != nil {
		return Homework{}, err
	}
	l, err := s.findLesson(ctx, p, accountID, lesson, date)
	if err != nil {
		return Homework{}, err
	}
	h := store.Homework{ID: secret.ID("hw"), AccountID: accountID, Text: text}
	onLesson(&h, l)
	if err := s.Store.CreateHomework(ctx, h); err != nil {
		return Homework{}, err
	}
	h.CreatedAt, h.UpdatedAt = time.Now().Unix(), time.Now().Unix()
	return s.toHomework(h, true), nil
}

// HomeworkChange is a partial update. Move is "next" (the next lesson of the
// same subject) or a lesson id or instance, with Date the day it is on.
type HomeworkChange struct {
	Text *string
	Done *bool
	Move string
	Date string
}

func (s *Service) UpdateHomework(ctx context.Context, p Principal, accountID, id string, c HomeworkChange) (Homework, error) {
	if err := s.homeworkAccount(ctx, p, accountID); err != nil {
		return Homework{}, err
	}
	h, err := s.Store.Homework(ctx, accountID, id)
	if errors.Is(err, store.ErrNotFound) {
		return Homework{}, ErrNotFound
	} else if err != nil {
		return Homework{}, err
	}
	if c.Text != nil {
		if h.Text, err = cleanHomework(*c.Text); err != nil {
			return Homework{}, err
		}
	}
	if c.Done != nil {
		h.Done = *c.Done
	}
	switch {
	case c.Move == "next":
		l, err := s.nextLesson(ctx, p, accountID, time.Unix(h.LessonStart, 0), h.LessonSubjects, h.Instance)
		if err != nil {
			return Homework{}, err
		}
		onLesson(&h, l)
	case c.Move != "":
		l, err := s.findLesson(ctx, p, accountID, c.Move, c.Date)
		if err != nil {
			return Homework{}, err
		}
		onLesson(&h, l)
	}
	if err := s.Store.UpdateHomework(ctx, h); err != nil {
		return Homework{}, err
	}
	h.UpdatedAt = time.Now().Unix()
	return s.toHomework(h, true), nil
}

func (s *Service) DeleteHomework(ctx context.Context, p Principal, accountID, id string) error {
	if err := s.homeworkAccount(ctx, p, accountID); err != nil {
		return err
	}
	if err := s.Store.DeleteHomework(ctx, accountID, id); errors.Is(err, store.ErrNotFound) {
		return ErrNotFound
	} else {
		return err
	}
}
