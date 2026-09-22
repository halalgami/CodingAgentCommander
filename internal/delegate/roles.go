// Package delegate runs one scoped, read-only Ollama Cloud worker per call and
// returns a validated result. See
// docs/superpowers/specs/2026-09-03-ollama-subagent-delegation-design.md R2.
package delegate

import (
	"fmt"
	"strings"
)

// Role is a delegation profile. Phase 1 keeps these as Go constants rather than
// config.toml entries: config.Config has no Roles field and config.Save
// rewrites the file from the struct, so a hand-written [[roles]] section would
// be deleted on the next model add.
type Role struct {
	Name           string
	Model          string // catalog ID of an ollama-cloud model, e.g. "ollama-glm-5.3"
	Tools          []string
	PermissionMode string
	MaxTurns       int
	TimeoutSec     int
	Effort         string
	PromptAppend   string
}

// Roles returns the built-in profiles. Model choices come from the four-arm
// measurement in spec §4.4: glm was fastest-correct, qwen most thorough.
func Roles() []Role {
	const cite = "Report findings with file:line citations, quoting the line you cite. Never guess a path or a line number."
	return []Role{
		{
			Name: "scout", Model: "ollama-glm-5.3",
			Tools:          []string{"Read", "Grep", "Glob"},
			PermissionMode: "default",
			// MaxTurns bounds a worker that would otherwise run away: glm took
			// 44 turns on an unbounded edit task. TimeoutSec exceeds the
			// measured 246s tail so a slow-but-healthy run is not killed, and
			// exceeds the proxy's request_timeout (Task 4) so an upstream
			// stall surfaces as an upstream error rather than our own kill.
			MaxTurns: 20, TimeoutSec: 300, Effort: "low",
			PromptAppend: cite,
		},
		{
			Name: "reviewer", Model: "ollama-qwen3.5:397b",
			Tools:          []string{"Read", "Grep", "Glob"},
			PermissionMode: "default",
			MaxTurns:       30, TimeoutSec: 300, Effort: "low",
			PromptAppend: cite + " State explicitly what you could not determine.",
		},
	}
}

// RoleByName looks up a built-in role.
func RoleByName(name string) (Role, error) {
	var names []string
	for _, r := range Roles() {
		if r.Name == name {
			return r, nil
		}
		names = append(names, r.Name)
	}
	return Role{}, fmt.Errorf("refusing to run unknown role %q; valid roles: %s", name, strings.Join(names, ", "))
}
