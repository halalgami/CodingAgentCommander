// Package askai is the only place Al Asturlab calls a model (spec §7). It runs
// a headless `claude -p` with no tools, no settings and a JSON schema, in an
// empty directory, and returns the structured object. Callers decide when to
// run it; the owner's rule is: only on a user click.
package askai

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"time"

	"github.com/halalgami/CodingAgentCommander/internal/proc"
)

// Timeout bounds one model call.
const Timeout = 90 * time.Second

type Runner interface {
	Run(ctx context.Context, model, prompt, schema string) (json.RawMessage, error)
}

// Argv builds the claude argv. The prompt goes on stdin, never argv: on
// Windows an npm .cmd shim runs through cmd.exe, which cuts argv at the first
// newline (dropping the flags after it) and does not honour quoting.
func Argv(model, schema string) []string {
	return []string{
		"-p",
		"--model", model,
		"--output-format", "json",
		"--setting-sources", "", // no hooks, no MCP, no project settings
		"--tools", "", // the model can run nothing
		"--strict-mcp-config",          // and no MCP servers from ~/.claude.json
		"--permission-mode", "default", // anything that slips through is denied in -p
		"--no-session-persistence", // no transcript, so the ledger never sees these calls
		"--max-turns", "3",         // the structured-output tool counts as a turn
		"--json-schema", schema,
	}
}

type envelope struct {
	IsError          bool            `json:"is_error"`
	APIErrorStatus   int             `json:"api_error_status"`
	Result           string          `json:"result"`
	StructuredOutput json.RawMessage `json:"structured_output"`
}

// Decode pulls the structured object out of claude's JSON envelope.
func Decode(stdout []byte) (json.RawMessage, error) {
	var env envelope
	if err := json.Unmarshal(bytes.TrimSpace(stdout), &env); err != nil {
		return nil, fmt.Errorf("model output was not JSON: %.200s", stdout)
	}
	if env.IsError || env.APIErrorStatus != 0 {
		return nil, fmt.Errorf("model error (status %d): %.300s", env.APIErrorStatus, env.Result)
	}
	if len(env.StructuredOutput) > 0 && string(env.StructuredOutput) != "null" {
		return env.StructuredOutput, nil
	}
	s := strings.TrimSpace(env.Result)
	s = strings.TrimPrefix(s, "```json")
	s = strings.TrimPrefix(s, "```")
	s = strings.TrimSuffix(strings.TrimSpace(s), "```")
	s = strings.TrimSpace(s)
	if json.Valid([]byte(s)) && strings.HasPrefix(s, "{") {
		return json.RawMessage(s), nil
	}
	return nil, errors.New("the model gave no structured answer")
}

// Claude runs the real CLI. Bin "" means "claude" on PATH.
type Claude struct{ Bin string }

func (c Claude) Run(ctx context.Context, model, prompt, schema string) (json.RawMessage, error) {
	bin := c.Bin
	if bin == "" {
		bin = "claude"
	}
	dir, err := os.MkdirTemp("", "commander-askai-")
	if err != nil {
		return nil, err
	}
	defer os.RemoveAll(dir)
	ctx, cancel := context.WithTimeout(ctx, Timeout)
	defer cancel()
	cmd := proc.Hide(exec.CommandContext(ctx, bin, Argv(model, schema)...))
	cmd.Stdin = strings.NewReader(prompt)
	cmd.Dir = dir
	cmd.Env = cleanEnv(os.Environ())
	var out, errb bytes.Buffer
	cmd.Stdout, cmd.Stderr = &out, &errb
	runErr := cmd.Run()
	if ctx.Err() == context.DeadlineExceeded {
		return nil, fmt.Errorf("the model took longer than %s", Timeout)
	}
	raw, err := Decode(out.Bytes())
	if err != nil && runErr != nil && out.Len() == 0 {
		return nil, fmt.Errorf("could not run claude: %v %.200s", runErr, errb.String())
	}
	return raw, err
}

// cleanEnv drops the variables that mark a process as running inside a Claude
// Code session, so a Commander launched from one does not hand the child that
// identity. Auth and provider variables (CLAUDE_CODE_OAUTH_TOKEN,
// CLAUDE_CODE_USE_BEDROCK, …) stay: the user may sign in that way.
func cleanEnv(parent []string) []string {
	out := make([]string, 0, len(parent))
	for _, kv := range parent {
		k, _, _ := strings.Cut(kv, "=")
		if k == "CLAUDECODE" || k == "CLAUDE_CODE_ENTRYPOINT" || k == "CLAUDE_CODE_SSE_PORT" {
			continue
		}
		out = append(out, kv)
	}
	return out
}
