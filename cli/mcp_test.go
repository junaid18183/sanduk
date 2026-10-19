package cli

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"
)

// mcpSession drives the server with a sequence of JSON-RPC lines and returns
// the decoded responses.
func mcpSession(t *testing.T, lines ...string) []map[string]any {
	t.Helper()
	var out bytes.Buffer
	a := newApp()
	// PersistentPreRunE normally primes the settings; do it the way runCmd does.
	if _, err := runApp(t, a, "", "list", "--names"); err != nil {
		t.Fatalf("priming: %v", err)
	}
	if err := a.serveMCP(strings.NewReader(strings.Join(lines, "\n")+"\n"), &out); err != nil {
		t.Fatalf("serve: %v", err)
	}

	var got []map[string]any
	for _, line := range strings.Split(strings.TrimSpace(out.String()), "\n") {
		if line == "" {
			continue
		}
		var m map[string]any
		if err := json.Unmarshal([]byte(line), &m); err != nil {
			t.Fatalf("response %q: %v", line, err)
		}
		got = append(got, m)
	}
	return got
}

// toolText returns the text payload of a tools/call response.
func toolText(t *testing.T, resp map[string]any) string {
	t.Helper()
	result, ok := resp["result"].(map[string]any)
	if !ok {
		t.Fatalf("no result in %v", resp)
	}
	content, ok := result["content"].([]any)
	if !ok || len(content) == 0 {
		t.Fatalf("no content in %v", result)
	}
	first := content[0].(map[string]any)
	return first["text"].(string)
}

func TestMCPInitialize(t *testing.T) {
	testEnv(t)
	got := mcpSession(t, `{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":"2025-06-18"}}`)
	if len(got) != 1 {
		t.Fatalf("got %d responses", len(got))
	}
	result := got[0]["result"].(map[string]any)
	if result["protocolVersion"] != "2025-06-18" {
		t.Fatalf("protocolVersion = %v, want the client's", result["protocolVersion"])
	}
	if _, ok := result["capabilities"].(map[string]any)["tools"]; !ok {
		t.Fatalf("no tools capability: %v", result)
	}
	if !strings.Contains(result["instructions"].(string), "sanduk exec") {
		t.Fatalf("instructions should steer the agent to exec: %v", result["instructions"])
	}
	// serverInfo carries sanduk's build stamp, not the protocol version.
	info := result["serverInfo"].(map[string]any)
	if info["name"] != "sanduk" || info["version"] != version {
		t.Fatalf("serverInfo = %v, want sanduk %q", info, version)
	}
}

// A notification carries no id and must get no reply.
func TestMCPNotificationGetsNoResponse(t *testing.T) {
	testEnv(t)
	got := mcpSession(t, `{"jsonrpc":"2.0","method":"notifications/initialized"}`)
	if len(got) != 0 {
		t.Fatalf("a notification was answered: %v", got)
	}
}

func TestMCPToolsListIsReadOnly(t *testing.T) {
	testEnv(t)
	got := mcpSession(t, `{"jsonrpc":"2.0","id":1,"method":"tools/list"}`)
	tools := got[0]["result"].(map[string]any)["tools"].([]any)
	if len(tools) != 3 {
		t.Fatalf("got %d tools, want 3", len(tools))
	}
	names := map[string]bool{}
	for _, raw := range tools {
		tool := raw.(map[string]any)
		names[tool["name"].(string)] = true
	}
	for _, want := range []string{"list_entries", "describe_entry", "check_expiry"} {
		if !names[want] {
			t.Fatalf("missing tool %s: %v", want, names)
		}
	}
	// Nothing that reads a value or runs a command may be exposed.
	for name := range names {
		for _, forbidden := range []string{"get", "reveal", "exec", "run", "value"} {
			if strings.Contains(name, forbidden) {
				t.Fatalf("tool %q looks like it exposes secrets or execution", name)
			}
		}
	}
}

func TestMCPListEntriesReturnsNoValues(t *testing.T) {
	testEnv(t)
	if _, err := runCmd(t, "sk-test-secret-value", "add", "stripe", "--as", "STRIPE_KEY",
		"--description", "billing"); err != nil {
		t.Fatal(err)
	}
	if _, err := runCmd(t, `{"DB_URL":"postgres://sk-test-db"}`, "add-group", "warden",
		"--from-json", "-"); err != nil {
		t.Fatal(err)
	}

	got := mcpSession(t, `{"jsonrpc":"2.0","id":1,"method":"tools/call","params":{"name":"list_entries"}}`)
	text := toolText(t, got[0])

	for _, want := range []string{"stripe", "STRIPE_KEY", "billing", "warden", "group"} {
		if !strings.Contains(text, want) {
			t.Fatalf("list_entries = %s, missing %q", text, want)
		}
	}
	for _, secret := range []string{"sk-test-secret-value", "sk-test-db"} {
		if strings.Contains(text, secret) {
			t.Fatalf("list_entries leaked a value: %s", text)
		}
	}
}

func TestMCPDescribeEntryNamesVariablesOnly(t *testing.T) {
	testEnv(t)
	if _, err := runCmd(t, `{"DB_URL":"postgres://sk-test-db","API_KEY":"sk-test-api"}`,
		"add-group", "warden", "--from-json", "-"); err != nil {
		t.Fatal(err)
	}

	got := mcpSession(t,
		`{"jsonrpc":"2.0","id":1,"method":"tools/call","params":{"name":"describe_entry","arguments":{"name":"warden"}}}`)
	text := toolText(t, got[0])

	if !strings.Contains(text, "DB_URL") || !strings.Contains(text, "API_KEY") {
		t.Fatalf("describe_entry = %s", text)
	}
	for _, secret := range []string{"sk-test-db", "sk-test-api", "postgres://"} {
		if strings.Contains(text, secret) {
			t.Fatalf("describe_entry leaked a value: %s", text)
		}
	}
}

func TestMCPDescribeFileSecretNamesThePathVariable(t *testing.T) {
	testEnv(t)
	src := writeTemp(t, "sa.json", `{"private_key":"sk-test-file-value"}`)
	if _, err := runCmd(t, "", "add-file", "gcp", "--from-file", src,
		"--as", "GOOGLE_APPLICATION_CREDENTIALS"); err != nil {
		t.Fatal(err)
	}

	got := mcpSession(t,
		`{"jsonrpc":"2.0","id":1,"method":"tools/call","params":{"name":"describe_entry","arguments":{"name":"gcp"}}}`)
	text := toolText(t, got[0])

	if !strings.Contains(text, "GOOGLE_APPLICATION_CREDENTIALS") || !strings.Contains(text, "file") {
		t.Fatalf("describe_entry = %s", text)
	}
	if strings.Contains(text, "sk-test-file-value") {
		t.Fatalf("describe_entry leaked the file contents: %s", text)
	}
}

func TestMCPCheckExpiry(t *testing.T) {
	testEnv(t)
	if _, err := runCmd(t, "sk-test-value", "add", "stale", "--expires", future(-2)); err != nil {
		t.Fatal(err)
	}
	got := mcpSession(t,
		`{"jsonrpc":"2.0","id":1,"method":"tools/call","params":{"name":"check_expiry","arguments":{"within_days":7}}}`)
	text := toolText(t, got[0])
	if !strings.Contains(text, "stale") || !strings.Contains(text, "expired") {
		t.Fatalf("check_expiry = %s", text)
	}
}

func TestMCPUnknownToolIsAToolError(t *testing.T) {
	testEnv(t)
	got := mcpSession(t,
		`{"jsonrpc":"2.0","id":1,"method":"tools/call","params":{"name":"read_secret","arguments":{}}}`)
	result := got[0]["result"].(map[string]any)
	if result["isError"] != true {
		t.Fatalf("unknown tool should be a tool error: %v", result)
	}
}

func TestMCPUnknownMethodIsAProtocolError(t *testing.T) {
	testEnv(t)
	got := mcpSession(t, `{"jsonrpc":"2.0","id":1,"method":"resources/list"}`)
	errObj, ok := got[0]["error"].(map[string]any)
	if !ok {
		t.Fatalf("expected a protocol error: %v", got[0])
	}
	if int(errObj["code"].(float64)) != rpcMethodNotFound {
		t.Fatalf("code = %v", errObj["code"])
	}
}

func TestMCPMalformedLineIsAParseError(t *testing.T) {
	testEnv(t)
	got := mcpSession(t, `{not json`)
	errObj := got[0]["error"].(map[string]any)
	if int(errObj["code"].(float64)) != rpcParseError {
		t.Fatalf("code = %v", errObj["code"])
	}
}
