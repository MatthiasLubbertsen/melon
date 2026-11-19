package core

import (
	"context"
	"testing"
	"time"

	"github.com/matthiaslubbertsen/melon/internal/secret"
	"github.com/matthiaslubbertsen/melon/internal/store"
	"github.com/matthiaslubbertsen/melon/internal/zermelo"
)

// A parent linked before Melon knew about children has one account for the
// parent's own code. Loading the profile turns it into one per child.
func TestUpgradeOldParentAccount(t *testing.T) {
	db, err := store.Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	box, _ := secret.NewBox([]byte("test-secret-test-secret"))
	loc, _ := time.LoadLocation("Europe/Amsterdam")
	svc := &Service{Store: db, Up: zermelo.Demo{Loc: loc}, Cache: zermelo.NewCache(), Box: box, Loc: loc, Demo: true, Usage: NewUsage()}
	ctx := context.Background()

	user := secret.ID("usr")
	if err := db.CreateUser(ctx, user); err != nil {
		t.Fatal(err)
	}
	old := store.Account{ID: secret.ID("acc"), UserID: user, School: "demo", ZermeloUser: "o.devries", DisplayName: "Eva de Vries", Role: "parent"}
	oldID, err := db.UpsertAccount(ctx, old, func(id string) []byte { return box.Seal(zermelo.DemoParentToken, id) })
	if err != nil {
		t.Fatal(err)
	}

	// An API key scoped to the old account must not trigger the swap.
	accs, err := svc.Accounts(ctx, Principal{UserID: user, AccountIDs: []string{oldID}})
	if err != nil || len(accs) != 1 || accs[0].ID != oldID {
		t.Fatalf("scoped principal: %v %v", accs, err)
	}

	accs, err = svc.Accounts(ctx, Principal{UserID: user, All: true})
	if err != nil {
		t.Fatal(err)
	}
	if len(accs) != 2 {
		t.Fatalf("want 2 child accounts, got %v", accs)
	}
	for i, want := range []string{"100001", "100005"} {
		if accs[i].UserCode != want || accs[i].ParentCode != "o.devries" || accs[i].ID == oldID {
			t.Fatalf("child %d: %+v", i, accs[i])
		}
	}
	if _, err := svc.Schedule(ctx, Principal{UserID: user, All: true}, accs[0].ID, ScheduleQuery{From: time.Date(2026, 11, 9, 0, 0, 0, 0, loc), To: time.Date(2026, 11, 13, 0, 0, 0, 0, loc)}); err != nil {
		t.Fatalf("child schedule: %v", err)
	}
}
