package core

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/matthiaslubbertsen/melon/internal/secret"
	"github.com/matthiaslubbertsen/melon/internal/store"
	"github.com/matthiaslubbertsen/melon/internal/subjects"
)

// Detention is an extra hour at school the account added itself, such as
// detention ("nakomen"). Zermelo never hears about it; Melon puts it in the
// account's own schedule as a lesson of type "detention".
type Detention struct {
	ID     string  `json:"id"`
	Date   string  `json:"date"`
	Start  string  `json:"start"`
	End    string  `json:"end"`
	Period *Period `json:"period"`
	// Subject is the Zermelo subject code it is for, or empty.
	Subject  string `json:"subject"`
	Title    string `json:"title"`
	Location string `json:"location"`
}

type DetentionInput struct {
	Date     string  `json:"date"`
	Start    string  `json:"start"`
	End      string  `json:"end"`
	Period   *Period `json:"period"`
	Subject  string  `json:"subject"`
	Title    string  `json:"title"`
	Location string  `json:"location"`
}

func toDetention(d store.Detention) Detention {
	out := Detention{ID: d.ID, Date: d.Date, Start: d.Start, End: d.End, Subject: d.Subject, Title: d.Title, Location: d.Location}
	if d.PeriodFrom > 0 {
		out.Period = &Period{From: d.PeriodFrom, To: d.PeriodTo}
	}
	return out
}

func (s *Service) Detentions(ctx context.Context, p Principal, accountID string, from, to time.Time) ([]Detention, error) {
	a, _, err := s.account(ctx, p, accountID)
	if err != nil {
		return nil, err
	}
	rows, err := s.Store.Detentions(ctx, a.ID, from.Format(time.DateOnly), to.Format(time.DateOnly))
	if err != nil {
		return nil, err
	}
	out := make([]Detention, 0, len(rows))
	for _, d := range rows {
		out = append(out, toDetention(d))
	}
	return out, nil
}

func (s *Service) AddDetention(ctx context.Context, p Principal, accountID string, in DetentionInput) (Detention, error) {
	a, _, err := s.account(ctx, p, accountID)
	if err != nil {
		return Detention{}, err
	}
	bad := func(msg string) (Detention, error) {
		return Detention{}, Errf(http.StatusBadRequest, "invalid_request", "%s", msg)
	}
	if _, err := time.ParseInLocation(time.DateOnly, in.Date, s.Loc); err != nil {
		return bad("'date' must be a date like 2026-11-12.")
	}
	start, err1 := time.Parse("15:04", in.Start)
	end, err2 := time.Parse("15:04", in.End)
	if err1 != nil || err2 != nil {
		return bad("'start' and 'end' must be times like 15:30.")
	}
	if !end.After(start) {
		return bad("'end' must be after 'start'.")
	}
	d := store.Detention{
		ID: secret.ID("det"), AccountID: a.ID, Date: in.Date, Start: start.Format("15:04"), End: end.Format("15:04"),
		Subject:  strings.ToLower(strings.TrimSpace(in.Subject)),
		Title:    strings.TrimSpace(in.Title),
		Location: strings.TrimSpace(in.Location),
	}
	if in.Period != nil {
		if in.Period.From < 1 || in.Period.To < in.Period.From || in.Period.To > 20 {
			return bad("'period' must run from one period number (1 to 20) to the same or a later one.")
		}
		d.PeriodFrom, d.PeriodTo = in.Period.From, in.Period.To
	}
	if len(d.Subject) > 32 || len([]rune(d.Title)) > 80 || len([]rune(d.Location)) > 40 {
		return bad("'subject' can be at most 32 characters, 'title' 80 and 'location' 40.")
	}
	if err := s.Store.AddDetention(ctx, d); err != nil {
		return Detention{}, err
	}
	return toDetention(d), nil
}

func (s *Service) DeleteDetention(ctx context.Context, p Principal, accountID, id string) error {
	a, _, err := s.account(ctx, p, accountID)
	if err != nil {
		return err
	}
	err = s.Store.DeleteDetention(ctx, a.ID, id)
	if errors.Is(err, store.ErrNotFound) {
		return ErrNotFound
	}
	return err
}

// detentionLessons turns an account's detentions in [from, to] into lessons.
func (s *Service) detentionLessons(ctx context.Context, a store.Account, from, to time.Time, lang string, overrides map[string]string, global map[string]subjects.Name) ([]Lesson, error) {
	rows, err := s.Store.Detentions(ctx, a.ID, from.Format(time.DateOnly), to.Format(time.DateOnly))
	if err != nil {
		return nil, err
	}
	out := make([]Lesson, 0, len(rows))
	for _, d := range rows {
		day, err := time.ParseInLocation(time.DateOnly, d.Date, s.Loc)
		if err != nil {
			continue
		}
		at := func(hhmm string) time.Time {
			var h, m int
			fmt.Sscanf(hhmm, "%d:%d", &h, &m)
			return time.Date(day.Year(), day.Month(), day.Day(), h, m, 0, 0, s.Loc)
		}
		l := Lesson{
			ID: d.ID, Date: d.Date, Start: at(d.Start), End: at(d.End), Type: "detention",
			Subjects: []Subject{}, Teachers: []Teacher{}, Locations: []string{}, Groups: []string{},
			Status: "scheduled", Changes: []string{}, Previous: map[string]string{}, Note: d.Title,
		}
		if d.PeriodFrom > 0 {
			l.Period = &Period{From: d.PeriodFrom, To: d.PeriodTo}
		}
		if d.Location != "" {
			l.Locations = []string{d.Location}
		}
		switch {
		case d.Subject != "":
			name, _ := subjects.Resolve(d.Subject, lang, overrides, global)
			l.Subjects = []Subject{{Code: d.Subject, Name: name}}
			l.Title = name
		case d.Title != "":
			l.Title, l.Note = d.Title, ""
		case lang == "en":
			l.Title = "Detention"
		default:
			l.Title = "Nakomen"
		}
		out = append(out, l)
	}
	return out, nil
}
