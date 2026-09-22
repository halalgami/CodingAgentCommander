package delegate

import (
	"strconv"
	"strings"
)

// Spec is everything one worker run needs. Built by Run's caller; Argv and Env
// are pure functions of it so the security-critical parts are unit-testable
// without spawning anything.
type Spec struct {
	Role        Role
	Task        string   // must be self-contained: the worker sees no history
	Paths       []string // optional hint, delivered as --add-dir
	Root        string   // worker cwd
	ProxyURL    string
	MasterKey   string
	WorkerModel string // LiteLLM model_name, e.g. "ollama-glm-5.3-oai"
	Schema      string // JSON Schema text; Claude Code's --json-schema takes inline JSON, not a path
}

// Argv builds the claude argv. The task sits immediately after -p because
// --tools is variadic and would otherwise swallow a positional prompt.
func Argv(s Spec) []string {
	a := []string{
		"-p", s.Task,
		"--model", s.WorkerModel,
		"--output-format", "json",
		// Load NO settings at all. This used to be "project", to drop the
		// operator's global hooks and plugins (which inflated the edit spike) —
		// but Run sets cmd.Dir to the scanned root, so "project" resolved to
		// <root>/.claude/settings.json: the settings file of the very repo the
		// worker was pointed at. Its `hooks` block runs arbitrary shell commands,
		// fires before the first model turn (so a dead upstream does not save
		// us), and is not constrained by --tools, --permission-mode or
		// --max-turns. `env` and enableAllProjectMcpServers are two more routes
		// to the same place. Reproduced against claude 2.1.210 and re-verified
		// closed with this value; see TestArgvLoadsNoSettingSources.
		//
		// The empty value keeps the original goal — the operator's global hooks
		// stay out — while giving the repo no say either. Read/Grep/Glob need no
		// project settings, so nothing is lost.
		"--setting-sources", "",
		"--permission-mode", s.Role.PermissionMode,
		"--effort", s.Role.Effort,
		"--max-turns", strconv.Itoa(s.Role.MaxTurns),
		// --tools is the ONLY flag that restricts availability. --allowedTools
		// is a permission allowlist and does not filter the schema at all.
		// Comma-joined into one element so the variadic flag ends here.
		"--tools", strings.Join(s.Role.Tools, ","),
		"--json-schema", s.Schema,
	}
	if s.Role.PromptAppend != "" {
		a = append(a, "--append-system-prompt", s.Role.PromptAppend)
	}
	for _, p := range s.Paths {
		a = append(a, "--add-dir", p)
	}
	return a
}

// Env constructs the worker environment. It never inherits the parent's
// Anthropic or Claude Code variables: a worker that picks up the parent's
// subscription OAuth spends the quota this feature exists to protect.
func Env(parent []string, s Spec) []string {
	out := make([]string, 0, len(parent)+4)
	for _, kv := range parent {
		k, _, _ := strings.Cut(kv, "=")
		if strings.HasPrefix(k, "ANTHROPIC_") || strings.HasPrefix(k, "CLAUDE_CODE_") {
			continue
		}
		out = append(out, kv)
	}
	return append(out,
		"ANTHROPIC_BASE_URL="+s.ProxyURL,
		"ANTHROPIC_AUTH_TOKEN="+s.MasterKey,
		"ANTHROPIC_MODEL="+s.WorkerModel,
		// Claude Code is the retrier behind the measured 244s/249s stalls;
		// LiteLLM's num_retries never controlled them (spec §4.7).
		"CLAUDE_CODE_MAX_RETRIES=0",
	)
}
