// Package server exposes Melon over HTTP: the REST API, the web app's
// management endpoints, OAuth, the MCP endpoint, the calendar feed and the
// embedded web app itself.
package server

import (
	"context"
	"encoding/json"
	"errors"
	"io/fs"
	"log/slog"
	"net"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"

	"golang.org/x/time/rate"

	"github.com/matthiaslubbertsen/melon/internal/core"
	"github.com/matthiaslubbertsen/melon/internal/secret"
	"github.com/matthiaslubbertsen/melon/internal/store"
)

const (
	sessionCookie = "melon_session"
	sessionTTL    = 365 * 24 * time.Hour

	apiKeyPrefix  = "melon_sk_"
	accessPrefix  = "melon_at_"
	refreshPrefix = "melon_rt_"
)

type Server struct {
	Core  *core.Service
	Store *store.Store
	// PublicURL is the externally visible origin (no trailing slash). When
	// empty it is derived per request.
	PublicURL  string
	TrustProxy bool
	Demo       bool
	// DefaultSchool prefills the school field when linking; users can change it.
	DefaultSchool string
	Version       string
	Static        fs.FS  // built web app
	OpenAPI       []byte // openapi.yaml

	limitMu  sync.Mutex
	limiters map[string]*clientLimiter
}

type clientLimiter struct {
	lim  *rate.Limiter
	seen time.Time
}

func (s *Server) Handler() http.Handler {
	s.limiters = map[string]*clientLimiter{}
	go s.janitor()

	mux := http.NewServeMux()
	mux.HandleFunc("GET /healthz", func(w http.ResponseWriter, _ *http.Request) { w.Write([]byte("ok\n")) })

	// Public REST API.
	mux.HandleFunc("GET /api/v1/subjects", s.handleDefaultSubjects)
	mux.HandleFunc("GET /api/v1/accounts", s.authed(s.handleAccounts))
	mux.HandleFunc("GET /api/v1/accounts/{account}", s.authed(s.handleAccount))
	mux.HandleFunc("GET /api/v1/accounts/{account}/schedule", s.authed(s.handleSchedule))
	mux.HandleFunc("GET /api/v1/accounts/{account}/people", s.authed(s.handlePeople))
	mux.HandleFunc("GET /api/v1/accounts/{account}/announcements", s.authed(s.handleAnnouncements))
	mux.HandleFunc("GET /api/v1/accounts/{account}/subjects", s.authed(s.handleSubjects))
	mux.HandleFunc("PUT /api/v1/accounts/{account}/subjects/{code}", s.authed(s.handleSetSubject))
	mux.HandleFunc("DELETE /api/v1/accounts/{account}/subjects/{code}", s.authed(s.handleResetSubject))
	mux.HandleFunc("GET /api/v1/accounts/{account}/teachers", s.authed(s.handleTeachers))
	mux.HandleFunc("PUT /api/v1/accounts/{account}/teachers/{code}", s.authed(s.handleSetTeacher))
	mux.HandleFunc("DELETE /api/v1/accounts/{account}/teachers/{code}", s.authed(s.handleResetTeacher))
	mux.HandleFunc("GET /api/openapi.yaml", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/yaml; charset=utf-8")
		w.Write(s.OpenAPI)
	})

	// Web app management; browser session only.
	mux.HandleFunc("GET /api/app/session", s.handleSession)
	mux.HandleFunc("DELETE /api/app/session", s.sessionOnly(s.handleForget))
	mux.HandleFunc("POST /api/app/accounts", s.handleLink)
	mux.HandleFunc("POST /api/app/demo", s.handleDemo)
	mux.HandleFunc("DELETE /api/app/accounts/{account}", s.sessionOnly(s.handleUnlink))
	mux.HandleFunc("GET /api/app/accounts/{account}/calendar", s.sessionOnly(s.handleCalendarGet))
	mux.HandleFunc("POST /api/app/accounts/{account}/calendar", s.sessionOnly(s.handleCalendarEnable))
	mux.HandleFunc("DELETE /api/app/accounts/{account}/calendar", s.sessionOnly(s.handleCalendarDisable))
	mux.HandleFunc("GET /api/app/keys", s.sessionOnly(s.handleKeys))
	mux.HandleFunc("POST /api/app/keys", s.sessionOnly(s.handleCreateKey))
	mux.HandleFunc("DELETE /api/app/keys/{id}", s.sessionOnly(s.handleDeleteKey))
	mux.HandleFunc("GET /api/app/grants", s.sessionOnly(s.handleGrants))
	mux.HandleFunc("DELETE /api/app/grants/{id}", s.sessionOnly(s.handleDeleteGrant))
	mux.HandleFunc("GET /api/app/oauth/request", s.handleOAuthRequest)
	mux.HandleFunc("POST /api/app/oauth/approve", s.sessionOnly(s.handleOAuthApprove))

	// OAuth 2.1 authorization server (for MCP clients).
	for _, suffix := range []string{"", "/mcp"} {
		mux.HandleFunc("GET /.well-known/oauth-authorization-server"+suffix, s.handleASMetadata)
		mux.HandleFunc("GET /.well-known/oauth-protected-resource"+suffix, s.handleResourceMetadata)
	}
	mux.HandleFunc("POST /oauth/register", s.handleRegister)
	mux.HandleFunc("POST /oauth/token", s.handleToken)
	mux.HandleFunc("POST /oauth/revoke", s.handleRevoke)

	mux.HandleFunc("/mcp", s.handleMCP)
	mux.HandleFunc("GET /calendar/{token}", s.handleCalendarFeed)

	mux.HandleFunc("/api/", func(w http.ResponseWriter, r *http.Request) { writeErr(w, core.ErrNotFound) })
	mux.Handle("/", s.staticHandler())

	return s.middleware(mux)
}

// ---- middleware ----

func (s *Server) middleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		h := w.Header()
		h.Set("X-Content-Type-Options", "nosniff")
		h.Set("Referrer-Policy", "no-referrer")
		h.Set("X-Frame-Options", "DENY")
		h.Set("Content-Security-Policy", "default-src 'self'; style-src 'self' 'unsafe-inline'; img-src 'self' data: blob:; font-src 'self' data:; connect-src 'self'; worker-src 'self' blob:; frame-ancestors 'none'; base-uri 'self'; form-action 'self'")

		p := r.URL.Path
		tokenAPI := strings.HasPrefix(p, "/api/v1/") || p == "/mcp" || strings.HasPrefix(p, "/oauth/") || strings.HasPrefix(p, "/.well-known/") || p == "/api/openapi.yaml"
		if tokenAPI {
			// Bearer-token endpoints are open to browser-based clients on
			// other origins. Cookies are never honoured cross-origin: there
			// is no Allow-Credentials, and see sameOrigin below.
			h.Set("Access-Control-Allow-Origin", "*")
			h.Set("Access-Control-Expose-Headers", "WWW-Authenticate, Retry-After, Mcp-Session-Id")
			if r.Method == http.MethodOptions {
				h.Set("Access-Control-Allow-Methods", "GET, POST, PUT, DELETE, OPTIONS")
				h.Set("Access-Control-Allow-Headers", "Authorization, Content-Type, Accept, Mcp-Protocol-Version, Mcp-Session-Id")
				h.Set("Access-Control-Max-Age", "600")
				w.WriteHeader(http.StatusNoContent)
				return
			}
		}
		if strings.HasPrefix(p, "/api/") || tokenAPI || strings.HasPrefix(p, "/calendar/") {
			h.Set("Cache-Control", "no-store")
			if !s.allow(r) {
				h.Set("Retry-After", "5")
				writeErr(w, core.Errf(http.StatusTooManyRequests, "rate_limited", "Too many requests. Slow down and try again shortly."))
				return
			}
		}
		r.Body = http.MaxBytesReader(w, r.Body, 1<<20)
		next.ServeHTTP(w, r)
	})
}

// allow applies a per-client budget: by credential when one is presented,
// otherwise by IP. Linking gets a much smaller budget because that is where
// guessing link codes would happen.
func (s *Server) allow(r *http.Request) bool {
	key, per, burst := "ip:"+s.clientIP(r), rate.Limit(10), 60
	if auth := r.Header.Get("Authorization"); auth != "" {
		key = "cred:" + secret.Hash(auth)
	} else if c, err := r.Cookie(sessionCookie); err == nil {
		key = "cred:" + secret.Hash(c.Value)
	}
	switch {
	case r.Method == http.MethodPost && r.URL.Path == "/api/app/accounts":
		key, per, burst = "link:"+s.clientIP(r), rate.Limit(0.2), 10
	case strings.HasPrefix(r.URL.Path, "/oauth/"):
		// Hosted MCP clients serve many users from few addresses.
		key, per, burst = "oauth:"+s.clientIP(r), rate.Limit(2), 30
	}
	s.limitMu.Lock()
	defer s.limitMu.Unlock()
	cl := s.limiters[key]
	if cl == nil {
		cl = &clientLimiter{lim: rate.NewLimiter(per, burst)}
		s.limiters[key] = cl
	}
	cl.seen = time.Now()
	return cl.lim.Allow()
}

func (s *Server) janitor() {
	for range time.Tick(10 * time.Minute) {
		s.limitMu.Lock()
		for k, cl := range s.limiters {
			if time.Since(cl.seen) > 10*time.Minute {
				delete(s.limiters, k)
			}
		}
		s.limitMu.Unlock()
		s.Store.Sweep(context.Background())
	}
}

func (s *Server) clientIP(r *http.Request) string {
	if s.TrustProxy {
		if xff := r.Header.Get("X-Forwarded-For"); xff != "" {
			parts := strings.Split(xff, ",")
			return strings.TrimSpace(parts[len(parts)-1])
		}
	}
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		return r.RemoteAddr
	}
	return host
}

func (s *Server) isHTTPS(r *http.Request) bool {
	if strings.HasPrefix(s.PublicURL, "https://") || r.TLS != nil {
		return true
	}
	return s.TrustProxy && r.Header.Get("X-Forwarded-Proto") == "https"
}

// baseURL is the origin clients should use to reach this server.
func (s *Server) baseURL(r *http.Request) string {
	if s.PublicURL != "" {
		return s.PublicURL
	}
	scheme := "http"
	if s.isHTTPS(r) {
		scheme = "https"
	}
	return scheme + "://" + r.Host
}

// ---- authentication ----

// principal identifies the caller from a bearer credential or the session cookie.
func (s *Server) principal(r *http.Request) (core.Principal, error) {
	unauthorized := core.Errf(http.StatusUnauthorized, "unauthorized", "Missing or invalid credentials.")
	if auth := r.Header.Get("Authorization"); auth != "" {
		token, ok := strings.CutPrefix(auth, "Bearer ")
		if !ok {
			return core.Principal{}, unauthorized
		}
		token = strings.TrimSpace(token)
		switch {
		case strings.HasPrefix(token, apiKeyPrefix):
			k, err := s.Store.APIKeyByHash(r.Context(), secret.Hash(token))
			if err != nil {
				return core.Principal{}, unauthorized
			}
			return core.Principal{UserID: k.UserID, Kind: "api_key", AccountIDs: k.AccountIDs}, nil
		case strings.HasPrefix(token, accessPrefix):
			g, err := s.Store.OAuthGrantByToken(r.Context(), secret.Hash(token), "access")
			if err != nil {
				return core.Principal{}, unauthorized
			}
			return core.Principal{UserID: g.UserID, Kind: "oauth", AccountIDs: g.AccountIDs}, nil
		}
		return core.Principal{}, unauthorized
	}
	if user, ok := s.sessionUser(r); ok {
		if !sameOrigin(r) {
			return core.Principal{}, core.Errf(http.StatusForbidden, "forbidden", "Cross-site requests may not use the browser session.")
		}
		return core.Principal{UserID: user, Kind: "session", All: true}, nil
	}
	return core.Principal{}, unauthorized
}

func (s *Server) sessionUser(r *http.Request) (string, bool) {
	c, err := r.Cookie(sessionCookie)
	if err != nil || c.Value == "" {
		return "", false
	}
	user, err := s.Store.SessionUser(r.Context(), secret.Hash(c.Value), sessionTTL)
	return user, err == nil
}

// sameOrigin rejects cookie-authenticated requests that another site
// triggered. Combined with SameSite=Lax cookies this is the CSRF defence.
func sameOrigin(r *http.Request) bool {
	if site := r.Header.Get("Sec-Fetch-Site"); site != "" {
		return site == "same-origin" || site == "none"
	}
	if origin := r.Header.Get("Origin"); origin != "" {
		u, err := url.Parse(origin)
		return err == nil && u.Host == r.Host
	}
	return r.Method == http.MethodGet || r.Method == http.MethodHead
}

type handler func(w http.ResponseWriter, r *http.Request, p core.Principal)

func (s *Server) authed(next handler) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		p, err := s.principal(r)
		if err != nil {
			writeErr(w, err)
			return
		}
		next(w, r, p)
	}
}

// sessionOnly admits the browser session and nothing else, so API keys and
// OAuth tokens can never mint keys, read feed URLs or unlink accounts.
func (s *Server) sessionOnly(next handler) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		user, ok := s.sessionUser(r)
		if !ok || r.Header.Get("Authorization") != "" {
			writeErr(w, core.Errf(http.StatusUnauthorized, "unauthorized", "This needs a browser session."))
			return
		}
		if !sameOrigin(r) {
			writeErr(w, core.Errf(http.StatusForbidden, "forbidden", "Cross-site requests may not use the browser session."))
			return
		}
		next(w, r, core.Principal{UserID: user, Kind: "session", All: true})
	}
}

// ensureSession returns the caller's profile, creating one (and its cookie)
// on first use. Melon has no passwords: a profile is the browser it lives in.
func (s *Server) ensureSession(w http.ResponseWriter, r *http.Request) (string, error) {
	if user, ok := s.sessionUser(r); ok {
		return user, nil
	}
	user := secret.ID("usr")
	if err := s.Store.CreateUser(r.Context(), user); err != nil {
		return "", err
	}
	token := secret.Token("")
	if err := s.Store.CreateSession(r.Context(), secret.Hash(token), user, sessionTTL); err != nil {
		return "", err
	}
	http.SetCookie(w, &http.Cookie{
		Name: sessionCookie, Value: token, Path: "/", MaxAge: int(sessionTTL.Seconds()),
		HttpOnly: true, Secure: s.isHTTPS(r), SameSite: http.SameSiteLaxMode,
	})
	return user, nil
}

// ---- JSON helpers ----

type meta struct {
	FetchedAt *time.Time `json:"fetchedAt,omitempty"`
	Stale     bool       `json:"stale,omitempty"`
}

func writeJSON(w http.ResponseWriter, status int, body any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(body)
}

func writeData(w http.ResponseWriter, data any) {
	writeJSON(w, http.StatusOK, map[string]any{"data": data})
}

func writeFetched(w http.ResponseWriter, data any, fetchedAt time.Time, stale bool) {
	writeJSON(w, http.StatusOK, map[string]any{"data": data, "meta": meta{FetchedAt: &fetchedAt, Stale: stale}})
}

func writeErr(w http.ResponseWriter, err error) {
	var ce *core.Error
	if !errors.As(err, &ce) {
		var tooBig *http.MaxBytesError
		if errors.As(err, &tooBig) {
			ce = core.Errf(http.StatusRequestEntityTooLarge, "invalid_request", "Request body too large.")
		} else {
			slog.Error("internal error", "err", err)
			ce = core.Errf(http.StatusInternalServerError, "internal_error", "Something went wrong on our side.")
		}
	}
	if ce.Status == http.StatusServiceUnavailable {
		w.Header().Set("Retry-After", "30")
	}
	writeJSON(w, ce.Status, map[string]any{"error": ce})
}

func readJSON(r *http.Request, into any) error {
	if err := json.NewDecoder(r.Body).Decode(into); err != nil {
		var tooBig *http.MaxBytesError
		if errors.As(err, &tooBig) {
			return err
		}
		return core.Errf(http.StatusBadRequest, "invalid_request", "The request body is not valid JSON.")
	}
	return nil
}

// ---- static web app ----

func (s *Server) staticHandler() http.Handler {
	files := http.FileServerFS(s.Static)
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet && r.Method != http.MethodHead {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		path := strings.TrimPrefix(r.URL.Path, "/")
		if path != "" {
			if f, err := s.Static.Open(path); err == nil {
				st, _ := f.Stat()
				f.Close()
				if st != nil && !st.IsDir() {
					if strings.HasPrefix(path, "assets/") {
						w.Header().Set("Cache-Control", "public, max-age=31536000, immutable")
					}
					files.ServeHTTP(w, r)
					return
				}
			}
			if strings.HasPrefix(path, "assets/") {
				http.NotFound(w, r)
				return
			}
		}
		// Client-side routes all get the app shell.
		index, err := fs.ReadFile(s.Static, "index.html")
		if err != nil {
			http.Error(w, "web app not built", http.StatusServiceUnavailable)
			return
		}
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.Header().Set("Cache-Control", "no-cache")
		w.Write(index)
	})
}
