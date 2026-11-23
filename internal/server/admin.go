package server

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"sort"
	"strings"
	"time"

	"github.com/matthiaslubbertsen/melon/internal/core"
	"github.com/matthiaslubbertsen/melon/internal/secret"
	"github.com/matthiaslubbertsen/melon/internal/store"
	"github.com/matthiaslubbertsen/melon/internal/subjects"
	"github.com/matthiaslubbertsen/melon/internal/zermelo"
)

// The admin dashboard. Who is an admin comes from MELON_ADMINS: a profile id,
// or a Zermelo account (school and user code). An account counts once it is
// linked to the profile, and linking proves it with Zermelo, so naming your
// own student number is enough and nobody can claim it without its login.
//
// Admins can open any profile as its owner. That session is marked, short,
// never slides, and the admin's own session waits in a cookie that only the
// app's API sees, so ending it (or letting it run out) puts them back.

const (
	adminCookie      = "melon_admin"
	impersonationTTL = 2 * time.Hour
)

// Admins is the parsed MELON_ADMINS setting.
type Admins struct {
	profiles map[string]bool
	accounts map[[2]string]bool // school, lowercase user code
}

// ParseAdmins reads a comma separated list of profile ids (usr_...) and
// Zermelo accounts written as school:usercode. A bare user code means the
// default school, and is refused when there is none.
func ParseAdmins(v, defaultSchool string) (Admins, error) {
	a := Admins{profiles: map[string]bool{}, accounts: map[[2]string]bool{}}
	for _, entry := range strings.FieldsFunc(v, func(r rune) bool { return r == ',' || r == ' ' || r == ';' }) {
		if strings.HasPrefix(entry, "usr_") {
			a.profiles[entry] = true
			continue
		}
		school, code, ok := strings.Cut(entry, ":")
		if !ok {
			school, code = defaultSchool, entry
			if school == "" {
				return a, fmt.Errorf("admin %q needs a school (school:%s) because MELON_DEFAULT_SCHOOL is not set", entry, entry)
			}
		}
		norm, valid := zermelo.NormalizeSchool(school)
		code = strings.ToLower(strings.TrimSpace(code))
		if !valid || code == "" {
			return a, fmt.Errorf("admin %q is not a profile id (usr_...) or school:usercode", entry)
		}
		a.accounts[[2]string{norm, code}] = true
	}
	return a, nil
}

// Count is how many admins are configured.
func (a Admins) Count() int { return len(a.profiles) + len(a.accounts) }

// Schools lists the schools admin accounts belong to.
func (a Admins) Schools() []string {
	var out []string
	for k := range a.accounts {
		out = append(out, k[0])
	}
	return out
}

func (a Admins) match(userID string, accounts []store.Account) bool {
	if a.profiles[userID] {
		return true
	}
	for _, acc := range accounts {
		if a.accounts[[2]string{acc.School, strings.ToLower(acc.TokenUser())}] {
			return true
		}
	}
	return false
}

func (s *Server) isAdmin(ctx context.Context, userID string) (bool, error) {
	if s.Admins.Count() == 0 {
		return false, nil
	}
	if s.Admins.profiles[userID] {
		return true, nil
	}
	accounts, err := s.Store.Accounts(ctx, userID)
	if err != nil {
		return false, err
	}
	return s.Admins.match(userID, accounts), nil
}

// adminOnly admits a browser session of an admin that is not, right now,
// looking at Melon as someone else.
func (s *Server) adminOnly(next handler) http.HandlerFunc {
	return s.sessionOnly(func(w http.ResponseWriter, r *http.Request, p core.Principal) {
		if _, by := s.sessionInfo(r); by != "" {
			writeErr(w, core.Errf(http.StatusForbidden, "impersonating", "Stop viewing as someone else first."))
			return
		}
		ok, err := s.isAdmin(r.Context(), p.UserID)
		if err != nil {
			writeErr(w, err)
			return
		}
		if !ok {
			writeErr(w, core.Errf(http.StatusForbidden, "forbidden", "This needs an admin."))
			return
		}
		next(w, r, p)
	})
}

func (s *Server) audit(ctx context.Context, p core.Principal, action, target, detail string) {
	if err := s.Store.AddLog(ctx, p.UserID, action, target, detail); err != nil {
		// The action itself already happened; say so where an operator looks.
		slog.Error("cannot write the admin log", "admin", p.UserID, "action", action, "target", target, "err", err)
	}
}

func (s *Server) adminRoutes(mux *http.ServeMux) {
	mux.HandleFunc("GET /api/app/admin/overview", s.adminOnly(s.handleAdminOverview))
	mux.HandleFunc("GET /api/app/admin/profiles", s.adminOnly(s.handleAdminProfiles))
	mux.HandleFunc("GET /api/app/admin/profiles/{id}", s.adminOnly(s.handleAdminProfile))
	mux.HandleFunc("DELETE /api/app/admin/profiles/{id}", s.adminOnly(s.handleAdminDeleteProfile))
	mux.HandleFunc("POST /api/app/admin/profiles/{id}/impersonate", s.adminOnly(s.handleImpersonate))
	mux.HandleFunc("PUT /api/app/admin/profiles/{id}/pin", s.adminOnly(s.handleAdminSetPIN))
	mux.HandleFunc("DELETE /api/app/admin/profiles/{id}/pin", s.adminOnly(s.handleAdminClearPIN))
	mux.HandleFunc("POST /api/app/admin/profiles/{id}/unlock", s.adminOnly(s.handleAdminUnlock))
	mux.HandleFunc("DELETE /api/app/admin/profiles/{id}/sessions", s.adminOnly(s.handleAdminSignOut))
	mux.HandleFunc("GET /api/app/admin/accounts/{id}/token", s.adminOnly(s.handleAdminZermeloToken))
	mux.HandleFunc("POST /api/app/admin/accounts/{id}/check", s.adminOnly(s.handleAdminCheckAccount))
	mux.HandleFunc("DELETE /api/app/admin/keys/{id}", s.adminOnly(s.handleAdminDeleteKey))
	mux.HandleFunc("GET /api/app/admin/subjects", s.adminOnly(s.handleAdminSubjects))
	mux.HandleFunc("PUT /api/app/admin/subjects/{code}", s.adminOnly(s.handleAdminSetSubject))
	mux.HandleFunc("DELETE /api/app/admin/subjects/{code}", s.adminOnly(s.handleAdminDeleteSubject))
	mux.HandleFunc("GET /api/app/admin/teachers", s.adminOnly(s.handleAdminTeachers))
	mux.HandleFunc("PUT /api/app/admin/teachers/{school}/{code}", s.adminOnly(s.handleAdminSetTeacher))
	mux.HandleFunc("DELETE /api/app/admin/teachers/{school}/{code}", s.adminOnly(s.handleAdminDeleteTeacher))
	mux.HandleFunc("GET /api/app/admin/suggestions", s.adminOnly(s.handleAdminSuggestions))
	mux.HandleFunc("GET /api/app/admin/log", s.adminOnly(s.handleAdminLog))
	// Not admin-gated: the session is the other profile's until this ends it.
	mux.HandleFunc("POST /api/app/admin/stop", s.handleStopImpersonating)
}

// ---- overview ----

func (s *Server) handleAdminOverview(w http.ResponseWriter, r *http.Request, _ core.Principal) {
	out, err := s.overview(r.Context(), lang(r))
	if err != nil {
		writeErr(w, err)
		return
	}
	writeData(w, out)
}

func (s *Server) overview(ctx context.Context, l string) (map[string]any, error) {
	// Show what was counted up to now, not up to the last flush.
	if err := s.Core.Usage.Flush(ctx, s.Store); err != nil {
		return nil, err
	}
	stats, err := s.Store.Stats(ctx)
	if err != nil {
		return nil, err
	}
	planner, err := s.planner(ctx, l)
	if err != nil {
		return nil, err
	}
	unreadFeedback, err := s.Store.UnreadFeedback(ctx)
	if err != nil {
		return nil, err
	}
	since := s.Core.Today().AddDate(0, 0, -29).Format(time.DateOnly)
	usage, err := s.Store.UsageSince(ctx, since)
	if err != nil {
		return nil, err
	}
	requests, lessons, err := s.Store.UsageTotal(ctx)
	if err != nil {
		return nil, err
	}
	schools, err := s.Store.Schools(ctx)
	if err != nil {
		return nil, err
	}
	return map[string]any{
		"stats":          stats,
		"unreadFeedback": unreadFeedback,
		"totals":         map[string]int64{"requests": requests, "lessons": lessons},
		"since":          since,
		"today":          s.Core.Today().Format(time.DateOnly),
		"usage":          usage,
		"schools":        schools,
		"planner":        planner,
	}, nil
}

type plannerSubject struct {
	store.PlannerSubject
	Name string `json:"name"`
}

type plannerDay struct {
	Day        string `json:"day"`
	Homework   int    `json:"homework"`
	Detentions int    `json:"detentions"`
}

// planner is what people do with homework and detentions: totals, the
// subjects they are for, and how many were added each of the last 30 days.
func (s *Server) planner(ctx context.Context, lang string) (map[string]any, error) {
	today := s.Core.Today()
	stats, err := s.Store.Planner(ctx, today.Format(time.DateOnly))
	if err != nil {
		return nil, err
	}
	top, err := s.Store.PlannerSubjects(ctx, 10)
	if err != nil {
		return nil, err
	}
	names := map[string]string{}
	if defaults, err := s.Core.DefaultSubjects(ctx, lang); err == nil {
		for _, d := range defaults {
			names[strings.ToLower(d.Code)] = d.Name
		}
	}
	subjects := make([]plannerSubject, 0, len(top))
	for _, ps := range top {
		name := names[ps.Code]
		if name == "" {
			name = strings.ToUpper(ps.Code)
		}
		subjects = append(subjects, plannerSubject{PlannerSubject: ps, Name: name})
	}
	first := today.AddDate(0, 0, -29)
	homework, detentions, err := s.Store.PlannerAddedSince(ctx, first.Unix())
	if err != nil {
		return nil, err
	}
	days := make([]plannerDay, 30)
	index := map[string]int{}
	for i := range days {
		days[i].Day = first.AddDate(0, 0, i).Format(time.DateOnly)
		index[days[i].Day] = i
	}
	count := func(list []int64, add func(*plannerDay)) {
		for _, at := range list {
			if i, ok := index[time.Unix(at, 0).In(s.Core.Loc).Format(time.DateOnly)]; ok {
				add(&days[i])
			}
		}
	}
	count(homework, func(d *plannerDay) { d.Homework++ })
	count(detentions, func(d *plannerDay) { d.Detentions++ })
	return map[string]any{"stats": stats, "subjects": subjects, "days": days}, nil
}

// ---- profiles ----

type adminAccount struct {
	core.Account
	Calendar  bool      `json:"calendar"`
	CreatedAt time.Time `json:"createdAt"`
	// TokenExpires is when Zermelo said the token stops working, if it said.
	TokenExpires *time.Time `json:"tokenExpires"`
}

type adminProfile struct {
	ID         string         `json:"id"`
	CreatedAt  time.Time      `json:"createdAt"`
	LastSeenAt *time.Time     `json:"lastSeenAt"`
	Login      *loginView     `json:"login"`
	Accounts   []adminAccount `json:"accounts"`
	Keys       int            `json:"keys"`
	Admin      bool           `json:"admin"`
	// Self marks the admin's own profile, which cannot be viewed as or deleted here.
	Self bool `json:"self"`
	// PIN says whether the profile can be signed in to with a user number and PIN.
	PIN bool `json:"pin"`
}

func (s *Server) profileView(p store.ProfileSummary, viewer string) adminProfile {
	out := adminProfile{ID: p.ID, Self: p.ID == viewer, CreatedAt: time.Unix(p.CreatedAt, 0).UTC(), LastSeenAt: unixPtr(p.LastSeen), Keys: p.Keys, Accounts: []adminAccount{}, Admin: s.Admins.match(p.ID, p.Accounts), PIN: p.Login != nil}
	if p.Login != nil {
		out.Login = &loginView{UserID: p.Login.Name, School: p.Login.School}
	}
	for _, a := range p.Accounts {
		out.Accounts = append(out.Accounts, adminAccount{
			Account:      core.Account{ID: a.ID, School: a.School, Name: a.DisplayName, Role: a.Role, UserCode: a.ZermeloUser, CanViewOthers: a.Wide, Nickname: a.Nickname, ParentCode: a.OwnerCode, ParentName: a.OwnerName},
			Calendar:     a.CalEnc != nil,
			CreatedAt:    time.Unix(a.CreatedAt, 0).UTC(),
			TokenExpires: unixPtr(a.TokenExpires),
		})
	}
	return out
}

func (s *Server) handleAdminProfiles(w http.ResponseWriter, r *http.Request, viewer core.Principal) {
	q := r.URL.Query().Get("q")
	if len(q) > 64 {
		q = q[:64]
	}
	pin := r.URL.Query().Get("pin")
	if pin != "yes" && pin != "no" {
		pin = ""
	}
	rows, err := s.Store.Profiles(r.Context(), q, pin, 50)
	if err != nil {
		writeErr(w, err)
		return
	}
	out := []adminProfile{}
	for _, p := range rows {
		out = append(out, s.profileView(p, viewer.UserID))
	}
	writeData(w, out)
}

// profile loads the profile named in the path, or answers 404.
func (s *Server) profile(w http.ResponseWriter, r *http.Request) (store.ProfileSummary, bool) {
	p, err := s.Store.Profile(r.Context(), r.PathValue("id"))
	if errors.Is(err, store.ErrNotFound) {
		writeErr(w, core.ErrNotFound)
		return p, false
	} else if err != nil {
		writeErr(w, err)
		return p, false
	}
	return p, true
}

func (s *Server) handleAdminProfile(w http.ResponseWriter, r *http.Request, viewer core.Principal) {
	out, err := s.profileDetail(r.Context(), r.PathValue("id"), viewer.UserID)
	if err != nil {
		writeErr(w, err)
		return
	}
	writeData(w, out)
}

// profileDetail is everything an admin sees about one profile. Secrets stay
// out: API keys by hint, Zermelo tokens not at all.
func (s *Server) profileDetail(ctx context.Context, id, viewer string) (map[string]any, error) {
	p, err := s.Store.Profile(ctx, id)
	if errors.Is(err, store.ErrNotFound) {
		return nil, core.ErrNotFound
	} else if err != nil {
		return nil, err
	}
	type lock struct {
		Fails       int64      `json:"fails"`
		LockedUntil *time.Time `json:"lockedUntil"`
	}
	var pinLock *lock
	if p.Login != nil {
		fails, until, err := s.Store.LoginLock(ctx, p.ID)
		if err != nil && !errors.Is(err, store.ErrNotFound) {
			return nil, err
		}
		pinLock = &lock{Fails: fails}
		if until > time.Now().Unix() {
			pinLock.LockedUntil = unixPtr(until)
		}
	}
	sessions, err := s.Store.SessionCount(ctx, p.ID)
	if err != nil {
		return nil, err
	}
	keys, err := s.Store.APIKeys(ctx, p.ID)
	if err != nil {
		return nil, err
	}
	keyViews := []apiKeyView{}
	for _, k := range keys {
		keyViews = append(keyViews, apiKeyView{ID: k.ID, Name: k.Name, Hint: k.Hint, AccountIDs: k.AccountIDs, CreatedAt: time.Unix(k.CreatedAt, 0).UTC(), LastUsedAt: unixPtr(k.LastUsed), Admin: k.Admin})
	}
	grants, err := s.Store.OAuthGrants(ctx, p.ID)
	if err != nil {
		return nil, err
	}
	grantViews := []map[string]any{}
	for _, g := range grants {
		grantViews = append(grantViews, map[string]any{"id": g.ID, "app": g.ClientName, "accountIds": g.AccountIDs, "createdAt": time.Unix(g.CreatedAt, 0).UTC(), "lastUsedAt": unixPtr(g.LastUsed), "admin": g.Admin})
	}
	subjectNames, teacherNames, err := s.Store.NameCounts(ctx, p.ID)
	if err != nil {
		return nil, err
	}
	return map[string]any{
		"profile":  s.profileView(p, viewer),
		"pinLock":  pinLock,
		"sessions": sessions,
		"keys":     keyViews,
		"grants":   grantViews,
		"names":    map[string]int{"subjects": subjectNames, "teachers": teacherNames},
	}, nil
}

func (s *Server) handleAdminDeleteProfile(w http.ResponseWriter, r *http.Request, p core.Principal) {
	target, ok := s.profile(w, r)
	if !ok {
		return
	}
	if target.ID == p.UserID {
		writeErr(w, core.Errf(http.StatusBadRequest, "invalid_request", "Delete your own profile from the settings instead."))
		return
	}
	if err := s.Store.DeleteUser(r.Context(), target.ID); err != nil {
		writeErr(w, err)
		return
	}
	for _, a := range target.Accounts {
		s.Core.Cache.Drop("acct:" + a.ID + ":")
	}
	s.audit(r.Context(), p, "delete_profile", target.ID, describe(target))
	w.WriteHeader(http.StatusNoContent)
}

// describe names a profile for the audit log: its accounts, so the entry
// still means something once the profile is gone.
func describe(p store.ProfileSummary) string {
	var parts []string
	for _, a := range p.Accounts {
		parts = append(parts, fmt.Sprintf("%s (%s:%s)", a.DisplayName, a.School, a.ZermeloUser))
	}
	return strings.Join(parts, ", ")
}

// ---- impersonation ----

func (s *Server) handleImpersonate(w http.ResponseWriter, r *http.Request, p core.Principal) {
	target, ok := s.profile(w, r)
	if !ok {
		return
	}
	if target.ID == p.UserID {
		writeErr(w, core.Errf(http.StatusBadRequest, "invalid_request", "That is your own profile."))
		return
	}
	own, err := r.Cookie(sessionCookie)
	if err != nil {
		writeErr(w, core.Errf(http.StatusUnauthorized, "unauthorized", "This needs a browser session."))
		return
	}
	token := secret.Token("")
	if err := s.Store.CreateImpersonation(r.Context(), secret.Hash(token), target.ID, p.UserID, impersonationTTL); err != nil {
		writeErr(w, err)
		return
	}
	// The admin's own session waits here, for the app's API only.
	http.SetCookie(w, &http.Cookie{Name: adminCookie, Value: own.Value, Path: "/api/app/", MaxAge: int(sessionTTL.Seconds()), HttpOnly: true, Secure: s.isHTTPS(r), SameSite: http.SameSiteStrictMode})
	http.SetCookie(w, &http.Cookie{Name: sessionCookie, Value: token, Path: "/", MaxAge: int(impersonationTTL.Seconds()), HttpOnly: true, Secure: s.isHTTPS(r), SameSite: http.SameSiteLaxMode})
	s.audit(r.Context(), p, "impersonate", target.ID, describe(target))
	out, err := s.sessionBody(r, target.ID, p.UserID)
	if err != nil {
		writeErr(w, err)
		return
	}
	writeData(w, out)
}

// restoreAdmin puts the admin's own session back after viewing as someone
// else, and reports whose it is ("" when there is none to restore).
func (s *Server) restoreAdmin(w http.ResponseWriter, r *http.Request) string {
	c, err := r.Cookie(adminCookie)
	if err != nil || c.Value == "" {
		return ""
	}
	http.SetCookie(w, &http.Cookie{Name: adminCookie, Path: "/api/app/", MaxAge: -1, HttpOnly: true, Secure: s.isHTTPS(r), SameSite: http.SameSiteStrictMode})
	user, by, err := s.Store.Session(r.Context(), secret.Hash(c.Value), sessionTTL)
	if err != nil || by != "" {
		return ""
	}
	http.SetCookie(w, &http.Cookie{
		Name: sessionCookie, Value: c.Value, Path: "/", MaxAge: int(sessionTTL.Seconds()),
		HttpOnly: true, Secure: s.isHTTPS(r), SameSite: http.SameSiteLaxMode,
	})
	return user
}

func (s *Server) handleStopImpersonating(w http.ResponseWriter, r *http.Request) {
	if !sameOrigin(r) {
		writeErr(w, core.Errf(http.StatusForbidden, "forbidden", "Cross-site requests are not allowed."))
		return
	}
	user, by := s.sessionInfo(r)
	if user != "" && by == "" {
		writeErr(w, core.Errf(http.StatusConflict, "not_impersonating", "You are not viewing as someone else."))
		return
	}
	if c, err := r.Cookie(sessionCookie); err == nil && user != "" {
		if err := s.Store.DeleteSession(r.Context(), secret.Hash(c.Value)); err != nil {
			writeErr(w, err)
			return
		}
	}
	admin := s.restoreAdmin(w, r)
	if admin == "" {
		s.clearSessionCookie(w, r)
	}
	out, err := s.sessionBody(r, admin, "")
	if err != nil {
		writeErr(w, err)
		return
	}
	writeData(w, out)
}

// ---- sign-in help ----

func (s *Server) handleAdminSetPIN(w http.ResponseWriter, r *http.Request, p core.Principal) {
	target, ok := s.profile(w, r)
	if !ok {
		return
	}
	var body struct {
		AccountID string `json:"accountId"`
		PIN       string `json:"pin"`
	}
	if err := readJSON(r, &body); err != nil {
		writeErr(w, err)
		return
	}
	if !pinShape.MatchString(body.PIN) {
		writeErr(w, core.Errf(http.StatusBadRequest, "invalid_pin", "A PIN is exactly four digits."))
		return
	}
	var acc *store.Account
	for i := range target.Accounts {
		if target.Accounts[i].ID == body.AccountID {
			acc = &target.Accounts[i]
		}
	}
	if acc == nil {
		writeErr(w, core.Errf(http.StatusBadRequest, "invalid_request", "Pick one of the accounts linked to this profile to sign in with."))
		return
	}
	l := store.Login{UserID: target.ID, Name: acc.TokenUser(), School: acc.School, PINHash: s.Core.Box.PINHash(target.ID, body.PIN)}
	if err := s.Store.SetLogin(r.Context(), l); err != nil {
		writeErr(w, err)
		return
	}
	s.audit(r.Context(), p, "set_pin", target.ID, fmt.Sprintf("sign in as %s:%s", acc.School, acc.TokenUser()))
	writeData(w, loginView{UserID: l.Name, School: l.School})
}

func (s *Server) handleAdminClearPIN(w http.ResponseWriter, r *http.Request, p core.Principal) {
	target, ok := s.profile(w, r)
	if !ok {
		return
	}
	if err := s.Store.DeleteLogin(r.Context(), target.ID); err != nil {
		writeErr(w, err)
		return
	}
	s.audit(r.Context(), p, "remove_pin", target.ID, "")
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) handleAdminUnlock(w http.ResponseWriter, r *http.Request, p core.Principal) {
	target, ok := s.profile(w, r)
	if !ok {
		return
	}
	if err := s.Store.EndPINAttempt(r.Context(), target.ID); err != nil {
		writeErr(w, err)
		return
	}
	s.audit(r.Context(), p, "unlock_pin", target.ID, "")
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) handleAdminSignOut(w http.ResponseWriter, r *http.Request, p core.Principal) {
	target, ok := s.profile(w, r)
	if !ok {
		return
	}
	if target.ID == p.UserID {
		writeErr(w, core.Errf(http.StatusBadRequest, "invalid_request", "That would sign you out too."))
		return
	}
	if err := s.Store.DeleteSessions(r.Context(), target.ID); err != nil {
		writeErr(w, err)
		return
	}
	s.audit(r.Context(), p, "sign_out", target.ID, "")
	w.WriteHeader(http.StatusNoContent)
}

// ---- API keys ----

// handleAdminZermeloToken shows the Zermelo token Melon holds for an account
// (what the link code was exchanged for). Every look is logged.
func (s *Server) handleAdminZermeloToken(w http.ResponseWriter, r *http.Request, p core.Principal) {
	a, token, err := s.Core.AnyAccount(r.Context(), r.PathValue("id"))
	if err != nil {
		writeErr(w, err)
		return
	}
	s.audit(r.Context(), p, "reveal_token", a.UserID, fmt.Sprintf("%s (%s:%s)", a.DisplayName, a.School, a.ZermeloUser))
	writeData(w, map[string]any{"token": token, "school": a.School, "expiresAt": unixPtr(a.TokenExpires)})
}

func (s *Server) handleAdminDeleteKey(w http.ResponseWriter, r *http.Request, p core.Principal) {
	id := r.PathValue("id")
	owner, err := s.Store.APIKeyOwner(r.Context(), id)
	if errors.Is(err, store.ErrNotFound) {
		writeErr(w, core.ErrNotFound)
		return
	} else if err != nil {
		writeErr(w, err)
		return
	}
	if err := s.Store.DeleteAPIKey(r.Context(), owner, id); err != nil {
		writeErr(w, err)
		return
	}
	s.audit(r.Context(), p, "revoke_key", owner, id)
	w.WriteHeader(http.StatusNoContent)
}

// ---- names ----

type adminSubject struct {
	Code      string `json:"code"`
	NL        string `json:"nl"`
	EN        string `json:"en"`
	BuiltinNL string `json:"builtinNl"`
	BuiltinEN string `json:"builtinEn"`
	// Global is true when an admin set this name; otherwise it is built in.
	Global bool `json:"global"`
}

func (s *Server) handleAdminSubjects(w http.ResponseWriter, r *http.Request, _ core.Principal) {
	rows, err := s.Store.GlobalSubjects(r.Context())
	if err != nil {
		writeErr(w, err)
		return
	}
	byCode := map[string]adminSubject{}
	for code, n := range subjects.Defaults {
		byCode[code] = adminSubject{Code: code, NL: n.NL, EN: n.EN, BuiltinNL: n.NL, BuiltinEN: n.EN}
	}
	for _, g := range rows {
		e := byCode[g.Code]
		e.Code, e.NL, e.EN, e.Global = g.Code, g.NL, g.EN, true
		byCode[g.Code] = e
	}
	out := make([]adminSubject, 0, len(byCode))
	for _, e := range byCode {
		out = append(out, e)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Code < out[j].Code })
	writeData(w, out)
}

func cleanCode(code string) (string, bool) {
	code = strings.ToLower(strings.TrimSpace(code))
	return code, code != "" && len(code) <= 32
}

func (s *Server) handleAdminSetSubject(w http.ResponseWriter, r *http.Request, p core.Principal) {
	var body struct {
		NL string `json:"nl"`
		EN string `json:"en"`
	}
	if err := readJSON(r, &body); err != nil {
		writeErr(w, err)
		return
	}
	code, ok := cleanCode(r.PathValue("code"))
	body.NL, body.EN = strings.TrimSpace(body.NL), strings.TrimSpace(body.EN)
	if !ok || body.NL == "" || len(body.NL) > 80 || len(body.EN) > 80 {
		writeErr(w, core.Errf(http.StatusBadRequest, "invalid_request", "A subject needs a code (max 32 characters) and a Dutch name (max 80)."))
		return
	}
	if err := s.Store.SetGlobalSubject(r.Context(), store.GlobalSubject{Code: code, NL: body.NL, EN: body.EN}); err != nil {
		writeErr(w, err)
		return
	}
	s.audit(r.Context(), p, "set_subject", code, body.NL+" / "+body.EN)
	writeData(w, store.GlobalSubject{Code: code, NL: body.NL, EN: body.EN})
}

func (s *Server) handleAdminDeleteSubject(w http.ResponseWriter, r *http.Request, p core.Principal) {
	code, _ := cleanCode(r.PathValue("code"))
	if err := s.Store.DeleteGlobalSubject(r.Context(), code); errors.Is(err, store.ErrNotFound) {
		writeErr(w, core.ErrNotFound)
		return
	} else if err != nil {
		writeErr(w, err)
		return
	}
	s.audit(r.Context(), p, "reset_subject", code, "")
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) school(w http.ResponseWriter, v string) (string, bool) {
	school, ok := zermelo.NormalizeSchool(v)
	if !ok {
		writeErr(w, core.Errf(http.StatusBadRequest, "invalid_request", "That is not a portal name like \"jfc\"."))
	}
	return school, ok
}

func (s *Server) handleAdminTeachers(w http.ResponseWriter, r *http.Request, _ core.Principal) {
	school, ok := s.school(w, r.URL.Query().Get("school"))
	if !ok {
		return
	}
	names, err := s.Store.SchoolTeachers(r.Context(), school)
	if err != nil {
		writeErr(w, err)
		return
	}
	out := []core.Teacher{}
	for code, name := range names {
		out = append(out, core.Teacher{Code: code, Name: name})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Code < out[j].Code })
	writeData(w, out)
}

func (s *Server) handleAdminSetTeacher(w http.ResponseWriter, r *http.Request, p core.Principal) {
	var body struct {
		Name string `json:"name"`
	}
	if err := readJSON(r, &body); err != nil {
		writeErr(w, err)
		return
	}
	school, ok := s.school(w, r.PathValue("school"))
	if !ok {
		return
	}
	code, ok := cleanCode(r.PathValue("code"))
	body.Name = strings.TrimSpace(body.Name)
	if !ok || body.Name == "" || len(body.Name) > 80 {
		writeErr(w, core.Errf(http.StatusBadRequest, "invalid_request", "A teacher needs a code (max 32 characters) and a name (max 80)."))
		return
	}
	if err := s.Store.SetSchoolTeacher(r.Context(), school, code, body.Name); err != nil {
		writeErr(w, err)
		return
	}
	s.audit(r.Context(), p, "set_teacher", school+":"+code, body.Name)
	writeData(w, core.Teacher{Code: code, Name: body.Name})
}

func (s *Server) handleAdminDeleteTeacher(w http.ResponseWriter, r *http.Request, p core.Principal) {
	school, ok := s.school(w, r.PathValue("school"))
	if !ok {
		return
	}
	code, _ := cleanCode(r.PathValue("code"))
	if err := s.Store.DeleteSchoolTeacher(r.Context(), school, code); errors.Is(err, store.ErrNotFound) {
		writeErr(w, core.ErrNotFound)
		return
	} else if err != nil {
		writeErr(w, err)
		return
	}
	s.audit(r.Context(), p, "reset_teacher", school+":"+code, "")
	w.WriteHeader(http.StatusNoContent)
}

// handleAdminSuggestions lists the names users gave codes on their own
// accounts that differ from what everyone gets now: candidates for a global
// subject name or a school's teacher name.
func (s *Server) handleAdminSuggestions(w http.ResponseWriter, r *http.Request, _ core.Principal) {
	out, err := s.suggestions(r.Context())
	if err != nil {
		writeErr(w, err)
		return
	}
	writeData(w, out)
}

type suggestion struct {
	store.Suggestion
	Current string `json:"current"`
}

func (s *Server) suggestions(ctx context.Context) (map[string]any, error) {
	globalRows, err := s.Store.GlobalSubjects(ctx)
	if err != nil {
		return nil, err
	}
	global := map[string]subjects.Name{}
	for _, g := range globalRows {
		global[g.Code] = subjects.Name{NL: g.NL, EN: g.EN}
	}
	subjectRows, err := s.Store.SubjectSuggestions(ctx)
	if err != nil {
		return nil, err
	}
	subjectOut := []suggestion{}
	for _, sg := range subjectRows {
		cur, _ := subjects.Lookup(sg.Code, global)
		if strings.EqualFold(sg.Name, cur.NL) || strings.EqualFold(sg.Name, cur.EN) {
			continue
		}
		subjectOut = append(subjectOut, suggestion{sg, cur.NL})
	}
	teacherRows, err := s.Store.TeacherSuggestions(ctx)
	if err != nil {
		return nil, err
	}
	perSchool := map[string]map[string]string{}
	teacherOut := []suggestion{}
	for _, sg := range teacherRows {
		names, ok := perSchool[sg.School]
		if !ok {
			if names, err = s.Store.SchoolTeachers(ctx, sg.School); err != nil {
				return nil, err
			}
			perSchool[sg.School] = names
		}
		cur := names[sg.Code]
		if strings.EqualFold(sg.Name, cur) {
			continue
		}
		teacherOut = append(teacherOut, suggestion{sg, cur})
	}
	return map[string]any{"subjects": subjectOut, "teachers": teacherOut}, nil
}

// ---- audit log ----

func (s *Server) handleAdminLog(w http.ResponseWriter, r *http.Request, _ core.Principal) {
	out, err := s.adminLog(r.Context(), 200)
	if err != nil {
		writeErr(w, err)
		return
	}
	writeData(w, out)
}

type logEntry struct {
	store.LogEntry
	At        time.Time `json:"at"`
	AdminName string    `json:"adminName"`
}

func (s *Server) adminLog(ctx context.Context, limit int) ([]logEntry, error) {
	rows, err := s.Store.Log(ctx, limit)
	if err != nil {
		return nil, err
	}
	// Name admins by their first account; ids alone are hard to read.
	names := map[string]string{}
	out := []logEntry{}
	for _, e := range rows {
		name, ok := names[e.Admin]
		if !ok {
			if accounts, err := s.Store.Accounts(ctx, e.Admin); err == nil && len(accounts) > 0 {
				name = accounts[0].DisplayName
			}
			names[e.Admin] = name
		}
		out = append(out, logEntry{e, time.Unix(e.At, 0).UTC(), name})
	}
	return out, nil
}

// stillAdmin re-checks MELON_ADMINS for a credential that opted in to the
// admin tools, so taking someone off the list takes effect at once.
func (s *Server) stillAdmin(r *http.Request, userID string) bool {
	ok, err := s.isAdmin(r.Context(), userID)
	if err != nil {
		slog.Warn("cannot check whether the profile is an admin", "err", err)
	}
	return ok
}

// handleAdminCheckAccount asks Zermelo, live, whether the account still works.
func (s *Server) handleAdminCheckAccount(w http.ResponseWriter, r *http.Request, _ core.Principal) {
	out, err := s.Core.CheckAccount(r.Context(), r.PathValue("id"))
	if err != nil {
		writeErr(w, err)
		return
	}
	writeData(w, out)
}
