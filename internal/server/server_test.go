package server

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"net/http/cookiejar"
	"net/http/httptest"
	"strings"
	"testing"
	"testing/fstest"
	"time"

	"github.com/matthiaslubbertsen/melon/internal/core"
	"github.com/matthiaslubbertsen/melon/internal/secret"
	"github.com/matthiaslubbertsen/melon/internal/store"
	"github.com/matthiaslubbertsen/melon/internal/zermelo"
)

type client struct {
	t    *testing.T
	base string
	http *http.Client
}

// do sends a request; a non-empty bearer replaces the cookie session.
func (c client) do(method, path, bearer string, body any) (int, map[string]any) {
	c.t.Helper()
	var rd io.Reader
	if body != nil {
		b, _ := json.Marshal(body)
		rd = bytes.NewReader(b)
	}
	req, _ := http.NewRequest(method, c.base+path, rd)
	req.Header.Set("Content-Type", "application/json")
	hc := c.http
	if bearer != "" {
		req.Header.Set("Authorization", "Bearer "+bearer)
		hc = http.DefaultClient
	} else {
		req.Header.Set("Origin", c.base)
	}
	res, err := hc.Do(req)
	if err != nil {
		c.t.Fatal(err)
	}
	defer res.Body.Close()
	var out map[string]any
	_ = json.NewDecoder(res.Body).Decode(&out)
	return res.StatusCode, out
}

func setup(t *testing.T) client {
	t.Helper()
	dir := t.TempDir()
	db, err := store.Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	box, _ := secret.NewBox([]byte("test-secret-test-secret"))
	loc, _ := time.LoadLocation("Europe/Amsterdam")
	svc := &core.Service{Store: db, Up: zermelo.Router{Real: zermelo.Demo{Loc: loc}, Demo: zermelo.Demo{Loc: loc}}, Cache: zermelo.NewCache(), Box: box, Loc: loc, Demo: true}
	srv := &Server{Core: svc, Store: db, Demo: true, Version: "test", Static: fstest.MapFS{"index.html": {Data: []byte("app")}}}
	ts := httptest.NewServer(srv.Handler())
	t.Cleanup(ts.Close)
	jar, _ := cookiejar.New(nil)
	return client{t: t, base: ts.URL, http: &http.Client{Jar: jar}}
}

func ids(t *testing.T, body map[string]any) (student, teacher string) {
	t.Helper()
	for _, a := range body["data"].([]any) {
		m := a.(map[string]any)
		if m["role"] == "student" {
			student = m["id"].(string)
		} else {
			teacher = m["id"].(string)
		}
	}
	return
}

func TestKeyScopeAndSessionOnly(t *testing.T) {
	c := setup(t)
	if code, _ := c.do("GET", "/api/v1/accounts", "", nil); code != 401 {
		t.Fatalf("anonymous: got %d", code)
	}
	code, body := c.do("POST", "/api/app/demo", "", map[string]any{})
	if code != 201 {
		t.Fatalf("demo link: %d %v", code, body)
	}
	student, teacher := ids(t, body)

	code, body = c.do("POST", "/api/app/keys", "", map[string]any{"name": "k", "accountIds": []string{student}})
	if code != 201 {
		t.Fatalf("create key: %d %v", code, body)
	}
	key := body["data"].(map[string]any)["key"].(string)

	// The key sees exactly the account it was scoped to.
	_, body = c.do("GET", "/api/v1/accounts", key, nil)
	if got := body["data"].([]any); len(got) != 1 || got[0].(map[string]any)["id"] != student {
		t.Fatalf("key scope leaked: %v", got)
	}
	if code, _ := c.do("GET", "/api/v1/accounts/"+teacher+"/schedule", key, nil); code != 404 {
		t.Fatalf("out-of-scope account: got %d", code)
	}
	if code, _ := c.do("GET", "/api/v1/accounts/"+student+"/schedule", key, nil); code != 200 {
		t.Fatalf("in-scope schedule: got %d", code)
	}
	// Keys cannot manage the profile.
	if code, _ := c.do("GET", "/api/app/keys", key, nil); code != 401 {
		t.Fatalf("key reached session-only endpoint: %d", code)
	}
	if code, _ := c.do("DELETE", "/api/app/accounts/"+student, key, nil); code != 401 {
		t.Fatalf("key could unlink: %d", code)
	}
	// A key cannot be scoped to someone else's account.
	other := setup(t)
	other.base, other.http = c.base, func() *http.Client { j, _ := cookiejar.New(nil); return &http.Client{Jar: j} }()
	other.do("POST", "/api/app/demo", "", map[string]any{})
	if code, _ := other.do("POST", "/api/app/keys", "", map[string]any{"name": "x", "accountIds": []string{student}}); code != 400 {
		t.Fatalf("foreign account in key scope: got %d", code)
	}
	if code, _ := other.do("GET", "/api/v1/accounts/"+student+"/schedule", "", nil); code != 404 {
		t.Fatalf("foreign account readable: got %d", code)
	}
	// Students cannot look at other people.
	if code, _ := c.do("GET", "/api/v1/accounts/"+student+"/schedule?person=100002", "", nil); code != 403 {
		t.Fatalf("student viewed another person: %d", code)
	}
	if code, _ := c.do("GET", "/api/v1/accounts/"+teacher+"/schedule?person=100002", "", nil); code != 200 {
		t.Fatalf("teacher could not view a student: %d", code)
	}
	// Revoked keys stop working at once.
	_, body = c.do("GET", "/api/app/keys", "", nil)
	id := body["data"].([]any)[0].(map[string]any)["id"].(string)
	c.do("DELETE", "/api/app/keys/"+id, "", nil)
	if code, _ := c.do("GET", "/api/v1/accounts", key, nil); code != 401 {
		t.Fatalf("revoked key still works: %d", code)
	}
}

func TestCrossSiteCookieRejected(t *testing.T) {
	c := setup(t)
	c.do("POST", "/api/app/demo", "", map[string]any{})
	req, _ := http.NewRequest("GET", c.base+"/api/v1/accounts", nil)
	req.Header.Set("Sec-Fetch-Site", "cross-site")
	res, _ := c.http.Do(req)
	if res.StatusCode != 403 {
		t.Fatalf("cross-site cookie request: got %d", res.StatusCode)
	}
	req, _ = http.NewRequest("POST", c.base+"/api/app/keys", strings.NewReader(`{}`))
	res, _ = c.http.Do(req) // no Origin at all
	if res.StatusCode != 403 {
		t.Fatalf("POST without origin: got %d", res.StatusCode)
	}
}

func TestMCP(t *testing.T) {
	c := setup(t)
	_, body := c.do("POST", "/api/app/demo", "", map[string]any{})
	student, _ := ids(t, body)
	_, body = c.do("POST", "/api/app/keys", "", map[string]any{"name": "k", "accountIds": []string{student}})
	key := body["data"].(map[string]any)["key"].(string)

	req, _ := http.NewRequest("POST", c.base+"/mcp", strings.NewReader(`{"jsonrpc":"2.0","id":1,"method":"tools/list"}`))
	res, _ := http.DefaultClient.Do(req)
	if res.StatusCode != 401 || !strings.Contains(res.Header.Get("WWW-Authenticate"), "resource_metadata=") {
		t.Fatalf("unauthenticated MCP: %d %q", res.StatusCode, res.Header.Get("WWW-Authenticate"))
	}
	code, body := c.do("POST", "/mcp", key, map[string]any{"jsonrpc": "2.0", "id": 1, "method": "tools/call", "params": map[string]any{"name": "get_schedule", "arguments": map[string]any{"week": "2026-W46"}}})
	if code != 200 || body["error"] != nil {
		t.Fatalf("get_schedule: %d %v", code, body)
	}
	result := body["result"].(map[string]any)
	if result["isError"] == true || result["structuredContent"] == nil {
		t.Fatalf("unexpected tool result: %v", result)
	}
	// A student account cannot search people; the model gets a readable error.
	_, body = c.do("POST", "/mcp", key, map[string]any{"jsonrpc": "2.0", "id": 2, "method": "tools/call", "params": map[string]any{"name": "search_people", "arguments": map[string]any{"query": "a"}}})
	if body["result"].(map[string]any)["isError"] != true {
		t.Fatalf("expected tool error, got %v", body)
	}
}
