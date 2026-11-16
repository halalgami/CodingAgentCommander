package askai

import (
	"context"
	"os"
	"path/filepath"
	"runtime"
	"slices"
	"strings"
	"testing"
)

func pairAt(a []string, k, v string) bool {
	i := slices.Index(a, k)
	return i >= 0 && i+1 < len(a) && a[i+1] == v
}

func TestArgvNoToolsNoSettings(t *testing.T) {
	a := Argv("haiku", "{}")
	if a[0] != "-p" {
		t.Fatalf("-p first: %v", a)
	}
	// The prompt goes on stdin, never argv (Windows .cmd shims split argv at newlines).
	for _, f := range []string{"--no-session-persistence", "--strict-mcp-config"} {
		if !slices.Contains(a, f) {
			t.Fatalf("missing %s in %v", f, a)
		}
	}
	for _, kv := range [][2]string{
		{"--model", "haiku"}, {"--output-format", "json"}, {"--setting-sources", ""},
		{"--tools", ""}, {"--max-turns", "3"}, {"--json-schema", "{}"}, {"--permission-mode", "default"},
	} {
		if !pairAt(a, kv[0], kv[1]) {
			t.Fatalf("missing %s %q in %v", kv[0], kv[1], a)
		}
	}
}

func TestDecode(t *testing.T) {
	cases := []struct {
		name, in, want, err string
	}{
		{"structured", `{"result":"x","structured_output":{"a":1}}`, `{"a":1}`, ""},
		{"result text", `{"result":"{\"a\":2}"}`, `{"a":2}`, ""},
		{"fenced result", "{\"result\":\"```json\\n{\\\"a\\\":3}\\n```\"}", `{"a":3}`, ""},
		{"is_error", `{"is_error":true,"result":"Credit balance is too low"}`, "", "Credit balance"},
		{"api status", `{"api_error_status":429,"result":"rate"}`, "", "429"},
		{"not json", `hello`, "", "not JSON"},
		{"result not json", `{"result":"sorry, no"}`, "", "no structured answer"},
	}
	for _, c := range cases {
		got, err := Decode([]byte(c.in))
		if c.err != "" {
			if err == nil || !strings.Contains(err.Error(), c.err) {
				t.Fatalf("%s: err %v, want containing %q", c.name, err, c.err)
			}
			continue
		}
		if err != nil || string(got) != c.want {
			t.Fatalf("%s: got %s %v, want %s", c.name, got, err, c.want)
		}
	}
}

func TestClaudeRunsInEmptyDirWithoutClaudeEnv(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("shell fake")
	}
	dir := t.TempDir()
	bin := filepath.Join(dir, "fake-claude")
	// Prints the cwd listing and whether CLAUDE_CODE_FOO leaked, as structured output.
	os.WriteFile(bin, []byte(`#!/bin/sh
n=$(ls -A | wc -l | tr -d ' ')
in=$(cat)
printf '{"structured_output":{"files":%s,"leak":"%s","tok":"%s","stdin":"%s"}}' "$n" "$CLAUDECODE$CLAUDE_CODE_ENTRYPOINT" "$CLAUDE_CODE_OAUTH_TOKEN" "$in"
`), 0o755)
	t.Setenv("CLAUDECODE", "1")
	t.Setenv("CLAUDE_CODE_ENTRYPOINT", "cli")
	t.Setenv("CLAUDE_CODE_OAUTH_TOKEN", "tok")
	out, err := Claude{Bin: bin}.Run(context.Background(), "haiku", "PROMPT & more", "{}")
	if err != nil {
		t.Fatal(err)
	}
	if string(out) != `{"files":0,"leak":"","tok":"tok","stdin":"PROMPT & more"}` {
		t.Fatalf("got %s", out)
	}
}
