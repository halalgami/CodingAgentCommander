package delegate

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// Result statuses. failed_work means the worker ran and could not finish — the
// orchestrator's call. failed_infra means transport, auth or timeout: every
// role shares the transport, so retrying another role is useless.
const (
	StatusOK          = "ok"
	StatusNoFindings  = "no_findings"
	StatusFailedWork  = "failed_work"
	StatusFailedInfra = "failed_infra"
)

// Citation is a claim with provenance. Verified is set by VerifyCitations, never
// by the worker: a worker cannot quote a line it never read, which makes this
// the only mechanical defence against fabricated provenance. It cannot catch a
// fabricated conclusion.
type Citation struct {
	File     string `json:"file"`
	Line     int    `json:"line"`
	Quote    string `json:"quote"`
	Verified bool   `json:"verified"`
}

// Result is what the orchestrator receives. The worker supplies only Status,
// Answer, Citations and Caveats; everything below Caveats is measured by us.
type Result struct {
	Status    string     `json:"status"`
	Answer    string     `json:"answer"`
	Citations []Citation `json:"citations"`
	Caveats   string     `json:"caveats,omitempty"`

	Model             string `json:"model,omitempty"`
	ElapsedMS         int64  `json:"elapsed_ms,omitempty"`
	TokensIn          int    `json:"tokens_in,omitempty"`
	TokensOut         int    `json:"tokens_out,omitempty"`
	Transcript        string `json:"transcript,omitempty"`
	PermissionDenials int    `json:"permission_denials,omitempty"`
}

// SchemaJSON is passed to `claude --json-schema`. It survives the LiteLLM
// bridge because Claude Code implements it as an extra tool named
// StructuredOutput rather than as response_format (spec §4.9).
func SchemaJSON() []byte {
	return []byte(`{
  "type": "object",
  "properties": {
    "status": {"type": "string", "enum": ["ok", "no_findings", "failed_work"]},
    "answer": {"type": "string"},
    "citations": {
      "type": "array",
      "items": {
        "type": "object",
        "properties": {
          "file": {"type": "string"},
          "line": {"type": "integer"},
          "quote": {"type": "string"}
        },
        "required": ["file", "line", "quote"]
      }
    },
    "caveats": {"type": "string"}
  },
  "required": ["status", "answer", "citations"]
}`)
}

// workerFields is the subset a worker may set. Decoding into this and copying
// across is what discards worker-supplied metrics.
type workerFields struct {
	Status    string     `json:"status"`
	Answer    string     `json:"answer"`
	Citations []Citation `json:"citations"`
	Caveats   string     `json:"caveats"`
}

// ParseResult decodes a worker payload, tolerating a markdown code fence.
func ParseResult(raw string) (Result, error) {
	s := strings.TrimSpace(raw)
	if strings.HasPrefix(s, "```") {
		if i := strings.Index(s, "\n"); i >= 0 {
			s = s[i+1:]
		}
		s = strings.TrimSuffix(strings.TrimSpace(s), "```")
	}
	var w workerFields
	if err := json.Unmarshal([]byte(strings.TrimSpace(s)), &w); err != nil {
		return Result{}, fmt.Errorf("decode worker result: %w", err)
	}
	switch w.Status {
	case StatusOK, StatusNoFindings, StatusFailedWork:
	default:
		return Result{}, fmt.Errorf("refusing to accept status %q: not one of ok, no_findings, failed_work", w.Status)
	}
	// Verified is never trusted from the wire.
	cs := make([]Citation, 0, len(w.Citations))
	for _, c := range w.Citations {
		cs = append(cs, Citation{File: c.File, Line: c.Line, Quote: c.Quote})
	}
	return Result{Status: w.Status, Answer: w.Answer, Citations: cs, Caveats: w.Caveats}, nil
}

// VerifyCitations checks each quote against the line it names, inside root.
// A mismatch is flagged, never dropped: discarding a good answer over one
// sloppy quote is worse than annotating it.
func VerifyCitations(root string, cs []Citation) []Citation {
	out := make([]Citation, 0, len(cs))
	for _, c := range cs {
		c.Verified = verifyOne(root, c)
		out = append(out, c)
	}
	return out
}

// verifyOne never returns an error: any failure — a bad root, a missing file,
// a symlink escape, an out-of-range line, an empty quote — collapses to a
// plain `verified: false`. This mirrors docPath (projectdocs_read.go), which
// solves the same containment problem for the doc viewer.
//
// Containment is checked in two places because os.ReadFile follows symlinks
// and a lexical prefix check on the joined path does not see through them:
//   - os.Lstat refuses the final path element if IT is a symlink (a file
//     inside root pointing outside).
//   - filepath.EvalSymlinks resolves the rest of the path, and the
//     containment check runs again on the RESOLVED path (a symlinked parent
//     directory inside root pointing outside).
//
// Neither guard alone is enough: Lstat only inspects the last element, and a
// containment check without EvalSymlinks never sees a resolved parent that
// left root.
// minQuoteLen is the shortest quote that carries enough signal to be evidence.
// Below this, Contains stops discriminating: " " trimmed to "" matched every
// line in the file, and a single "e" matches almost as many. The flag is
// documented as the only mechanical defence against fabricated provenance, and
// runlog counts it as the paraphrase tripwire, so a quote that cannot fail the
// check is worse than no check — it launders a fabrication as confirmed.
const minQuoteLen = 8

func verifyOne(root string, c Citation) bool {
	// Trim BEFORE the emptiness test, not after. Testing the raw string and
	// comparing the trimmed one is what let a whitespace-only quote through.
	quote := strings.TrimSpace(c.Quote)
	if len(quote) < minQuoteLen || c.Line < 1 || filepath.IsAbs(c.File) {
		return false
	}
	absRoot, err := filepath.Abs(root)
	if err != nil {
		return false
	}
	realRoot, err := filepath.EvalSymlinks(absRoot)
	if err != nil {
		return false
	}
	abs := filepath.Clean(filepath.Join(realRoot, filepath.Clean("/"+c.File)))
	// Lstat does NOT follow the final symlink, unlike Stat. That is what makes
	// a symlinked file a refusal rather than a read.
	fi, err := os.Lstat(abs)
	if err != nil {
		return false
	}
	if fi.Mode()&os.ModeSymlink != 0 {
		return false
	}
	real, err := filepath.EvalSymlinks(abs)
	if err != nil {
		return false
	}
	if err := verifyContained(realRoot, real); err != nil {
		return false
	}
	b, err := os.ReadFile(real)
	if err != nil {
		return false
	}
	lines := strings.Split(string(b), "\n")
	if c.Line > len(lines) {
		return false
	}
	// Whitespace-tolerant, content-strict: no fuzzy matching. A quote may be a
	// fragment of the line, so containment rather than equality.
	return strings.Contains(strings.TrimSpace(lines[c.Line-1]), quote)
}

// verifyContained compares paths rather than strings: filepath.Rel is the
// check, and a leading ".." is the failure.
func verifyContained(root, path string) error {
	rel, err := filepath.Rel(root, path)
	if err != nil {
		return err
	}
	if rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return fmt.Errorf("refusing to verify: %s resolves outside %s", path, root)
	}
	return nil
}
