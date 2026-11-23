package server

import (
	"fmt"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"
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

func TestWeekPlan(t *testing.T) {
	c := setup(t)
	_, body := c.do("POST", "/api/app/demo", "", map[string]any{})
	student, teacher := ids(t, body)
	_, body = c.do("GET", "/api/v1/accounts/"+student+"/schedule", "", nil)
	lessons := body["data"].(map[string]any)["lessons"].([]any)
	var code, date string
	for _, l := range lessons {
		m := l.(map[string]any)
		if subs := m["subjects"].([]any); len(subs) > 0 {
			code, date = subs[0].(map[string]any)["code"].(string), m["date"].(string)
			break
		}
	}
	d, _ := time.Parse(time.DateOnly, date)
	y, w := d.ISOWeek()
	week := fmt.Sprintf("%04d-W%02d", y, w)
	link := "https://jfc.itslearning.com/plans/courses/32813/plan/413396"
	path := "/api/v1/accounts/" + student + "/plans/" + code + "/" + week

	for _, bad := range []string{"javascript:alert(1)", "itslearning.com/plan", "ftp://x/y"} {
		if status, _ := c.do("PUT", path, "", map[string]any{"url": bad}); status != http.StatusBadRequest {
			t.Fatalf("%q accepted: %d", bad, status)
		}
	}
	if status, body := c.do("PUT", path, "", map[string]any{"url": link}); status != http.StatusOK {
		t.Fatalf("set: %d %v", status, body)
	}
	plans := func(account string) (with, without int) {
		_, body := c.do("GET", "/api/v1/accounts/"+account+"/schedule?week="+week, "", nil)
		for _, l := range body["data"].(map[string]any)["lessons"].([]any) {
			m := l.(map[string]any)
			has := false
			for _, s := range m["subjects"].([]any) {
				has = has || s.(map[string]any)["code"] == code
			}
			if has && m["weekPlan"] == link {
				with++
			} else if m["weekPlan"] != "" && account == student {
				t.Fatalf("plan on the wrong lesson: %v", m)
			} else if has {
				without++
			}
		}
		return
	}
	if with, without := plans(student); with == 0 || without != 0 {
		t.Fatalf("plan on %d of %d lessons of %s", with, with+without, code)
	}
	if status, _ := c.do("DELETE", path, "", nil); status != http.StatusNoContent {
		t.Fatalf("delete: %d", status)
	}
	if with, _ := plans(student); with != 0 {
		t.Fatal("plan still shown after delete")
	}
	if status, _ := c.do("PUT", "/api/v1/accounts/"+teacher+"/plans/"+code+"/2026-W99", "", map[string]any{"url": link}); status != http.StatusBadRequest {
		t.Fatalf("bad week accepted: %d", status)
	}
}

func TestHomeworkMCP(t *testing.T) {
	c := setup(t)
	_, body := c.do("POST", "/api/app/demo", "", map[string]any{})
	student, _ := ids(t, body)
	_, body = c.do("POST", "/api/app/keys", "", map[string]any{"name": "k", "accountIds": []string{student}})
	key := body["data"].(map[string]any)["key"].(string)
	call := func(name string, args map[string]any) map[string]any {
		t.Helper()
		_, body := c.do("POST", "/mcp", key, map[string]any{"jsonrpc": "2.0", "id": 1, "method": "tools/call", "params": map[string]any{"name": name, "arguments": args}})
		res := body["result"].(map[string]any)
		if res["isError"] == true {
			t.Fatalf("%s: %v", name, res)
		}
		return res["structuredContent"].(map[string]any)
	}

	sched := call("get_schedule", map[string]any{"from": "today", "to": time.Now().AddDate(0, 0, 13).Format(time.DateOnly)})
	var lesson map[string]any
	for _, l := range sched["schedule"].(map[string]any)["lessons"].([]any) {
		if m := l.(map[string]any); m["status"] != "cancelled" && len(m["subjects"].([]any)) > 0 {
			lesson = m
			break
		}
	}
	if lesson == nil {
		t.Skip("no lesson in the next two weeks")
	}
	code := lesson["subjects"].([]any)[0].(map[string]any)["code"].(string)
	d, _ := time.Parse(time.DateOnly, lesson["date"].(string))
	y, w := d.ISOWeek()
	week := fmt.Sprintf("%04d-W%02d", y, w)

	hw := call("add_homework", map[string]any{"lesson": lesson["id"], "date": lesson["date"], "text": "Lees **H3**"})
	id := hw["id"].(string)
	if got := call("update_homework", map[string]any{"id": id, "done": true}); got["done"] != true || got["text"] != "Lees **H3**" {
		t.Fatalf("update_homework: %v", got)
	}
	call("set_week_plan", map[string]any{"code": code, "week": week, "url": "https://jfc.itslearning.com/plans/courses/1/plan/2"})

	sched = call("get_schedule", map[string]any{"from": lesson["date"], "to": lesson["date"]})
	for _, l := range sched["schedule"].(map[string]any)["lessons"].([]any) {
		if m := l.(map[string]any); m["id"] == lesson["id"] {
			if len(m["homework"].([]any)) != 1 || m["weekPlan"] != "https://jfc.itslearning.com/plans/courses/1/plan/2" {
				t.Fatalf("get_schedule lesson: %v", m)
			}
		}
	}
	if list := call("list_homework", map[string]any{})["homework"].([]any); len(list) != 1 {
		t.Fatalf("list_homework: %v", list)
	}
	call("delete_homework", map[string]any{"id": id})
	if list := call("list_homework", map[string]any{})["homework"].([]any); len(list) != 0 {
		t.Fatalf("still listed: %v", list)
	}
}

func TestCalendarFeedHomework(t *testing.T) {
	c := setup(t)
	_, body := c.do("POST", "/api/app/demo", "", map[string]any{})
	student, _ := ids(t, body)
	_, body = c.do("POST", "/api/app/accounts/"+student+"/calendar", "", map[string]any{})
	feedURL := body["data"].(map[string]any)["url"].(string)
	feed := func() string {
		t.Helper()
		res, err := http.Get(feedURL)
		if err != nil {
			t.Fatal(err)
		}
		defer res.Body.Close()
		b, _ := io.ReadAll(res.Body)
		return strings.ReplaceAll(string(b), "\r\n ", "") // unfold
	}
	event := func(ics, id string) string {
		t.Helper()
		for _, ev := range strings.Split(ics, "BEGIN:VEVENT") {
			if strings.Contains(ev, "-"+id+"@melon") {
				return ev
			}
		}
		t.Fatalf("no event for lesson %s", id)
		return ""
	}
	_, body = c.do("GET", "/api/v1/accounts/"+student+"/schedule?from=today&to="+time.Now().AddDate(0, 0, 13).Format(time.DateOnly), "", nil)
	var lesson map[string]any
	for _, l := range body["data"].(map[string]any)["lessons"].([]any) {
		if m := l.(map[string]any); m["status"] != "cancelled" && len(m["subjects"].([]any)) > 0 {
			lesson = m
			break
		}
	}
	if lesson == nil {
		t.Skip("no lesson in the next two weeks")
	}
	id := lesson["id"].(string)
	before := event(feed(), id)

	link := c.base + "/?account=" + student + "&day=" + lesson["date"].(string) + "&lesson=" + lesson["instance"].(string)
	if !strings.Contains(before, "URL:"+link) || !strings.Contains(before, `Openen in Melon: `+link) {
		t.Fatalf("no link to the lesson in:\n%s", before)
	}

	_, body = c.do("POST", "/api/v1/accounts/"+student+"/homework", "", map[string]any{"lesson": id, "date": lesson["date"], "text": "- [x] Lees **H3**\n- [ ] Maak [opgave 12](https://example.com/12)"})
	hw := body["data"].(map[string]any)["id"].(string)
	after := event(feed(), id)
	if !strings.Contains(after, `Huiswerk:\n☑ Lees H3\n☐ Maak opgave 12 (https://example.com/12)`) {
		t.Fatalf("homework not as plain text:\n%s", after)
	}
	lastMod := func(ev string) string {
		for _, l := range strings.Split(ev, "\r\n") {
			if strings.HasPrefix(l, "LAST-MODIFIED:") {
				return l
			}
		}
		return ""
	}
	if lastMod(after) <= lastMod(before) {
		t.Fatalf("adding homework did not bump LAST-MODIFIED: %s -> %s", lastMod(before), lastMod(after))
	}
	c.do("PATCH", "/api/v1/accounts/"+student+"/homework/"+hw, "", map[string]any{"done": true})
	if ev := event(feed(), id); !strings.Contains(ev, `✓ Huiswerk (af):`) {
		t.Fatalf("done homework not marked:\n%s", ev)
	}
}
