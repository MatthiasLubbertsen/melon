package core

import (
	"strings"
	"testing"
	"time"

	"github.com/matthiaslubbertsen/melon/internal/secret"
	"github.com/matthiaslubbertsen/melon/internal/subjects"
	"github.com/matthiaslubbertsen/melon/internal/zermelo"
)

func TestBoxBindsContext(t *testing.T) {
	box, err := secret.NewBox([]byte("0123456789abcdef"))
	if err != nil {
		t.Fatal(err)
	}
	sealed := box.Seal("token", "acc_1")
	if strings.Contains(string(sealed), "token") {
		t.Fatal("plaintext visible in ciphertext")
	}
	if got, err := box.Open(sealed, "acc_1"); err != nil || got != "token" {
		t.Fatalf("roundtrip: %q %v", got, err)
	}
	if _, err := box.Open(sealed, "acc_2"); err == nil {
		t.Fatal("ciphertext opened under another row's context")
	}
}

func TestNormalizeSchool(t *testing.T) {
	for in, want := range map[string]string{"jfc": "jfc", " JFC ": "jfc", "https://jfc.zportal.nl/app": "jfc", "jfc.zportal.nl": "jfc"} {
		if got, ok := zermelo.NormalizeSchool(in); !ok || got != want {
			t.Errorf("%q: got %q %v", in, got, ok)
		}
	}
	for _, in := range []string{"", "evil.com", "jfc.zportal.nl.evil.com", "jfc@evil", "-x"} {
		if got, ok := zermelo.NormalizeSchool(in); ok {
			t.Errorf("%q accepted as %q", in, got)
		}
	}
}

func TestWeekStart(t *testing.T) {
	loc, _ := time.LoadLocation("Europe/Amsterdam")
	for week, want := range map[string]string{"202646": "2026-11-09", "202601": "2025-12-29", "202053": "2020-12-28"} {
		got, err := zermelo.WeekStart(week, loc)
		if err != nil || got.Format(time.DateOnly) != want {
			t.Errorf("%s: got %s %v", week, got.Format(time.DateOnly), err)
		}
	}
}

func TestSubjectResolve(t *testing.T) {
	if n, _ := subjects.Resolve("WI", "nl", nil); n != "Wiskunde" {
		t.Errorf("wi: %q", n)
	}
	if n, _ := subjects.Resolve("wi2", "en", nil); n != "Mathematics" {
		t.Errorf("wi2: %q", n)
	}
	if n, _ := subjects.Resolve("wi", "nl", map[string]string{"wi": "Wis B"}); n != "Wis B" {
		t.Errorf("override: %q", n)
	}
	if n, known := subjects.Resolve("xyz", "nl", nil); n != "xyz" || known {
		t.Errorf("unknown: %q %v", n, known)
	}
}

func TestChangeStatus(t *testing.T) {
	loc := time.UTC
	l := toLesson(zermelo.Appointment{ID: 1, Subjects: []string{"wi"}, TeacherChanged: true, TimeChanged: true}, loc, "nl", nil)
	if l.Status != "changed" || strings.Join(l.Changes, ",") != "teacher,time" {
		t.Errorf("changed: %+v", l)
	}
	l = toLesson(zermelo.Appointment{ID: 1, Cancelled: true, LocationChanged: true}, loc, "nl", nil)
	if l.Status != "cancelled" {
		t.Errorf("cancelled must win: %+v", l)
	}
}
