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
	// A teacher name set on one account shows up in that account's schedule.
	if code, _ := c.do("PUT", "/api/v1/accounts/"+student+"/teachers/MUL", "", map[string]any{"name": "Meneer Mulder"}); code != 200 {
		t.Fatalf("name teacher: got %d", code)
	}
	_, body = c.do("GET", "/api/v1/accounts/"+student+"/schedule?week=2026-W46", "", nil)
	raw, _ := json.Marshal(body)
	if !strings.Contains(string(raw), `{"code":"mul","name":"Meneer Mulder"}`) {
		t.Fatal("teacher name missing from schedule")
	}
	if code, _ := other.do("PUT", "/api/v1/accounts/"+student+"/teachers/mul", "", map[string]any{"name": "x"}); code != 404 {
		t.Fatalf("foreign profile named a teacher: got %d", code)
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

func (c client) fresh() client {
	jar, _ := cookiejar.New(nil)
	return client{t: c.t, base: c.base, http: &http.Client{Jar: jar}}
}

func TestSignInWithPIN(t *testing.T) {
	c := setup(t)
	_, body := c.do("POST", "/api/app/demo", "", map[string]any{})
	student, _ := ids(t, body)

	// Without a PIN, signing out would lose the profile for good.
	if code, _ := c.do("POST", "/api/app/logout", "", nil); code != 409 {
		t.Fatalf("sign out without PIN: got %d", code)
	}
	for _, pin := range []string{"123", "12345", "12a4", ""} {
		if code, _ := c.do("PUT", "/api/app/pin", "", map[string]any{"accountId": student, "pin": pin}); code != 400 {
			t.Fatalf("PIN %q accepted: %d", pin, code)
		}
	}
	code, body := c.do("PUT", "/api/app/pin", "", map[string]any{"accountId": student, "pin": "4821"})
	if code != 200 {
		t.Fatalf("set PIN: %d %v", code, body)
	}
	number := body["data"].(map[string]any)["userId"].(string)
	_, body = c.do("GET", "/api/app/session", "", nil)
	if got := body["data"].(map[string]any)["login"].(map[string]any)["userId"]; got != number {
		t.Fatalf("session does not show the sign-in: %v", got)
	}

	// Another device gets the whole profile: both Zermelo accounts.
	phone := c.fresh()
	if code, _ := phone.do("POST", "/api/app/login", "", map[string]any{"userId": number, "pin": "0000"}); code != 401 {
		t.Fatalf("wrong PIN: got %d", code)
	}
	if code, _ := phone.do("POST", "/api/app/login", "", map[string]any{"userId": "999999", "pin": "4821"}); code != 401 {
		t.Fatalf("unknown number: got %d", code)
	}
	code, body = phone.do("POST", "/api/app/login", "", map[string]any{"userId": number, "pin": "4821"})
	if code != 200 || len(body["data"].(map[string]any)["accounts"].([]any)) != 2 {
		t.Fatalf("sign in: %d %v", code, body)
	}
	if code, _ := phone.do("GET", "/api/v1/accounts/"+student+"/schedule", "", nil); code != 200 {
		t.Fatalf("signed-in device cannot read schedule: %d", code)
	}

	// Signing out keeps the profile; the first device still works.
	if code, _ := phone.do("POST", "/api/app/logout", "", nil); code != 204 {
		t.Fatalf("sign out: got %d", code)
	}
	if code, _ := phone.do("GET", "/api/v1/accounts", "", nil); code != 401 {
		t.Fatalf("signed-out device still in: %d", code)
	}
	if code, _ := c.do("GET", "/api/v1/accounts", "", nil); code != 200 {
		t.Fatalf("first device lost its session: %d", code)
	}

	// Changing the PIN signs other devices out.
	phone.do("POST", "/api/app/login", "", map[string]any{"userId": number, "pin": "4821"})
	c.do("PUT", "/api/app/pin", "", map[string]any{"accountId": student, "pin": "7305"})
	if code, _ := phone.do("GET", "/api/v1/accounts", "", nil); code != 401 {
		t.Fatalf("old device survived a PIN change: %d", code)
	}

	// Linking the same Zermelo account elsewhere never opens this profile.
	// Setting a PIN there takes over the number instead.
	thief := c.fresh()
	_, body = thief.do("POST", "/api/app/demo", "", map[string]any{})
	theirs, _ := ids(t, body)
	thief.do("PUT", "/api/app/pin", "", map[string]any{"accountId": theirs, "pin": "1111"})
	if code, _ := c.fresh().do("POST", "/api/app/login", "", map[string]any{"userId": number, "pin": "7305"}); code != 401 {
		t.Fatalf("old PIN still works after takeover: %d", code)
	}
	_, body = c.do("GET", "/api/app/session", "", nil)
	if body["data"].(map[string]any)["login"] != nil {
		t.Fatal("first profile still claims the number")
	}
	if code, _ := c.do("GET", "/api/v1/accounts", "", nil); code != 200 {
		t.Fatalf("takeover killed the first profile's session: %d", code)
	}
}

func TestPINLockout(t *testing.T) {
	c := setup(t)
	_, body := c.do("POST", "/api/app/demo", "", map[string]any{})
	student, _ := ids(t, body)
	_, body = c.do("PUT", "/api/app/pin", "", map[string]any{"accountId": student, "pin": "4821"})
	number := body["data"].(map[string]any)["userId"].(string)

	guesser := c.fresh()
	for i := range store.PINFreeTries {
		if code, _ := guesser.do("POST", "/api/app/login", "", map[string]any{"userId": number, "pin": "0000"}); code != 401 {
			t.Fatalf("guess %d: got %d", i+1, code)
		}
	}
	// Locked now, even for the right PIN.
	if code, body := guesser.do("POST", "/api/app/login", "", map[string]any{"userId": number, "pin": "4821"}); code != 429 || body["error"].(map[string]any)["code"] != "login_locked" {
		t.Fatalf("after lockout: %d %v", code, body)
	}
	// A device that is still signed in resets it by choosing a new PIN.
	c.do("PUT", "/api/app/pin", "", map[string]any{"accountId": student, "pin": "9164"})
	if code, _ := guesser.do("POST", "/api/app/login", "", map[string]any{"userId": number, "pin": "9164"}); code != 200 {
		t.Fatalf("sign in after reset: got %d", code)
	}
}
