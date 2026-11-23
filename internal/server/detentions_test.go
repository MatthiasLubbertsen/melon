package server

import (
	"net/http"
	"testing"
	"time"
)

func TestDetentions(t *testing.T) {
	c := setup(t)
	if code, _ := c.do("POST", "/api/app/demo", "", map[string]any{}); code != http.StatusCreated {
		t.Fatalf("demo: %d", code)
	}
	_, accs := c.do("GET", "/api/v1/accounts", "", nil)
	student, _ := ids(t, accs)
	day := time.Now().Format(time.DateOnly)
	base := "/api/v1/accounts/" + student + "/detentions"

	for _, bad := range []map[string]any{
		{"date": "nope", "start": "15:30", "end": "16:30"},
		{"date": day, "start": "16:30", "end": "15:30"},
		{"date": day, "start": "15:30", "end": "16:30", "period": map[string]any{"from": 3, "to": 2}},
	} {
		if code, _ := c.do("POST", base, "", bad); code != http.StatusBadRequest {
			t.Errorf("%v: got %d, want 400", bad, code)
		}
	}

	code, body := c.do("POST", base, "", map[string]any{"date": day, "start": "15:30", "end": "16:20", "period": map[string]any{"from": 8, "to": 8}, "subject": "WI", "location": "112"})
	if code != http.StatusCreated {
		t.Fatalf("add: %d %v", code, body)
	}
	id := body["data"].(map[string]any)["id"].(string)

	find := func(path string) map[string]any {
		_, sched := c.do("GET", path, "", nil)
		for _, l := range sched["data"].(map[string]any)["lessons"].([]any) {
			if m := l.(map[string]any); m["id"] == id {
				return m
			}
		}
		return nil
	}
	l := find("/api/v1/accounts/" + student + "/schedule?from=today")
	if l == nil {
		t.Fatal("detention missing from the schedule")
	}
	if l["type"] != "detention" || l["title"] != "Wiskunde" || l["locations"].([]any)[0] != "112" {
		t.Errorf("detention lesson: %v", l)
	}

	if code, _ := c.do("DELETE", base+"/"+id, "", nil); code != http.StatusNoContent {
		t.Fatalf("delete: %d", code)
	}
	if code, _ := c.do("DELETE", base+"/"+id, "", nil); code != http.StatusNotFound {
		t.Errorf("second delete: %d", code)
	}
	if find("/api/v1/accounts/"+student+"/schedule?from=today") != nil {
		t.Error("deleted detention still in the schedule")
	}
}
