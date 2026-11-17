package store

import (
	"context"
	"database/sql"
	"path/filepath"
	"testing"
	"time"
)

// A database made before admin support gains the new columns on open, and
// what was in it keeps working.
func TestMigratesOlderDatabase(t *testing.T) {
	dir := t.TempDir()
	old, err := sql.Open("sqlite", "file:"+filepath.Join(dir, "melon.db"))
	if err != nil {
		t.Fatal(err)
	}
	_, err = old.Exec(`
CREATE TABLE users (id TEXT PRIMARY KEY, created_at INTEGER NOT NULL);
CREATE TABLE sessions (
  token_hash TEXT PRIMARY KEY, user_id TEXT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
  created_at INTEGER NOT NULL, last_seen INTEGER NOT NULL, expires_at INTEGER NOT NULL
);
CREATE TABLE api_keys (
  id TEXT PRIMARY KEY, user_id TEXT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
  name TEXT NOT NULL, key_hash TEXT NOT NULL UNIQUE, hint TEXT NOT NULL,
  created_at INTEGER NOT NULL, last_used INTEGER NOT NULL DEFAULT 0
);
INSERT INTO users VALUES ('usr_a', 1);
INSERT INTO sessions VALUES ('h', 'usr_a', 1, 1, 9999999999);
INSERT INTO api_keys VALUES ('key_a', 'usr_a', 'old', 'kh', 'hint', 1, 0);`)
	old.Close()
	if err != nil {
		t.Fatal(err)
	}
	s, err := Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	ctx := context.Background()
	if user, by, err := s.Session(ctx, "h", time.Hour); err != nil || user != "usr_a" || by != "" {
		t.Fatalf("old session: %q %q %v", user, by, err)
	}
	if k, err := s.APIKeyByHash(ctx, "kh"); err != nil || k.UserID != "usr_a" || k.Admin {
		t.Fatalf("old key: %+v %v", k, err)
	}
	// Opening again is a no-op.
	s.Close()
	if s, err = Open(dir); err != nil {
		t.Fatal(err)
	}
	if err := s.CreateImpersonation(ctx, "i", "usr_a", "usr_admin", time.Hour); err != nil {
		t.Fatal(err)
	}
	if _, by, _ := s.Session(ctx, "i", time.Hour); by != "usr_admin" {
		t.Fatalf("impersonator: %q", by)
	}
}
