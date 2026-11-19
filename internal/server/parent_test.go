package server

import (
	"testing"

	"github.com/matthiaslubbertsen/melon/internal/zermelo"
)

func TestParentAccount(t *testing.T) {
	c := setup(t)
	link := func() {
		t.Helper()
		if code, body := c.do("POST", "/api/app/accounts", "", map[string]any{"school": "demo", "token": zermelo.DemoParentToken}); code != 201 {
			t.Fatalf("link parent: %d %v", code, body)
		}
	}
	link()

	// One account per child, each pinned to the child's code.
	accounts := func() []map[string]any {
		_, body := c.do("GET", "/api/app/session", "", nil)
		var out []map[string]any
		for _, a := range data(t, body)["accounts"].([]any) {
			out = append(out, a.(map[string]any))
		}
		return out
	}
	accs := accounts()
	if len(accs) != 2 {
		t.Fatalf("want 2 child accounts, got %v", accs)
	}
	for i, want := range []string{"100001", "100005"} {
		a := accs[i]
		if a["userCode"] != want || a["role"] != "parent" || a["parentCode"] != "o.devries" || a["canViewOthers"] != false {
			t.Fatalf("child %d: %v", i, a)
		}
	}
	if accs[0]["name"] != "Sam de Vries" {
		t.Fatalf("child named after Zermelo: %v", accs[0]["name"])
	}

	// Each child's schedule opens; nobody else's does.
	for _, a := range accs {
		code, body := c.do("GET", "/api/v1/accounts/"+a["id"].(string)+"/schedule?week=2026-W46", "", nil)
		if code != 200 {
			t.Fatalf("child schedule: %d %v", code, body)
		}
		sched := data(t, body)
		if sched["person"].(map[string]any)["code"] != a["userCode"] || len(sched["lessons"].([]any)) == 0 {
			t.Fatalf("schedule: %v", sched["person"])
		}
	}
	if code, _ := c.do("GET", "/api/v1/accounts/"+accs[0]["id"].(string)+"/schedule?week=2026-W46&person=100002", "", nil); code != 403 {
		t.Fatalf("parent read another student: %d", code)
	}

	// A nickname tells the children apart, and an empty one restores Zermelo's name.
	id := accs[1]["id"].(string)
	code, body := c.do("PUT", "/api/app/accounts/"+id+"/nickname", "", map[string]any{"nickname": "  Jongste  "})
	if code != 200 || data(t, body)["name"] != "Jongste" || data(t, body)["zermeloName"] != "Finn Visser" {
		t.Fatalf("nickname: %d %v", code, body)
	}
	if code, _ := c.do("PUT", "/api/app/accounts/"+id+"/nickname", "", map[string]any{"nickname": "x123456789x123456789x123456789x123456789x"}); code != 400 {
		t.Fatalf("long nickname: %d", code)
	}
	c.do("PUT", "/api/app/accounts/"+id+"/nickname", "", map[string]any{"nickname": ""})
	if accounts()[1]["name"] != "Finn Visser" {
		t.Fatal("nickname not cleared")
	}

	// Linking again updates the children instead of adding more.
	link()
	if n := len(accounts()); n != 2 {
		t.Fatalf("relink made %d accounts", n)
	}

	// The sign-in number is the parent's own, never a child's student number.
	if code, body := c.do("PUT", "/api/app/pin", "", map[string]any{"accountId": accs[0]["id"], "pin": "4821"}); code != 200 || data(t, body)["userId"] != "o.devries" {
		t.Fatalf("PIN for a parent: %d %v", code, body)
	}
}

// The admin list and "view as" show a parent's profile under the parent's own
// name, and searching for the parent finds it.
func TestAdminSeesParentName(t *testing.T) {
	admin := setupAdmins(t, zermelo.DemoSchool+":bkn")
	admin.do("POST", "/api/app/demo", "", map[string]any{})
	parent := admin.fresh()
	if code, body := parent.do("POST", "/api/app/accounts", "", map[string]any{"school": "demo", "token": zermelo.DemoParentToken}); code != 201 {
		t.Fatalf("link parent: %d %v", code, body)
	}
	_, body := admin.do("GET", "/api/app/admin/profiles?q=Eva", "", nil)
	list := body["data"].([]any)
	if len(list) != 1 {
		t.Fatalf("search by parent name: %v", body)
	}
	for _, a := range list[0].(map[string]any)["accounts"].([]any) {
		if a := a.(map[string]any); a["parentCode"] != "o.devries" || a["parentName"] != "Eva de Vries" {
			t.Fatalf("admin account: %v", a)
		}
	}
}
