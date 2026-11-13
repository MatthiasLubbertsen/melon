package server

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/matthiaslubbertsen/melon/internal/core"
)

// The MCP endpoint speaks Streamable HTTP in its simplest form: stateless,
// one JSON response per POST, tools only. It calls the same core.Service as
// the REST API, so both always agree.

var mcpVersions = []string{"2025-11-25", "2025-06-18", "2025-03-26", "2024-11-05"}

type rpcRequest struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      json.RawMessage `json:"id,omitempty"`
	Method  string          `json:"method"`
	Params  json.RawMessage `json:"params,omitempty"`
}

type rpcError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}

type rpcResponse struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      json.RawMessage `json:"id"`
	Result  any             `json:"result,omitempty"`
	Error   *rpcError       `json:"error,omitempty"`
}

type mcpTool struct {
	Name        string         `json:"name"`
	Title       string         `json:"title"`
	Description string         `json:"description"`
	InputSchema map[string]any `json:"inputSchema"`
	Annotations map[string]any `json:"annotations"`
	run         func(ctx context.Context, s *Server, p core.Principal, args mcpArgs) (any, error)
}

type mcpArgs struct {
	Account     string `json:"account"`
	From        string `json:"from"`
	To          string `json:"to"`
	Week        string `json:"week"`
	Person      string `json:"person"`
	ChangesOnly bool   `json:"changes_only"`
	Language    string `json:"language"`
	Query       string `json:"query"`
	Code        string `json:"code"`
	Name        string `json:"name"`
}

func schema(required []string, props map[string]any) map[string]any {
	if required == nil {
		required = []string{}
	}
	return map[string]any{"type": "object", "properties": props, "required": required, "additionalProperties": false}
}

func str(desc string) map[string]any { return map[string]any{"type": "string", "description": desc} }

var (
	accountProp  = str("Account id from list_accounts. May be omitted when exactly one account is available.")
	languageProp = map[string]any{"type": "string", "enum": []string{"nl", "en"}, "description": "Language for subject names. Defaults to nl."}
	readOnly     = map[string]any{"readOnlyHint": true, "openWorldHint": true}
)

var mcpTools = []mcpTool{
	{
		Name: "list_accounts", Title: "List accounts",
		Description: "List the Zermelo accounts this connection may use: whose schedule it is, the school, and whether the account can look up other people (teachers, mentors and admins can).",
		InputSchema: schema(nil, map[string]any{}), Annotations: readOnly,
		run: func(ctx context.Context, s *Server, p core.Principal, _ mcpArgs) (any, error) {
			accounts, err := s.Core.Accounts(ctx, p)
			return map[string]any{"accounts": accounts}, err
		},
	},
	{
		Name: "get_schedule", Title: "Get schedule",
		Description: "Get the school schedule (lessons, exams, activities) for a date range, live from Zermelo. Each lesson has a status: scheduled, changed (see 'changes': teacher, location, group, time) or cancelled. Defaults to today. Times are local school time (Europe/Amsterdam). Maximum range is 42 days.",
		InputSchema: schema(nil, map[string]any{
			"account":      accountProp,
			"from":         str("First day, YYYY-MM-DD, or 'today' / 'tomorrow'. Defaults to today."),
			"to":           str("Last day (inclusive), YYYY-MM-DD. Defaults to 'from'."),
			"week":         str("An ISO week such as 2026-W46, instead of from/to."),
			"person":       str("Code of another student or teacher (from search_people). Only for accounts that can view others."),
			"changes_only": map[string]any{"type": "boolean", "description": "Only return changed and cancelled lessons."},
			"language":     languageProp,
		}), Annotations: readOnly,
		run: func(ctx context.Context, s *Server, p core.Principal, a mcpArgs) (any, error) {
			id, err := s.mcpAccount(ctx, p, a.Account)
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
			sched, err := s.Core.Schedule(ctx, p, id, core.ScheduleQuery{From: f, To: t, Person: a.Person, Lang: a.Language})
			if err != nil {
				return nil, err
			}
			return map[string]any{
				"schedule":  summarize(sched, a.ChangesOnly),
				"today":     s.Core.Today().Format(time.DateOnly),
				"fetchedAt": sched.FetchedAt.In(s.Core.Loc).Format(time.RFC3339),
				"stale":     sched.Stale,
			}, nil
		},
	},
	{
		Name: "search_people", Title: "Search people",
		Description: "Find students and teachers by name or code, to then view their schedule with get_schedule. Only works for accounts where canViewOthers is true.",
		InputSchema: schema([]string{"query"}, map[string]any{"account": accountProp, "query": str("Part of a name, or a code.")}),
		Annotations: readOnly,
		run: func(ctx context.Context, s *Server, p core.Principal, a mcpArgs) (any, error) {
			id, err := s.mcpAccount(ctx, p, a.Account)
			if err != nil {
				return nil, err
			}
			people, err := s.Core.People(ctx, p, id, a.Query, 25)
			return map[string]any{"people": people}, err
		},
	},
	{
		Name: "get_announcements", Title: "Get announcements",
		Description: "Get the school's current announcements for an account.",
		InputSchema: schema(nil, map[string]any{"account": accountProp}), Annotations: readOnly,
		run: func(ctx context.Context, s *Server, p core.Principal, a mcpArgs) (any, error) {
			id, err := s.mcpAccount(ctx, p, a.Account)
			if err != nil {
				return nil, err
			}
			list, _, err := s.Core.Announcements(ctx, p, id)
			return map[string]any{"announcements": list}, err
		},
	},
	{
		Name: "list_subjects", Title: "List subject names",
		Description: "List how subject codes (like 'wi') map to readable names (like 'Wiskunde') for an account, including the user's own overrides.",
		InputSchema: schema(nil, map[string]any{"account": accountProp, "language": languageProp}), Annotations: readOnly,
		run: func(ctx context.Context, s *Server, p core.Principal, a mcpArgs) (any, error) {
			id, err := s.mcpAccount(ctx, p, a.Account)
			if err != nil {
				return nil, err
			}
			list, err := s.Core.Subjects(ctx, p, id, a.Language)
			return map[string]any{"subjects": list}, err
		},
	},
	{
		Name: "rename_subject", Title: "Rename a subject",
		Description: "Set the readable name the user wants for a subject code on an account. Pass an empty name to go back to Melon's default. This only changes how Melon displays the subject; nothing is written to Zermelo.",
		InputSchema: schema([]string{"code", "name"}, map[string]any{"account": accountProp, "code": str("Subject code, e.g. 'wi'."), "name": str("New readable name, or empty to reset.")}),
		Annotations: map[string]any{"readOnlyHint": false, "destructiveHint": false, "idempotentHint": true, "openWorldHint": false},
		run: func(ctx context.Context, s *Server, p core.Principal, a mcpArgs) (any, error) {
			id, err := s.mcpAccount(ctx, p, a.Account)
			if err != nil {
				return nil, err
			}
			if strings.TrimSpace(a.Name) == "" {
				return map[string]any{"code": a.Code, "reset": true}, s.Core.ResetSubject(ctx, p, id, a.Code)
			}
			return map[string]any{"code": a.Code, "name": strings.TrimSpace(a.Name)}, s.Core.SetSubject(ctx, p, id, a.Code, a.Name)
		},
	},
}

// mcpAccount resolves the account argument, defaulting when unambiguous.
func (s *Server) mcpAccount(ctx context.Context, p core.Principal, id string) (string, error) {
	if id != "" {
		return id, nil
	}
	accounts, err := s.Core.Accounts(ctx, p)
	if err != nil {
		return "", err
	}
	switch len(accounts) {
	case 0:
		return "", core.Errf(http.StatusForbidden, "no_accounts", "This connection has no accounts. The user should link one in Melon and reconnect.")
	case 1:
		return accounts[0].ID, nil
	}
	var names []string
	for _, a := range accounts {
		names = append(names, fmt.Sprintf("%s (%s, %s)", a.ID, a.Name, a.School))
	}
	return "", core.Errf(http.StatusBadRequest, "account_required", "Several accounts are available; pass 'account' as one of: %s.", strings.Join(names, "; "))
}

func (s *Server) handleMCP(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		// No server-initiated streams and no sessions to delete.
		w.Header().Set("Allow", "POST, OPTIONS")
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	p, err := s.principal(r)
	if err != nil || p.Kind == "session" {
		w.Header().Set("WWW-Authenticate", fmt.Sprintf(`Bearer realm="Melon", resource_metadata="%s/.well-known/oauth-protected-resource"`, s.baseURL(r)))
		writeErr(w, core.Errf(http.StatusUnauthorized, "unauthorized", "Connect with OAuth or pass a Melon API key as a Bearer token."))
		return
	}

	var raw json.RawMessage
	if err := json.NewDecoder(r.Body).Decode(&raw); err != nil {
		writeJSON(w, http.StatusBadRequest, rpcResponse{JSONRPC: "2.0", ID: json.RawMessage("null"), Error: &rpcError{-32700, "Parse error"}})
		return
	}
	var batch []rpcRequest
	single := !strings.HasPrefix(strings.TrimSpace(string(raw)), "[")
	if single {
		var one rpcRequest
		if err := json.Unmarshal(raw, &one); err != nil {
			writeJSON(w, http.StatusBadRequest, rpcResponse{JSONRPC: "2.0", ID: json.RawMessage("null"), Error: &rpcError{-32600, "Invalid request"}})
			return
		}
		batch = []rpcRequest{one}
	} else if err := json.Unmarshal(raw, &batch); err != nil || len(batch) == 0 || len(batch) > 20 {
		writeJSON(w, http.StatusBadRequest, rpcResponse{JSONRPC: "2.0", ID: json.RawMessage("null"), Error: &rpcError{-32600, "Invalid request"}})
		return
	}

	var responses []rpcResponse
	for _, req := range batch {
		if len(req.ID) == 0 { // notification (or a client's response): nothing to answer
			continue
		}
		res := rpcResponse{JSONRPC: "2.0", ID: req.ID}
		result, rerr := s.mcpDispatch(r.Context(), p, req)
		if rerr != nil {
			res.Error = rerr
		} else {
			res.Result = result
		}
		responses = append(responses, res)
	}
	switch {
	case len(responses) == 0:
		w.WriteHeader(http.StatusAccepted)
	case single:
		writeJSON(w, http.StatusOK, responses[0])
	default:
		writeJSON(w, http.StatusOK, responses)
	}
}

func (s *Server) mcpDispatch(ctx context.Context, p core.Principal, req rpcRequest) (any, *rpcError) {
	switch req.Method {
	case "initialize":
		var params struct {
			ProtocolVersion string `json:"protocolVersion"`
		}
		_ = json.Unmarshal(req.Params, &params)
		version := mcpVersions[0]
		for _, v := range mcpVersions {
			if v == params.ProtocolVersion {
				version = v
			}
		}
		return map[string]any{
			"protocolVersion": version,
			"capabilities":    map[string]any{"tools": map[string]any{}},
			"serverInfo":      map[string]any{"name": "melon", "title": "Melon", "version": s.Version},
			"instructions":    "Melon gives live access to Dutch school schedules from Zermelo. Start with list_accounts. Schedules change often: fetch again rather than relying on earlier results, and always point out cancelled or changed lessons.",
		}, nil
	case "ping":
		return map[string]any{}, nil
	case "tools/list":
		return map[string]any{"tools": mcpTools}, nil
	case "tools/call":
		var params struct {
			Name      string          `json:"name"`
			Arguments json.RawMessage `json:"arguments"`
		}
		if err := json.Unmarshal(req.Params, &params); err != nil {
			return nil, &rpcError{-32602, "Invalid params"}
		}
		for _, tool := range mcpTools {
			if tool.Name != params.Name {
				continue
			}
			var args mcpArgs
			if len(params.Arguments) > 0 && string(params.Arguments) != "null" {
				if err := json.Unmarshal(params.Arguments, &args); err != nil {
					return toolError("Invalid arguments: " + err.Error()), nil
				}
			}
			if args.Language != "en" {
				args.Language = "nl"
			}
			out, err := tool.run(ctx, s, p, args)
			if err != nil {
				var ce *core.Error
				if errors.As(err, &ce) {
					return toolError(ce.Message + " (" + ce.Code + ")"), nil
				}
				return toolError("Melon hit an internal error."), nil
			}
			text, _ := json.Marshal(out)
			return map[string]any{"content": []map[string]any{{"type": "text", "text": string(text)}}, "structuredContent": out}, nil
		}
		return nil, &rpcError{-32602, "Unknown tool: " + params.Name}
	}
	return nil, &rpcError{-32601, "Method not found: " + req.Method}
}

// toolError reports a failure the model can read and act on.
func toolError(msg string) map[string]any {
	return map[string]any{"content": []map[string]any{{"type": "text", "text": msg}}, "isError": true}
}
