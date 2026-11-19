package store

import (
	"context"
	"database/sql"
	"strings"
)

// Everything an admin looks at or changes across profiles. Nothing here is
// reachable without the admin check in the server package.

// ---- overview ----

type Stats struct {
	Profiles      int `json:"profiles"`
	ProfilesNew7d int `json:"profilesNew7d"`
	WithPIN       int `json:"withPin"`
	LockedPINs    int `json:"lockedPins"`
	Accounts      int `json:"accounts"`
	Students      int `json:"students"`
	Staff         int `json:"staff"`
	Parents       int `json:"parents"`
	Schools       int `json:"schools"`
	Active1d      int `json:"active1d"`
	Active7d      int `json:"active7d"`
	Active30d     int `json:"active30d"`
	Sessions      int `json:"sessions"`
	APIKeys       int `json:"apiKeys"`
	OAuthApps     int `json:"oauthApps"`
	Calendars     int `json:"calendars"`
	SubjectNames  int `json:"subjectNames"`
	TeacherNames  int `json:"teacherNames"`
}

func (s *Store) Stats(ctx context.Context) (Stats, error) {
	n := now()
	var st Stats
	// Active means a session was used: last_seen moves at most once an hour,
	// which is plenty for counting days.
	err := s.db.QueryRowContext(ctx, `SELECT
		(SELECT COUNT(*) FROM users),
		(SELECT COUNT(*) FROM users WHERE created_at > ?),
		(SELECT COUNT(*) FROM logins),
		(SELECT COUNT(*) FROM logins WHERE locked_until > ?),
		(SELECT COUNT(*) FROM accounts),
		(SELECT COUNT(*) FROM accounts WHERE role = 'student'),
		(SELECT COUNT(*) FROM accounts WHERE role = 'employee'),
		(SELECT COUNT(*) FROM accounts WHERE role = 'parent'),
		(SELECT COUNT(DISTINCT school) FROM accounts),
		(SELECT COUNT(DISTINCT user_id) FROM sessions WHERE last_seen > ? AND impersonator IS NULL),
		(SELECT COUNT(DISTINCT user_id) FROM sessions WHERE last_seen > ? AND impersonator IS NULL),
		(SELECT COUNT(DISTINCT user_id) FROM sessions WHERE last_seen > ? AND impersonator IS NULL),
		(SELECT COUNT(*) FROM sessions WHERE expires_at > ? AND impersonator IS NULL),
		(SELECT COUNT(*) FROM api_keys),
		(SELECT COUNT(DISTINCT grant_id) FROM oauth_tokens WHERE expires_at > ?),
		(SELECT COUNT(*) FROM accounts WHERE cal_hash IS NOT NULL),
		(SELECT COUNT(*) FROM subject_overrides),
		(SELECT COUNT(*) FROM teacher_names)`,
		n-7*86400, n, n-86400, n-7*86400, n-30*86400, n, n).Scan(
		&st.Profiles, &st.ProfilesNew7d, &st.WithPIN, &st.LockedPINs, &st.Accounts, &st.Students, &st.Staff, &st.Parents, &st.Schools,
		&st.Active1d, &st.Active7d, &st.Active30d, &st.Sessions, &st.APIKeys, &st.OAuthApps, &st.Calendars, &st.SubjectNames, &st.TeacherNames)
	return st, err
}

// Usage is one day of schedule traffic through one channel (web, api_key,
// oauth or calendar).
type Usage struct {
	Day      string `json:"day"`
	Channel  string `json:"channel"`
	Requests int64  `json:"requests"`
	Lessons  int64  `json:"lessons"`
}

func (s *Store) AddUsage(ctx context.Context, u Usage) error {
	_, err := s.db.ExecContext(ctx, `INSERT INTO usage_daily (day, channel, requests, lessons) VALUES (?, ?, ?, ?)
		ON CONFLICT DO UPDATE SET requests = requests + excluded.requests, lessons = lessons + excluded.lessons`,
		u.Day, u.Channel, u.Requests, u.Lessons)
	return err
}

// UsageSince lists the days from since (YYYY-MM-DD) on, oldest first.
func (s *Store) UsageSince(ctx context.Context, since string) ([]Usage, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT day, channel, requests, lessons FROM usage_daily WHERE day >= ? ORDER BY day, channel`, since)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Usage{}
	for rows.Next() {
		var u Usage
		if err := rows.Scan(&u.Day, &u.Channel, &u.Requests, &u.Lessons); err != nil {
			return nil, err
		}
		out = append(out, u)
	}
	return out, rows.Err()
}

// UsageTotal is every request and lesson ever counted.
func (s *Store) UsageTotal(ctx context.Context) (requests, lessons int64, err error) {
	err = s.db.QueryRowContext(ctx, `SELECT COALESCE(SUM(requests), 0), COALESCE(SUM(lessons), 0) FROM usage_daily`).Scan(&requests, &lessons)
	return
}

type School struct {
	School   string `json:"school"`
	Profiles int    `json:"profiles"`
	Accounts int    `json:"accounts"`
	Students int    `json:"students"`
	Staff    int    `json:"staff"`
	Teachers int    `json:"teacherNames"` // names an admin set for this school
}

func (s *Store) Schools(ctx context.Context) ([]School, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT school, SUM(p), SUM(a), SUM(st), SUM(em), SUM(t) FROM (
		SELECT school, COUNT(DISTINCT user_id) AS p, COUNT(*) AS a, SUM(role = 'student') AS st, SUM(role = 'employee') AS em, 0 AS t FROM accounts GROUP BY school
		UNION ALL
		SELECT school, 0, 0, 0, 0, COUNT(*) FROM school_teachers GROUP BY school
	) GROUP BY school ORDER BY SUM(a) DESC, school`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []School{}
	for rows.Next() {
		var sc School
		if err := rows.Scan(&sc.School, &sc.Profiles, &sc.Accounts, &sc.Students, &sc.Staff, &sc.Teachers); err != nil {
			return nil, err
		}
		out = append(out, sc)
	}
	return out, rows.Err()
}

// ---- profiles ----

type ProfileSummary struct {
	ID        string
	CreatedAt int64
	LastSeen  int64
	Login     *Login
	Accounts  []Account
	Keys      int
}

// Profiles finds profiles by profile id, account name, user code, school or
// sign-in number. An empty query lists the most recently active ones. pin
// narrows them to profiles with ("yes") or without ("no") a PIN sign-in.
func (s *Store) Profiles(ctx context.Context, query, pin string, limit int) ([]ProfileSummary, error) {
	q := "%" + strings.NewReplacer(`\`, `\\`, "%", `\%`, "_", `\_`).Replace(strings.TrimSpace(query)) + "%"
	rows, err := s.db.QueryContext(ctx, `SELECT u.id, u.created_at,
			COALESCE((SELECT MAX(last_seen) FROM sessions WHERE user_id = u.id AND impersonator IS NULL), 0) AS seen,
			(SELECT COUNT(*) FROM api_keys WHERE user_id = u.id)
		FROM users u
		WHERE (? = '%%' OR u.id LIKE ? ESCAPE '\'
			OR EXISTS (SELECT 1 FROM accounts a WHERE a.user_id = u.id AND (a.display_name LIKE ? ESCAPE '\' OR a.zermelo_user LIKE ? ESCAPE '\' OR a.school LIKE ? ESCAPE '\' OR a.owner_name LIKE ? ESCAPE '\' OR a.owner_code LIKE ? ESCAPE '\'))
			OR EXISTS (SELECT 1 FROM logins l WHERE l.user_id = u.id AND l.login LIKE ? ESCAPE '\'))
			AND (? = '' OR (? = 'yes') = EXISTS (SELECT 1 FROM logins l WHERE l.user_id = u.id))
		ORDER BY seen DESC, u.created_at DESC LIMIT ?`, q, q, q, q, q, q, q, q, pin, pin, limit)
	if err != nil {
		return nil, err
	}
	var out []ProfileSummary
	for rows.Next() {
		var p ProfileSummary
		if err := rows.Scan(&p.ID, &p.CreatedAt, &p.LastSeen, &p.Keys); err != nil {
			rows.Close()
			return nil, err
		}
		out = append(out, p)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return nil, err
	}
	for i := range out {
		if err := s.fillProfile(ctx, &out[i]); err != nil {
			return nil, err
		}
	}
	return out, nil
}

func (s *Store) fillProfile(ctx context.Context, p *ProfileSummary) error {
	var err error
	if p.Accounts, err = s.Accounts(ctx, p.ID); err != nil {
		return err
	}
	if l, err := s.Login(ctx, p.ID); err == nil {
		p.Login = &l
	} else if err != ErrNotFound {
		return err
	}
	return nil
}

// Profile loads one profile's summary.
func (s *Store) Profile(ctx context.Context, id string) (ProfileSummary, error) {
	p := ProfileSummary{ID: id}
	err := s.db.QueryRowContext(ctx, `SELECT created_at,
		COALESCE((SELECT MAX(last_seen) FROM sessions WHERE user_id = ? AND impersonator IS NULL), 0),
		(SELECT COUNT(*) FROM api_keys WHERE user_id = ?)
		FROM users WHERE id = ?`, id, id, id).Scan(&p.CreatedAt, &p.LastSeen, &p.Keys)
	if err != nil {
		return p, one(err)
	}
	return p, s.fillProfile(ctx, &p)
}

// LoginLock tells how sign-in stands: wrong tries in a row and until when it is locked.
func (s *Store) LoginLock(ctx context.Context, userID string) (fails, lockedUntil int64, err error) {
	err = s.db.QueryRowContext(ctx, `SELECT fails, locked_until FROM logins WHERE user_id = ?`, userID).Scan(&fails, &lockedUntil)
	return fails, lockedUntil, one(err)
}

// SessionCount is how many devices are signed in to the profile.
func (s *Store) SessionCount(ctx context.Context, userID string) (int, error) {
	var n int
	err := s.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM sessions WHERE user_id = ? AND expires_at > ? AND impersonator IS NULL`, userID, now()).Scan(&n)
	return n, err
}

// NameCounts is how many subject and teacher names the profile set itself.
func (s *Store) NameCounts(ctx context.Context, userID string) (subjects, teachers int, err error) {
	err = s.db.QueryRowContext(ctx, `SELECT
		(SELECT COUNT(*) FROM subject_overrides o JOIN accounts a ON a.id = o.account_id WHERE a.user_id = ?),
		(SELECT COUNT(*) FROM teacher_names t JOIN accounts a ON a.id = t.account_id WHERE a.user_id = ?)`, userID, userID).Scan(&subjects, &teachers)
	return
}

// APIKeyOwner returns the profile a key belongs to.
func (s *Store) APIKeyOwner(ctx context.Context, id string) (string, error) {
	var userID string
	err := s.db.QueryRowContext(ctx, `SELECT user_id FROM api_keys WHERE id = ?`, id).Scan(&userID)
	return userID, one(err)
}

// ---- names: global subjects, teachers per school, and what users suggest ----

type GlobalSubject struct {
	Code string `json:"code"`
	NL   string `json:"nl"`
	EN   string `json:"en"`
}

func (s *Store) GlobalSubjects(ctx context.Context) ([]GlobalSubject, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT code, nl, en FROM global_subjects ORDER BY code`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []GlobalSubject{}
	for rows.Next() {
		var g GlobalSubject
		if err := rows.Scan(&g.Code, &g.NL, &g.EN); err != nil {
			return nil, err
		}
		out = append(out, g)
	}
	return out, rows.Err()
}

func (s *Store) SetGlobalSubject(ctx context.Context, g GlobalSubject) error {
	_, err := s.db.ExecContext(ctx, `INSERT INTO global_subjects (code, nl, en, updated_at) VALUES (?, ?, ?, ?) ON CONFLICT DO UPDATE SET nl = excluded.nl, en = excluded.en, updated_at = excluded.updated_at`, g.Code, g.NL, g.EN, now())
	return err
}

func (s *Store) DeleteGlobalSubject(ctx context.Context, code string) error {
	return affected(s.db.ExecContext(ctx, `DELETE FROM global_subjects WHERE code = ?`, code))
}

// SchoolTeachers maps lowercase teacher codes to the names an admin set for the school.
func (s *Store) SchoolTeachers(ctx context.Context, school string) (map[string]string, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT code, name FROM school_teachers WHERE school = ?`, school)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := map[string]string{}
	for rows.Next() {
		var c, n string
		if err := rows.Scan(&c, &n); err != nil {
			return nil, err
		}
		out[c] = n
	}
	return out, rows.Err()
}

func (s *Store) SetSchoolTeacher(ctx context.Context, school, code, name string) error {
	_, err := s.db.ExecContext(ctx, `INSERT INTO school_teachers (school, code, name, updated_at) VALUES (?, ?, ?, ?) ON CONFLICT DO UPDATE SET name = excluded.name, updated_at = excluded.updated_at`, school, code, name, now())
	return err
}

func (s *Store) DeleteSchoolTeacher(ctx context.Context, school, code string) error {
	return affected(s.db.ExecContext(ctx, `DELETE FROM school_teachers WHERE school = ? AND code = ?`, school, code))
}

// Suggestion is a name users gave a code on their own accounts, and how many
// profiles agree on it.
type Suggestion struct {
	School   string   `json:"school,omitempty"`
	Code     string   `json:"code"`
	Name     string   `json:"name"`
	Profiles int      `json:"profiles"`
	Schools  []string `json:"schools,omitempty"`
}

func (s *Store) SubjectSuggestions(ctx context.Context) ([]Suggestion, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT o.code, o.name, COUNT(DISTINCT a.user_id), GROUP_CONCAT(DISTINCT a.school)
		FROM subject_overrides o JOIN accounts a ON a.id = o.account_id
		GROUP BY o.code, o.name ORDER BY COUNT(DISTINCT a.user_id) DESC, o.code LIMIT 500`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Suggestion{}
	for rows.Next() {
		var sg Suggestion
		var schools sql.NullString
		if err := rows.Scan(&sg.Code, &sg.Name, &sg.Profiles, &schools); err != nil {
			return nil, err
		}
		sg.Schools = split(schools.String)
		out = append(out, sg)
	}
	return out, rows.Err()
}

func (s *Store) TeacherSuggestions(ctx context.Context) ([]Suggestion, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT a.school, t.code, t.name, COUNT(DISTINCT a.user_id)
		FROM teacher_names t JOIN accounts a ON a.id = t.account_id
		GROUP BY a.school, t.code, t.name ORDER BY COUNT(DISTINCT a.user_id) DESC, a.school, t.code LIMIT 500`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Suggestion{}
	for rows.Next() {
		var sg Suggestion
		if err := rows.Scan(&sg.School, &sg.Code, &sg.Name, &sg.Profiles); err != nil {
			return nil, err
		}
		out = append(out, sg)
	}
	return out, rows.Err()
}

// ---- audit log ----

type LogEntry struct {
	ID     int64  `json:"id"`
	At     int64  `json:"-"`
	Admin  string `json:"admin"`
	Action string `json:"action"`
	Target string `json:"target"`
	Detail string `json:"detail"`
}

func (s *Store) AddLog(ctx context.Context, admin, action, target, detail string) error {
	_, err := s.db.ExecContext(ctx, `INSERT INTO admin_log (at, admin, action, target, detail) VALUES (?, ?, ?, ?, ?)`, now(), admin, action, target, detail)
	return err
}

func (s *Store) Log(ctx context.Context, limit int) ([]LogEntry, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT id, at, admin, action, target, detail FROM admin_log ORDER BY id DESC LIMIT ?`, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []LogEntry{}
	for rows.Next() {
		var e LogEntry
		if err := rows.Scan(&e.ID, &e.At, &e.Admin, &e.Action, &e.Target, &e.Detail); err != nil {
			return nil, err
		}
		out = append(out, e)
	}
	return out, rows.Err()
}

// UserExists reports whether a profile with this id exists.
func (s *Store) UserExists(ctx context.Context, id string) (bool, error) {
	var n int
	err := s.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM users WHERE id = ?`, id).Scan(&n)
	return n > 0, err
}
