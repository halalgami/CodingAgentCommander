package delegate

import (
	"encoding/json"
	"strconv"
	"strings"
	"testing"
)

func testSpec() Spec {
	return Spec{
		Role:        Roles()[0],
		Task:        "find the thing",
		Root:        "/repo",
		ProxyURL:    "http://localhost:4711",
		MasterKey:   "sk-local",
		WorkerModel: "ollama-glm-5.3-oai",
		Schema:      `{"type":"object"}`,
	}
}

func argIndex(a []string, want string) int {
	for i, v := range a {
		if v == want {
			return i
		}
	}
	return -1
}

func TestArgvTaskFollowsDashP(t *testing.T) {
	a := Argv(testSpec())
	i := argIndex(a, "-p")
	if i < 0 || a[i+1] != "find the thing" {
		t.Fatalf("task must sit immediately after -p; got %v", a)
	}
}

func TestArgvNeverBare(t *testing.T) {
	// --bare deletes the harness this architecture exists to reuse (spec §4.5).
	if argIndex(Argv(testSpec()), "--bare") >= 0 {
		t.Error("--bare must never be passed")
	}
}

func TestArgvScopesWithToolsNotAllowedTools(t *testing.T) {
	a := Argv(testSpec())
	if argIndex(a, "--allowedTools") >= 0 {
		t.Error("--allowedTools scopes nothing (spec §4.6); use --tools")
	}
	i := argIndex(a, "--tools")
	if i < 0 {
		t.Fatal("missing --tools")
	}
	// One comma-joined element, so the variadic flag cannot swallow later args.
	if a[i+1] != "Read,Grep,Glob" {
		t.Errorf("--tools value = %q, want Read,Grep,Glob", a[i+1])
	}
}

func TestArgvCarriesLimitsAndSchema(t *testing.T) {
	s := testSpec()
	a := Argv(s)
	// Presence alone would not have caught the --json-schema regression this
	// branch already hit once (a temp-file path instead of inline JSON, see
	// TestArgvJSONSchemaValueIsInlineJSONNotAPath): assert each flag's VALUE
	// matches the spec/role it was built from.
	wantValues := map[string]string{
		"--model":         s.WorkerModel,
		"--output-format": "json",
		// Empty, not "project": cmd.Dir is the scanned root, so "project" meant
		// the REPO UNDER REVIEW supplied the settings — and its hooks ran. See
		// TestArgvLoadsNoSettingSources for the reasoning this assertion used to
		// point the wrong way.
		"--setting-sources": "",
		"--permission-mode": s.Role.PermissionMode,
		"--effort":          s.Role.Effort,
		"--max-turns":       strconv.Itoa(s.Role.MaxTurns),
	}
	for flag, want := range wantValues {
		i := argIndex(a, flag)
		if i < 0 {
			t.Errorf("missing %s in %v", flag, a)
			continue
		}
		if got := a[i+1]; got != want {
			t.Errorf("%s = %q, want %q", flag, got, want)
		}
	}
	if i := argIndex(a, "--json-schema"); i < 0 || a[i+1] != s.Schema {
		t.Errorf("--json-schema value = %v, want %q", a, s.Schema)
	}
}

// TestArgvJSONSchemaValueIsInlineJSONNotAPath is a regression test: Claude
// Code's --json-schema flag takes the schema JSON inline, not a file path
// (there is no --json-schema-file variant). Merely asserting the flag is
// present would not have caught the real defect, where the value passed was
// a temp-file path that failed to parse as JSON at run time; the assertion
// must be on the value's validity.
func TestArgvJSONSchemaValueIsInlineJSONNotAPath(t *testing.T) {
	a := Argv(testSpec())
	i := argIndex(a, "--json-schema")
	if i < 0 {
		t.Fatal("missing --json-schema")
	}
	value := a[i+1]
	var parsed map[string]any
	if err := json.Unmarshal([]byte(value), &parsed); err != nil {
		t.Fatalf("--json-schema value must be inline JSON, not a path: %q does not parse as JSON: %v", value, err)
	}
}

func TestArgvPathsBecomeAddDir(t *testing.T) {
	s := testSpec()
	s.Paths = []string{"internal/router", "app.go"}
	a := Argv(s)
	if n := strings.Count(strings.Join(a, " "), "--add-dir"); n != 2 {
		t.Errorf("got %d --add-dir flags, want 2: paths must be delivered as flags, not prose", n)
	}
}

func TestEnvStripsParentAuth(t *testing.T) {
	parent := []string{
		"PATH=/usr/bin",
		"ANTHROPIC_API_KEY=sk-parent",
		"ANTHROPIC_AUTH_TOKEN=oauth-parent",
		"ANTHROPIC_BASE_URL=https://api.anthropic.com",
		"CLAUDE_CODE_OAUTH_TOKEN=oauth2-parent",
	}
	got := Env(parent, testSpec())
	joined := strings.Join(got, "\n")
	for _, leak := range []string{"sk-parent", "oauth-parent", "oauth2-parent", "api.anthropic.com"} {
		if strings.Contains(joined, leak) {
			t.Errorf("parent value %q survived into worker env: a worker inheriting parent auth spends Opus quota", leak)
		}
	}
	if !strings.Contains(joined, "PATH=/usr/bin") {
		t.Error("unrelated parent env must survive")
	}
	for _, want := range []string{
		"ANTHROPIC_BASE_URL=http://localhost:4711",
		"ANTHROPIC_AUTH_TOKEN=sk-local",
		"ANTHROPIC_MODEL=ollama-glm-5.3-oai",
		"CLAUDE_CODE_MAX_RETRIES=0",
	} {
		if !strings.Contains(joined, want) {
			t.Errorf("missing %q", want)
		}
	}
}

// The worker's cwd is the scanned root, so any setting source that resolves
// relative to it resolves INSIDE the repo under review. "project" did exactly
// that, and a project settings file's `hooks` block runs shell commands before
// the first model turn — unconstrained by --tools, --permission-mode or
// --max-turns. Reproduced against claude 2.1.210; this pins the value closed.
func TestArgvLoadsNoSettingSources(t *testing.T) {
	role, err := RoleByName("scout")
	if err != nil {
		t.Fatal(err)
	}
	a := Argv(Spec{Task: "t", WorkerModel: "m", Role: role})
	for i, v := range a {
		if v != "--setting-sources" {
			continue
		}
		if i+1 >= len(a) {
			t.Fatal("--setting-sources has no value")
		}
		if got := a[i+1]; got != "" {
			t.Fatalf("--setting-sources %q: anything but the empty value lets a scanned repo supply hooks", got)
		}
		return
	}
	t.Fatal("--setting-sources absent: the default loads project settings from the scanned repo")
}
