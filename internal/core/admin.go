package core

import (
	"context"
	"errors"
	"net/http"
	"strings"
	"time"

	"github.com/matthiaslubbertsen/melon/internal/store"
	"github.com/matthiaslubbertsen/melon/internal/zermelo"
)

// Admin helpers: they reach any account, whoever owns it. The server only
// calls them after its admin check.

// AnyAccount loads an account by id with its decrypted Zermelo token.
func (s *Service) AnyAccount(ctx context.Context, id string) (store.Account, string, error) {
	a, err := s.Store.AccountByID(ctx, id)
	if errors.Is(err, store.ErrNotFound) {
		return a, "", ErrNotFound
	} else if err != nil {
		return a, "", err
	}
	token, err := s.Box.Open(a.TokenEnc, a.ID)
	if err != nil {
		return a, "", Errf(http.StatusConflict, "token_unreadable", "This account's Zermelo token cannot be decrypted; was MELON_SECRET changed?")
	}
	return a, token, nil
}

// AsOwner is a principal that may use exactly this account, as its owner
// would, marked as an admin's.
func AsOwner(a store.Account) Principal {
	return Principal{UserID: a.UserID, Kind: "admin", AccountIDs: []string{a.ID}}
}

// AccountCheck is what Zermelo says about an account's token right now.
type AccountCheck struct {
	Account      Account    `json:"account"`
	ProfileID    string     `json:"profileId"`
	TokenWorks   bool       `json:"tokenWorks"`
	Problem      string     `json:"problem,omitempty"`
	TokenExpires *time.Time `json:"tokenExpires"`
	// What Zermelo says now; it can differ from what Melon stored when linking.
	ZermeloUser string `json:"zermeloUser,omitempty"`
	ZermeloName string `json:"zermeloName,omitempty"`
	// SameUser is false when the token now belongs to someone other than
	// the user code Melon stored for the account.
	SameUser     bool      `json:"sameUser"`
	CanListUsers bool      `json:"canListUsers"`
	UsersVisible int       `json:"usersVisible"`
	StoredAsWide bool      `json:"storedAsWide"`
	CheckedAt    time.Time `json:"checkedAt"`
}

// CheckAccount asks Zermelo, bypassing the cache, whether the account's
// token still works and what it can see.
func (s *Service) CheckAccount(ctx context.Context, id string) (AccountCheck, error) {
	a, token, err := s.AnyAccount(ctx, id)
	if err != nil {
		var ce *Error
		if errors.As(err, &ce) && ce.Code == "token_unreadable" {
			return AccountCheck{Account: toAccount(a), ProfileID: a.UserID, Problem: ce.Message, StoredAsWide: a.Wide, CheckedAt: time.Now()}, nil
		}
		return AccountCheck{}, err
	}
	out := AccountCheck{Account: toAccount(a), ProfileID: a.UserID, StoredAsWide: a.Wide, CheckedAt: time.Now()}
	if a.TokenExpires > 0 {
		t := time.Unix(a.TokenExpires, 0).In(s.Loc)
		out.TokenExpires = &t
	}
	me, err := s.Up.Me(ctx, a.School, token)
	if err != nil {
		if errors.Is(err, zermelo.ErrUnauthorized) {
			out.Problem = "Zermelo no longer accepts this token; the user has to link the account again."
		} else {
			out.Problem = s.upstreamErr(err).Error()
		}
		return out, nil
	}
	out.TokenWorks = true
	out.ZermeloUser, out.ZermeloName = me.Code, me.Name()
	out.SameUser = strings.EqualFold(me.Code, a.ZermeloUser)
	if users, err := s.Up.Users(ctx, a.School, token); err == nil {
		out.UsersVisible = len(users)
		out.CanListUsers = len(users) > 1
	}
	return out, nil
}
