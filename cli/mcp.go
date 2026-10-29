package cli

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"sort"
	"time"

	"github.com/junaid18183/sanduk/vault"
	"github.com/spf13/cobra"
)

// The MCP server exists so a coding agent can find out what credentials are
// available without ever being shown one. It is deliberately read-only and
// value-free: there is no tool here that returns a secret, and none that runs
// a command. An agent learns an entry's name and the variables it contributes,
// then writes an ordinary `sanduk exec` invocation, which the user approves
// through whatever already governs the agent's shell access.
//
// Keeping execution outside sanduk matters: an agent that can already run
// commands gains nothing from a second execution path, while a server that ran
// commands on its behalf would be a way around the approval the user relies on.

// mcpProtocolVersion is the version answered when a client does not name one.
const mcpProtocolVersion = "2024-11-05"

// JSON-RPC error codes used here, from the JSON-RPC 2.0 specification.
const (
	rpcParseError     = -32700
	rpcInvalidRequest = -32600
	rpcMethodNotFound = -32601
	rpcInternalError  = -32603
)

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

func newMCPCmd(a *app) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "mcp",
		Short: "Serve entry metadata to an AI agent over MCP (names only, never values)",
		Long: "Run a Model Context Protocol server on stdin/stdout so a coding agent can\n" +
			"discover what credentials exist without being shown any of them.\n\n" +
			"Every tool is read-only and returns names, kinds, variable names,\n" +
			"descriptions and rotation dates. None returns a secret value, and none\n" +
			"runs a command: an agent that needs a secret writes a normal\n" +
			"\"sanduk exec <entry> -- <command>\", which stays subject to whatever\n" +
			"approval already governs the commands it runs.\n\n" +
			"Configure it as a stdio server, for example:\n\n" +
			"  {\"command\": \"sanduk\", \"args\": [\"mcp\"]}\n\n" +
			"The server inherits this invocation's backend settings, so point it at the\n" +
			"vault you want it to describe with --backend/--config or the SANDUK_*\n" +
			"variables. It is a plain pipe server: it writes nothing to stdout but\n" +
			"protocol messages.",
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			return a.serveMCP(cmd.InOrStdin(), cmd.OutOrStdout())
		},
	}
	return cmd
}

// serveMCP runs the JSON-RPC loop until the input closes.
func (a *app) serveMCP(in io.Reader, out io.Writer) error {
	sc := bufio.NewScanner(in)
	sc.Buffer(make([]byte, 0, 64*1024), 8<<20)
	enc := json.NewEncoder(out)

	for sc.Scan() {
		line := sc.Bytes()
		if len(line) == 0 {
			continue
		}
		var req rpcRequest
		if err := json.Unmarshal(line, &req); err != nil {
			if err := enc.Encode(rpcResponse{
				JSONRPC: "2.0",
				ID:      json.RawMessage("null"),
				Error:   &rpcError{Code: rpcParseError, Message: "invalid JSON"},
			}); err != nil {
				return err
			}
			continue
		}
		// A request without an id is a notification: act on it, answer nothing.
		if len(req.ID) == 0 {
			continue
		}
		resp := a.handleMCP(req)
		if err := enc.Encode(resp); err != nil {
			return err
		}
	}
	return sc.Err()
}

func (a *app) handleMCP(req rpcRequest) rpcResponse {
	resp := rpcResponse{JSONRPC: "2.0", ID: req.ID}
	switch req.Method {
	case "initialize":
		resp.Result = a.mcpInitialize(req.Params)
	case "tools/list":
		resp.Result = map[string]any{"tools": mcpTools()}
	case "tools/call":
		result, err := a.mcpCallTool(req.Params)
		if err != nil {
			// A failing tool is reported inside the result, not as a protocol
			// error, so the agent can read what went wrong and retry.
			resp.Result = mcpToolError(err)
			return resp
		}
		resp.Result = result
	case "ping":
		resp.Result = map[string]any{}
	default:
		resp.Error = &rpcError{Code: rpcMethodNotFound, Message: "unknown method " + req.Method}
	}
	return resp
}

func (a *app) mcpInitialize(params json.RawMessage) map[string]any {
	// Not named "version": that is the package-level build stamp, and
	// shadowing it here once put the protocol version in serverInfo.
	protocol := mcpProtocolVersion
	var p struct {
		ProtocolVersion string `json:"protocolVersion"`
	}
	if len(params) > 0 && json.Unmarshal(params, &p) == nil && p.ProtocolVersion != "" {
		// Answer in the client's dialect; this server's surface is the same
		// in every version it is likely to meet.
		protocol = p.ProtocolVersion
	}
	return map[string]any{
		"protocolVersion": protocol,
		"capabilities":    map[string]any{"tools": map[string]any{}},
		"serverInfo":      map[string]any{"name": "sanduk", "version": version},
		"instructions": "sanduk stores this machine's API keys. These tools report what is " +
			"stored — names, kinds and the environment variables each entry provides — " +
			"and never a secret value. To give a program a secret, do not ask for the " +
			"value: run it through sanduk, e.g. `sanduk exec <entry> -- <command>`, or " +
			"`sanduk exec <entry> --only VAR -- <command>` for a single variable. " +
			"A file secret is injected as a path in the variable named by describe_entry.",
	}
}

func mcpTools() []map[string]any {
	noArgs := map[string]any{"type": "object", "properties": map[string]any{}}
	return []map[string]any{
		{
			"name": "list_entries",
			"description": "List every entry in the vault with its kind, the environment " +
				"variable it is exposed as (for a secret or file), its description and any " +
				"rotation deadline. Each entry also carries a scope: \"project\" for one " +
				"belonging to a repository (with \"current\": true for this working " +
				"directory's, the entry `sanduk exec` resolves to here with no arguments), " +
				"\"member\" for a secret a group links to, and \"global\" for one that " +
				"stands on its own. Reads metadata only and never returns a secret value.",
			"inputSchema": noArgs,
		},
		{
			"name": "describe_entry",
			"description": "Describe one entry: its kind, every environment variable it " +
				"contributes (for a group, each field and link), and its rotation deadline. " +
				"Returns variable names only, never values. Use this to learn which " +
				"variables `sanduk exec <name>` will set.",
			"inputSchema": map[string]any{
				"type": "object",
				"properties": map[string]any{
					"name": map[string]any{"type": "string", "description": "the entry name"},
				},
				"required": []string{"name"},
			},
		},
		{
			"name": "check_expiry",
			"description": "List entries whose rotation deadline has passed or is near. " +
				"Reads metadata only.",
			"inputSchema": map[string]any{
				"type": "object",
				"properties": map[string]any{
					"within_days": map[string]any{
						"type":        "integer",
						"description": "how many days ahead counts as due (default 14)",
					},
				},
			},
		},
	}
}

func (a *app) mcpCallTool(params json.RawMessage) (map[string]any, error) {
	var call struct {
		Name      string          `json:"name"`
		Arguments json.RawMessage `json:"arguments"`
	}
	if err := json.Unmarshal(params, &call); err != nil {
		return nil, fmt.Errorf("invalid tool call")
	}
	switch call.Name {
	case "list_entries":
		return a.mcpListEntries()
	case "describe_entry":
		var args struct {
			Name string `json:"name"`
		}
		_ = json.Unmarshal(call.Arguments, &args)
		if args.Name == "" {
			return nil, fmt.Errorf("describe_entry needs a name")
		}
		return a.mcpDescribeEntry(args.Name)
	case "check_expiry":
		var args struct {
			WithinDays *int `json:"within_days"`
		}
		_ = json.Unmarshal(call.Arguments, &args)
		within := 14
		if args.WithinDays != nil {
			within = *args.WithinDays
		}
		return a.mcpCheckExpiry(within)
	default:
		return nil, fmt.Errorf("unknown tool %q", call.Name)
	}
}

// mcpEntry is the shape returned for one entry. It has no value field, by
// construction rather than by omission.
type mcpEntry struct {
	Name        string   `json:"name"`
	Kind        string   `json:"kind"`
	Variables   []string `json:"variables,omitempty"`
	Description string   `json:"description,omitempty"`
	Expires     string   `json:"expires,omitempty"`
	// Scope tells an agent which entries are worth its attention: the one
	// belonging to the directory it is working in, rather than one of the
	// many that belong somewhere else. Every entry is still returned —
	// hiding one would just make the agent ask for it another way.
	Scope string `json:"scope,omitempty"`
	// Current marks the entry of the working directory, the one "sanduk
	// exec" resolves to here with no arguments.
	Current bool `json:"current,omitempty"`
}

func (a *app) mcpListEntries() (map[string]any, error) {
	v, err := a.openVault()
	if err != nil {
		return nil, err
	}
	names, err := v.List()
	if err != nil {
		return nil, err
	}
	cfg, err := a.loadConfig()
	if err != nil {
		return nil, err
	}

	// The scope each entry is in, so the agent can tell this repository's
	// secrets from every other repository's. A project file that is not
	// approved narrows nothing, exactly as for the CLI.
	layers, _ := a.projectChain(cfg)
	projects := cfg.GroupLocations()
	for _, l := range layers {
		if l.Trusted {
			projects[l.Entry] = l.Dir
		}
	}
	scopes := make(map[string]vault.Scoped, len(names))
	for _, s := range vault.ScopeEntries(names, cfg, projects, currentEntries(layers)) {
		scopes[s.Name] = s
	}

	entries := make([]mcpEntry, 0, len(names))
	for _, name := range names {
		e := mcpEntry{
			Name:    name,
			Expires: cfg.ExpiryOf(name),
			Scope:   scopes[name].Scope,
			Current: scopes[name].Current,
		}
		switch g, isGroup := cfg.Groups[name]; {
		case isGroup:
			e.Kind = "group"
			e.Description = g.Description
		default:
			if m, isFile := cfg.Files[name]; isFile {
				e.Kind = "file"
				e.Variables = []string{cfg.EnvNameForSecret(name)}
				e.Description = m.Description
			} else if m, isSecret := cfg.Secrets[name]; isSecret {
				e.Kind = "secret"
				e.Variables = []string{cfg.EnvNameForSecret(name)}
				e.Description = m.Description
			} else {
				// Listing must not read values, so an entry with no metadata
				// keeps its kind unknown here; describe_entry can tell.
				e.Kind = "unknown"
			}
		}
		entries = append(entries, e)
	}
	return mcpJSON(map[string]any{"entries": entries})
}

func (a *app) mcpDescribeEntry(name string) (map[string]any, error) {
	v, err := a.openVault()
	if err != nil {
		return nil, err
	}
	cfg, err := a.loadConfig()
	if err != nil {
		return nil, err
	}
	e, err := v.Get(name)
	if err != nil {
		return nil, err
	}

	out := mcpEntry{Name: name, Kind: e.Type, Expires: cfg.ExpiryOf(name)}
	switch e.Type {
	case vault.TypeSecret:
		out.Variables = []string{cfg.EnvNameForSecret(name)}
		out.Description = cfg.Secrets[name].Description
	case vault.TypeFile:
		out.Variables = []string{cfg.EnvNameForSecret(name)}
		out.Description = cfg.Files[name].Description
	case vault.TypeGroup:
		vars := make([]string, 0, e.FieldCount())
		for k := range e.Fields {
			vars = append(vars, k)
		}
		for k := range e.Links {
			vars = append(vars, k)
		}
		for k := range e.FieldLinks {
			vars = append(vars, k)
		}
		sort.Strings(vars)
		out.Variables = vars
		out.Description = cfg.Groups[name].Description
	}
	return mcpJSON(out)
}

func (a *app) mcpCheckExpiry(within int) (map[string]any, error) {
	cfg, err := a.loadConfig()
	if err != nil {
		return nil, err
	}
	due, err := cfg.Expiries(time.Now())
	if err != nil {
		return nil, err
	}
	type row struct {
		Name    string `json:"name"`
		Expires string `json:"expires"`
		Days    int    `json:"days_remaining"`
		Status  string `json:"status"`
	}
	rows := make([]row, 0, len(due))
	for _, e := range due {
		status := "ok"
		switch {
		case e.Expired():
			status = "expired"
		case e.Days <= within:
			status = "due"
		}
		rows = append(rows, row{Name: e.Name, Expires: e.Date, Days: e.Days, Status: status})
	}
	return mcpJSON(map[string]any{"entries": rows})
}

// mcpJSON wraps a payload as an MCP text content block.
func mcpJSON(payload any) (map[string]any, error) {
	b, err := json.MarshalIndent(payload, "", "  ")
	if err != nil {
		return nil, err
	}
	return map[string]any{
		"content": []map[string]any{{"type": "text", "text": string(b)}},
	}, nil
}

func mcpToolError(err error) map[string]any {
	return map[string]any{
		"isError": true,
		"content": []map[string]any{{"type": "text", "text": err.Error()}},
	}
}
