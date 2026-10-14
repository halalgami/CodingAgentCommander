// Package autocont decides when a model that ended its turn right after
// announcing a step ("Now the implementation:") should be told to carry on.
// Measured on local transcripts (spec 2026-10-09-auto-continue-design.md): the
// rule catches 96% of GLM pauses with 3 false alarms in 1,881 Claude stops.
package autocont

import (
	"slices"
	"strconv"
	"strings"
	"sync"
)

// Nudge is the Stop-hook block reason; Claude Code hands it to the model. Its
// escape hatch ends in "?", which Detect never blocks.
const Nudge = "You announced your next step but ended your turn without doing it. Do it now — make the tool call in this turn. If you actually need input from the user, ask them a direct question instead."

// Limit is the runaway cap: auto-continues per prompt before giving up.
const Limit = 20

// MinHookVersion is the oldest Claude Code verified to run http hooks. 2.1.210
// also DROPS EVERY HOOK when one has an unknown type, so older builds must keep
// the curl Stop hook.
const MinHookVersion = "2.1.210"

const maxSessions = 256

// Detect reports whether msg reads as an announced-but-not-taken step.
func Detect(msg string) bool {
	s := strings.TrimSpace(msg)
	for {
		t := strings.TrimSpace(strings.TrimSuffix(s, "</think>"))
		t = strings.TrimRight(t, "*_` \t\n")
		if t == s {
			break
		}
		s = t
	}
	if s == "" || strings.HasSuffix(s, "?") || strings.Contains(strings.ToLower(s), "let me know") {
		return false
	}
	if strings.HasSuffix(s, ":") {
		return true
	}
	last := s
	if i := strings.LastIndexAny(strings.TrimRight(s, ".!"), ".!\n"); i >= 0 {
		last = strings.TrimSpace(s[i+1:])
	}
	return strings.HasPrefix(strings.ToLower(last), "let me ")
}

// HookReady reports whether a `claude --version` line is at least MinHookVersion.
func HookReady(version string) bool {
	have, ok := semver(version)
	want, _ := semver(MinHookVersion)
	if !ok {
		return false
	}
	for i := range have {
		if have[i] != want[i] {
			return have[i] > want[i]
		}
	}
	return true
}

func semver(s string) ([3]int, bool) {
	var v [3]int
	f, _, _ := strings.Cut(strings.TrimSpace(s), " ")
	parts := strings.Split(f, ".")
	if len(parts) != 3 {
		return v, false
	}
	for i, p := range parts {
		n, err := strconv.Atoi(p)
		if err != nil {
			return v, false
		}
		v[i] = n
	}
	return v, true
}

// Verdict is Tracker.Decide's answer for one Stop.
type Verdict int

const (
	Allow       Verdict = iota // a real stop
	Block                      // nudge the model
	GaveUpStuck                // paused again with no tool run since the last nudge
	GaveUpCap                  // Limit reached on this prompt
)

type entry struct {
	prompt   string
	count    int
	progress bool
	gaveUp   bool // gave up on this prompt: stay quiet until the next one
}

// Tracker counts auto-continues per Claude session and prompt.
type Tracker struct {
	mu    sync.Mutex
	m     map[string]*entry
	order []string // insertion order, for eviction
}

func NewTracker() *Tracker { return &Tracker{m: map[string]*entry{}} }

// Decide records one Stop. paused is Detect's verdict on its message.
func (t *Tracker) Decide(sessionID, promptID string, paused bool) Verdict {
	t.mu.Lock()
	defer t.mu.Unlock()
	e := t.m[sessionID]
	if e == nil {
		e = &entry{}
		t.m[sessionID] = e
		t.order = append(t.order, sessionID)
		t.evict()
	}
	if e.prompt != promptID {
		*e = entry{prompt: promptID}
	}
	switch {
	case !paused || e.gaveUp:
		return Allow
	case e.count > 0 && !e.progress:
		e.gaveUp = true
		return GaveUpStuck
	case e.count >= Limit:
		e.gaveUp = true
		return GaveUpCap
	}
	e.count++
	e.progress = false
	return Block
}

func (t *Tracker) evict() {
	for len(t.m) > maxSessions && len(t.order) > 0 {
		delete(t.m, t.order[0])
		t.order = t.order[1:]
	}
}

// Progress marks that the session ran a tool since its last nudge.
func (t *Tracker) Progress(sessionID string) {
	t.mu.Lock()
	defer t.mu.Unlock()
	if e := t.m[sessionID]; e != nil {
		e.progress = true
	}
}

// Reset starts a fresh count: the user sent a new prompt.
func (t *Tracker) Reset(sessionID string) {
	t.mu.Lock()
	defer t.mu.Unlock()
	if e := t.m[sessionID]; e != nil {
		*e = entry{}
	}
}

// Drop forgets the session (window killed or relaunched), including its place
// in the eviction order, so a later re-add counts as new.
func (t *Tracker) Drop(sessionID string) {
	t.mu.Lock()
	defer t.mu.Unlock()
	delete(t.m, sessionID)
	t.order = slices.DeleteFunc(t.order, func(k string) bool { return k == sessionID })
}

// Count is the number of auto-continues on the session's current prompt.
func (t *Tracker) Count(sessionID string) int {
	t.mu.Lock()
	defer t.mu.Unlock()
	if e := t.m[sessionID]; e != nil {
		return e.count
	}
	return 0
}
