package server

import (
	"errors"
	"net/http"
	"strings"
	"time"

	"github.com/matthiaslubbertsen/melon/internal/core"
	"github.com/matthiaslubbertsen/melon/internal/secret"
	"github.com/matthiaslubbertsen/melon/internal/store"
	"github.com/matthiaslubbertsen/melon/internal/zermelo"
)

// handleSession tells the web app what it is working with. It never fails
// for anonymous visitors; they simply have no accounts yet.
func (s *Server) handleSession(w http.ResponseWriter, r *http.Request) {
	user, by := s.sessionInfo(r)
	if user == "" {
		// Viewing as someone else ran out (or that profile was removed):
		// back to the admin's own session.
		user = s.restoreAdmin(w, r)
	}
	out, err := s.sessionBody(r, user, by)
	if err != nil {
		writeErr(w, err)
		return
	}
	writeData(w, out)
}

// sessionBody describes the profile to the web app. by is the admin viewing
// it as its owner, if any.
func (s *Server) sessionBody(r *http.Request, user, by string) (map[string]any, error) {
	out := map[string]any{"demo": s.Demo, "version": s.Version, "defaultSchool": s.DefaultSchool, "accounts": []core.Account{}, "login": nil, "admin": false, "impersonating": false, "releasesSeen": nil}
	if user == "" {
		return out, nil
	}
	if by != "" {
		out["impersonating"] = true
	} else if ok, err := s.isAdmin(r.Context(), user); err != nil {
		return nil, err
	} else {
		out["admin"] = ok
	}
	accounts, err := s.Core.Accounts(r.Context(), core.Principal{UserID: user, All: true})
	if err != nil {
		return nil, err
	}
	out["accounts"] = accounts
	seen, err := s.Store.ReleasesSeen(r.Context(), user)
	if err != nil {
		return nil, err
	}
	out["releasesSeen"] = seen
	if l, err := s.Store.Login(r.Context(), user); err == nil {
		out["login"] = loginView{UserID: l.Name, School: l.School}
	} else if !errors.Is(err, store.ErrNotFound) {
		return nil, err
	}
	return out, nil
}

// handleForget deletes the profile and everything it owns: linked accounts,
// stored tokens, API keys, app authorizations and calendar feeds.
func (s *Server) handleForget(w http.ResponseWriter, r *http.Request, p core.Principal) {
	accounts, _ := s.Core.Accounts(r.Context(), p)
	if err := s.Store.DeleteUser(r.Context(), p.UserID); err != nil {
		writeErr(w, err)
		return
	}
	for _, a := range accounts {
		s.Core.Cache.Drop("acct:" + a.ID + ":")
	}
	s.clearSessionCookie(w, r)
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) handleLink(w http.ResponseWriter, r *http.Request) {
	if !sameOrigin(r) {
		writeErr(w, core.Errf(http.StatusForbidden, "forbidden", "Cross-site requests are not allowed."))
		return
	}
	var body struct {
		School string `json:"school"`
		Code   string `json:"code"`
		Token  string `json:"token"`
		// PIN is required when the profile has no sign-in yet (always the
		// case for a new one), so nobody ends up with a profile that only
		// opens on this one device.
		PIN string `json:"pin"`
	}
	if err := readJSON(r, &body); err != nil {
		writeErr(w, err)
		return
	}
	if strings.TrimSpace(body.School) == "" {
		writeErr(w, core.Errf(http.StatusBadRequest, "invalid_request", "A school is required."))
		return
	}
	needPIN, err := s.needsPIN(r)
	if err != nil {
		writeErr(w, err)
		return
	}
	if needPIN && !pinShape.MatchString(body.PIN) {
		writeErr(w, core.Errf(http.StatusBadRequest, "invalid_pin", "Choose a PIN of exactly four digits."))
		return
	}
	// Verify with Zermelo before creating a profile, so failed attempts
	// leave nothing behind.
	pending, err := s.Core.Verify(r.Context(), body.School, body.Code, body.Token)
	if err != nil {
		writeErr(w, err)
		return
	}
	user, err := s.ensureSession(w, r)
	if err != nil {
		writeErr(w, err)
		return
	}
	account, err := s.Core.Save(r.Context(), user, pending)
	if err != nil {
		writeErr(w, err)
		return
	}
	if needPIN {
		acc, err := s.Store.Account(r.Context(), user, account.ID)
		if err != nil {
			writeErr(w, err)
			return
		}
		l := store.Login{UserID: user, Name: acc.TokenUser(), School: acc.School, PINHash: s.Core.Box.PINHash(user, body.PIN)}
		if err := s.Store.SetLogin(r.Context(), l); err != nil {
			writeErr(w, err)
			return
		}
	}
	writeJSON(w, http.StatusCreated, map[string]any{"data": account})
}

// needsPIN reports whether linking from this request has to set a PIN: there
// is no profile yet, or the one there is has no sign-in.
func (s *Server) needsPIN(r *http.Request) (bool, error) {
	user, ok := s.sessionUser(r)
	if !ok {
		return true, nil
	}
	_, err := s.Store.Login(r.Context(), user)
	if errors.Is(err, store.ErrNotFound) {
		return true, nil
	}
	return false, err
}

// handleDemo links the generated demo accounts (a student and a teacher).
func (s *Server) handleDemo(w http.ResponseWriter, r *http.Request) {
	if !s.Demo {
		writeErr(w, core.ErrNotFound)
		return
	}
	if !sameOrigin(r) {
		writeErr(w, core.Errf(http.StatusForbidden, "forbidden", "Cross-site requests are not allowed."))
		return
	}
	user, err := s.ensureSession(w, r)
	if err != nil {
		writeErr(w, err)
		return
	}
	out := []core.Account{}
	for _, token := range []string{zermelo.DemoStudentToken, zermelo.DemoTeacherToken} {
		a, err := s.Core.Link(r.Context(), user, zermelo.DemoSchool, "", token)
		if err != nil {
			writeErr(w, err)
			return
		}
		out = append(out, a)
	}
	writeJSON(w, http.StatusCreated, map[string]any{"data": out})
}

func (s *Server) handleUnlink(w http.ResponseWriter, r *http.Request, p core.Principal) {
	if err := s.Core.Unlink(r.Context(), p, r.PathValue("account")); err != nil {
		writeErr(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// ---- calendar feed management ----

func (s *Server) calendarBody(r *http.Request, token string) map[string]any {
	if token == "" {
		return map[string]any{"data": map[string]any{"enabled": false}}
	}
	return map[string]any{"data": map[string]any{"enabled": true, "url": s.baseURL(r) + "/calendar/" + token + ".ics"}}
}

func (s *Server) handleCalendarGet(w http.ResponseWriter, r *http.Request, p core.Principal) {
	token, err := s.Core.CalendarToken(r.Context(), p, r.PathValue("account"))
	if err != nil {
		writeErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, s.calendarBody(r, token))
}

func (s *Server) handleCalendarEnable(w http.ResponseWriter, r *http.Request, p core.Principal) {
	token, err := s.Core.EnableCalendar(r.Context(), p, r.PathValue("account"))
	if err != nil {
		writeErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, s.calendarBody(r, token))
}

func (s *Server) handleCalendarDisable(w http.ResponseWriter, r *http.Request, p core.Principal) {
	if err := s.Core.DisableCalendar(r.Context(), p, r.PathValue("account")); err != nil {
		writeErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, s.calendarBody(r, ""))
}

// ---- API keys ----

type apiKeyView struct {
	ID         string     `json:"id"`
	Name       string     `json:"name"`
	Hint       string     `json:"hint"`
	AccountIDs []string   `json:"accountIds"`
	CreatedAt  time.Time  `json:"createdAt"`
	LastUsedAt *time.Time `json:"lastUsedAt"`
	// Admin is true when the key may use the admin MCP tools.
	Admin bool `json:"admin"`
	// Key is only present in the response that creates it.
	Key string `json:"key,omitempty"`
}

func unixPtr(v int64) *time.Time {
	if v == 0 {
		return nil
	}
	t := time.Unix(v, 0).UTC()
	return &t
}

func (s *Server) handleKeys(w http.ResponseWriter, r *http.Request, p core.Principal) {
	keys, err := s.Store.APIKeys(r.Context(), p.UserID)
	if err != nil {
		writeErr(w, err)
		return
	}
	out := []apiKeyView{}
	for _, k := range keys {
		out = append(out, apiKeyView{ID: k.ID, Name: k.Name, Hint: k.Hint, AccountIDs: k.AccountIDs, CreatedAt: time.Unix(k.CreatedAt, 0).UTC(), LastUsedAt: unixPtr(k.LastUsed), Admin: k.Admin})
	}
	writeData(w, out)
}

func (s *Server) handleCreateKey(w http.ResponseWriter, r *http.Request, p core.Principal) {
	var body struct {
		Name       string   `json:"name"`
		AccountIDs []string `json:"accountIds"`
		Admin      bool     `json:"admin"`
	}
	if err := readJSON(r, &body); err != nil {
		writeErr(w, err)
		return
	}
	body.Name = strings.TrimSpace(body.Name)
	if body.Name == "" || len(body.Name) > 60 || len(body.AccountIDs) == 0 {
		writeErr(w, core.Errf(http.StatusBadRequest, "invalid_request", "A key needs a name (max 60 characters) and at least one account."))
		return
	}
	if body.Admin {
		if ok, err := s.isAdmin(r.Context(), p.UserID); err != nil || !ok {
			writeErr(w, core.Errf(http.StatusForbidden, "forbidden", "Only admins can make a key for the admin tools."))
			return
		}
	}
	key := secret.Token(apiKeyPrefix)
	k := store.APIKey{ID: secret.ID("key"), UserID: p.UserID, Name: body.Name, Hint: key[:len(apiKeyPrefix)+4] + "…" + key[len(key)-4:], AccountIDs: body.AccountIDs, Admin: body.Admin}
	if err := s.Store.CreateAPIKey(r.Context(), k, secret.Hash(key)); errors.Is(err, store.ErrNotFound) {
		writeErr(w, core.Errf(http.StatusBadRequest, "invalid_request", "One of those accounts is not linked to this profile."))
		return
	} else if err != nil {
		writeErr(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, map[string]any{"data": apiKeyView{ID: k.ID, Name: k.Name, Hint: k.Hint, AccountIDs: k.AccountIDs, CreatedAt: time.Now().UTC(), Admin: k.Admin, Key: key}})
}

func (s *Server) handleDeleteKey(w http.ResponseWriter, r *http.Request, p core.Principal) {
	if err := s.Store.DeleteAPIKey(r.Context(), p.UserID, r.PathValue("id")); errors.Is(err, store.ErrNotFound) {
		writeErr(w, core.ErrNotFound)
		return
	} else if err != nil {
		writeErr(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// ---- connected apps (OAuth grants) ----

func (s *Server) handleGrants(w http.ResponseWriter, r *http.Request, p core.Principal) {
	grants, err := s.Store.OAuthGrants(r.Context(), p.UserID)
	if err != nil {
		writeErr(w, err)
		return
	}
	out := []map[string]any{}
	for _, g := range grants {
		out = append(out, map[string]any{"id": g.ID, "app": g.ClientName, "accountIds": g.AccountIDs, "createdAt": time.Unix(g.CreatedAt, 0).UTC(), "lastUsedAt": unixPtr(g.LastUsed), "admin": g.Admin})
	}
	writeData(w, out)
}

func (s *Server) handleDeleteGrant(w http.ResponseWriter, r *http.Request, p core.Principal) {
	if err := s.Store.DeleteOAuthGrant(r.Context(), p.UserID, r.PathValue("id")); errors.Is(err, store.ErrNotFound) {
		writeErr(w, core.ErrNotFound)
		return
	} else if err != nil {
		writeErr(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// handleNickname names one of the profile's accounts, e.g. a parent's two
// children that would otherwise both show the parent's school.
// handleReleasesSeen records the newest release note the profile was shown,
// so it is not shown again on any device. Viewing Melon as someone else
// leaves it alone: the note is still news to its owner.
func (s *Server) handleReleasesSeen(w http.ResponseWriter, r *http.Request, p core.Principal) {
	var body struct {
		ID string `json:"id"`
	}
	if err := readJSON(r, &body); err != nil {
		writeErr(w, err)
		return
	}
	if body.ID == "" || len(body.ID) > 64 {
		writeErr(w, core.Errf(http.StatusBadRequest, "invalid_request", "id must be a release note id."))
		return
	}
	if _, by := s.sessionInfo(r); by == "" {
		if err := s.Store.SetReleasesSeen(r.Context(), p.UserID, body.ID); err != nil {
			writeErr(w, err)
			return
		}
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) handleNickname(w http.ResponseWriter, r *http.Request, p core.Principal) {
	var body struct {
		Nickname string `json:"nickname"`
	}
	if err := readJSON(r, &body); err != nil {
		writeErr(w, err)
		return
	}
	a, err := s.Core.SetNickname(r.Context(), p, r.PathValue("account"), body.Nickname)
	if err != nil {
		writeErr(w, err)
		return
	}
	writeData(w, a)
}
