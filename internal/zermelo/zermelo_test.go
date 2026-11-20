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

// A live schedule where an OO lesson dropped out, as jfc's Zermelo sent it for
// Tuesday 24 Nov 2026: an empty, cancelled slot (status 4001, "Inschrijven niet
// mogelijk") holding the cancelled lesson (4007 "uitgevallen", 4009 "door iemand
// anders bepaald"). Next to it, an empty slot that still offers a lesson to
// choose from.
const liveScheduleWithDroppedLesson = `{"response":{"status":200,"data":[{"week":"202648","appointments":[
	{"status":[{"code":4001,"nl":"Inschrijven niet mogelijk.","en":"Enrollment not possible."}],
	 "actions":[
		{"appointment":{"start":1795504500,"end":1795507500,"cancelled":true,"plannedAttendance":false,"studentEnrolled":false,"allowedActions":"none","optional":false,"attendanceOverruled":true,"appointmentType":"lesson","scheduleLastModified":1795161991,"online":false,"onlineLocationUrl":null,"id":30427195,"appointmentInstance":1945284,"startTimeSlot":1,"endTimeSlot":1,"subjects":["oo"],"groups":["ta3.oo2"],"locations":["165"],"teachers":["brp"],"timeChanged":false,"groupChanged":false,"locationChanged":false,"teacherChanged":false,"changeDescription":"Les vervalt","schedulerRemark":"","content":null},
		 "status":[{"code":4007,"nl":"De les/afspraak is uitgevallen."},{"code":4009,"nl":"Dit is door iemand anders bepaald."}],
		 "allowed":false}
	 ],
	 "start":1795504500,"end":1795507500,"cancelled":true,"appointmentType":"lesson","online":false,"optional":false,"appointmentInstance":null,"startTimeSlot":1,"endTimeSlot":1,
	 "subjects":[],"groups":[],"locations":[],"teachers":[],"onlineLocationUrl":null,"teacherChanged":null,"locationChanged":null,"groupChanged":null,"timeChanged":null,"changeDescription":null,"scheduleLastModified":null,"schedulerRemark":null,"content":null,"id":null},
	{"id":null,"appointmentType":"choice","startTimeSlot":2,"endTimeSlot":2,"start":1795507500,"end":1795510500,"subjects":[],"actions":[
		{"appointment":{"id":30428258,"appointmentType":"lesson","cancelled":false,"startTimeSlot":2,"endTimeSlot":2,"start":1795507500,"end":1795510500,"subjects":["oo"]}}
	]}
]}]}}`

func TestLiveScheduleShowsDroppedLessonInEmptySlot(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(liveScheduleWithDroppedLesson))
	}))
	defer srv.Close()

	h := NewHTTP()
	h.BaseURL = func(string) string { return srv.URL }
	got, err := h.LiveSchedule(context.Background(), "jfc", "token", "student", "15315", "202648")
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 2 {
		t.Fatalf("got %d appointments, want 2: %+v", len(got), got)
	}
	// The empty slot became the lesson that dropped out, still cancelled.
	if l := got[0]; l.ID != 30427195 || !l.Cancelled || len(l.Subjects) != 1 || l.Subjects[0] != "oo" || len(l.Locations) != 1 || l.Locations[0] != "165" {
		t.Errorf("dropped-out slot = %+v, want the cancelled oo lesson in 165", l)
	}
	// A slot that still offers a choice is not turned into that option.
	if l := got[1]; l.ID != 0 || l.Type != "choice" {
		t.Errorf("open choice slot = %+v, want it left as the choice placeholder", l)
	}
}

func TestChildren(t *testing.T) {
	answer := func(w http.ResponseWriter, status int, data string) {
		w.WriteHeader(status)
		_, _ = w.Write([]byte(`{"response":{"status":200,"data":` + data + `}}`))
	}
	refuse := func(w http.ResponseWriter, _ *http.Request) { answer(w, 403, `[]`) }
	for _, tc := range []struct {
		name     string
		byParent func(w http.ResponseWriter, r *http.Request) // users?familyMember=
		students func(w http.ResponseWriter, r *http.Request)
		want     []string
		names    []string
	}{
		{
			// What a real parent token at a school allows: everything else is refused.
			name: "users of this parent",
			byParent: func(w http.ResponseWriter, r *http.Request) {
				if r.URL.Query().Get("familyMember") != "o.jansen" {
					t.Errorf("users asked for %q", r.URL.RawQuery)
				}
				answer(w, 200, `[{"code":"15315","firstName":"Lotte","prefix":null,"lastName":"Jansen","isStudent":true}]`)
			},
			students: refuse,
			want:     []string{"15315"}, names: []string{"Lotte Jansen"},
		},
		{
			name:     "students lists the parent",
			byParent: refuse,
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
			byParent: refuse,
			students: refuse,
			want:     []string{"15315"}, names: []string{"Lotte Jansen"},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				switch {
				case r.URL.Path == "/api/v3/users" && r.URL.Query().Has("familyMember"):
					tc.byParent(w, r)
				case r.URL.Path == "/api/v3/users":
					answer(w, 200, `[{"code":"15315","firstName":"Lotte","lastName":"Jansen","isStudent":true}]`)
				case r.URL.Path == "/api/v3/students":
					tc.students(w, r)
				case r.URL.Path == "/api/v3/familymembers":
					answer(w, 200, `[{"students":["15315"]}]`)
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
