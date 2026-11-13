package zermelo

import (
	"context"
	"fmt"
	"hash/fnv"
	"math/rand"
	"strconv"
	"strings"
	"time"
)

// DemoSchool is the portal name served by generated data instead of Zermelo.
const DemoSchool = "demo"

// Demo tokens. They only work against the generated school.
const (
	DemoStudentToken = "demo-student"
	DemoTeacherToken = "demo-teacher"
)

// Demo is a fake school with deterministic, week-relative data. It powers the
// "try the demo" button and CI screenshots, so no real account is ever needed.
type Demo struct{ Loc *time.Location }

var demoStudents = func() []User {
	names := []string{"Sam|de|Vries", "Lotte||Jansen", "Daan|van den|Berg", "Emma||Bakker", "Finn||Visser", "Sara||Smit", "Milan||Meijer", "Julia|de|Boer", "Noah||Mulder", "Tess|van|Dijk", "Luuk||Bos", "Yara||El Amrani", "Sem||Hendriks", "Fleur||Dekker", "Mees|van|Leeuwen", "Zoë||Peters", "Jesse||Kok", "Nina||Jacobs", "Thijs|de|Haan", "Isa||Vermeulen", "Ruben||Kaya", "Lynn|van der|Heijden", "Bram||Hoekstra", "Mila||Koster"}
	out := make([]User, len(names))
	for i, n := range names {
		p := strings.Split(n, "|")
		out[i] = User{Code: strconv.Itoa(100001 + i), FirstName: p[0], Prefix: p[1], LastName: p[2], IsStudent: true}
	}
	return out
}()

var demoTeachers = []User{
	{Code: "bkn", FirstName: "Noor", LastName: "Bakker", IsEmployee: true},
	{Code: "jns", FirstName: "Pieter", LastName: "Jansen", IsEmployee: true},
	{Code: "vdb", FirstName: "Anouk", Prefix: "van den", LastName: "Broek", IsEmployee: true},
	{Code: "smt", FirstName: "Karim", LastName: "Smits", IsEmployee: true},
	{Code: "dkr", FirstName: "Marieke", LastName: "Dekker", IsEmployee: true},
	{Code: "mul", FirstName: "Hans", LastName: "Mulder", IsEmployee: true},
	{Code: "vis", FirstName: "Sanne", LastName: "Visser", IsEmployee: true},
	{Code: "ozt", FirstName: "Elif", LastName: "Öztürk", IsEmployee: true},
}

func (d Demo) user(token string) (User, error) {
	switch token {
	case DemoStudentToken:
		return demoStudents[0], nil
	case DemoTeacherToken:
		return demoTeachers[0], nil
	}
	return User{}, ErrUnauthorized
}

func (d Demo) ExchangeCode(_ context.Context, _, code string) (string, error) {
	switch strings.ToLower(strings.TrimSpace(code)) {
	case "":
		return "", ErrBadCode
	case "docent", "teacher":
		return DemoTeacherToken, nil
	}
	return DemoStudentToken, nil
}

func (d Demo) TokenInfo(_ context.Context, _, token string) (TokenInfo, error) {
	u, err := d.user(token)
	return TokenInfo{User: u.Code}, err
}

func (d Demo) Me(_ context.Context, _, token string) (User, error) { return d.user(token) }

func (d Demo) Users(_ context.Context, _, token string) ([]User, error) {
	u, err := d.user(token)
	if err != nil {
		return nil, err
	}
	if !u.IsEmployee {
		return nil, ErrForbidden
	}
	return append(append([]User{}, demoTeachers...), demoStudents...), nil
}

func (d Demo) Announcements(_ context.Context, _, token string) ([]Announcement, error) {
	if _, err := d.user(token); err != nil {
		return nil, err
	}
	monday := mondayOf(time.Now().In(d.Loc))
	return []Announcement{{
		ID: 1, Start: monday.Unix(), End: monday.AddDate(0, 0, 7).Unix(),
		Title: "Ouderavond 10-minutengesprekken",
		Text:  "Donderdag vervallen het 7e en 8e uur in verband met de ouderavond.",
	}}, nil
}

type demoSubject struct{ code, teacher, room string }

var demoSubjects = []demoSubject{
	{"ne", "jns", "112"}, {"en", "vdb", "204"}, {"wi", "bkn", "118"}, {"na", "smt", "N03"},
	{"sk", "smt", "N05"}, {"bi", "dkr", "N01"}, {"gs", "mul", "210"}, {"ak", "vis", "215"},
	{"du", "ozt", "206"}, {"fa", "vdb", "207"}, {"lo", "mul", "gym1"}, {"ec", "vis", "121"},
	{"mu", "dkr", "A02"}, {"ckv", "ozt", "A04"}, {"men", "jns", "112"},
}

// Period start times as minutes after midnight; lessons last 50 minutes.
var demoPeriods = []int{8*60 + 30, 9*60 + 20, 10*60 + 30, 11*60 + 20, 12*60 + 40, 13*60 + 30, 14*60 + 35, 15*60 + 25}

func mondayOf(t time.Time) time.Time {
	wd := (int(t.Weekday()) + 6) % 7
	return time.Date(t.Year(), t.Month(), t.Day()-wd, 0, 0, 0, 0, t.Location())
}

// WeekStart converts Zermelo's yyyyWW into the Monday of that ISO week.
func WeekStart(week string, loc *time.Location) (time.Time, error) {
	if len(week) != 6 {
		return time.Time{}, fmt.Errorf("bad week %q", week)
	}
	y, err1 := strconv.Atoi(week[:4])
	w, err2 := strconv.Atoi(week[4:])
	if err1 != nil || err2 != nil || w < 1 || w > 53 {
		return time.Time{}, fmt.Errorf("bad week %q", week)
	}
	// 4 January is always in ISO week 1.
	return mondayOf(time.Date(y, 1, 4, 0, 0, 0, 0, loc)).AddDate(0, 0, (w-1)*7), nil
}

func seed(parts ...string) *rand.Rand {
	h := fnv.New64a()
	h.Write([]byte(strings.Join(parts, "/")))
	return rand.New(rand.NewSource(int64(h.Sum64())))
}

// Appointments gives the demo teacher the same by-code lookup as the real API.
func (d Demo) Appointments(ctx context.Context, school, token, code string, from, _ time.Time) ([]Appointment, error) {
	kind := "teacher"
	if _, err := strconv.Atoi(code); err == nil {
		kind = "student"
	}
	y, w := from.In(d.Loc).ISOWeek()
	return d.LiveSchedule(ctx, school, token, kind, code, fmt.Sprintf("%04d%02d", y, w))
}

func (d Demo) LiveSchedule(_ context.Context, _, token, kind, code, week string) ([]Appointment, error) {
	me, err := d.user(token)
	if err != nil {
		return nil, err
	}
	if code != me.Code && !me.IsEmployee {
		return nil, ErrForbidden
	}
	monday, err := WeekStart(week, d.Loc)
	if err != nil {
		return nil, err
	}
	var out []Appointment
	id := monday.Unix() / 100
	for day := 0; day < 5; day++ {
		date := monday.AddDate(0, 0, day)
		at := func(period int) (int64, int64) {
			m := demoPeriods[period-1]
			s := time.Date(date.Year(), date.Month(), date.Day(), m/60, m%60, 0, 0, d.Loc)
			return s.Unix(), s.Add(50 * time.Minute).Unix()
		}
		var lessons []Appointment
		r := seed(kind, code, strconv.Itoa(day))
		first := 1 + r.Intn(2)
		count := 5 + r.Intn(2)
		order := r.Perm(len(demoSubjects))
		for i := 0; i < count; i++ {
			p := first + i
			a := Appointment{Type: "lesson", StartSlot: &p, EndSlot: &p}
			a.Start, a.End = at(p)
			if kind == "teacher" {
				year := 1 + r.Intn(4)
				class := string(rune('a' + r.Intn(3)))
				subj := "wi"
				for _, s := range demoSubjects {
					if s.teacher == code {
						subj = s.code
						break
					}
				}
				a.Subjects = []string{subj}
				a.Teachers = []string{code}
				a.Groups = []string{fmt.Sprintf("%d%s", year, class)}
				a.Locations = []string{"118"}
			} else {
				s := demoSubjects[order[i]]
				a.Subjects = []string{s.code}
				a.Teachers = []string{s.teacher}
				a.Groups = []string{"3a." + s.code}
				a.Locations = []string{s.room}
			}
			lessons = append(lessons, a)
		}
		n := len(lessons)
		switch day {
		case 0:
			lessons[0].Cancelled = true
			lessons[0].ChangeDescription = "Deze les vervalt."
			lessons[3].LocationChanged = true
			lessons[3].ChangeDescription = "Het lokaal is gewijzigd (was " + lessons[3].Locations[0] + ")."
			lessons[3].Locations = []string{"301"}
		case 1:
			substitute(&lessons[2], kind, "ozt", "204")
			lessons[n-1].Cancelled = true
			lessons[n-1].ChangeDescription = "Deze les vervalt."
		case 2:
			lessons[1].Type = "exam"
			lessons[1].SchedulerRemark = "Toets hoofdstuk 3"
			lessons[4].Cancelled = true
			lessons[4].ChangeDescription = "Deze les vervalt."
			lessons[2].LocationChanged = true
			lessons[2].ChangeDescription = "Het lokaal is gewijzigd (was " + lessons[2].Locations[0] + ")."
			lessons[2].Locations = []string{"B12"}
		case 3:
			// Move one lesson to the first free period after the last one.
			from := *lessons[2].StartSlot
			to := first + n
			moved := lessons[2]
			moved.StartSlot, moved.EndSlot = &to, &to
			moved.Start, moved.End = at(to)
			moved.TimeChanged = true
			moved.ChangeDescription = fmt.Sprintf("De les is verplaatst (was het %de uur).", from)
			lessons = append(append(lessons[:2:2], lessons[3:]...), moved)
			substitute(&lessons[0], kind, "dkr", "B12")
		case 4:
			lessons[n-1].Cancelled = true
			lessons[n-1].ChangeDescription = "Deze les vervalt."
			p := first + n
			act := Appointment{Type: "activity", StartSlot: &p, EndSlot: &p, Locations: []string{"aula"}, SchedulerRemark: "Voorlichting profielkeuze"}
			act.Start, act.End = at(p)
			lessons = append(lessons, act)
		}
		for i := range lessons {
			id++
			lessons[i].ID = id
			lessons[i].Instance = id
			lessons[i].LastModified = monday.Unix()
		}
		out = append(out, lessons...)
	}
	return out, nil
}

// substitute gives a student's lesson another teacher. A teacher's own
// schedule gets another room instead, since they are still the one teaching.
func substitute(a *Appointment, kind, teacher, room string) {
	if kind == "teacher" {
		a.LocationChanged = true
		a.ChangeDescription = "Het lokaal is gewijzigd (was " + a.Locations[0] + ")."
		a.Locations = []string{room}
		return
	}
	a.TeacherChanged = true
	a.ChangeDescription = "De docent is gewijzigd (was " + a.Teachers[0] + ")."
	a.Teachers = []string{teacher}
}

// Router sends the demo school to generated data and everything else to Zermelo.
type Router struct {
	Real Upstream
	Demo Upstream // nil when demo mode is off
}

func (r Router) pick(school string) Upstream {
	if school == DemoSchool && r.Demo != nil {
		return r.Demo
	}
	return r.Real
}

func (r Router) ExchangeCode(ctx context.Context, school, code string) (string, error) {
	return r.pick(school).ExchangeCode(ctx, school, code)
}
func (r Router) TokenInfo(ctx context.Context, school, token string) (TokenInfo, error) {
	return r.pick(school).TokenInfo(ctx, school, token)
}
func (r Router) Me(ctx context.Context, school, token string) (User, error) {
	return r.pick(school).Me(ctx, school, token)
}
func (r Router) Users(ctx context.Context, school, token string) ([]User, error) {
	return r.pick(school).Users(ctx, school, token)
}
func (r Router) Appointments(ctx context.Context, school, token, code string, from, to time.Time) ([]Appointment, error) {
	return r.pick(school).Appointments(ctx, school, token, code, from, to)
}
func (r Router) LiveSchedule(ctx context.Context, school, token, kind, code, week string) ([]Appointment, error) {
	return r.pick(school).LiveSchedule(ctx, school, token, kind, code, week)
}
func (r Router) Announcements(ctx context.Context, school, token string) ([]Announcement, error) {
	return r.pick(school).Announcements(ctx, school, token)
}
