package server

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/matthiaslubbertsen/melon/internal/zermelo"
)

func TestParseAdmins(t *testing.T) {
	a, err := ParseAdmins("usr_abc, jfc:12345 ; 777", "demo")
	if err != nil {
		t.Fatal(err)
	}
	if !a.profiles["usr_abc"] || !a.accounts[[2]string{"jfc", "12345"}] || !a.accounts[[2]string{"demo", "777"}] || a.Count() != 3 {
		t.Fatalf("parsed: %+v", a)
	}
	if _, err := ParseAdmins("12345", ""); err == nil {
		t.Fatal("bare user code without a default school accepted")
	}
	if _, err := ParseAdmins("not a school!:1", ""); err == nil {
		t.Fatal("bad school accepted")
	}
	if a, err := ParseAdmins("", ""); err != nil || a.Count() != 0 {
		t.Fatalf("empty: %v %d", err, a.Count())
	}
}

func data(t *testing.T, body map[string]any) map[string]any {
	t.Helper()
	d, ok := body["data"].(map[string]any)
	if !ok {
		t.Fatalf("no data object: %v", body)
	}
	return d
}

func TestAdmin(t *testing.T) {
	// The demo teacher is the admin, so the profile that opens the demo is one.
	admin := setupAdmins(t, zermelo.DemoSchool+":bkn")
	_, body := admin.do("POST", "/api/app/demo", "", map[string]any{})
	adminStudent, _ := ids(t, body)
	if _, body := admin.do("GET", "/api/app/session", "", nil); data(t, body)["admin"] != true {
		t.Fatalf("demo teacher is not an admin: %v", body)
	}

	// A profile with only the demo student is not.
	user := admin.fresh()
	code, body := user.do("POST", "/api/app/accounts", "", map[string]any{"school": "demo", "token": zermelo.DemoStudentToken})
	if code != 201 {
		t.Fatalf("link student: %d %v", code, body)
	}
	student := data(t, body)["id"].(string)
	if _, body := user.do("GET", "/api/app/session", "", nil); data(t, body)["admin"] != false {
		t.Fatal("student is an admin")
	}
	for _, path := range []string{"/api/app/admin/overview", "/api/app/admin/profiles", "/api/app/admin/log"} {
		if code, _ := user.do("GET", path, "", nil); code != 403 {
			t.Fatalf("%s as non-admin: %d", path, code)
		}
	}
	_, body = user.do("POST", "/api/app/keys", "", map[string]any{"name": "script", "accountIds": []string{student}})
	key := data(t, body)["key"].(string)
	// Keys never reach the admin API, even an admin's.
	_, body = admin.do("POST", "/api/app/keys", "", map[string]any{"name": "mine", "accountIds": []string{adminStudent}})
	if code, _ := admin.do("GET", "/api/app/admin/overview", data(t, body)["key"].(string), nil); code != 401 {
		t.Fatalf("API key reached the admin API: %d", code)
	}

	// Usage is counted per channel.
	user.do("GET", "/api/v1/accounts/"+student+"/schedule?week=2026-W46", "", nil)
	user.do("GET", "/api/v1/accounts/"+student+"/schedule?week=2026-W46", key, nil)
	code, body = admin.do("GET", "/api/app/admin/overview", "", nil)
	if code != 200 {
		t.Fatalf("overview: %d %v", code, body)
	}
	ov := data(t, body)
	if st := ov["stats"].(map[string]any); st["profiles"] != 2.0 || st["accounts"] != 3.0 || st["apiKeys"] != 2.0 {
		t.Fatalf("stats: %v", st)
	}
	channels := map[string]bool{}
	for _, u := range ov["usage"].([]any) {
		channels[u.(map[string]any)["channel"].(string)] = true
	}
	if !channels["session"] || !channels["api_key"] || ov["totals"].(map[string]any)["lessons"].(float64) == 0 {
		t.Fatalf("usage: %v %v", ov["usage"], ov["totals"])
	}

	// Find the user, check their account with Zermelo and show its token.
	_, body = admin.do("GET", "/api/app/admin/profiles?q=100001", "", nil)
	var profile string
	for _, p := range body["data"].([]any) {
		if p.(map[string]any)["admin"] == false {
			profile = p.(map[string]any)["id"].(string)
		}
	}
	if profile == "" {
		t.Fatalf("search did not find the user: %v", body)
	}
	_, body = admin.do("GET", "/api/app/admin/profiles/"+profile, "", nil)
	if keys := data(t, body)["keys"].([]any); len(keys) != 1 || keys[0].(map[string]any)["key"] != nil {
		t.Fatalf("keys: %v", keys)
	}
	if code, body := admin.do("POST", "/api/app/admin/accounts/"+student+"/check", "", nil); code != 200 || data(t, body)["tokenWorks"] != true || data(t, body)["zermeloUser"] != "100001" {
		t.Fatalf("check: %d %v", code, body)
	}
	if code, body := admin.do("GET", "/api/app/admin/accounts/"+student+"/token", "", nil); code != 200 || data(t, body)["token"] != zermelo.DemoStudentToken {
		t.Fatalf("zermelo token: %d %v", code, body)
	}
	if code, _ := user.do("GET", "/api/app/admin/accounts/"+student+"/token", "", nil); code != 403 {
		t.Fatalf("non-admin read a token: %d", code)
	}

	// Give the user a PIN they forgot, lock it, unlock it.
	code, body = admin.do("PUT", "/api/app/admin/profiles/"+profile+"/pin", "", map[string]any{"accountId": student, "pin": "2468"})
	if code != 200 {
		t.Fatalf("admin set PIN: %d %v", code, body)
	}
	for range 6 {
		user.fresh().do("POST", "/api/app/login", "", map[string]any{"userId": "100001", "pin": "0000"})
	}
	if code, _ := user.fresh().do("POST", "/api/app/login", "", map[string]any{"userId": "100001", "pin": "2468"}); code != 429 {
		t.Fatalf("not locked: %d", code)
	}
	if code, _ := admin.do("POST", "/api/app/admin/profiles/"+profile+"/unlock", "", nil); code != 204 {
		t.Fatalf("unlock: %d", code)
	}
	if code, _ := user.fresh().do("POST", "/api/app/login", "", map[string]any{"userId": "100001", "pin": "2468"}); code != 200 {
		t.Fatalf("sign in after unlock: %d", code)
	}

	// Look at Melon as the user, then come back.
	code, body = admin.do("POST", "/api/app/admin/profiles/"+profile+"/impersonate", "", nil)
	if code != 200 || data(t, body)["impersonating"] != true {
		t.Fatalf("impersonate: %d %v", code, body)
	}
	if accts := data(t, body)["accounts"].([]any); len(accts) != 1 || accts[0].(map[string]any)["id"] != student {
		t.Fatalf("not the user's accounts: %v", accts)
	}
	if code, _ := admin.do("GET", "/api/v1/accounts/"+student+"/schedule", "", nil); code != 200 {
		t.Fatalf("schedule as the user: %d", code)
	}
	if code, _ := admin.do("GET", "/api/app/admin/overview", "", nil); code != 403 {
		t.Fatalf("admin API while impersonating: %d", code)
	}
	if code, _ := user.do("POST", "/api/app/admin/stop", "", nil); code != 409 {
		t.Fatalf("stop without impersonating: %d", code)
	}
	code, body = admin.do("POST", "/api/app/admin/stop", "", nil)
	if code != 200 || data(t, body)["admin"] != true || data(t, body)["impersonating"] != false {
		t.Fatalf("stop: %d %v", code, body)
	}
	// Viewing as someone else is not "being active".
	_, body = admin.do("GET", "/api/app/admin/profiles/"+profile, "", nil)
	if data(t, body)["sessions"] != 2.0 {
		t.Fatalf("impersonation counted as a device: %v", data(t, body)["sessions"])
	}

	// Global subject and school teacher names reach every account.
	if code, _ := admin.do("PUT", "/api/app/admin/subjects/GS", "", map[string]any{"nl": "Geschiedenis (globaal)"}); code != 200 {
		t.Fatalf("set subject: %d", code)
	}
	if code, _ := admin.do("PUT", "/api/app/admin/teachers/demo/MUL", "", map[string]any{"name": "Meneer Mulder"}); code != 200 {
		t.Fatalf("set teacher: %d", code)
	}
	_, body = user.do("GET", "/api/v1/accounts/"+student+"/schedule?week=2026-W46", "", nil)
	raw, _ := json.Marshal(body)
	if !strings.Contains(string(raw), `{"code":"gs","name":"Geschiedenis (globaal)"}`) || !strings.Contains(string(raw), `{"code":"mul","name":"Meneer Mulder"}`) {
		t.Fatal("global names missing from the schedule")
	}
	_, body = user.do("GET", "/api/v1/subjects?lang=en", "", nil)
	raw, _ = json.Marshal(body)
	if !strings.Contains(string(raw), `{"code":"gs","name":"History"}`) {
		t.Fatalf("global subject without English lost the built-in English: %s", raw)
	}

	// What users name things shows up as a suggestion, until it is adopted.
	user.do("PUT", "/api/v1/accounts/"+student+"/subjects/ne", "", map[string]any{"name": "Nederlands!"})
	user.do("PUT", "/api/v1/accounts/"+student+"/teachers/mul", "", map[string]any{"name": "Dhr. Mulder"})
	_, body = admin.do("GET", "/api/app/admin/suggestions", "", nil)
	raw, _ = json.Marshal(body)
	if !strings.Contains(string(raw), `"code":"ne","current":"Nederlands","name":"Nederlands!"`) || !strings.Contains(string(raw), `"code":"mul","current":"Meneer Mulder","name":"Dhr. Mulder","profiles":1,"school":"demo"`) {
		t.Fatalf("suggestions: %s", raw)
	}
	admin.do("PUT", "/api/app/admin/teachers/demo/mul", "", map[string]any{"name": "Dhr. Mulder"})
	_, body = admin.do("GET", "/api/app/admin/suggestions", "", nil)
	if teachers := data(t, body)["teachers"].([]any); len(teachers) != 0 {
		t.Fatalf("adopted suggestion still listed: %v", teachers)
	}

	// Sign the user out everywhere, then remove them.
	if code, _ := admin.do("DELETE", "/api/app/admin/profiles/"+profile+"/sessions", "", nil); code != 204 {
		t.Fatalf("sign out: %d", code)
	}
	if code, _ := user.do("GET", "/api/v1/accounts", "", nil); code != 401 {
		t.Fatalf("user still signed in: %d", code)
	}
	if code, _ := admin.do("DELETE", "/api/app/admin/profiles/"+profile, "", nil); code != 204 {
		t.Fatalf("delete profile: %d", code)
	}
	if code, _ := admin.do("GET", "/api/v1/accounts", key, nil); code != 401 {
		t.Fatalf("deleted profile's key still works: %d", code)
	}

	// Everything above is in the log, newest first.
	_, body = admin.do("GET", "/api/app/admin/log", "", nil)
	var actions []string
	for _, e := range body["data"].([]any) {
		actions = append(actions, e.(map[string]any)["action"].(string))
	}
	want := "delete_profile,sign_out,set_teacher,set_teacher,set_subject,impersonate,unlock_pin,set_pin,reveal_token"
	if strings.Join(actions, ",") != want {
		t.Fatalf("log: %v", actions)
	}
}

func TestImpersonationRunsOut(t *testing.T) {
	admin := setupAdmins(t, zermelo.DemoSchool+":bkn")
	admin.do("POST", "/api/app/demo", "", map[string]any{})
	user := admin.fresh()
	user.do("POST", "/api/app/accounts", "", map[string]any{"school": "demo", "token": zermelo.DemoStudentToken})
	_, body := admin.do("GET", "/api/app/admin/profiles", "", nil)
	var profile string
	for _, p := range body["data"].([]any) {
		if p.(map[string]any)["admin"] == false {
			profile = p.(map[string]any)["id"].(string)
		}
	}
	admin.do("POST", "/api/app/admin/profiles/"+profile+"/impersonate", "", nil)
	// The user signs out everywhere (or the session expires): the admin's own
	// session comes back on the next load instead of leaving them signed out.
	user.do("DELETE", "/api/app/session", "", nil)
	_, body = admin.do("GET", "/api/app/session", "", nil)
	if d := data(t, body); d["admin"] != true || d["impersonating"] != false || len(d["accounts"].([]any)) != 2 {
		t.Fatalf("admin not restored: %v", d)
	}
}

func mcpCall(t *testing.T, c client, key, tool string, args map[string]any) (map[string]any, bool) {
	t.Helper()
	_, body := c.do("POST", "/mcp", key, map[string]any{"jsonrpc": "2.0", "id": 1, "method": "tools/call", "params": map[string]any{"name": tool, "arguments": args}})
	if body["error"] != nil {
		return nil, false
	}
	res := body["result"].(map[string]any)
	if res["isError"] == true {
		return res, false
	}
	return res["structuredContent"].(map[string]any), true
}

func TestAdminMCP(t *testing.T) {
	admin := setupAdmins(t, zermelo.DemoSchool+":bkn")
	_, body := admin.do("POST", "/api/app/demo", "", map[string]any{})
	own, _ := ids(t, body)
	user := admin.fresh()
	_, body = user.do("POST", "/api/app/accounts", "", map[string]any{"school": "demo", "token": zermelo.DemoStudentToken})
	student := data(t, body)["id"].(string)

	// Only admins may make a key for the admin tools, and only on purpose.
	if code, _ := user.do("POST", "/api/app/keys", "", map[string]any{"name": "x", "accountIds": []string{student}, "admin": true}); code != 403 {
		t.Fatalf("non-admin made an admin key: %d", code)
	}
	_, body = admin.do("POST", "/api/app/keys", "", map[string]any{"name": "plain", "accountIds": []string{own}})
	plain := data(t, body)["key"].(string)
	_, body = admin.do("POST", "/api/app/keys", "", map[string]any{"name": "debug", "accountIds": []string{own}, "admin": true})
	if data(t, body)["admin"] != true {
		t.Fatalf("admin key: %v", body)
	}
	debug := data(t, body)["key"].(string)

	toolNames := func(key string) string {
		_, body := admin.do("POST", "/mcp", key, map[string]any{"jsonrpc": "2.0", "id": 1, "method": "tools/list"})
		var names []string
		for _, tl := range body["result"].(map[string]any)["tools"].([]any) {
			names = append(names, tl.(map[string]any)["name"].(string))
		}
		return strings.Join(names, ",")
	}
	if strings.Contains(toolNames(plain), "admin_") {
		t.Fatal("a key without the admin opt-in lists admin tools")
	}
	if _, ok := mcpCall(t, admin, plain, "admin_overview", nil); ok {
		t.Fatal("a key without the admin opt-in ran an admin tool")
	}
	if !strings.Contains(toolNames(debug), "admin_get_schedule") {
		t.Fatal("admin key misses admin tools")
	}

	out, ok := mcpCall(t, admin, debug, "admin_find_profiles", map[string]any{"query": "100001"})
	if !ok {
		t.Fatalf("find: %v", out)
	}
	var profile string
	for _, p := range out["profiles"].([]any) {
		if p.(map[string]any)["admin"] == false {
			profile = p.(map[string]any)["id"].(string)
		}
	}
	if out, ok := mcpCall(t, admin, debug, "admin_get_profile", map[string]any{"profile": profile}); !ok || len(out["profile"].(map[string]any)["accounts"].([]any)) != 1 {
		t.Fatalf("profile: %v", out)
	}
	if out, ok := mcpCall(t, admin, debug, "admin_check_account", map[string]any{"account": student}); !ok || out["tokenWorks"] != true || out["sameUser"] != true {
		t.Fatalf("check: %v", out)
	}
	// Someone else's schedule, with their own names in it.
	user.do("PUT", "/api/v1/accounts/"+student+"/teachers/mul", "", map[string]any{"name": "Meester Mulder"})
	out, ok = mcpCall(t, admin, debug, "admin_get_schedule", map[string]any{"account": student, "week": "2026-W46"})
	raw, _ := json.Marshal(out)
	if !ok || !strings.Contains(string(raw), "Meester Mulder") {
		t.Fatalf("schedule: %s", raw)
	}
	if out, ok := mcpCall(t, admin, debug, "admin_overview", nil); !ok || out["stats"].(map[string]any)["profiles"] != 2.0 {
		t.Fatalf("overview: %v", out)
	}
	if _, ok := mcpCall(t, admin, debug, "admin_suggestions", nil); !ok {
		t.Fatal("suggestions failed")
	}
	out, _ = mcpCall(t, admin, debug, "admin_get_log", nil)
	raw, _ = json.Marshal(out)
	if !strings.Contains(string(raw), `"action":"read_schedule"`) || !strings.Contains(string(raw), `"action":"check_account"`) {
		t.Fatalf("log: %s", raw)
	}
	// Session-only admin actions are not reachable with the key at all.
	if code, _ := admin.do("GET", "/api/app/admin/accounts/"+student+"/token", debug, nil); code != 401 {
		t.Fatalf("admin key read a Zermelo token: %d", code)
	}
}

// Taking someone off MELON_ADMINS turns their admin keys into plain keys.
func TestAdminKeyNeedsAdminStill(t *testing.T) {
	admin := setupAdmins(t, zermelo.DemoSchool+":bkn")
	_, body := admin.do("POST", "/api/app/demo", "", map[string]any{})
	student, teacher := ids(t, body)
	_, body = admin.do("POST", "/api/app/keys", "", map[string]any{"name": "debug", "accountIds": []string{student}, "admin": true})
	debug := data(t, body)["key"].(string)
	if _, ok := mcpCall(t, admin, debug, "admin_overview", nil); !ok {
		t.Fatal("admin key cannot use admin tools")
	}
	// Unlinking the admin account is as good as leaving MELON_ADMINS.
	admin.do("DELETE", "/api/app/accounts/"+teacher, "", nil)
	if _, ok := mcpCall(t, admin, debug, "admin_overview", nil); ok {
		t.Fatal("admin tools still work after losing admin")
	}
}
