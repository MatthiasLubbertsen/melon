package zermelo

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
)

// A live schedule as Zermelo sends it when a student is expected at two lessons
// at once: one placeholder of type "conflict" that holds the real lessons, next
// to an entry that only lists an alternative and a plain lesson.
const liveScheduleWithConflict = `{"response":{"status":200,"data":[{"week":"202647","appointments":[
	{"id":null,"appointmentType":"conflict","startTimeSlot":6,"endTimeSlot":6,"start":1795090500,"end":1795093500,"subjects":[],"actions":[
		{"appointment":{"id":30394053,"appointmentType":"lesson","startTimeSlot":6,"endTimeSlot":6,"start":1795090500,"end":1795093500,"subjects":["na"]}},
		{"appointment":{"id":30393358,"appointmentType":"lesson","startTimeSlot":6,"endTimeSlot":6,"start":1795090500,"end":1795093500,"subjects":["wd"]}}
	]},
	{"id":30735638,"appointmentType":"activity","startTimeSlot":6,"endTimeSlot":6,"start":1794831300,"end":1794834300,"subjects":["LU"],"actions":[
		{"appointment":{"id":30365264,"appointmentType":"lesson","startTimeSlot":6,"endTimeSlot":6,"start":1794831300,"end":1794834300,"subjects":["sk"]}}
	]},
	{"id":30396678,"appointmentType":"lesson","startTimeSlot":7,"endTimeSlot":7,"start":1795093500,"end":1795096500,"subjects":["sp"],"actions":[]}
]}]}}`

func TestLiveScheduleExpandsConflicts(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v3/liveschedule" {
			http.NotFound(w, r)
			return
		}
		_, _ = w.Write([]byte(liveScheduleWithConflict))
	}))
	defer srv.Close()

	h := NewHTTP()
	h.BaseURL = func(string) string { return srv.URL }
	got, err := h.LiveSchedule(context.Background(), "jfc", "token", "student", "15315", "202647")
	if err != nil {
		t.Fatal(err)
	}

	// The placeholder is gone; its two lessons took its place. The alternative
	// is not a conflict, so that entry stays and its alternative is not added.
	want := []struct {
		id      int64
		subject string
	}{{30394053, "na"}, {30393358, "wd"}, {30735638, "LU"}, {30396678, "sp"}}
	if len(got) != len(want) {
		t.Fatalf("got %d appointments, want %d: %+v", len(got), len(want), got)
	}
	for i, w := range want {
		if got[i].ID != w.id || len(got[i].Subjects) != 1 || got[i].Subjects[0] != w.subject {
			t.Errorf("appointment %d = id %d subjects %v, want id %d subject %s", i, got[i].ID, got[i].Subjects, w.id, w.subject)
		}
		if got[i].Type == "conflict" {
			t.Errorf("appointment %d is still a conflict placeholder", i)
		}
	}
}

func TestChildren(t *testing.T) {
	answer := func(w http.ResponseWriter, status int, data string) {
		w.WriteHeader(status)
		_, _ = w.Write([]byte(`{"response":{"status":200,"data":` + data + `}}`))
	}
	for _, tc := range []struct {
		name     string
		students func(w http.ResponseWriter, r *http.Request)
		want     []string
		names    []string
	}{
		{
			name: "students lists the parent",
			students: func(w http.ResponseWriter, r *http.Request) {
				if r.URL.Query().Get("familyMembers") != "o.jansen" {
					t.Errorf("students asked for %q", r.URL.RawQuery)
				}
				answer(w, 200, `[{"userCode":"15315","firstName":"Lotte","lastName":"Jansen"},{"userCode":"15316","firstName":"Daan","prefix":"van","lastName":"Jansen"}]`)
			},
			want: []string{"15315", "15316"}, names: []string{"Lotte Jansen", "Daan van Jansen"},
		},
		{
			name:     "only the parent's own list is readable",
			students: func(w http.ResponseWriter, r *http.Request) { answer(w, 403, `[]`) },
			want:     []string{"15315"}, names: []string{"Lotte Jansen"},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				switch r.URL.Path {
				case "/api/v3/students":
					tc.students(w, r)
				case "/api/v3/familymembers":
					answer(w, 200, `[{"students":["15315"]}]`)
				case "/api/v3/users":
					answer(w, 200, `[{"code":"15315","firstName":"Lotte","lastName":"Jansen","isStudent":true}]`)
				default:
					http.NotFound(w, r)
				}
			}))
			defer srv.Close()
			h := NewHTTP()
			h.BaseURL = func(string) string { return srv.URL }
			got, err := h.Children(context.Background(), "jfc", "token", "o.jansen")
			if err != nil {
				t.Fatal(err)
			}
			if len(got) != len(tc.want) {
				t.Fatalf("got %+v", got)
			}
			for i := range got {
				if got[i].Code != tc.want[i] || got[i].Name() != tc.names[i] {
					t.Errorf("child %d = %s %q", i, got[i].Code, got[i].Name())
				}
			}
		})
	}
}
