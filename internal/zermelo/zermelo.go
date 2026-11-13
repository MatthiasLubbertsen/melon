// Package zermelo talks to Zermelo Portal (https://<school>.zportal.nl).
// Melon only ever reads from Zermelo; the single POST is the code exchange.
package zermelo

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"

	"golang.org/x/time/rate"
)

// Errors callers can branch on.
var (
	ErrUnauthorized = errors.New("zermelo: token invalid or expired")
	ErrForbidden    = errors.New("zermelo: not allowed")
	ErrBadCode      = errors.New("zermelo: link code invalid or expired")
	ErrNoSchool     = errors.New("zermelo: school not found")
	ErrRateLimited  = errors.New("zermelo: rate limited")
	ErrUnavailable  = errors.New("zermelo: unavailable")
)

type User struct {
	Code           string `json:"code"`
	FirstName      string `json:"firstName"`
	Prefix         string `json:"prefix"`
	LastName       string `json:"lastName"`
	IsStudent      bool   `json:"isStudent"`
	IsEmployee     bool   `json:"isEmployee"`
	IsFamilyMember bool   `json:"isFamilyMember"`
}

func (u User) Name() string {
	return strings.Join(strings.Fields(u.FirstName+" "+u.Prefix+" "+u.LastName), " ")
}

type TokenInfo struct {
	User    string `json:"user"`
	Expires int64  `json:"expires"`
}

type Appointment struct {
	ID                int64    `json:"id"`
	Instance          int64    `json:"appointmentInstance"`
	Start             int64    `json:"start"`
	End               int64    `json:"end"`
	Cancelled         bool     `json:"cancelled"`
	Type              string   `json:"appointmentType"`
	Online            bool     `json:"online"`
	Optional          bool     `json:"optional"`
	StartSlot         *int     `json:"startTimeSlot"`
	EndSlot           *int     `json:"endTimeSlot"`
	Subjects          []string `json:"subjects"`
	Groups            []string `json:"groups"`
	Locations         []string `json:"locations"`
	Teachers          []string `json:"teachers"`
	OnlineLocationURL string   `json:"onlineLocationUrl"`
	TeacherChanged    bool     `json:"teacherChanged"`
	LocationChanged   bool     `json:"locationChanged"`
	GroupChanged      bool     `json:"groupChanged"`
	TimeChanged       bool     `json:"timeChanged"`
	ChangeDescription string   `json:"changeDescription"`
	SchedulerRemark   string   `json:"schedulerRemark"`
	Content           string   `json:"content"`
	LastModified      int64    `json:"scheduleLastModified"`
}

type Announcement struct {
	ID    int64  `json:"id"`
	Start int64  `json:"start"`
	End   int64  `json:"end"`
	Title string `json:"title"`
	Text  string `json:"text"`
}

// Upstream is the slice of Zermelo that Melon needs.
type Upstream interface {
	ExchangeCode(ctx context.Context, school, code string) (token string, err error)
	TokenInfo(ctx context.Context, school, token string) (TokenInfo, error)
	Me(ctx context.Context, school, token string) (User, error)
	Users(ctx context.Context, school, token string) ([]User, error)
	// LiveSchedule returns one ISO week (yyyyWW) for a student or teacher.
	LiveSchedule(ctx context.Context, school, token, kind, code, week string) ([]Appointment, error)
	Announcements(ctx context.Context, school, token string) ([]Announcement, error)
}

var schoolRe = regexp.MustCompile(`^[a-z0-9]([a-z0-9-]{0,61}[a-z0-9])?$`)

// NormalizeSchool accepts "jfc", "jfc.zportal.nl" or a full portal URL and
// returns the bare portal name. Restricting it to a single DNS label is what
// keeps user input from steering requests to arbitrary hosts.
func NormalizeSchool(in string) (string, bool) {
	s := strings.ToLower(strings.TrimSpace(in))
	s = strings.TrimPrefix(strings.TrimPrefix(s, "https://"), "http://")
	if i := strings.IndexAny(s, "/?#"); i >= 0 {
		s = s[:i]
	}
	s = strings.TrimSuffix(s, ".zportal.nl")
	return s, schoolRe.MatchString(s)
}

// HTTP is the real Zermelo client. It keeps a token bucket per school so one
// busy school cannot starve another and none of them gets hammered.
type HTTP struct {
	Client *http.Client
	// BaseURL overrides https://<school>.zportal.nl in tests.
	BaseURL func(school string) string

	mu       sync.Mutex
	limiters map[string]*rate.Limiter
	backoff  map[string]time.Time
}

func NewHTTP() *HTTP {
	return &HTTP{
		Client:   &http.Client{Timeout: 15 * time.Second},
		limiters: map[string]*rate.Limiter{},
		backoff:  map[string]time.Time{},
	}
}

// Per school: 4 requests/second sustained, bursts of 8.
const (
	perSecond = 4
	burst     = 8
)

func (h *HTTP) gate(ctx context.Context, school string) error {
	h.mu.Lock()
	until := h.backoff[school]
	lim := h.limiters[school]
	if lim == nil {
		lim = rate.NewLimiter(perSecond, burst)
		h.limiters[school] = lim
	}
	h.mu.Unlock()
	if time.Now().Before(until) {
		return ErrRateLimited
	}
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	if err := lim.Wait(ctx); err != nil {
		return ErrRateLimited
	}
	return nil
}

func (h *HTTP) base(school string) string {
	if h.BaseURL != nil {
		return h.BaseURL(school)
	}
	return "https://" + school + ".zportal.nl"
}

type envelope struct {
	Response struct {
		Status  int             `json:"status"`
		Message string          `json:"message"`
		Data    json.RawMessage `json:"data"`
	} `json:"response"`
}

func (h *HTTP) do(ctx context.Context, school string, req *http.Request) ([]byte, int, error) {
	if _, ok := NormalizeSchool(school); !ok {
		return nil, 0, ErrNoSchool
	}
	if err := h.gate(ctx, school); err != nil {
		return nil, 0, err
	}
	req.Header.Set("User-Agent", "Melon (+https://github.com/matthiaslubbertsen/melon)")
	req.Header.Set("Accept", "application/json")
	res, err := h.Client.Do(req.WithContext(ctx))
	if err != nil {
		var dns interface{ NotFound() bool }
		if errors.As(err, &dns) && dns.NotFound() {
			return nil, 0, ErrNoSchool
		}
		return nil, 0, fmt.Errorf("%w: %v", ErrUnavailable, redact(err))
	}
	defer res.Body.Close()
	body, err := io.ReadAll(io.LimitReader(res.Body, 32<<20))
	if err != nil {
		return nil, 0, fmt.Errorf("%w: %v", ErrUnavailable, err)
	}
	if res.StatusCode == http.StatusTooManyRequests {
		wait := 30 * time.Second
		if s, err := strconv.Atoi(res.Header.Get("Retry-After")); err == nil && s > 0 && s < 600 {
			wait = time.Duration(s) * time.Second
		}
		h.mu.Lock()
		h.backoff[school] = time.Now().Add(wait)
		h.mu.Unlock()
		return nil, res.StatusCode, ErrRateLimited
	}
	return body, res.StatusCode, nil
}

// redact strips URLs from transport errors so nothing sensitive is logged.
func redact(err error) string {
	var ue *url.Error
	if errors.As(err, &ue) {
		return ue.Err.Error()
	}
	return err.Error()
}

func (h *HTTP) get(ctx context.Context, school, token, path string, q url.Values, out any) error {
	u := h.base(school) + "/api/v3/" + path
	if len(q) > 0 {
		u += "?" + q.Encode()
	}
	req, err := http.NewRequest(http.MethodGet, u, nil)
	if err != nil {
		return err
	}
	// Header, not query string: tokens must not end up in access logs.
	req.Header.Set("Authorization", "Bearer "+token)
	body, status, err := h.do(ctx, school, req)
	if err != nil {
		return err
	}
	switch {
	case status == 401:
		return ErrUnauthorized
	case status == 403:
		return ErrForbidden
	case status == 404:
		// Every endpoint Melon calls exists, so a 404 means an unknown portal.
		return ErrNoSchool
	case status >= 500:
		return fmt.Errorf("%w: status %d", ErrUnavailable, status)
	}
	var env envelope
	if err := json.Unmarshal(body, &env); err != nil {
		// Unknown portals answer with an HTML page rather than API JSON.
		return ErrNoSchool
	}
	if status >= 400 {
		return fmt.Errorf("%w: status %d: %s", ErrUnavailable, status, env.Response.Message)
	}
	return json.Unmarshal(env.Response.Data, out)
}

func (h *HTTP) ExchangeCode(ctx context.Context, school, code string) (string, error) {
	form := url.Values{"grant_type": {"authorization_code"}, "code": {code}}
	req, err := http.NewRequest(http.MethodPost, h.base(school)+"/api/v3/oauth/token", strings.NewReader(form.Encode()))
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	body, status, err := h.do(ctx, school, req)
	if err != nil {
		return "", err
	}
	var out struct {
		AccessToken string `json:"access_token"`
	}
	if json.Unmarshal(body, &out) == nil && out.AccessToken != "" {
		return out.AccessToken, nil
	}
	if status >= 500 {
		return "", ErrUnavailable
	}
	if status == 404 {
		return "", ErrNoSchool
	}
	return "", ErrBadCode
}

func (h *HTTP) TokenInfo(ctx context.Context, school, token string) (TokenInfo, error) {
	var out []TokenInfo
	if err := h.get(ctx, school, token, "tokens/~current", url.Values{"fields": {"user,expires"}}, &out); err != nil {
		return TokenInfo{}, err
	}
	if len(out) == 0 {
		return TokenInfo{}, ErrUnauthorized
	}
	return out[0], nil
}

const userFields = "code,firstName,prefix,lastName,isStudent,isEmployee,isFamilyMember"

func (h *HTTP) Me(ctx context.Context, school, token string) (User, error) {
	var out []User
	if err := h.get(ctx, school, token, "users/~me", url.Values{"fields": {userFields}}, &out); err != nil {
		return User{}, err
	}
	if len(out) == 0 {
		return User{}, ErrUnauthorized
	}
	return out[0], nil
}

func (h *HTTP) Users(ctx context.Context, school, token string) ([]User, error) {
	var out []User
	err := h.get(ctx, school, token, "users", url.Values{"archived": {"false"}, "fields": {userFields}}, &out)
	return out, err
}

func (h *HTTP) LiveSchedule(ctx context.Context, school, token, kind, code, week string) ([]Appointment, error) {
	if kind != "student" && kind != "teacher" {
		return nil, fmt.Errorf("zermelo: unknown schedule kind %q", kind)
	}
	var out []struct {
		Appointments []Appointment `json:"appointments"`
	}
	if err := h.get(ctx, school, token, "liveschedule", url.Values{kind: {code}, "week": {week}}, &out); err != nil {
		return nil, err
	}
	if len(out) == 0 {
		return nil, nil
	}
	return out[0].Appointments, nil
}

func (h *HTTP) Announcements(ctx context.Context, school, token string) ([]Announcement, error) {
	var out []Announcement
	err := h.get(ctx, school, token, "announcements", url.Values{"user": {"~me"}, "current": {"true"}}, &out)
	return out, err
}

func isOutage(err error) bool {
	return errors.Is(err, ErrUnavailable) || errors.Is(err, ErrRateLimited)
}
