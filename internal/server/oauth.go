package server

import (
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"errors"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/matthiaslubbertsen/melon/internal/core"
	"github.com/matthiaslubbertsen/melon/internal/secret"
	"github.com/matthiaslubbertsen/melon/internal/store"
)

// Melon is its own OAuth 2.1 authorization server for MCP clients: public
// clients only, dynamic registration, authorization code with mandatory PKCE
// (S256), short-lived access tokens and rotating refresh tokens.

const (
	codeTTL    = 2 * time.Minute
	accessTTL  = time.Hour
	refreshTTL = 90 * 24 * time.Hour
)

func (s *Server) handleASMetadata(w http.ResponseWriter, r *http.Request) {
	base := s.baseURL(r)
	writeJSON(w, http.StatusOK, map[string]any{
		"issuer":                                base,
		"authorization_endpoint":                base + "/oauth/authorize",
		"token_endpoint":                        base + "/oauth/token",
		"registration_endpoint":                 base + "/oauth/register",
		"revocation_endpoint":                   base + "/oauth/revoke",
		"response_types_supported":              []string{"code"},
		"grant_types_supported":                 []string{"authorization_code", "refresh_token"},
		"code_challenge_methods_supported":      []string{"S256"},
		"token_endpoint_auth_methods_supported": []string{"none"},
		"scopes_supported":                      []string{"schedule"},
	})
}

func (s *Server) handleResourceMetadata(w http.ResponseWriter, r *http.Request) {
	base := s.baseURL(r)
	writeJSON(w, http.StatusOK, map[string]any{
		"resource":                 base + "/mcp",
		"authorization_servers":    []string{base},
		"bearer_methods_supported": []string{"header"},
		"scopes_supported":         []string{"schedule"},
		"resource_name":            "Melon",
	})
}

func oauthErr(w http.ResponseWriter, status int, code, desc string) {
	writeJSON(w, status, map[string]string{"error": code, "error_description": desc})
}

// validRedirect allows https, loopback http, and private-use app schemes.
func validRedirect(raw string) bool {
	u, err := url.Parse(raw)
	if err != nil || u.Scheme == "" || u.Fragment != "" {
		return false
	}
	switch strings.ToLower(u.Scheme) {
	case "https":
		return u.Host != ""
	case "http":
		h := u.Hostname()
		return h == "localhost" || h == "127.0.0.1" || h == "::1"
	case "javascript", "data", "vbscript", "file", "blob", "about":
		return false
	}
	return true
}

func (s *Server) handleRegister(w http.ResponseWriter, r *http.Request) {
	var body struct {
		ClientName   string   `json:"client_name"`
		RedirectURIs []string `json:"redirect_uris"`
	}
	if err := readJSON(r, &body); err != nil {
		oauthErr(w, http.StatusBadRequest, "invalid_client_metadata", "Body must be JSON.")
		return
	}
	if len(body.RedirectURIs) == 0 || len(body.RedirectURIs) > 10 {
		oauthErr(w, http.StatusBadRequest, "invalid_redirect_uri", "Between 1 and 10 redirect_uris are required.")
		return
	}
	for _, u := range body.RedirectURIs {
		if len(u) > 500 || !validRedirect(u) {
			oauthErr(w, http.StatusBadRequest, "invalid_redirect_uri", "Redirect URIs must be https, loopback http, or an app scheme.")
			return
		}
	}
	name := strings.TrimSpace(body.ClientName)
	if name == "" {
		name = "Unnamed app"
	}
	if len(name) > 80 {
		name = name[:80]
	}
	c := store.OAuthClient{ID: secret.ID("app"), Name: name, RedirectURIs: body.RedirectURIs}
	if err := s.Store.CreateOAuthClient(r.Context(), c); err != nil {
		oauthErr(w, http.StatusInternalServerError, "server_error", "Could not register the client.")
		return
	}
	writeJSON(w, http.StatusCreated, map[string]any{
		"client_id": c.ID, "client_name": c.Name, "redirect_uris": c.RedirectURIs,
		"token_endpoint_auth_method": "none",
		"grant_types":                []string{"authorization_code", "refresh_token"},
		"response_types":             []string{"code"},
		"client_id_issued_at":        time.Now().Unix(),
	})
}

// checkAuthRequest validates the parts of an authorization request that must
// hold before the user is shown anything or redirected anywhere.
func (s *Server) checkAuthRequest(r *http.Request, clientID, redirectURI string) (store.OAuthClient, error) {
	c, err := s.Store.OAuthClient(r.Context(), clientID)
	if errors.Is(err, store.ErrNotFound) {
		return c, core.Errf(http.StatusBadRequest, "invalid_client", "This app is not registered with Melon.")
	} else if err != nil {
		return c, err
	}
	for _, u := range c.RedirectURIs {
		if u == redirectURI {
			return c, nil
		}
	}
	return c, core.Errf(http.StatusBadRequest, "invalid_redirect_uri", "The app asked to return to an address it did not register.")
}

// handleOAuthRequest lets the consent screen check a request and learn the
// app's name. The app name is self-declared, which the screen makes clear by
// also showing where the user will be sent.
func (s *Server) handleOAuthRequest(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	c, err := s.checkAuthRequest(r, q.Get("client_id"), q.Get("redirect_uri"))
	if err != nil {
		writeErr(w, err)
		return
	}
	u, _ := url.Parse(q.Get("redirect_uri"))
	host := u.Host
	if host == "" {
		host = u.Scheme + "://"
	}
	writeData(w, map[string]any{"app": c.Name, "returnsTo": host})
}

func (s *Server) handleOAuthApprove(w http.ResponseWriter, r *http.Request, p core.Principal) {
	var body struct {
		ClientID            string   `json:"clientId"`
		RedirectURI         string   `json:"redirectUri"`
		State               string   `json:"state"`
		CodeChallenge       string   `json:"codeChallenge"`
		CodeChallengeMethod string   `json:"codeChallengeMethod"`
		ResponseType        string   `json:"responseType"`
		AccountIDs          []string `json:"accountIds"`
	}
	if err := readJSON(r, &body); err != nil {
		writeErr(w, err)
		return
	}
	c, err := s.checkAuthRequest(r, body.ClientID, body.RedirectURI)
	if err != nil {
		writeErr(w, err)
		return
	}
	redirect, _ := url.Parse(body.RedirectURI)
	finish := func(params url.Values) {
		if body.State != "" {
			params.Set("state", body.State)
		}
		q := redirect.Query()
		for k, v := range params {
			q[k] = v
		}
		redirect.RawQuery = q.Encode()
		writeData(w, map[string]string{"redirect": redirect.String()})
	}
	switch {
	case body.ResponseType != "code":
		finish(url.Values{"error": {"unsupported_response_type"}})
		return
	case body.CodeChallenge == "" || body.CodeChallengeMethod != "S256":
		finish(url.Values{"error": {"invalid_request"}, "error_description": {"PKCE with S256 is required."}})
		return
	case len(body.AccountIDs) == 0:
		writeErr(w, core.Errf(http.StatusBadRequest, "invalid_request", "Choose at least one account to share."))
		return
	}
	code := secret.Token("")
	g := store.OAuthGrant{ID: secret.ID("grant"), UserID: p.UserID, ClientID: c.ID, AccountIDs: body.AccountIDs}
	if err := s.Store.CreateOAuthGrant(r.Context(), g, secret.Hash(code), body.RedirectURI, body.CodeChallenge, codeTTL); errors.Is(err, store.ErrNotFound) {
		writeErr(w, core.Errf(http.StatusBadRequest, "invalid_request", "One of those accounts is not linked to this profile."))
		return
	} else if err != nil {
		writeErr(w, err)
		return
	}
	finish(url.Values{"code": {code}})
}

func (s *Server) issueTokens(w http.ResponseWriter, r *http.Request, grantID string) {
	access, refresh := secret.Token(accessPrefix), secret.Token(refreshPrefix)
	if err := s.Store.CreateOAuthToken(r.Context(), secret.Hash(access), grantID, "access", accessTTL); err != nil {
		oauthErr(w, http.StatusInternalServerError, "server_error", "Could not issue tokens.")
		return
	}
	if err := s.Store.CreateOAuthToken(r.Context(), secret.Hash(refresh), grantID, "refresh", refreshTTL); err != nil {
		oauthErr(w, http.StatusInternalServerError, "server_error", "Could not issue tokens.")
		return
	}
	w.Header().Set("Pragma", "no-cache")
	writeJSON(w, http.StatusOK, map[string]any{
		"access_token": access, "token_type": "Bearer", "expires_in": int(accessTTL.Seconds()),
		"refresh_token": refresh, "scope": "schedule",
	})
}

func (s *Server) handleToken(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		oauthErr(w, http.StatusBadRequest, "invalid_request", "Body must be form-encoded.")
		return
	}
	switch r.PostForm.Get("grant_type") {
	case "authorization_code":
		c, err := s.Store.ConsumeOAuthCode(r.Context(), secret.Hash(r.PostForm.Get("code")))
		if err != nil {
			oauthErr(w, http.StatusBadRequest, "invalid_grant", "The authorization code is invalid, expired or already used.")
			return
		}
		sum := sha256.Sum256([]byte(r.PostForm.Get("code_verifier")))
		challenge := base64.RawURLEncoding.EncodeToString(sum[:])
		if c.ClientID != r.PostForm.Get("client_id") || c.RedirectURI != r.PostForm.Get("redirect_uri") ||
			subtle.ConstantTimeCompare([]byte(challenge), []byte(c.Challenge)) != 1 {
			// The code is already consumed; drop the consent it belonged to.
			_ = s.Store.DeleteOAuthGrant(r.Context(), s.grantOwner(r, c.GrantID), c.GrantID)
			oauthErr(w, http.StatusBadRequest, "invalid_grant", "Client, redirect URI or PKCE verifier do not match the authorization request.")
			return
		}
		s.issueTokens(w, r, c.GrantID)
	case "refresh_token":
		hash := secret.Hash(r.PostForm.Get("refresh_token"))
		g, err := s.Store.OAuthGrantByToken(r.Context(), hash, "refresh")
		if err != nil || g.ClientID != r.PostForm.Get("client_id") {
			oauthErr(w, http.StatusBadRequest, "invalid_grant", "The refresh token is invalid, expired or revoked.")
			return
		}
		// Rotation: each refresh token works once.
		_ = s.Store.DeleteOAuthToken(r.Context(), hash)
		s.issueTokens(w, r, g.ID)
	default:
		oauthErr(w, http.StatusBadRequest, "unsupported_grant_type", "Use authorization_code or refresh_token.")
	}
}

func (s *Server) grantOwner(r *http.Request, grantID string) string {
	owner, _ := s.Store.OAuthGrantOwner(r.Context(), grantID)
	return owner
}

// handleRevoke implements RFC 7009; it always answers 200.
func (s *Server) handleRevoke(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err == nil {
		_ = s.Store.DeleteOAuthToken(r.Context(), secret.Hash(r.PostForm.Get("token")))
	}
	w.WriteHeader(http.StatusOK)
}
