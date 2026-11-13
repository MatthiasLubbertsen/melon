package server

import (
	"net/http"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/matthiaslubbertsen/melon/internal/core"
	"github.com/matthiaslubbertsen/melon/internal/subjects"
)

func lang(r *http.Request) string {
	if r.URL.Query().Get("lang") == "en" {
		return "en"
	}
	return "nl"
}

func (s *Server) handleAccounts(w http.ResponseWriter, r *http.Request, p core.Principal) {
	accounts, err := s.Core.Accounts(r.Context(), p)
	if err != nil {
		writeErr(w, err)
		return
	}
	writeData(w, accounts)
}

func (s *Server) handleAccount(w http.ResponseWriter, r *http.Request, p core.Principal) {
	a, err := s.Core.Account(r.Context(), p, r.PathValue("account"))
	if err != nil {
		writeErr(w, err)
		return
	}
	writeData(w, a)
}

// parseRange reads from/to or week, defaulting to the current week.
func (s *Server) parseRange(from, to, week string) (time.Time, time.Time, error) {
	bad := func(msg string) (time.Time, time.Time, error) {
		return time.Time{}, time.Time{}, core.Errf(http.StatusBadRequest, "invalid_request", "%s", msg)
	}
	loc := s.Core.Loc
	today := s.Core.Today()
	monday := today.AddDate(0, 0, -((int(today.Weekday()) + 6) % 7))
	switch {
	case week != "":
		if from != "" || to != "" {
			return bad("Use either 'week' or 'from'/'to', not both.")
		}
		y, w, ok := strings.Cut(strings.ToUpper(week), "-W")
		year, err1 := strconv.Atoi(y)
		num, err2 := strconv.Atoi(w)
		if !ok || err1 != nil || err2 != nil || num < 1 || num > 53 || year < 2000 || year > 2100 {
			return bad("'week' must look like 2026-W46.")
		}
		jan4 := time.Date(year, 1, 4, 0, 0, 0, 0, loc)
		start := jan4.AddDate(0, 0, -((int(jan4.Weekday())+6)%7)+(num-1)*7)
		return start, start.AddDate(0, 0, 6), nil
	case from == "" && to == "":
		return monday, monday.AddDate(0, 0, 6), nil
	}
	parse := func(v string, fallback time.Time) (time.Time, bool) {
		switch v {
		case "":
			return fallback, true
		case "today":
			return today, true
		case "tomorrow":
			return today.AddDate(0, 0, 1), true
		}
		t, err := time.ParseInLocation(time.DateOnly, v, loc)
		return t, err == nil
	}
	f, ok := parse(from, today)
	if !ok {
		return bad("'from' must be a date like 2026-11-12.")
	}
	t, ok := parse(to, f)
	if !ok {
		return bad("'to' must be a date like 2026-11-12.")
	}
	return f, t, nil
}

type scheduleResponse struct {
	core.Schedule
	Summary scheduleSummary `json:"summary"`
}

type scheduleSummary struct {
	Lessons   int `json:"lessons"`
	Changed   int `json:"changed"`
	Cancelled int `json:"cancelled"`
}

func summarize(sched core.Schedule, changesOnly bool) scheduleResponse {
	out := scheduleResponse{Schedule: sched}
	kept := []core.Lesson{}
	for _, l := range sched.Lessons {
		out.Summary.Lessons++
		switch l.Status {
		case "changed":
			out.Summary.Changed++
		case "cancelled":
			out.Summary.Cancelled++
		}
		if !changesOnly || l.Status != "scheduled" {
			kept = append(kept, l)
		}
	}
	out.Lessons = kept
	return out
}

func (s *Server) handleSchedule(w http.ResponseWriter, r *http.Request, p core.Principal) {
	q := r.URL.Query()
	from, to, err := s.parseRange(q.Get("from"), q.Get("to"), q.Get("week"))
	if err != nil {
		writeErr(w, err)
		return
	}
	sched, err := s.Core.Schedule(r.Context(), p, r.PathValue("account"), core.ScheduleQuery{
		From: from, To: to, Person: q.Get("person"), Lang: lang(r), Fresh: q.Get("fresh") == "true",
	})
	if err != nil {
		writeErr(w, err)
		return
	}
	writeFetched(w, summarize(sched, q.Get("changesOnly") == "true"), sched.FetchedAt, sched.Stale)
}

func (s *Server) handlePeople(w http.ResponseWriter, r *http.Request, p core.Principal) {
	limit, _ := strconv.Atoi(r.URL.Query().Get("limit"))
	people, err := s.Core.People(r.Context(), p, r.PathValue("account"), r.URL.Query().Get("q"), limit)
	if err != nil {
		writeErr(w, err)
		return
	}
	writeData(w, people)
}

func (s *Server) handleAnnouncements(w http.ResponseWriter, r *http.Request, p core.Principal) {
	list, res, err := s.Core.Announcements(r.Context(), p, r.PathValue("account"))
	if err != nil {
		writeErr(w, err)
		return
	}
	writeFetched(w, list, res.FetchedAt, res.Stale)
}

func (s *Server) handleDefaultSubjects(w http.ResponseWriter, r *http.Request) {
	out := []core.Subject{}
	for code := range subjects.Defaults {
		name, _ := subjects.Resolve(code, lang(r), nil)
		out = append(out, core.Subject{Code: code, Name: name})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Code < out[j].Code })
	writeData(w, out)
}

func (s *Server) handleSubjects(w http.ResponseWriter, r *http.Request, p core.Principal) {
	list, err := s.Core.Subjects(r.Context(), p, r.PathValue("account"), lang(r))
	if err != nil {
		writeErr(w, err)
		return
	}
	writeData(w, list)
}

func (s *Server) handleSetSubject(w http.ResponseWriter, r *http.Request, p core.Principal) {
	var body struct {
		Name string `json:"name"`
	}
	if err := readJSON(r, &body); err != nil {
		writeErr(w, err)
		return
	}
	code := strings.ToLower(r.PathValue("code"))
	if err := s.Core.SetSubject(r.Context(), p, r.PathValue("account"), code, body.Name); err != nil {
		writeErr(w, err)
		return
	}
	def, _ := subjects.Resolve(code, lang(r), nil)
	writeData(w, core.SubjectEntry{Code: code, Name: strings.TrimSpace(body.Name), DefaultName: def, Custom: true})
}

func (s *Server) handleResetSubject(w http.ResponseWriter, r *http.Request, p core.Principal) {
	if err := s.Core.ResetSubject(r.Context(), p, r.PathValue("account"), r.PathValue("code")); err != nil {
		writeErr(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) handleTeachers(w http.ResponseWriter, r *http.Request, p core.Principal) {
	list, err := s.Core.Teachers(r.Context(), p, r.PathValue("account"))
	if err != nil {
		writeErr(w, err)
		return
	}
	writeData(w, list)
}

func (s *Server) handleSetTeacher(w http.ResponseWriter, r *http.Request, p core.Principal) {
	var body struct {
		Name string `json:"name"`
	}
	if err := readJSON(r, &body); err != nil {
		writeErr(w, err)
		return
	}
	code := strings.ToLower(r.PathValue("code"))
	if err := s.Core.SetTeacher(r.Context(), p, r.PathValue("account"), code, body.Name); err != nil {
		writeErr(w, err)
		return
	}
	writeData(w, core.TeacherEntry{Code: code, Name: strings.TrimSpace(body.Name), Custom: true})
}

func (s *Server) handleResetTeacher(w http.ResponseWriter, r *http.Request, p core.Principal) {
	if err := s.Core.ResetTeacher(r.Context(), p, r.PathValue("account"), r.PathValue("code")); err != nil {
		writeErr(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) handleCalendarFeed(w http.ResponseWriter, r *http.Request) {
	token := strings.TrimSuffix(r.PathValue("token"), ".ics")
	ics, err := s.Core.CalendarFeed(r.Context(), token, lang(r))
	if err != nil {
		writeErr(w, err)
		return
	}
	w.Header().Set("Content-Type", "text/calendar; charset=utf-8")
	w.Header().Set("Content-Disposition", `inline; filename="melon.ics"`)
	w.Write([]byte(ics))
}
