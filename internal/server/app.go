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
	out := map[string]any{"demo": s.Demo, "version": s.Version, "accounts": []core.Account{}}
	if user, ok := s.sessionUser(r); ok {
		accounts, err := s.Core.Accounts(r.Context(), core.Principal{UserID: user, All: true})
		if err != nil {
			writeErr(w, err)
			return
		}
		out["accounts"] = accounts
	}
	writeData(w, out)
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
	http.SetCookie(w, &http.Cookie{Name: sessionCookie, Path: "/", MaxAge: -1, HttpOnly: true, Secure: s.isHTTPS(r), SameSite: http.SameSiteLaxMode})
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
	}
	if err := readJSON(r, &body); err != nil {
		writeErr(w, err)
		return
	}
	if strings.TrimSpace(body.School) == "" {
		writeErr(w, core.Errf(http.StatusBadRequest, "invalid_request", "A school is required."))
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
	writeJSON(w, http.StatusCreated, map[string]any{"data": account})
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
		out = append(out, apiKeyView{ID: k.ID, Name: k.Name, Hint: k.Hint, AccountIDs: k.AccountIDs, CreatedAt: time.Unix(k.CreatedAt, 0).UTC(), LastUsedAt: unixPtr(k.LastUsed)})
	}
	writeData(w, out)
}

func (s *Server) handleCreateKey(w http.ResponseWriter, r *http.Request, p core.Principal) {
	var body struct {
		Name       string   `json:"name"`
		AccountIDs []string `json:"accountIds"`
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
	key := secret.Token(apiKeyPrefix)
	k := store.APIKey{ID: secret.ID("key"), UserID: p.UserID, Name: body.Name, Hint: key[:len(apiKeyPrefix)+4] + "…" + key[len(key)-4:], AccountIDs: body.AccountIDs}
	if err := s.Store.CreateAPIKey(r.Context(), k, secret.Hash(key)); errors.Is(err, store.ErrNotFound) {
		writeErr(w, core.Errf(http.StatusBadRequest, "invalid_request", "One of those accounts is not linked to this profile."))
		return
	} else if err != nil {
		writeErr(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, map[string]any{"data": apiKeyView{ID: k.ID, Name: k.Name, Hint: k.Hint, AccountIDs: k.AccountIDs, CreatedAt: time.Now().UTC(), Key: key}})
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
		out = append(out, map[string]any{"id": g.ID, "app": g.ClientName, "accountIds": g.AccountIDs, "createdAt": time.Unix(g.CreatedAt, 0).UTC(), "lastUsedAt": unixPtr(g.LastUsed)})
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
