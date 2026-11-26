package server

import (
	"testing"

	"github.com/matthiaslubbertsen/melon/internal/zermelo"
)

func TestSignUpNeedsPIN(t *testing.T) {
	c := setup(t)
	link := func(pin string) (int, map[string]any) {
		body := map[string]any{"school": "demo", "token": zermelo.DemoStudentToken}
		if pin != "" {
			body["pin"] = pin
		}
		return c.do("POST", "/api/app/accounts", "", body)
	}
	for _, pin := range []string{"", "12", "abcd"} {
		code, body := link(pin)
		if code != 400 || body["error"].(map[string]any)["code"] != "invalid_pin" {
			t.Fatalf("pin %q: want invalid_pin, got %d %v", pin, code, body)
		}
	}
	if code, body := link("4321"); code != 201 {
		t.Fatalf("link with pin: %d %v", code, body)
	}
	_, body := c.do("GET", "/api/app/session", "", nil)
	login, _ := data(t, body)["login"].(map[string]any)
	if login == nil || login["userId"] != "100001" {
		t.Fatalf("login not set: %v", body)
	}
	// The profile now has a PIN, so the next link needs none and keeps it.
	if code, body := c.do("POST", "/api/app/accounts", "", map[string]any{"school": "demo", "token": zermelo.DemoTeacherToken}); code != 201 {
		t.Fatalf("second link: %d %v", code, body)
	}
	if code, body := c.fresh().do("POST", "/api/app/login", "", map[string]any{"userId": "100001", "pin": "4321"}); code != 200 {
		t.Fatalf("sign in with the new pin: %d %v", code, body)
	}
}
