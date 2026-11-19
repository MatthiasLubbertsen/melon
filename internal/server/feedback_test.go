package server

import (
	"testing"

	"github.com/matthiaslubbertsen/melon/internal/zermelo"
)

func TestFeedback(t *testing.T) {
	admin := setupAdmins(t, zermelo.DemoSchool+":bkn")
	admin.do("POST", "/api/app/demo", "", map[string]any{})
	user := admin.fresh()
	code, body := user.do("POST", "/api/app/accounts", "", map[string]any{"school": "demo", "token": zermelo.DemoStudentToken})
	if code != 201 {
		t.Fatalf("link: %d %v", code, body)
	}
	account := data(t, body)["id"].(string)

	// Bad input is refused.
	for _, b := range []map[string]any{{"kind": "rant", "text": "hi"}, {"kind": "bug", "text": "   "}} {
		if code, _ := user.do("POST", "/api/app/feedback", "", b); code != 400 {
			t.Fatalf("%v: %d", b, code)
		}
	}

	// The user reports a bug.
	code, body = user.do("POST", "/api/app/feedback", "", map[string]any{"kind": "bug", "text": "De weekweergave springt.", "page": "/", "account": account})
	if code != 201 {
		t.Fatalf("create: %d %v", code, body)
	}
	id := data(t, body)["thread"].(map[string]any)["id"].(string)

	// Admins see it as unread; reading it clears that.
	_, body = admin.do("GET", "/api/app/admin/overview", "", nil)
	if data(t, body)["unreadFeedback"] != 1.0 {
		t.Fatalf("unread for admin: %v", data(t, body)["unreadFeedback"])
	}
	_, body = admin.do("GET", "/api/app/admin/feedback", "", nil)
	list := body["data"].([]any)
	if len(list) != 1 || list[0].(map[string]any)["unread"] != true || list[0].(map[string]any)["profileName"] == "" || list[0].(map[string]any)["accountName"] == "" {
		t.Fatalf("admin list: %v", list)
	}
	admin.do("GET", "/api/app/admin/feedback/"+id, "", nil)
	if _, body := admin.do("GET", "/api/app/admin/overview", "", nil); data(t, body)["unreadFeedback"] != 0.0 {
		t.Fatal("still unread after reading")
	}

	// The admin answers and closes it; the user sees the answer as new.
	if code, _ := admin.do("POST", "/api/app/admin/feedback/"+id+"/messages", "", map[string]any{"text": "Welke browser?"}); code != 200 {
		t.Fatalf("admin reply: %d", code)
	}
	if code, _ := admin.do("PUT", "/api/app/admin/feedback/"+id, "", map[string]any{"status": "closed"}); code != 200 {
		t.Fatalf("close: %d", code)
	}
	_, body = user.do("GET", "/api/app/feedback", "", nil)
	if mine := body["data"].([]any); len(mine) != 1 || mine[0].(map[string]any)["unread"] != true {
		t.Fatalf("user list: %v", mine)
	}

	// The user answers back, which reopens it.
	code, body = user.do("POST", "/api/app/feedback/"+id+"/messages", "", map[string]any{"text": "Firefox"})
	if code != 200 {
		t.Fatalf("user reply: %d %v", code, body)
	}
	thread := data(t, body)
	if thread["thread"].(map[string]any)["status"] != "open" || len(thread["messages"].([]any)) != 3 {
		t.Fatalf("thread: %v", thread)
	}
	if m := thread["messages"].([]any)[1].(map[string]any); m["fromAdmin"] != true || m["author"] != nil {
		t.Fatalf("admin message leaks who wrote it: %v", m)
	}

	// Someone else's thread does not exist for you, and non-admins cannot list them all.
	other := admin.fresh()
	other.do("POST", "/api/app/accounts", "", map[string]any{"school": "demo", "token": zermelo.DemoStudentToken})
	if code, _ := other.do("GET", "/api/app/feedback/"+id, "", nil); code != 404 {
		t.Fatalf("other user read the thread: %d", code)
	}
	if code, _ := other.do("POST", "/api/app/feedback/"+id+"/messages", "", map[string]any{"text": "hoi"}); code != 404 {
		t.Fatalf("other user replied: %d", code)
	}
	if code, _ := admin.do("POST", "/api/app/admin/feedback/fb_nope/messages", "", map[string]any{"text": "hoi"}); code != 404 {
		t.Fatalf("reply to a missing thread: %d", code)
	}
	if code, _ := user.do("GET", "/api/app/admin/feedback", "", nil); code != 403 {
		t.Fatalf("non-admin listed feedback: %d", code)
	}
}

// A parent's feedback is from the parent, about the child that was open.
func TestFeedbackFromParent(t *testing.T) {
	admin := setupAdmins(t, zermelo.DemoSchool+":bkn")
	admin.do("POST", "/api/app/demo", "", map[string]any{})
	parent := admin.fresh()
	code, body := parent.do("POST", "/api/app/accounts", "", map[string]any{"school": "demo", "token": zermelo.DemoParentToken})
	if code != 201 {
		t.Fatalf("link parent: %d %v", code, body)
	}
	_, body = parent.do("GET", "/api/app/session", "", nil)
	child := data(t, body)["accounts"].([]any)[1].(map[string]any)
	if code, body := parent.do("POST", "/api/app/feedback", "", map[string]any{"kind": "idea", "text": "Zelfde dag graag.", "page": "/", "account": child["id"]}); code != 201 {
		t.Fatalf("create: %d %v", code, body)
	}
	_, body = admin.do("GET", "/api/app/admin/feedback", "", nil)
	f := body["data"].([]any)[0].(map[string]any)
	if f["profileName"] != "Eva de Vries" || f["accountName"] != child["zermeloName"] {
		t.Fatalf("from %v about %v, want Eva de Vries about %v", f["profileName"], f["accountName"], child["zermeloName"])
	}
}
