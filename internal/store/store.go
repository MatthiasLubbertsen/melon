// Package store is Melon's persistence layer (SQLite).
package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"path/filepath"
	"strings"
	"time"

	_ "modernc.org/sqlite"
)

var ErrNotFound = errors.New("not found")

type Store struct{ db *sql.DB }

const schema = `
CREATE TABLE IF NOT EXISTS users (
  id TEXT PRIMARY KEY, created_at INTEGER NOT NULL
);
CREATE TABLE IF NOT EXISTS sessions (
  token_hash TEXT PRIMARY KEY,
  user_id TEXT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
  created_at INTEGER NOT NULL, last_seen INTEGER NOT NULL, expires_at INTEGER NOT NULL
);
CREATE TABLE IF NOT EXISTS logins (
  user_id TEXT PRIMARY KEY REFERENCES users(id) ON DELETE CASCADE,
  login TEXT NOT NULL, school TEXT NOT NULL,
  pin_hash TEXT NOT NULL,
  fails INTEGER NOT NULL DEFAULT 0, locked_until INTEGER NOT NULL DEFAULT 0,
  created_at INTEGER NOT NULL,
  UNIQUE (login, school)
);
CREATE TABLE IF NOT EXISTS accounts (
  id TEXT PRIMARY KEY,
  user_id TEXT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
  school TEXT NOT NULL, zermelo_user TEXT NOT NULL,
  display_name TEXT NOT NULL, role TEXT NOT NULL,
  wide INTEGER NOT NULL DEFAULT 0,
  token_enc BLOB NOT NULL, token_expires INTEGER NOT NULL DEFAULT 0,
  cal_hash TEXT, cal_enc BLOB,
  created_at INTEGER NOT NULL,
  UNIQUE (user_id, school, zermelo_user)
);
CREATE UNIQUE INDEX IF NOT EXISTS accounts_cal ON accounts(cal_hash) WHERE cal_hash IS NOT NULL;
CREATE TABLE IF NOT EXISTS subject_overrides (
  account_id TEXT NOT NULL REFERENCES accounts(id) ON DELETE CASCADE,
  code TEXT NOT NULL, name TEXT NOT NULL,
  PRIMARY KEY (account_id, code)
);
CREATE TABLE IF NOT EXISTS teacher_names (
  account_id TEXT NOT NULL REFERENCES accounts(id) ON DELETE CASCADE,
  code TEXT NOT NULL, name TEXT NOT NULL,
  PRIMARY KEY (account_id, code)
);
CREATE TABLE IF NOT EXISTS api_keys (
  id TEXT PRIMARY KEY,
  user_id TEXT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
  name TEXT NOT NULL, key_hash TEXT NOT NULL UNIQUE, hint TEXT NOT NULL,
  created_at INTEGER NOT NULL, last_used INTEGER NOT NULL DEFAULT 0
);
CREATE TABLE IF NOT EXISTS api_key_accounts (
  key_id TEXT NOT NULL REFERENCES api_keys(id) ON DELETE CASCADE,
  account_id TEXT NOT NULL REFERENCES accounts(id) ON DELETE CASCADE,
  PRIMARY KEY (key_id, account_id)
);
CREATE TABLE IF NOT EXISTS oauth_clients (
  id TEXT PRIMARY KEY, name TEXT NOT NULL, redirect_uris TEXT NOT NULL, created_at INTEGER NOT NULL
);
CREATE TABLE IF NOT EXISTS oauth_grants (
  id TEXT PRIMARY KEY,
  user_id TEXT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
  client_id TEXT NOT NULL REFERENCES oauth_clients(id) ON DELETE CASCADE,
  created_at INTEGER NOT NULL, last_used INTEGER NOT NULL DEFAULT 0
);
CREATE TABLE IF NOT EXISTS oauth_grant_accounts (
  grant_id TEXT NOT NULL REFERENCES oauth_grants(id) ON DELETE CASCADE,
  account_id TEXT NOT NULL REFERENCES accounts(id) ON DELETE CASCADE,
  PRIMARY KEY (grant_id, account_id)
);
CREATE TABLE IF NOT EXISTS oauth_codes (
  code_hash TEXT PRIMARY KEY,
  grant_id TEXT NOT NULL REFERENCES oauth_grants(id) ON DELETE CASCADE,
  redirect_uri TEXT NOT NULL, challenge TEXT NOT NULL, expires_at INTEGER NOT NULL
);
CREATE TABLE IF NOT EXISTS oauth_tokens (
  token_hash TEXT PRIMARY KEY,
  grant_id TEXT NOT NULL REFERENCES oauth_grants(id) ON DELETE CASCADE,
  kind TEXT NOT NULL, expires_at INTEGER NOT NULL
);
`

func Open(dataDir string) (*Store, error) {
	dsn := "file:" + filepath.Join(dataDir, "melon.db") + "?_pragma=foreign_keys(1)&_pragma=journal_mode(WAL)&_pragma=busy_timeout(5000)"
	db, err := sql.Open("sqlite", dsn)
	if err != nil {
		return nil, err
	}
	db.SetMaxOpenConns(1) // SQLite: one writer; keeps things simple and safe.
	if _, err := db.Exec(schema); err != nil {
		return nil, err
	}
	return &Store{db: db}, nil
}

func (s *Store) Close() error { return s.db.Close() }

func now() int64 { return time.Now().Unix() }

func one(err error) error {
	if errors.Is(err, sql.ErrNoRows) {
		return ErrNotFound
	}
	return err
}

func affected(res sql.Result, err error) error {
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrNotFound
	}
	return nil
}

// ---- users & sessions ----

func (s *Store) CreateUser(ctx context.Context, id string) error {
	_, err := s.db.ExecContext(ctx, `INSERT INTO users (id, created_at) VALUES (?, ?)`, id, now())
	return err
}

func (s *Store) CreateSession(ctx context.Context, tokenHash, userID string, ttl time.Duration) error {
	_, err := s.db.ExecContext(ctx, `INSERT INTO sessions VALUES (?, ?, ?, ?, ?)`,
		tokenHash, userID, now(), now(), time.Now().Add(ttl).Unix())
	return err
}

// SessionUser resolves a session and slides its expiry.
func (s *Store) SessionUser(ctx context.Context, tokenHash string, ttl time.Duration) (string, error) {
	var user string
	var lastSeen int64
	err := s.db.QueryRowContext(ctx, `SELECT user_id, last_seen FROM sessions WHERE token_hash = ? AND expires_at > ?`, tokenHash, now()).Scan(&user, &lastSeen)
	if err != nil {
		return "", one(err)
	}
	if now()-lastSeen > 3600 {
		_, _ = s.db.ExecContext(ctx, `UPDATE sessions SET last_seen = ?, expires_at = ? WHERE token_hash = ?`, now(), time.Now().Add(ttl).Unix(), tokenHash)
	}
	return user, nil
}

func (s *Store) DeleteSession(ctx context.Context, tokenHash string) error {
	_, err := s.db.ExecContext(ctx, `DELETE FROM sessions WHERE token_hash = ?`, tokenHash)
	return err
}

// DeleteUser removes a profile and, by cascade, everything it owns.
func (s *Store) DeleteUser(ctx context.Context, id string) error {
	_, err := s.db.ExecContext(ctx, `DELETE FROM users WHERE id = ?`, id)
	return err
}

// DeleteOtherSessions signs the profile out everywhere except the given session.
func (s *Store) DeleteOtherSessions(ctx context.Context, userID, keepHash string) error {
	_, err := s.db.ExecContext(ctx, `DELETE FROM sessions WHERE user_id = ? AND token_hash != ?`, userID, keepHash)
	return err
}

// ---- sign-in (user number + PIN) ----

// Login is how a profile signs in on another device: the user code of one of
// its Zermelo accounts, plus a PIN.
type Login struct {
	UserID  string
	Name    string // the Zermelo user code, e.g. 16528
	School  string
	PINHash string
}

const (
	// PINFreeTries is how many wrong PINs in a row are free. After that every
	// further try locks sign-in, for PINLockBase and then twice as long each
	// time, up to PINLockMax. That leaves about a dozen guesses on day one and
	// one per day after, against 10,000 possible PINs.
	PINFreeTries = 5
	PINLockBase  = 15 * 60
	PINLockMax   = 24 * 3600
)

// SetLogin gives the profile a sign-in name and PIN. A name belongs to
// whoever proved the Zermelo account last, so an earlier holder loses it.
func (s *Store) SetLogin(ctx context.Context, l Login) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if _, err := tx.ExecContext(ctx, `DELETE FROM logins WHERE user_id = ? OR (login = ? AND school = ?)`, l.UserID, l.Name, l.School); err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, `INSERT INTO logins (user_id, login, school, pin_hash, created_at) VALUES (?,?,?,?,?)`, l.UserID, l.Name, l.School, l.PINHash, now()); err != nil {
		return err
	}
	return tx.Commit()
}

func (s *Store) Login(ctx context.Context, userID string) (Login, error) {
	l := Login{UserID: userID}
	err := s.db.QueryRowContext(ctx, `SELECT login, school, pin_hash FROM logins WHERE user_id = ?`, userID).Scan(&l.Name, &l.School, &l.PINHash)
	return l, one(err)
}

// LoginsByName finds who signs in with this name. School narrows it down
// when given; without it the same number at two schools yields two rows.
func (s *Store) LoginsByName(ctx context.Context, name, school string) ([]Login, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT user_id, login, school, pin_hash FROM logins WHERE login = ? COLLATE NOCASE AND (? = '' OR school = ?) ORDER BY created_at LIMIT 8`, name, school, school)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Login
	for rows.Next() {
		var l Login
		if err := rows.Scan(&l.UserID, &l.Name, &l.School, &l.PINHash); err != nil {
			return nil, err
		}
		out = append(out, l)
	}
	return out, rows.Err()
}

// BeginPINAttempt spends one try before the PIN is looked at, so parallel
// guesses cannot outrun the counter. It reports false while sign-in is
// locked, and then also how many seconds remain.
func (s *Store) BeginPINAttempt(ctx context.Context, userID string) (ok bool, wait int64, err error) {
	n := now()
	var fails int64
	err = s.db.QueryRowContext(ctx, `UPDATE logins SET fails = fails + 1,
		locked_until = CASE WHEN fails + 1 >= ? THEN ? + MIN(?, ? * (1 << MIN(fails + 1 - ?, 20))) ELSE 0 END
		WHERE user_id = ? AND locked_until <= ? RETURNING fails`,
		PINFreeTries, n, PINLockMax, PINLockBase, PINFreeTries, userID, n).Scan(&fails)
	if err == nil {
		return true, 0, nil
	}
	if !errors.Is(err, sql.ErrNoRows) {
		return false, 0, err
	}
	var until int64
	if err := s.db.QueryRowContext(ctx, `SELECT locked_until FROM logins WHERE user_id = ?`, userID).Scan(&until); err != nil {
		return false, 0, one(err)
	}
	return false, max(until-n, 1), nil
}

// EndPINAttempt forgets earlier wrong tries after a correct PIN.
func (s *Store) EndPINAttempt(ctx context.Context, userID string) error {
	_, err := s.db.ExecContext(ctx, `UPDATE logins SET fails = 0, locked_until = 0 WHERE user_id = ?`, userID)
	return err
}

func (s *Store) DeleteLogin(ctx context.Context, userID string) error {
	_, err := s.db.ExecContext(ctx, `DELETE FROM logins WHERE user_id = ?`, userID)
	return err
}

// ---- accounts ----

type Account struct {
	ID           string
	UserID       string
	School       string
	ZermeloUser  string
	DisplayName  string
	Role         string // student | employee | parent | other
	Wide         bool   // token can see other people
	TokenEnc     []byte
	TokenExpires int64
	CalEnc       []byte
	CreatedAt    int64
}

const accountCols = `id, user_id, school, zermelo_user, display_name, role, wide, token_enc, token_expires, cal_enc, created_at`

func scanAccount(row interface{ Scan(...any) error }) (Account, error) {
	var a Account
	err := row.Scan(&a.ID, &a.UserID, &a.School, &a.ZermeloUser, &a.DisplayName, &a.Role, &a.Wide, &a.TokenEnc, &a.TokenExpires, &a.CalEnc, &a.CreatedAt)
	return a, one(err)
}

// UpsertAccount inserts the account or, when this profile already linked the
// same Zermelo user, replaces its token. It returns the effective account id.
func (s *Store) UpsertAccount(ctx context.Context, a Account, seal func(id string) []byte) (string, error) {
	var existing string
	err := s.db.QueryRowContext(ctx, `SELECT id FROM accounts WHERE user_id = ? AND school = ? AND zermelo_user = ?`, a.UserID, a.School, a.ZermeloUser).Scan(&existing)
	switch {
	case err == nil:
		_, err = s.db.ExecContext(ctx, `UPDATE accounts SET display_name = ?, role = ?, wide = ?, token_enc = ?, token_expires = ? WHERE id = ?`,
			a.DisplayName, a.Role, a.Wide, seal(existing), a.TokenExpires, existing)
		return existing, err
	case errors.Is(err, sql.ErrNoRows):
		_, err = s.db.ExecContext(ctx, `INSERT INTO accounts (id, user_id, school, zermelo_user, display_name, role, wide, token_enc, token_expires, created_at) VALUES (?,?,?,?,?,?,?,?,?,?)`,
			a.ID, a.UserID, a.School, a.ZermeloUser, a.DisplayName, a.Role, a.Wide, seal(a.ID), a.TokenExpires, now())
		return a.ID, err
	default:
		return "", err
	}
}

func (s *Store) Accounts(ctx context.Context, userID string) ([]Account, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT `+accountCols+` FROM accounts WHERE user_id = ? ORDER BY created_at, rowid`, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Account
	for rows.Next() {
		a, err := scanAccount(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, a)
	}
	return out, rows.Err()
}

func (s *Store) Account(ctx context.Context, userID, id string) (Account, error) {
	return scanAccount(s.db.QueryRowContext(ctx, `SELECT `+accountCols+` FROM accounts WHERE user_id = ? AND id = ?`, userID, id))
}

func (s *Store) AccountByCalendar(ctx context.Context, calHash string) (Account, error) {
	return scanAccount(s.db.QueryRowContext(ctx, `SELECT `+accountCols+` FROM accounts WHERE cal_hash = ?`, calHash))
}

func (s *Store) DeleteAccount(ctx context.Context, userID, id string) error {
	return affected(s.db.ExecContext(ctx, `DELETE FROM accounts WHERE user_id = ? AND id = ?`, userID, id))
}

// SetCalendar stores (or with nil clears) the calendar feed secret.
func (s *Store) SetCalendar(ctx context.Context, userID, id string, hash *string, enc []byte) error {
	return affected(s.db.ExecContext(ctx, `UPDATE accounts SET cal_hash = ?, cal_enc = ? WHERE user_id = ? AND id = ?`, hash, enc, userID, id))
}

// ---- subject overrides ----

func (s *Store) SubjectOverrides(ctx context.Context, accountID string) (map[string]string, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT code, name FROM subject_overrides WHERE account_id = ?`, accountID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := map[string]string{}
	for rows.Next() {
		var c, n string
		if err := rows.Scan(&c, &n); err != nil {
			return nil, err
		}
		out[c] = n
	}
	return out, rows.Err()
}

func (s *Store) SetSubjectOverride(ctx context.Context, accountID, code, name string) error {
	_, err := s.db.ExecContext(ctx, `INSERT INTO subject_overrides VALUES (?, ?, ?) ON CONFLICT DO UPDATE SET name = excluded.name`, accountID, code, name)
	return err
}

func (s *Store) DeleteSubjectOverride(ctx context.Context, accountID, code string) error {
	_, err := s.db.ExecContext(ctx, `DELETE FROM subject_overrides WHERE account_id = ? AND code = ?`, accountID, code)
	return err
}

// ---- teacher names ----

func (s *Store) TeacherNames(ctx context.Context, accountID string) (map[string]string, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT code, name FROM teacher_names WHERE account_id = ?`, accountID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := map[string]string{}
	for rows.Next() {
		var c, n string
		if err := rows.Scan(&c, &n); err != nil {
			return nil, err
		}
		out[c] = n
	}
	return out, rows.Err()
}

func (s *Store) SetTeacherName(ctx context.Context, accountID, code, name string) error {
	_, err := s.db.ExecContext(ctx, `INSERT INTO teacher_names VALUES (?, ?, ?) ON CONFLICT DO UPDATE SET name = excluded.name`, accountID, code, name)
	return err
}

func (s *Store) DeleteTeacherName(ctx context.Context, accountID, code string) error {
	_, err := s.db.ExecContext(ctx, `DELETE FROM teacher_names WHERE account_id = ? AND code = ?`, accountID, code)
	return err
}

// ---- API keys ----

type APIKey struct {
	ID         string
	UserID     string
	Name       string
	Hint       string
	CreatedAt  int64
	LastUsed   int64
	AccountIDs []string
}

func (s *Store) CreateAPIKey(ctx context.Context, k APIKey, keyHash string) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if _, err := tx.ExecContext(ctx, `INSERT INTO api_keys (id, user_id, name, key_hash, hint, created_at) VALUES (?,?,?,?,?,?)`, k.ID, k.UserID, k.Name, keyHash, k.Hint, now()); err != nil {
		return err
	}
	for _, a := range k.AccountIDs {
		// The subquery guarantees a key can only be scoped to its owner's accounts.
		res, err := tx.ExecContext(ctx, `INSERT INTO api_key_accounts SELECT ?, id FROM accounts WHERE id = ? AND user_id = ?`, k.ID, a, k.UserID)
		if err := affected(res, err); err != nil {
			return err
		}
	}
	return tx.Commit()
}

func (s *Store) APIKeys(ctx context.Context, userID string) ([]APIKey, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT k.id, k.user_id, k.name, k.hint, k.created_at, k.last_used, COALESCE(GROUP_CONCAT(a.account_id), '')
		FROM api_keys k LEFT JOIN api_key_accounts a ON a.key_id = k.id WHERE k.user_id = ? GROUP BY k.id ORDER BY k.created_at DESC`, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []APIKey{}
	for rows.Next() {
		var k APIKey
		var ids string
		if err := rows.Scan(&k.ID, &k.UserID, &k.Name, &k.Hint, &k.CreatedAt, &k.LastUsed, &ids); err != nil {
			return nil, err
		}
		k.AccountIDs = split(ids)
		out = append(out, k)
	}
	return out, rows.Err()
}

// APIKeyByHash resolves a presented key to its owner and account scope.
func (s *Store) APIKeyByHash(ctx context.Context, keyHash string) (APIKey, error) {
	var k APIKey
	var ids string
	err := s.db.QueryRowContext(ctx, `SELECT k.id, k.user_id, k.name, k.last_used, COALESCE(GROUP_CONCAT(a.account_id), '')
		FROM api_keys k LEFT JOIN api_key_accounts a ON a.key_id = k.id WHERE k.key_hash = ? GROUP BY k.id`, keyHash).Scan(&k.ID, &k.UserID, &k.Name, &k.LastUsed, &ids)
	if err != nil {
		return k, one(err)
	}
	k.AccountIDs = split(ids)
	if now()-k.LastUsed > 60 {
		_, _ = s.db.ExecContext(ctx, `UPDATE api_keys SET last_used = ? WHERE id = ?`, now(), k.ID)
	}
	return k, nil
}

func (s *Store) DeleteAPIKey(ctx context.Context, userID, id string) error {
	return affected(s.db.ExecContext(ctx, `DELETE FROM api_keys WHERE user_id = ? AND id = ?`, userID, id))
}

// ---- OAuth ----

type OAuthClient struct {
	ID           string
	Name         string
	RedirectURIs []string
}

func (s *Store) CreateOAuthClient(ctx context.Context, c OAuthClient) error {
	uris, _ := json.Marshal(c.RedirectURIs)
	_, err := s.db.ExecContext(ctx, `INSERT INTO oauth_clients VALUES (?, ?, ?, ?)`, c.ID, c.Name, string(uris), now())
	return err
}

func (s *Store) OAuthClient(ctx context.Context, id string) (OAuthClient, error) {
	c := OAuthClient{ID: id}
	var uris string
	if err := s.db.QueryRowContext(ctx, `SELECT name, redirect_uris FROM oauth_clients WHERE id = ?`, id).Scan(&c.Name, &uris); err != nil {
		return c, one(err)
	}
	return c, json.Unmarshal([]byte(uris), &c.RedirectURIs)
}

type OAuthGrant struct {
	ID         string
	UserID     string
	ClientID   string
	ClientName string
	CreatedAt  int64
	LastUsed   int64
	AccountIDs []string
}

// CreateOAuthGrant records the user's consent and the one-time code for it.
func (s *Store) CreateOAuthGrant(ctx context.Context, g OAuthGrant, codeHash, redirectURI, challenge string, codeTTL time.Duration) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if _, err := tx.ExecContext(ctx, `INSERT INTO oauth_grants (id, user_id, client_id, created_at) VALUES (?,?,?,?)`, g.ID, g.UserID, g.ClientID, now()); err != nil {
		return err
	}
	for _, a := range g.AccountIDs {
		res, err := tx.ExecContext(ctx, `INSERT INTO oauth_grant_accounts SELECT ?, id FROM accounts WHERE id = ? AND user_id = ?`, g.ID, a, g.UserID)
		if err := affected(res, err); err != nil {
			return err
		}
	}
	if _, err := tx.ExecContext(ctx, `INSERT INTO oauth_codes VALUES (?,?,?,?,?)`, codeHash, g.ID, redirectURI, challenge, time.Now().Add(codeTTL).Unix()); err != nil {
		return err
	}
	return tx.Commit()
}

type OAuthCode struct {
	GrantID, ClientID, RedirectURI, Challenge string
}

// ConsumeOAuthCode atomically redeems a code; codes are single use.
func (s *Store) ConsumeOAuthCode(ctx context.Context, codeHash string) (OAuthCode, error) {
	var c OAuthCode
	err := s.db.QueryRowContext(ctx, `DELETE FROM oauth_codes WHERE code_hash = ? AND expires_at > ? RETURNING grant_id, redirect_uri, challenge`, codeHash, now()).Scan(&c.GrantID, &c.RedirectURI, &c.Challenge)
	if err != nil {
		return c, one(err)
	}
	err = s.db.QueryRowContext(ctx, `SELECT client_id FROM oauth_grants WHERE id = ?`, c.GrantID).Scan(&c.ClientID)
	return c, one(err)
}

func (s *Store) CreateOAuthToken(ctx context.Context, tokenHash, grantID, kind string, ttl time.Duration) error {
	_, err := s.db.ExecContext(ctx, `INSERT INTO oauth_tokens VALUES (?,?,?,?)`, tokenHash, grantID, kind, time.Now().Add(ttl).Unix())
	return err
}

// OAuthGrantByToken resolves a live token of the given kind to its grant.
func (s *Store) OAuthGrantByToken(ctx context.Context, tokenHash, kind string) (OAuthGrant, error) {
	var g OAuthGrant
	var ids string
	err := s.db.QueryRowContext(ctx, `SELECT g.id, g.user_id, g.client_id, g.last_used, COALESCE(GROUP_CONCAT(a.account_id), '')
		FROM oauth_tokens t JOIN oauth_grants g ON g.id = t.grant_id LEFT JOIN oauth_grant_accounts a ON a.grant_id = g.id
		WHERE t.token_hash = ? AND t.kind = ? AND t.expires_at > ? GROUP BY g.id`, tokenHash, kind, now()).Scan(&g.ID, &g.UserID, &g.ClientID, &g.LastUsed, &ids)
	if err != nil {
		return g, one(err)
	}
	g.AccountIDs = split(ids)
	if now()-g.LastUsed > 60 {
		_, _ = s.db.ExecContext(ctx, `UPDATE oauth_grants SET last_used = ? WHERE id = ?`, now(), g.ID)
	}
	return g, nil
}

func (s *Store) DeleteOAuthToken(ctx context.Context, tokenHash string) error {
	_, err := s.db.ExecContext(ctx, `DELETE FROM oauth_tokens WHERE token_hash = ?`, tokenHash)
	return err
}

func (s *Store) OAuthGrants(ctx context.Context, userID string) ([]OAuthGrant, error) {
	// Grants whose code was never redeemed have no tokens and are not shown.
	rows, err := s.db.QueryContext(ctx, `SELECT g.id, g.client_id, c.name, g.created_at, g.last_used, COALESCE(GROUP_CONCAT(a.account_id), '')
		FROM oauth_grants g JOIN oauth_clients c ON c.id = g.client_id LEFT JOIN oauth_grant_accounts a ON a.grant_id = g.id
		WHERE g.user_id = ? AND EXISTS (SELECT 1 FROM oauth_tokens t WHERE t.grant_id = g.id AND t.expires_at > ?)
		GROUP BY g.id ORDER BY g.created_at DESC`, userID, now())
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []OAuthGrant{}
	for rows.Next() {
		g := OAuthGrant{UserID: userID}
		var ids string
		if err := rows.Scan(&g.ID, &g.ClientID, &g.ClientName, &g.CreatedAt, &g.LastUsed, &ids); err != nil {
			return nil, err
		}
		g.AccountIDs = split(ids)
		out = append(out, g)
	}
	return out, rows.Err()
}

func (s *Store) DeleteOAuthGrant(ctx context.Context, userID, id string) error {
	return affected(s.db.ExecContext(ctx, `DELETE FROM oauth_grants WHERE user_id = ? AND id = ?`, userID, id))
}

// Sweep removes expired sessions, codes and tokens, and consents left with no way to use them.
func (s *Store) Sweep(ctx context.Context) {
	n := now()
	_, _ = s.db.ExecContext(ctx, `DELETE FROM sessions WHERE expires_at <= ?`, n)
	_, _ = s.db.ExecContext(ctx, `DELETE FROM oauth_codes WHERE expires_at <= ?`, n)
	_, _ = s.db.ExecContext(ctx, `DELETE FROM oauth_tokens WHERE expires_at <= ?`, n)
	_, _ = s.db.ExecContext(ctx, `DELETE FROM oauth_grants WHERE created_at < ? AND id NOT IN (SELECT grant_id FROM oauth_tokens) AND id NOT IN (SELECT grant_id FROM oauth_codes)`, n-600)
}

func split(s string) []string {
	if s == "" {
		return []string{}
	}
	return strings.Split(s, ",")
}

func (s *Store) OAuthGrantOwner(ctx context.Context, grantID string) (string, error) {
	var user string
	err := s.db.QueryRowContext(ctx, `SELECT user_id FROM oauth_grants WHERE id = ?`, grantID).Scan(&user)
	return user, one(err)
}
