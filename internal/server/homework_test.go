package server

import (
	"net/http"
	"testing"
)

func TestHomework(t *testing.T) {
	c := setup(t)
	status, body := c.do("POST", "/api/app/demo", "", map[string]any{})
	if status != http.StatusOK && status != http.StatusCreated {
		t.Fatalf("demo: %d %v", status, body)
	}
	student, teacher := ids(t, body)

	lessons := func(account string) []map[string]any {
		t.Helper()
		status, body := c.do("GET", "/api/v1/accounts/"+account+"/schedule", "", nil)
		if status != http.StatusOK {
			t.Fatalf("schedule: %d %v", status, body)
		}
		var out []map[string]any
		for _, l := range body["data"].(map[string]any)["lessons"].([]any) {
			out = append(out, l.(map[string]any))
		}
		return out
	}
	var cancelled map[string]any
	for _, l := range lessons(student) {
		if l["status"] == "cancelled" && len(l["subjects"].([]any)) > 0 {
			cancelled = l
			break
		}
	}
	if cancelled == nil {
		t.Fatal("the demo week has no cancelled lesson")
	}

	status, body = c.do("POST", "/api/v1/accounts/"+student+"/homework", "", map[string]any{"lesson": cancelled["id"], "date": cancelled["date"], "text": "  Lees **hoofdstuk 3**  "})
	if status != http.StatusCreated {
		t.Fatalf("add: %d %v", status, body)
	}
	hw := body["data"].(map[string]any)
	id := hw["id"].(string)
	if hw["text"] != "Lees **hoofdstuk 3**" || hw["lesson"].(map[string]any)["status"] != "cancelled" {
		t.Fatalf("add returned %v", hw)
	}

	// It shows on its lesson, and only on the account it belongs to.
	found := false
	for _, l := range lessons(student) {
		if n := len(l["homework"].([]any)); n > 0 {
			if l["instance"] != cancelled["instance"] || n != 1 {
				t.Fatalf("homework on the wrong lesson: %v", l)
			}
			found = true
		}
	}
	if !found {
		t.Fatal("homework missing from the schedule")
	}
	for _, l := range lessons(teacher) {
		if len(l["homework"].([]any)) > 0 {
			t.Fatal("homework leaked to another account")
		}
	}
	if status, _ := c.do("PATCH", "/api/v1/accounts/"+teacher+"/homework/"+id, "", map[string]any{"done": true}); status != http.StatusNotFound {
		t.Fatalf("other account could edit: %d", status)
	}

	// The lesson dropped out, so move it to the next lesson of the subject.
	status, body = c.do("PATCH", "/api/v1/accounts/"+student+"/homework/"+id, "", map[string]any{"moveTo": "next"})
	if status != http.StatusOK {
		t.Fatalf("move: %d %v", status, body)
	}
	moved := body["data"].(map[string]any)["lesson"].(map[string]any)
	code := cancelled["subjects"].([]any)[0].(map[string]any)["code"]
	if moved["status"] == "cancelled" || moved["instance"] == cancelled["instance"] || moved["subjects"].([]any)[0] != code {
		t.Fatalf("moved to %v", moved)
	}

	status, body = c.do("PATCH", "/api/v1/accounts/"+student+"/homework/"+id, "", map[string]any{"text": "", "done": true})
	if status != http.StatusBadRequest {
		t.Fatalf("empty text: %d %v", status, body)
	}
	status, body = c.do("PATCH", "/api/v1/accounts/"+student+"/homework/"+id, "", map[string]any{"done": true})
	if status != http.StatusOK || body["data"].(map[string]any)["done"] != true {
		t.Fatalf("done: %d %v", status, body)
	}

	status, body = c.do("GET", "/api/v1/accounts/"+student+"/homework", "", nil)
	if status != http.StatusOK || len(body["data"].([]any)) != 1 {
		t.Fatalf("list: %d %v", status, body)
	}
	if status, _ := c.do("DELETE", "/api/v1/accounts/"+student+"/homework/"+id, "", nil); status != http.StatusNoContent {
		t.Fatalf("delete: %d", status)
	}
	if _, body := c.do("GET", "/api/v1/accounts/"+student+"/homework", "", nil); len(body["data"].([]any)) != 0 {
		t.Fatalf("still listed after delete: %v", body)
	}
}
