package server

import (
	"errors"
	"fmt"
	"net/http"
	"regexp"
	"strings"

	"github.com/matthiaslubbertsen/melon/internal/core"
	"github.com/matthiaslubbertsen/melon/internal/secret"
	"github.com/matthiaslubbertsen/melon/internal/store"
)

// Signing in. A profile can be given a sign-in: the user number of one of its
// own Zermelo accounts (12345, say) plus a four digit PIN. That opens the
// whole profile, with every Zermelo account linked to it, on another device.
//
// The number is not a secret and the PIN is short, so the defence is the
// attempt counter in the store, not the hash. Proving a Zermelo account (by
// linking it) never opens an existing profile; it only lets you take the
// sign-in name for the profile you are in.

type loginView struct {
	UserID string `json:"userId"`
	School string `json:"school"`
}

var pinShape = regexp.MustCompile(`^[0-9]{4}$`)

func (s *Server) handleSignIn(w http.ResponseWriter, r *http.Request) {
	if !sameOrigin(r) {
		writeErr(w, core.Errf(http.StatusForbidden, "forbidden", "Cross-site requests are not allowed."))
		return
	}
	var body struct {
		UserID string `json:"userId"`
		PIN    string `json:"pin"`
		School string `json:"school"`
	}
	if err := readJSON(r, &body); err != nil {
		writeErr(w, err)
		return
	}
	wrong := core.Errf(http.StatusUnauthorized, "invalid_login", "That user number and PIN do not match.")
	name := strings.TrimSpace(body.UserID)
	if name == "" || len(name) > 64 || !pinShape.MatchString(body.PIN) {
		writeErr(w, wrong)
		return
	}
	ctx := r.Context()
	candidates, err := s.Store.LoginsByName(ctx, name, strings.ToLower(strings.TrimSpace(body.School)))
	if err != nil {
		writeErr(w, err)
		return
	}
	var wait int64
	tried := false
	for _, l := range candidates {
		ok, left, err := s.Store.BeginPINAttempt(ctx, l.UserID)
		if errors.Is(err, store.ErrNotFound) {
			continue
		} else if err != nil {
			writeErr(w, err)
			return
		}
		if !ok {
			wait = max(wait, left)
			continue
		}
		tried = true
		if !s.Core.Box.PINMatches(l.PINHash, l.UserID, body.PIN) {
			continue
		}
		if err := s.Store.EndPINAttempt(ctx, l.UserID); err != nil {
			writeErr(w, err)
			return
		}
		s.leaveSession(r, l.UserID)
		if err := s.startSession(w, r, l.UserID); err != nil {
			writeErr(w, err)
			return
		}
		out, err := s.sessionBody(r, l.UserID)
		if err != nil {
			writeErr(w, err)
			return
		}
		writeData(w, out)
		return
	}
	if !tried && wait > 0 {
		w.Header().Set("Retry-After", fmt.Sprint(wait))
		writeErr(w, core.Errf(http.StatusTooManyRequests, "login_locked", "Too many wrong PINs. Sign-in is locked for another %d minutes.", (wait+59)/60))
		return
	}
	writeErr(w, wrong)
}

// leaveSession ends the browser's current session before it gets another. A
// profile left with nothing in it and no way back is removed.
func (s *Server) leaveSession(r *http.Request, next string) {
	c, err := r.Cookie(sessionCookie)
	if err != nil || c.Value == "" {
		return
	}
	ctx := r.Context()
	old, ok := s.sessionUser(r)
	_ = s.Store.DeleteSession(ctx, secret.Hash(c.Value))
	if !ok || old == next {
		return
	}
	if _, err := s.Store.Login(ctx, old); !errors.Is(err, store.ErrNotFound) {
		return
	}
	if accounts, err := s.Store.Accounts(ctx, old); err == nil && len(accounts) == 0 {
		_ = s.Store.DeleteUser(ctx, old)
	}
}

// handleSignOut ends this browser's session and keeps the profile. Without a
// PIN there would be no way back in, so that case is refused.
func (s *Server) handleSignOut(w http.ResponseWriter, r *http.Request, p core.Principal) {
	if _, err := s.Store.Login(r.Context(), p.UserID); errors.Is(err, store.ErrNotFound) {
		writeErr(w, core.Errf(http.StatusConflict, "no_login", "Set a PIN first; without one you could not sign back in."))
		return
	} else if err != nil {
		writeErr(w, err)
		return
	}
	if c, err := r.Cookie(sessionCookie); err == nil {
		if err := s.Store.DeleteSession(r.Context(), secret.Hash(c.Value)); err != nil {
			writeErr(w, err)
			return
		}
	}
	s.clearSessionCookie(w, r)
	w.WriteHeader(http.StatusNoContent)
}

// handleSetPIN sets or changes the sign-in. The session is the authority
// here (it can already erase the profile), so the old PIN is not asked for;
// that is also how a forgotten PIN is replaced. Other devices are signed out.
func (s *Server) handleSetPIN(w http.ResponseWriter, r *http.Request, p core.Principal) {
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
	ctx := r.Context()
	acc, err := s.Store.Account(ctx, p.UserID, body.AccountID)
	if errors.Is(err, store.ErrNotFound) {
		writeErr(w, core.Errf(http.StatusBadRequest, "invalid_request", "Pick one of the accounts linked to this profile to sign in with."))
		return
	} else if err != nil {
		writeErr(w, err)
		return
	}
	l := store.Login{UserID: p.UserID, Name: acc.ZermeloUser, School: acc.School, PINHash: s.Core.Box.PINHash(p.UserID, body.PIN)}
	if err := s.Store.SetLogin(ctx, l); err != nil {
		writeErr(w, err)
		return
	}
	if c, err := r.Cookie(sessionCookie); err == nil {
		if err := s.Store.DeleteOtherSessions(ctx, p.UserID, secret.Hash(c.Value)); err != nil {
			writeErr(w, err)
			return
		}
	}
	writeData(w, loginView{UserID: l.Name, School: l.School})
}

func (s *Server) handleClearPIN(w http.ResponseWriter, r *http.Request, p core.Principal) {
	if err := s.Store.DeleteLogin(r.Context(), p.UserID); err != nil {
		writeErr(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
