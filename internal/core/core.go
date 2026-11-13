// Package core is Melon's domain layer. The REST API, the MCP server and the
// calendar feed are all thin shells around this one Service, so they cannot
// drift apart or disagree about who may see what.
package core

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"sort"
	"strings"
	"time"

	"github.com/matthiaslubbertsen/melon/internal/secret"
	"github.com/matthiaslubbertsen/melon/internal/store"
	"github.com/matthiaslubbertsen/melon/internal/subjects"
	"github.com/matthiaslubbertsen/melon/internal/zermelo"
)

// Error is a failure that is safe and useful to show to API clients.
type Error struct {
	Status  int    `json:"-"`
	Code    string `json:"code"`
	Message string `json:"message"`
}

func (e *Error) Error() string { return e.Code + ": " + e.Message }

func Errf(status int, code, format string, args ...any) *Error {
	return &Error{Status: status, Code: code, Message: fmt.Sprintf(format, args...)}
}

var (
	ErrNotFound = Errf(http.StatusNotFound, "not_found", "That does not exist, or this credential has no access to it.")
)

// Principal is who is asking, and which linked accounts they may touch.
type Principal struct {
	UserID string
	Kind   string // "session", "api_key" or "oauth"
	// All grants every account of the user (browser sessions only).
	All        bool
	AccountIDs []string
}

func (p Principal) allows(id string) bool {
	if p.All {
		return true
	}
	for _, a := range p.AccountIDs {
		if a == id {
			return true
		}
	}
	return false
}

type Service struct {
	Store *store.Store
	Up    zermelo.Upstream
	Cache *zermelo.Cache
	Box   *secret.Box
	Loc   *time.Location
	Demo  bool
}

const (
	scheduleTTL  = 30 * time.Second
	directoryTTL = 10 * time.Minute
	MaxRangeDays = 42
)

// ---- models ----

type Account struct {
	ID       string `json:"id"`
	School   string `json:"school"`
	Name     string `json:"name"`
	Role     string `json:"role"`
	UserCode string `json:"userCode"`
	// CanViewOthers is true when the Zermelo token sees more than its owner.
	CanViewOthers bool `json:"canViewOthers"`
}

type Person struct {
	Code string `json:"code"`
	Name string `json:"name"`
	Type string `json:"type"` // "student" or "employee"
}

type Subject struct {
	Code string `json:"code"`
	Name string `json:"name"`
}

// Teacher is a teacher code with the name to show for it. Name equals Code
// when nobody has told Melon who it is.
type Teacher struct {
	Code string `json:"code"`
	Name string `json:"name"`
}

type TeacherEntry struct {
	Code string `json:"code"`
	Name string `json:"name"`
	// Custom is true when the account set this name itself; otherwise it
	// comes from the school's staff list.
	Custom bool `json:"custom"`
}

type Period struct {
	From int `json:"from"`
	To   int `json:"to"`
}

type Lesson struct {
	ID         string     `json:"id"`
	Date       string     `json:"date"`
	Start      time.Time  `json:"start"`
	End        time.Time  `json:"end"`
	Period     *Period    `json:"period"`
	Type       string     `json:"type"`
	Title      string     `json:"title"`
	Subjects   []Subject  `json:"subjects"`
	Teachers   []Teacher  `json:"teachers"`
	Locations  []string   `json:"locations"`
	Groups     []string   `json:"groups"`
	Status     string     `json:"status"`  // scheduled | changed | cancelled
	Changes    []string   `json:"changes"` // teacher | location | group | time
	ChangeNote string     `json:"changeNote"`
	Note       string     `json:"note"`
	Online     bool       `json:"online"`
	OnlineURL  string     `json:"onlineUrl,omitempty"`
	Optional   bool       `json:"optional"`
	UpdatedAt  *time.Time `json:"updatedAt"`
}

type Schedule struct {
	Person    Person    `json:"person"`
	From      string    `json:"from"`
	To        string    `json:"to"`
	Lessons   []Lesson  `json:"lessons"`
	FetchedAt time.Time `json:"-"`
	Stale     bool      `json:"-"`
}

type Announcement struct {
	ID    string    `json:"id"`
	Title string    `json:"title"`
	Text  string    `json:"text"`
	Start time.Time `json:"start"`
	End   time.Time `json:"end"`
}

type SubjectEntry struct {
	Code string `json:"code"`
	Name string `json:"name"`
	// DefaultName is what Melon would call it without the user's override.
	DefaultName string `json:"defaultName"`
	Custom      bool   `json:"custom"`
}

// ---- accounts ----

func toAccount(a store.Account) Account {
	return Account{ID: a.ID, School: a.School, Name: a.DisplayName, Role: a.Role, UserCode: a.ZermeloUser, CanViewOthers: a.Wide}
}

// Pending is a Zermelo account that was verified but not yet stored.
type Pending struct {
	acc   store.Account
	token string
}

// Link connects a Zermelo account to the profile, from a link code or a token.
func (s *Service) Link(ctx context.Context, userID, school, code, token string) (Account, error) {
	pending, err := s.Verify(ctx, school, code, token)
	if err != nil {
		return Account{}, err
	}
	return s.Save(ctx, userID, pending)
}

// Verify checks a link code or token with Zermelo without storing anything,
// so a failed attempt leaves no trace.
func (s *Service) Verify(ctx context.Context, schoolIn, code, token string) (Pending, error) {
	fail := func(err error) (Pending, error) { return Pending{}, err }
	school, ok := zermelo.NormalizeSchool(schoolIn)
	if !ok || (school == zermelo.DemoSchool && !s.Demo) {
		return fail(Errf(http.StatusUnprocessableEntity, "school_not_found", "No Zermelo portal with that name was found."))
	}
	code = strings.Join(strings.Fields(code), "")
	token = strings.TrimSpace(token)
	switch {
	case code != "":
		var err error
		if token, err = s.Up.ExchangeCode(ctx, school, code); err != nil {
			return fail(s.upstreamErr(err))
		}
	case token == "":
		return fail(Errf(http.StatusBadRequest, "invalid_request", "Provide either a link code or an API token."))
	}
	me, err := s.Up.Me(ctx, school, token)
	if errors.Is(err, zermelo.ErrUnauthorized) {
		return fail(Errf(http.StatusUnprocessableEntity, "invalid_token", "Zermelo does not accept that token."))
	} else if err != nil {
		return fail(s.upstreamErr(err))
	}
	var expires int64
	if info, err := s.Up.TokenInfo(ctx, school, token); err == nil {
		expires = info.Expires
	}
	// A token is "wide" when Zermelo lets it list anyone besides its owner.
	users, err := s.Up.Users(ctx, school, token)
	wide := err == nil && len(users) > 1

	role := "other"
	switch {
	case me.IsStudent:
		role = "student"
	case me.IsEmployee:
		role = "employee"
	case me.IsFamilyMember:
		role = "parent"
	}
	name := me.Name()
	if name == "" {
		name = me.Code
	}
	return Pending{token: token, acc: store.Account{ID: secret.ID("acc"), School: school, ZermeloUser: me.Code, DisplayName: name, Role: role, Wide: wide, TokenExpires: expires}}, nil
}

// Save stores a verified account under the profile, encrypting its token.
func (s *Service) Save(ctx context.Context, userID string, pending Pending) (Account, error) {
	acc := pending.acc
	acc.UserID = userID
	id, err := s.Store.UpsertAccount(ctx, acc, func(id string) []byte { return s.Box.Seal(pending.token, id) })
	if err != nil {
		return Account{}, err
	}
	acc.ID = id
	s.Cache.Drop("acct:" + id + ":")
	return toAccount(acc), nil
}

func (s *Service) Accounts(ctx context.Context, p Principal) ([]Account, error) {
	rows, err := s.Store.Accounts(ctx, p.UserID)
	if err != nil {
		return nil, err
	}
	out := []Account{}
	for _, a := range rows {
		if p.allows(a.ID) {
			out = append(out, toAccount(a))
		}
	}
	return out, nil
}

// account loads one account the principal may use, with its decrypted token.
// The token stays inside this package.
func (s *Service) account(ctx context.Context, p Principal, id string) (store.Account, string, error) {
	if !p.allows(id) {
		return store.Account{}, "", ErrNotFound
	}
	a, err := s.Store.Account(ctx, p.UserID, id)
	if errors.Is(err, store.ErrNotFound) {
		return a, "", ErrNotFound
	} else if err != nil {
		return a, "", err
	}
	token, err := s.Box.Open(a.TokenEnc, a.ID)
	if err != nil {
		slog.Error("cannot decrypt account token; was MELON_SECRET changed?", "account", a.ID)
		return a, "", Errf(http.StatusConflict, "account_link_expired", "This account needs to be linked again.")
	}
	return a, token, nil
}

func (s *Service) Account(ctx context.Context, p Principal, id string) (Account, error) {
	a, _, err := s.account(ctx, p, id)
	return toAccount(a), err
}

func (s *Service) Unlink(ctx context.Context, p Principal, id string) error {
	if !p.allows(id) {
		return ErrNotFound
	}
	if err := s.Store.DeleteAccount(ctx, p.UserID, id); errors.Is(err, store.ErrNotFound) {
		return ErrNotFound
	} else if err != nil {
		return err
	}
	s.Cache.Drop("acct:" + id + ":")
	return nil
}

func (s *Service) upstreamErr(err error) error {
	switch {
	case errors.Is(err, zermelo.ErrBadCode):
		return Errf(http.StatusUnprocessableEntity, "invalid_link_code", "That link code is wrong or has expired. Codes are only valid for a short time.")
	case errors.Is(err, zermelo.ErrNoSchool):
		return Errf(http.StatusUnprocessableEntity, "school_not_found", "No Zermelo portal with that name was found.")
	case errors.Is(err, zermelo.ErrUnauthorized):
		return Errf(http.StatusConflict, "account_link_expired", "Zermelo no longer accepts this account's token. Link the account again.")
	case errors.Is(err, zermelo.ErrForbidden):
		return Errf(http.StatusForbidden, "forbidden", "Zermelo does not allow this account to see that.")
	case errors.Is(err, zermelo.ErrRateLimited):
		return Errf(http.StatusServiceUnavailable, "upstream_rate_limited", "Zermelo is receiving too many requests. Try again in a moment.")
	case errors.Is(err, zermelo.ErrUnavailable), errors.Is(err, context.DeadlineExceeded):
		slog.Warn("zermelo unavailable", "err", err)
		return Errf(http.StatusBadGateway, "upstream_unavailable", "Zermelo cannot be reached right now.")
	}
	return err
}

// ---- people ----

func (s *Service) directory(ctx context.Context, a store.Account, token string) ([]zermelo.User, error) {
	users, _, err := zermelo.Get(ctx, s.Cache, "acct:"+a.ID+":users", directoryTTL, func(ctx context.Context) ([]zermelo.User, error) {
		return s.Up.Users(ctx, a.School, token)
	})
	if err != nil {
		return nil, s.upstreamErr(err)
	}
	return users, nil
}

func toPerson(u zermelo.User) Person {
	p := Person{Code: u.Code, Name: u.Name(), Type: "student"}
	if u.IsEmployee && !u.IsStudent {
		p.Type = "employee"
	}
	if p.Name == "" {
		p.Name = u.Code
	}
	return p
}

// People searches the people this account can see, by name or code.
func (s *Service) People(ctx context.Context, p Principal, accountID, query string, limit int) ([]Person, error) {
	a, token, err := s.account(ctx, p, accountID)
	if err != nil {
		return nil, err
	}
	if !a.Wide {
		return nil, Errf(http.StatusForbidden, "forbidden", "This account can only see its own schedule.")
	}
	users, err := s.directory(ctx, a, token)
	if err != nil {
		return nil, err
	}
	if limit <= 0 || limit > 100 {
		limit = 25
	}
	terms := strings.Fields(fold(query))
	type hit struct {
		p    Person
		rank int
	}
	var hits []hit
	for _, u := range users {
		if !u.IsStudent && !u.IsEmployee {
			continue
		}
		person := toPerson(u)
		hay := fold(person.Name)
		code := strings.ToLower(person.Code)
		rank, ok := 2, true
		for _, t := range terms {
			switch {
			case code == t || strings.HasPrefix(hay, t):
				rank = 0
			case strings.Contains(" "+hay, " "+t) || strings.HasPrefix(code, t):
				rank = min(rank, 1)
			case strings.Contains(hay, t):
			default:
				ok = false
			}
		}
		if ok {
			hits = append(hits, hit{person, rank})
		}
	}
	sort.SliceStable(hits, func(i, j int) bool {
		if hits[i].rank != hits[j].rank {
			return hits[i].rank < hits[j].rank
		}
		return hits[i].p.Name < hits[j].p.Name
	})
	out := []Person{}
	for i := 0; i < len(hits) && i < limit; i++ {
		out = append(out, hits[i].p)
	}
	return out, nil
}

// fold lowercases and strips the diacritics common in Dutch names, so that
// "zoe" finds "Zoë".
func fold(s string) string {
	return strings.NewReplacer("ë", "e", "é", "e", "è", "e", "ê", "e", "ï", "i", "í", "i", "ö", "o", "ó", "o", "ü", "u", "ú", "u", "á", "a", "à", "a", "ä", "a", "ç", "c", "ñ", "n").Replace(strings.ToLower(s))
}

// ---- schedule ----

type ScheduleQuery struct {
	From, To time.Time // inclusive dates in the school's timezone
	Person   string    // empty: the account's own schedule
	Lang     string
	// Fresh skips the cache (down to a small floor that protects Zermelo).
	Fresh  bool
	MaxAge time.Duration
}

func (s *Service) Schedule(ctx context.Context, p Principal, accountID string, q ScheduleQuery) (Schedule, error) {
	a, token, err := s.account(ctx, p, accountID)
	if err != nil {
		return Schedule{}, err
	}
	if q.To.Before(q.From) {
		return Schedule{}, Errf(http.StatusBadRequest, "invalid_request", "'to' must not be before 'from'.")
	}
	if q.To.Sub(q.From) > MaxRangeDays*24*time.Hour {
		return Schedule{}, Errf(http.StatusBadRequest, "invalid_request", "A schedule request can span at most %d days.", MaxRangeDays)
	}

	person := Person{Code: a.ZermeloUser, Name: a.DisplayName, Type: "student"}
	if a.Role == "employee" {
		person.Type = "employee"
	}
	if q.Person != "" && q.Person != a.ZermeloUser {
		if !a.Wide {
			return Schedule{}, Errf(http.StatusForbidden, "forbidden", "This account can only see its own schedule.")
		}
		users, err := s.directory(ctx, a, token)
		if err != nil {
			return Schedule{}, err
		}
		found := false
		for _, u := range users {
			if u.Code == q.Person {
				person, found = toPerson(u), true
				break
			}
		}
		if !found {
			return Schedule{}, Errf(http.StatusNotFound, "person_not_found", "This account cannot see anyone with code %q.", q.Person)
		}
	}
	kind := "student"
	if person.Type == "employee" {
		kind = "teacher"
	}

	ttl := scheduleTTL
	if q.MaxAge > 0 {
		ttl = q.MaxAge
	}
	if q.Fresh {
		ttl = zermelo.MinAge
	}
	overrides, err := s.Store.SubjectOverrides(ctx, a.ID)
	if err != nil {
		return Schedule{}, err
	}
	teachers, _, err := s.teacherNames(ctx, a, token)
	if err != nil {
		return Schedule{}, err
	}

	// One upstream request per ISO week, in parallel.
	var weeks []string
	for d := mondayOf(q.From); !d.After(q.To); d = d.AddDate(0, 0, 7) {
		y, w := d.ISOWeek()
		weeks = append(weeks, fmt.Sprintf("%04d%02d", y, w))
	}
	type weekResult struct {
		appts []zermelo.Appointment
		res   zermelo.Result
		err   error
	}
	results := make([]weekResult, len(weeks))
	done := make(chan int, len(weeks))
	for i, week := range weeks {
		go func() {
			key := fmt.Sprintf("acct:%s:schedule:%s:%s:%s", a.ID, kind, person.Code, week)
			r := &results[i]
			r.appts, r.res, r.err = zermelo.Get(ctx, s.Cache, key, ttl, func(ctx context.Context) ([]zermelo.Appointment, error) {
				return s.Up.LiveSchedule(ctx, a.School, token, kind, person.Code, week)
			})
			done <- i
		}()
	}
	for range weeks {
		<-done
	}

	out := Schedule{Person: person, From: q.From.Format(time.DateOnly), To: q.To.Format(time.DateOnly), Lessons: []Lesson{}, FetchedAt: time.Now()}
	end := q.To.AddDate(0, 0, 1)
	for _, r := range results {
		if r.err != nil {
			return Schedule{}, s.upstreamErr(r.err)
		}
		if r.res.FetchedAt.Before(out.FetchedAt) {
			out.FetchedAt = r.res.FetchedAt
		}
		out.Stale = out.Stale || r.res.Stale
		for _, ap := range r.appts {
			start := time.Unix(ap.Start, 0).In(s.Loc)
			if start.Before(q.From) || !start.Before(end) {
				continue
			}
			out.Lessons = append(out.Lessons, toLesson(ap, s.Loc, q.Lang, overrides, teachers))
		}
	}
	sort.SliceStable(out.Lessons, func(i, j int) bool {
		a, b := out.Lessons[i], out.Lessons[j]
		if !a.Start.Equal(b.Start) {
			return a.Start.Before(b.Start)
		}
		// Within one slot, show what is happening before what was cancelled.
		return a.Status != "cancelled" && b.Status == "cancelled"
	})
	return out, nil
}

func toLesson(ap zermelo.Appointment, loc *time.Location, lang string, overrides, teachers map[string]string) Lesson {
	start := time.Unix(ap.Start, 0).In(loc)
	l := Lesson{
		ID:         fmt.Sprint(ap.ID),
		Date:       start.Format(time.DateOnly),
		Start:      start,
		End:        time.Unix(ap.End, 0).In(loc),
		Type:       strings.ToLower(ap.Type),
		Subjects:   []Subject{},
		Teachers:   []Teacher{},
		Locations:  dedupe(ap.Locations),
		Groups:     dedupe(ap.Groups),
		Status:     "scheduled",
		Changes:    []string{},
		ChangeNote: strings.TrimSpace(ap.ChangeDescription),
		Note:       strings.TrimSpace(ap.SchedulerRemark),
		Online:     ap.Online,
		OnlineURL:  ap.OnlineLocationURL,
		Optional:   ap.Optional,
	}
	if l.Type == "" {
		l.Type = "lesson"
	}
	if ap.StartSlot != nil {
		l.Period = &Period{From: *ap.StartSlot, To: *ap.StartSlot}
		if ap.EndSlot != nil {
			l.Period.To = *ap.EndSlot
		}
	}
	for _, code := range dedupe(ap.Teachers) {
		name := teachers[strings.ToLower(code)]
		if name == "" {
			name = code
		}
		l.Teachers = append(l.Teachers, Teacher{Code: code, Name: name})
	}
	var names []string
	for _, code := range dedupe(ap.Subjects) {
		name, _ := subjects.Resolve(code, lang, overrides)
		l.Subjects = append(l.Subjects, Subject{Code: code, Name: name})
		names = append(names, name)
	}
	switch {
	case len(names) > 0:
		l.Title = strings.Join(names, ", ")
	case l.Note != "":
		l.Title = l.Note
	default:
		l.Title = l.Type
	}
	for name, changed := range map[string]bool{"teacher": ap.TeacherChanged, "location": ap.LocationChanged, "group": ap.GroupChanged, "time": ap.TimeChanged} {
		if changed {
			l.Changes = append(l.Changes, name)
		}
	}
	sort.Strings(l.Changes)
	switch {
	case ap.Cancelled:
		l.Status = "cancelled"
	case len(l.Changes) > 0:
		l.Status = "changed"
	}
	if ap.LastModified > 0 {
		t := time.Unix(ap.LastModified, 0).In(loc)
		l.UpdatedAt = &t
	}
	return l
}

func dedupe(in []string) []string {
	out := make([]string, 0, len(in))
	seen := map[string]bool{}
	for _, v := range in {
		if v != "" && !seen[v] {
			seen[v] = true
			out = append(out, v)
		}
	}
	return out
}

func mondayOf(t time.Time) time.Time {
	wd := (int(t.Weekday()) + 6) % 7
	return time.Date(t.Year(), t.Month(), t.Day()-wd, 0, 0, 0, 0, t.Location())
}

// Today returns midnight today in the school timezone.
func (s *Service) Today() time.Time {
	n := time.Now().In(s.Loc)
	return time.Date(n.Year(), n.Month(), n.Day(), 0, 0, 0, 0, s.Loc)
}

// ---- announcements ----

func (s *Service) Announcements(ctx context.Context, p Principal, accountID string) ([]Announcement, zermelo.Result, error) {
	a, token, err := s.account(ctx, p, accountID)
	if err != nil {
		return nil, zermelo.Result{}, err
	}
	rows, res, err := zermelo.Get(ctx, s.Cache, "acct:"+a.ID+":announcements", 2*time.Minute, func(ctx context.Context) ([]zermelo.Announcement, error) {
		return s.Up.Announcements(ctx, a.School, token)
	})
	if errors.Is(err, zermelo.ErrForbidden) {
		return []Announcement{}, zermelo.Result{FetchedAt: time.Now()}, nil
	} else if err != nil {
		return nil, res, s.upstreamErr(err)
	}
	out := []Announcement{}
	for _, r := range rows {
		out = append(out, Announcement{ID: fmt.Sprint(r.ID), Title: strings.TrimSpace(r.Title), Text: strings.TrimSpace(r.Text), Start: time.Unix(r.Start, 0).In(s.Loc), End: time.Unix(r.End, 0).In(s.Loc)})
	}
	return out, res, nil
}

// ---- teachers ----

// teacherNames maps lowercase teacher codes to names: the school's staff list
// when the account can see it, with the account's own names on top. custom
// holds the codes the account named itself.
func (s *Service) teacherNames(ctx context.Context, a store.Account, token string) (names map[string]string, custom map[string]string, err error) {
	custom, err = s.Store.TeacherNames(ctx, a.ID)
	if err != nil {
		return nil, nil, err
	}
	names = map[string]string{}
	if a.Wide {
		// Best effort: a schedule without full names beats no schedule.
		if users, err := s.directory(ctx, a, token); err == nil {
			for _, u := range users {
				if u.IsEmployee && u.Name() != "" {
					names[strings.ToLower(u.Code)] = u.Name()
				}
			}
		}
	}
	for code, name := range custom {
		names[code] = name
	}
	return names, custom, nil
}

// Teachers lists every teacher code Melon has a name for on this account.
func (s *Service) Teachers(ctx context.Context, p Principal, accountID string) ([]TeacherEntry, error) {
	a, token, err := s.account(ctx, p, accountID)
	if err != nil {
		return nil, err
	}
	names, custom, err := s.teacherNames(ctx, a, token)
	if err != nil {
		return nil, err
	}
	out := []TeacherEntry{}
	for code, name := range names {
		_, isCustom := custom[code]
		out = append(out, TeacherEntry{Code: code, Name: name, Custom: isCustom})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Code < out[j].Code })
	return out, nil
}

func (s *Service) SetTeacher(ctx context.Context, p Principal, accountID, code, name string) error {
	a, _, err := s.account(ctx, p, accountID)
	if err != nil {
		return err
	}
	code = strings.ToLower(strings.TrimSpace(code))
	name = strings.TrimSpace(name)
	if code == "" || len(code) > 32 || name == "" || len(name) > 80 {
		return Errf(http.StatusBadRequest, "invalid_request", "A teacher needs a code (max 32 characters) and a name (max 80).")
	}
	return s.Store.SetTeacherName(ctx, a.ID, code, name)
}

func (s *Service) ResetTeacher(ctx context.Context, p Principal, accountID, code string) error {
	a, _, err := s.account(ctx, p, accountID)
	if err != nil {
		return err
	}
	return s.Store.DeleteTeacherName(ctx, a.ID, strings.ToLower(strings.TrimSpace(code)))
}

// ---- subjects ----

// Subjects lists every known code with its effective name for this account.
func (s *Service) Subjects(ctx context.Context, p Principal, accountID, lang string) ([]SubjectEntry, error) {
	a, _, err := s.account(ctx, p, accountID)
	if err != nil {
		return nil, err
	}
	overrides, err := s.Store.SubjectOverrides(ctx, a.ID)
	if err != nil {
		return nil, err
	}
	seen := map[string]bool{}
	out := []SubjectEntry{}
	add := func(code string) {
		if seen[code] {
			return
		}
		seen[code] = true
		def, _ := subjects.Resolve(code, lang, nil)
		name, _ := subjects.Resolve(code, lang, overrides)
		_, custom := overrides[code]
		out = append(out, SubjectEntry{Code: code, Name: name, DefaultName: def, Custom: custom})
	}
	for code := range overrides {
		add(code)
	}
	for code := range subjects.Defaults {
		add(code)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Code < out[j].Code })
	return out, nil
}

func (s *Service) SetSubject(ctx context.Context, p Principal, accountID, code, name string) error {
	a, _, err := s.account(ctx, p, accountID)
	if err != nil {
		return err
	}
	code = strings.ToLower(strings.TrimSpace(code))
	name = strings.TrimSpace(name)
	if code == "" || len(code) > 32 || name == "" || len(name) > 80 {
		return Errf(http.StatusBadRequest, "invalid_request", "A subject needs a code (max 32 characters) and a name (max 80).")
	}
	return s.Store.SetSubjectOverride(ctx, a.ID, code, name)
}

func (s *Service) ResetSubject(ctx context.Context, p Principal, accountID, code string) error {
	a, _, err := s.account(ctx, p, accountID)
	if err != nil {
		return err
	}
	return s.Store.DeleteSubjectOverride(ctx, a.ID, strings.ToLower(strings.TrimSpace(code)))
}
