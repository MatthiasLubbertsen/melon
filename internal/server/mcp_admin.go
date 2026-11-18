package server

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/matthiaslubbertsen/melon/internal/core"
)

// Admin tools for agents that debug Melon. They only exist for a credential
// an admin opted in (and only while that profile is still an admin), and
// they only read: showing Zermelo tokens, PINs, viewing as a user and
// deleting stay in the browser, behind the admin's own session.

var adminReadOnly = map[string]any{"readOnlyHint": true, "openWorldHint": false}

var adminTools = []mcpTool{
	{
		Name: "admin_overview", Title: "Admin: overview",
		Description: "Melon-wide numbers: profiles, active users (1/7/30 days), accounts per role, PINs (and locked ones), API keys, MCP apps, calendar feeds, lessons served in total and per day and channel for the last 30 days (channels: session = web app, api_key, oauth = MCP apps, calendar, admin), and accounts per school.",
		InputSchema: schema(nil, map[string]any{}), Annotations: adminReadOnly,
		run: func(ctx context.Context, s *Server, _ core.Principal, _ mcpArgs) (any, error) {
			return s.overview(ctx)
		},
	},
	{
		Name: "admin_find_profiles", Title: "Admin: find profiles",
		Description: "Find Melon profiles by account name, Zermelo user code (student number or teacher code), school, sign-in number or profile id (usr_...). Empty query lists the most recently active. A profile holds one or more linked Zermelo accounts; `pin` says whether it has a PIN sign-in.",
		InputSchema: schema(nil, map[string]any{
			"query": str("Part of a name, a user code, a school portal name or a profile id."),
			"limit": map[string]any{"type": "integer", "minimum": 1, "maximum": 50, "description": "At most this many. Defaults to 20."},
			"pin":   map[string]any{"type": "string", "enum": []string{"yes", "no"}, "description": "Only profiles with (yes) or without (no) a PIN sign-in."},
		}), Annotations: adminReadOnly,
		run: func(ctx context.Context, s *Server, _ core.Principal, a mcpArgs) (any, error) {
			if a.Limit <= 0 || a.Limit > 50 {
				a.Limit = 20
			}
			if a.PIN != "yes" && a.PIN != "no" {
				a.PIN = ""
			}
			rows, err := s.Store.Profiles(ctx, a.Query, a.PIN, a.Limit)
			if err != nil {
				return nil, err
			}
			out := []adminProfile{}
			for _, p := range rows {
				out = append(out, s.profileView(p))
			}
			return map[string]any{"profiles": out}, nil
		},
	},
	{
		Name: "admin_get_profile", Title: "Admin: get profile",
		Description: "Everything about one profile: linked accounts (school, role, user code, whether it can see others, calendar feed, Zermelo token expiry), sign-in number and PIN lock state, signed-in devices, API keys (by hint, never the key), connected MCP apps, and how many subject/teacher names the user set.",
		InputSchema: schema([]string{"profile"}, map[string]any{"profile": str("Profile id (usr_...) from admin_find_profiles.")}),
		Annotations: adminReadOnly,
		run: func(ctx context.Context, s *Server, _ core.Principal, a mcpArgs) (any, error) {
			return s.profileDetail(ctx, strings.TrimSpace(a.Profile))
		},
	},
	{
		Name: "admin_check_account", Title: "Admin: check account with Zermelo",
		Description: "Ask Zermelo right now (no cache) whether an account's token still works, who it belongs to, and how many people it can list. Use it when a user's schedule fails to load or a teacher has no search bar. Logged.",
		InputSchema: schema([]string{"account"}, map[string]any{"account": str("Account id (acc_...) from admin_get_profile.")}),
		Annotations: map[string]any{"readOnlyHint": true, "openWorldHint": true},
		run: func(ctx context.Context, s *Server, p core.Principal, a mcpArgs) (any, error) {
			out, err := s.Core.CheckAccount(ctx, strings.TrimSpace(a.Account))
			if err != nil {
				return nil, err
			}
			s.audit(ctx, p, "check_account", out.ProfileID, fmt.Sprintf("%s (%s:%s) via MCP", out.Account.Name, out.Account.School, out.Account.UserCode))
			return out, nil
		},
	},
	{
		Name: "admin_get_schedule", Title: "Admin: get a user's schedule",
		Description: "Get the schedule exactly as a user's account sees it in Melon (their subject and teacher names included), for debugging what they report. Same arguments as get_schedule, but 'account' is any account id. Logged.",
		InputSchema: schema([]string{"account"}, map[string]any{
			"account":      str("Account id (acc_...) from admin_get_profile."),
			"from":         str("First day, YYYY-MM-DD, or 'today' / 'tomorrow'. Defaults to today."),
			"to":           str("Last day (inclusive), YYYY-MM-DD. Defaults to 'from'."),
			"week":         str("An ISO week such as 2026-W46, instead of from/to."),
			"person":       str("Code of another student or teacher, for accounts that can view others."),
			"changes_only": map[string]any{"type": "boolean", "description": "Only return changed and cancelled lessons."},
			"language":     languageProp,
		}),
		Annotations: map[string]any{"readOnlyHint": true, "openWorldHint": true},
		run: func(ctx context.Context, s *Server, p core.Principal, a mcpArgs) (any, error) {
			acc, _, err := s.Core.AnyAccount(ctx, strings.TrimSpace(a.Account))
			if err != nil {
				return nil, err
			}
			from, to := a.From, a.To
			if from == "" && to == "" && a.Week == "" {
				from = "today"
			}
			f, t, err := s.parseRange(from, to, a.Week)
			if err != nil {
				return nil, err
			}
			sched, err := s.Core.Schedule(ctx, core.AsOwner(acc), acc.ID, core.ScheduleQuery{From: f, To: t, Person: a.Person, Lang: a.Language})
			if err != nil {
				return nil, err
			}
			s.audit(ctx, p, "read_schedule", acc.UserID, fmt.Sprintf("%s (%s:%s) %s..%s via MCP", acc.DisplayName, acc.School, acc.ZermeloUser, f.Format(time.DateOnly), t.Format(time.DateOnly)))
			return map[string]any{
				"schedule":  summarize(sched, a.ChangesOnly),
				"today":     s.Core.Today().Format(time.DateOnly),
				"fetchedAt": sched.FetchedAt.In(s.Core.Loc).Format(time.RFC3339),
				"stale":     sched.Stale,
			}, nil
		},
	},
	{
		Name: "admin_suggestions", Title: "Admin: name suggestions",
		Description: "Subject and teacher names users set on their own accounts that differ from what everyone gets now (the global subject names and the per-school teacher names), with how many profiles agree and the current name.",
		InputSchema: schema(nil, map[string]any{}), Annotations: adminReadOnly,
		run: func(ctx context.Context, s *Server, _ core.Principal, _ mcpArgs) (any, error) {
			return s.suggestions(ctx)
		},
	},
	{
		Name: "admin_get_log", Title: "Admin: log",
		Description: "What admins did, newest first: impersonations, PIN changes, unlocks, sign-outs, deletions, shown Zermelo tokens, revoked keys, name changes, and schedules read or accounts checked through these tools.",
		InputSchema: schema(nil, map[string]any{"limit": map[string]any{"type": "integer", "minimum": 1, "maximum": 200, "description": "At most this many. Defaults to 50."}}),
		Annotations: adminReadOnly,
		run: func(ctx context.Context, s *Server, _ core.Principal, a mcpArgs) (any, error) {
			if a.Limit <= 0 || a.Limit > 200 {
				a.Limit = 50
			}
			entries, err := s.adminLog(ctx, a.Limit)
			return map[string]any{"entries": entries}, err
		},
	},
}
