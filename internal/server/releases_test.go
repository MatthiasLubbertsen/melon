package server

import (
	"testing"

	"github.com/matthiaslubbertsen/melon/internal/store"
)

func TestReleasesSeenFollowsTheProfile(t *testing.T) {
	c := setup(t)
	_, body := c.do("POST", "/api/app/demo", "", map[string]any{})
	student, _ := ids(t, body)

	// A new profile has nothing to catch up on.
	if _, body := c.do("GET", "/api/app/session", "", nil); data(t, body)["releasesSeen"] != "" {
		t.Fatalf("new profile: %v", data(t, body)["releasesSeen"])
	}
	for _, id := range []string{"", string(make([]byte, 65))} {
		if code, _ := c.do("PUT", "/api/app/releases/seen", "", map[string]any{"id": id}); code != 400 {
			t.Fatalf("id %q accepted: %d", id, code)
		}
	}
	if code, _ := c.do("PUT", "/api/app/releases/seen", "", map[string]any{"id": "parents"}); code != 204 {
		t.Fatalf("mark seen: %d", code)
	}

	// Another device signed in to the same profile knows it was seen.
	_, body = c.do("PUT", "/api/app/pin", "", map[string]any{"accountId": student, "pin": "4821"})
	number := data(t, body)["userId"].(string)
	phone := c.fresh()
	if code, body := phone.do("POST", "/api/app/login", "", map[string]any{"userId": number, "pin": "4821"}); code != 200 || data(t, body)["releasesSeen"] != "parents" {
		t.Fatalf("other device: %d %v", code, body)
	}
}

func TestReleasesSeenStore(t *testing.T) {
	s, err := store.Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	ctx := t.Context()
	if err := s.CreateUser(ctx, "usr_new"); err != nil {
		t.Fatal(err)
	}
	if seen, err := s.ReleasesSeen(ctx, "usr_new"); err != nil || seen == nil || *seen != "" {
		t.Fatalf("new profile: %v %v", seen, err)
	}
	if err := s.SetReleasesSeen(ctx, "usr_new", "x"); err != nil {
		t.Fatal(err)
	}
	if seen, _ := s.ReleasesSeen(ctx, "usr_new"); seen == nil || *seen != "x" {
		t.Fatalf("after set: %v", seen)
	}
	if err := s.SetReleasesSeen(ctx, "usr_gone", "x"); err == nil {
		t.Fatal("set on a missing profile succeeded")
	}
}
