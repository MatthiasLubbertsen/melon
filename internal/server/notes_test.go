package server

import (
	"net/http"
	"testing"
)

func TestNotes(t *testing.T) {
	c := setup(t)
	status, body := c.do("POST", "/api/app/demo", "", map[string]any{})
	if status != http.StatusOK && status != http.StatusCreated {
		t.Fatalf("demo: %d %v", status, body)
	}
	student, teacher := ids(t, body)
	base := "/api/v1/accounts/" + student + "/notes"

	status, body = c.do("POST", base, "", map[string]any{"subject": " WIS ", "title": "Formules", "text": "  - **abc**-formule  "})
	if status != http.StatusCreated {
		t.Fatalf("add: %d %v", status, body)
	}
	note := body["data"].(map[string]any)
	if note["subject"] != "wis" || note["text"] != "- **abc**-formule" {
		t.Fatalf("add returned %v", note)
	}
	id := note["id"].(string)
	if status, _ := c.do("POST", base, "", map[string]any{"title": "Algemeen", "pinned": true}); status != http.StatusCreated {
		t.Fatalf("general note: %d", status)
	}
	if status, _ := c.do("POST", base, "", map[string]any{"title": "  "}); status != http.StatusBadRequest {
		t.Fatalf("empty note: %d", status)
	}

	list := func(q string) []any {
		t.Helper()
		status, body := c.do("GET", base+q, "", nil)
		if status != http.StatusOK {
			t.Fatalf("list: %d %v", status, body)
		}
		return body["data"].([]any)
	}
	if all := list(""); len(all) != 2 || all[0].(map[string]any)["title"] != "Algemeen" {
		t.Fatalf("pinned note not first: %v", all)
	}
	if l := list("?subject="); len(l) != 1 || l[0].(map[string]any)["subject"] != "" {
		t.Fatalf("general notes: %v", l)
	}
	if l := list("?subject=wis"); len(l) != 1 {
		t.Fatalf("subject notes: %v", l)
	}

	if status, _ := c.do("PATCH", "/api/v1/accounts/"+teacher+"/notes/"+id, "", map[string]any{"title": "x"}); status != http.StatusNotFound {
		t.Fatalf("other account could edit: %d", status)
	}
	status, body = c.do("PATCH", base+"/"+id, "", map[string]any{"subject": ""})
	if status != http.StatusOK || body["data"].(map[string]any)["subject"] != "" || body["data"].(map[string]any)["title"] != "Formules" {
		t.Fatalf("update: %d %v", status, body)
	}
	if status, _ := c.do("DELETE", base+"/"+id, "", nil); status != http.StatusNoContent {
		t.Fatalf("delete: %d", status)
	}
	if status, _ := c.do("DELETE", base+"/"+id, "", nil); status != http.StatusNotFound {
		t.Fatalf("delete twice: %d", status)
	}
}

func TestBoards(t *testing.T) {
	c := setup(t)
	_, body := c.do("POST", "/api/app/demo", "", map[string]any{})
	student, teacher := ids(t, body)
	acc := "/api/v1/accounts/" + student

	add := func(board string, card map[string]any) string {
		t.Helper()
		status, body := c.do("POST", acc+"/boards/"+board+"/cards", "", card)
		if status != http.StatusCreated {
			t.Fatalf("add card: %d %v", status, body)
		}
		return body["data"].(map[string]any)["id"].(string)
	}
	a := add("kanban", map[string]any{"title": "A"})
	b := add("kanban", map[string]any{"title": "B"})
	add("kanban", map[string]any{"title": "C", "index": 0})
	add("hackban", map[string]any{"title": "Melon bot", "lane": "hacking"})
	if status, _ := c.do("POST", acc+"/boards/kanban/cards", "", map[string]any{"title": "X", "lane": "shipped"}); status != http.StatusBadRequest {
		t.Fatalf("wrong lane: %d", status)
	}
	if status, _ := c.do("GET", acc+"/boards/trello/cards", "", nil); status != http.StatusBadRequest {
		t.Fatalf("unknown board: %d", status)
	}

	order := func(board string) []string {
		t.Helper()
		status, body := c.do("GET", acc+"/boards/"+board+"/cards", "", nil)
		if status != http.StatusOK {
			t.Fatalf("cards: %d %v", status, body)
		}
		var out []string
		for _, x := range body["data"].([]any) {
			m := x.(map[string]any)
			out = append(out, m["lane"].(string)+":"+m["title"].(string))
		}
		return out
	}
	eq := func(got []string, want ...string) {
		t.Helper()
		if len(got) != len(want) {
			t.Fatalf("got %v, want %v", got, want)
		}
		for i := range got {
			if got[i] != want[i] {
				t.Fatalf("got %v, want %v", got, want)
			}
		}
	}
	eq(order("kanban"), "todo:C", "todo:A", "todo:B")
	eq(order("hackban"), "hacking:Melon bot")

	// Drag A to done, then B in front of it.
	if status, body := c.do("PATCH", acc+"/cards/"+a, "", map[string]any{"lane": "done"}); status != http.StatusOK {
		t.Fatalf("move: %d %v", status, body)
	}
	if status, _ := c.do("PATCH", acc+"/cards/"+b, "", map[string]any{"lane": "done", "index": 0}); status != http.StatusOK {
		t.Fatalf("move: %d", status)
	}
	eq(order("kanban"), "todo:C", "done:B", "done:A")
	if status, _ := c.do("PATCH", acc+"/cards/"+a, "", map[string]any{"index": 0, "text": "- [x] klaar"}); status != http.StatusOK {
		t.Fatalf("reorder: %d", status)
	}
	eq(order("kanban"), "todo:C", "done:A", "done:B")

	if status, _ := c.do("DELETE", "/api/v1/accounts/"+teacher+"/cards/"+a, "", nil); status != http.StatusNotFound {
		t.Fatalf("other account could delete: %d", status)
	}
	if status, _ := c.do("DELETE", acc+"/cards/"+a, "", nil); status != http.StatusNoContent {
		t.Fatalf("delete: %d", status)
	}
	eq(order("kanban"), "todo:C", "done:B")
}
