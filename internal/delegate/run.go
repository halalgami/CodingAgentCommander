package delegate

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"time"

	"github.com/halalgami/CodingAgentCommander/internal/proc"
)

// workerWaitDelay is how long cmd.Run waits for I/O to drain after the context
// is cancelled. Named so proxy_test.go can assert the stale-sweep threshold
// stays above a live run's worst-case budget.
const workerWaitDelay = 2 * time.Second

// claudeBin is the worker executable. A package variable so tests can point it
// at testdata/stub-claude and never touch the network.
var claudeBin = "claude"

// envelope is the subset of `claude -p --output-format json` we read.
// total_cost_usd is deliberately absent: it is fabricated for Ollama models.
type envelope struct {
	Subtype        string `json:"subtype"`
	IsError        bool   `json:"is_error"`
	APIErrorStatus int    `json:"api_error_status"`
	NumTurns       int    `json:"num_turns"`
	DurationMS     int64  `json:"duration_ms"`
	Result         string `json:"result"`
	Usage          struct {
		InputTokens  int `json:"input_tokens"`
		OutputTokens int `json:"output_tokens"`
	} `json:"usage"`
	// ModelUsage is the per-model breakdown, and it is NOT redundant: a live
	// run was observed reporting top-level usage of 0/0 on a successful
	// 3-turn answer while modelUsage held 11,250 in / 775 out. Without this
	// fallback the metrics silently read zero on those runs, which would
	// hollow out the runs log's whole cost column.
	//
	// Its costUSD field is deliberately NOT modelled: it is fabricated for
	// Ollama models (Claude pricing applied to a free model — the same run
	// was billed $0.075625).
	ModelUsage map[string]struct {
		InputTokens  int `json:"inputTokens"`
		OutputTokens int `json:"outputTokens"`
	} `json:"modelUsage"`
	PermissionDenials []struct {
		ToolName string `json:"tool_name"`
	} `json:"permission_denials"`
}

// tokens returns the run's cumulative token counts, preferring the top-level
// usage aggregate and falling back to summing modelUsage when it reads zero.
// See the ModelUsage field comment for the measured case that requires this.
func (e envelope) tokens() (in, out int) {
	if e.Usage.InputTokens != 0 || e.Usage.OutputTokens != 0 {
		return e.Usage.InputTokens, e.Usage.OutputTokens
	}
	for _, mu := range e.ModelUsage {
		in += mu.InputTokens
		out += mu.OutputTokens
	}
	return in, out
}

// StateEnv overrides where transcripts and generated proxy configs live.
//
// It exists because Run writes a transcript on EVERY call, so without it the
// test suite wrote into the real ~/.local/state/commander/delegate — 280 stray
// files in a day, several sharing a millisecond, some empty. app.go's
// COMMANDER_CONFIG is the same pattern for the same reason, and this package's
// TestMain pins this one for the whole test binary.
const StateEnv = "COMMANDER_STATE"

// StateDir is where transcripts and generated proxy configs live.
//
// Falls back to os.TempDir() (always absolute) rather than a relative default
// when the home directory cannot be resolved. A relative fallback would have
// MkdirAll scribble ".local/state/..." into the worker's current directory,
// and Result.Transcript would come back as a path the caller cannot resolve.
func StateDir() string {
	if p := os.Getenv(StateEnv); p != "" {
		return p
	}
	home, err := os.UserHomeDir()
	if err != nil || home == "" {
		return filepath.Join(os.TempDir(), "commander", "delegate")
	}
	return filepath.Join(home, ".local", "state", "commander", "delegate")
}

// Run executes one worker and returns a validated result. A non-nil error means
// the run could not be attempted; a failed run reports itself through
// Result.Status so the orchestrator sees one shape either way.
func Run(ctx context.Context, s Spec) (Result, error) {
	if err := os.MkdirAll(StateDir(), 0o755); err != nil {
		return Result{}, fmt.Errorf("create state dir: %w", err)
	}
	stamp := time.Now().UTC().Format("20060102-150405.000")
	if s.Schema == "" {
		s.Schema = string(SchemaJSON())
	}
	transcript := filepath.Join(StateDir(), stamp+"-"+s.Role.Name+".json")

	ctx, cancel := context.WithTimeout(ctx, time.Duration(s.Role.TimeoutSec)*time.Second)
	defer cancel()

	cmd := proc.Hide(exec.CommandContext(ctx, claudeBin, Argv(s)...))
	cmd.Dir = s.Root // explicit: decides what Grep/Glob see and what citations resolve against
	cmd.Env = Env(os.Environ(), s)
	setPgid(cmd)
	// CommandContext kills only the direct child; take the group instead.
	cmd.Cancel = func() error { return killGroup(cmd) }
	cmd.WaitDelay = workerWaitDelay

	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	started := time.Now()
	runErr := cmd.Run()
	elapsed := time.Since(started).Milliseconds()
	_ = os.WriteFile(transcript, stdout.Bytes(), 0o600)

	fail := func(status, why string) (Result, error) {
		return Result{
			Status: status, Answer: why, Citations: []Citation{},
			Model: s.WorkerModel, ElapsedMS: elapsed, Transcript: transcript,
		}, nil
	}

	// Unmarshal before checking the deadline: a killed worker can still have
	// written a complete envelope to stdout before the kill landed, and every
	// route out of Run must carry permission_denials — the only
	// machine-readable signal that a read-only worker attempted a write —
	// including the deadline route.
	var env envelope
	envErr := json.Unmarshal(stdout.Bytes(), &env)

	// Every failure from here on folds in permission_denials and token counts
	// when an envelope was parsed, so the assignment is not repeated at each
	// call site below. When envErr != nil, env is the zero value and this is a
	// no-op, so behaviour is unchanged when stdout held nothing parseable.
	failEnv := func(status, why string) (Result, error) {
		r, _ := fail(status, why)
		if envErr == nil {
			r.TokensIn, r.TokensOut = env.tokens()
			r.PermissionDenials = len(env.PermissionDenials)
		}
		return r, nil
	}

	if ctx.Err() == context.DeadlineExceeded {
		return failEnv(StatusFailedInfra, fmt.Sprintf("worker exceeded its %ds deadline and was killed", s.Role.TimeoutSec))
	}

	if envErr != nil {
		if runErr != nil {
			return fail(StatusFailedInfra, fmt.Sprintf("worker produced no parseable envelope: %v; stderr: %.400s", runErr, stderr.String()))
		}
		return fail(StatusFailedWork, fmt.Sprintf("worker envelope was not JSON: %.400s", stdout.String()))
	}

	// Key on the error fields, NEVER on Subtype: a hard API failure still
	// reports "subtype":"success" alongside is_error and api_error_status.
	if env.IsError || env.APIErrorStatus != 0 {
		return failEnv(StatusFailedInfra, fmt.Sprintf("upstream error (status %d): %.400s", env.APIErrorStatus, env.Result))
	}

	res, err := ParseResult(env.Result)
	if err != nil {
		return failEnv(StatusFailedWork, fmt.Sprintf("worker result did not match the schema: %v", err))
	}

	res.Citations = VerifyCitations(s.Root, res.Citations)
	// Measured, never trusted from the worker.
	res.Model = s.WorkerModel
	res.ElapsedMS = elapsed
	res.TokensIn, res.TokensOut = env.tokens()
	res.Transcript = transcript
	res.PermissionDenials = len(env.PermissionDenials)
	return res, nil
}
