package delegate

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"
)

// Phase 1 of delegation exists to answer one question: does the orchestrator
// reach for this tool unprompted? That answer decides whether phase 2 is built
// or the CLI is deleted, so it has to be data rather than recollection a week
// later. Every run appends one line here.
//
// The heuristic below is what makes the line worth reading: a run Claude Code's
// Bash tool started is the only kind that counts, and it is distinguishable
// because that tool hands the child a pipe for stdout while a human at a
// terminal hands it a character device.
const (
	InvokerAgent = "agent"
	InvokerHuman = "human"
)

// InvokerFor classifies a run from stdout's file mode. It is a heuristic, and
// it is wrong in two directions, both measured rather than assumed:
//
//   - A human who pipes or redirects to a FILE reads as agent-invoked.
//   - An agent whose output is discarded to /dev/null reads as human-invoked,
//     because /dev/null is itself a character device.
//
// Neither matters for the case being counted. Claude Code's Bash tool captures
// stdout, which is always a pipe, and an orchestrator that discarded the answer
// would have no reason to call this at all. So the classification is reliable
// exactly where the measurement depends on it, and the noise sits in usage
// patterns that do not occur — stated here rather than left for someone to
// rediscover from a surprising log line.
func InvokerFor(mode os.FileMode) string {
	if mode&os.ModeCharDevice != 0 {
		return InvokerHuman
	}
	return InvokerAgent
}

// RunRecord is one line of the runs log. Deliberately flat and small so a week
// of it can be read with wc, grep and jq rather than a tool.
type RunRecord struct {
	At                string `json:"at"`
	Role              string `json:"role"`
	Model             string `json:"model"`
	Status            string `json:"status"`
	Invoker           string `json:"invoker"`
	ElapsedMS         int64  `json:"elapsed_ms"`
	TokensIn          int    `json:"tokens_in"`
	TokensOut         int    `json:"tokens_out"`
	Citations         int    `json:"citations"`
	Verified          int    `json:"verified"`
	PermissionDenials int    `json:"permission_denials"`
}

// NewRunRecord summarises a finished run. Citations are counted rather than
// copied: the transcript already holds the detail, and the log is for spotting
// a trend — in particular a verified count that keeps falling short of the
// total, which is the signal that a model is paraphrasing its provenance.
func NewRunRecord(role string, r Result, invoker string, at time.Time) RunRecord {
	verified := 0
	for _, c := range r.Citations {
		if c.Verified {
			verified++
		}
	}
	return RunRecord{
		At:                at.UTC().Format(time.RFC3339),
		Role:              role,
		Model:             r.Model,
		Status:            r.Status,
		Invoker:           invoker,
		ElapsedMS:         r.ElapsedMS,
		TokensIn:          r.TokensIn,
		TokensOut:         r.TokensOut,
		Citations:         len(r.Citations),
		Verified:          verified,
		PermissionDenials: r.PermissionDenials,
	}
}

// RunLogPath is the append-only log of every delegate run.
func RunLogPath() string { return filepath.Join(StateDir(), "runs.jsonl") }

// AppendRunTo appends one record to the named log, creating it and its parents
// if needed. Append-only on purpose: the count across a week IS the
// measurement, so a later run must never truncate an earlier one.
func AppendRunTo(path string, rec RunRecord) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return fmt.Errorf("create run log dir: %w", err)
	}
	line, err := json.Marshal(rec)
	if err != nil {
		return fmt.Errorf("marshal run record: %w", err)
	}
	f, err := os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o600)
	if err != nil {
		return fmt.Errorf("open run log: %w", err)
	}
	defer f.Close()
	if _, err := f.Write(append(line, '\n')); err != nil {
		return fmt.Errorf("write run log: %w", err)
	}
	return nil
}
