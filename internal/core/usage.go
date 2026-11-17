package core

import (
	"context"
	"sync"
	"time"

	"github.com/matthiaslubbertsen/melon/internal/store"
)

// Usage counts schedule requests and the lessons they returned, per day and
// channel, in memory. Flush moves the counts to the database, so a busy
// schedule endpoint does not write on every request.
type Usage struct {
	mu     sync.Mutex
	counts map[[2]string]*[2]int64
}

func NewUsage() *Usage { return &Usage{counts: map[[2]string]*[2]int64{}} }

// Record counts one schedule request. The channel is the kind of credential
// that asked: session (the web app), api_key, oauth or calendar.
func (u *Usage) Record(day time.Time, channel string, lessons int) {
	if u == nil {
		return
	}
	if channel == "" {
		channel = "other"
	}
	key := [2]string{day.Format(time.DateOnly), channel}
	u.mu.Lock()
	defer u.mu.Unlock()
	c := u.counts[key]
	if c == nil {
		c = &[2]int64{}
		u.counts[key] = c
	}
	c[0]++
	c[1] += int64(lessons)
}

// Flush writes what was counted since the last flush. Counts that cannot be
// written are kept for the next try.
func (u *Usage) Flush(ctx context.Context, st *store.Store) error {
	if u == nil {
		return nil
	}
	u.mu.Lock()
	pending := u.counts
	u.counts = map[[2]string]*[2]int64{}
	u.mu.Unlock()
	var firstErr error
	for key, c := range pending {
		if err := st.AddUsage(ctx, store.Usage{Day: key[0], Channel: key[1], Requests: c[0], Lessons: c[1]}); err != nil {
			if firstErr == nil {
				firstErr = err
			}
			u.mu.Lock()
			if back := u.counts[key]; back != nil {
				back[0] += c[0]
				back[1] += c[1]
			} else {
				u.counts[key] = c
			}
			u.mu.Unlock()
		}
	}
	return firstErr
}
