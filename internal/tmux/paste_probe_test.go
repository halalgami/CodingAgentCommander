package tmux

import "testing"

// Only a successful probe is cached: a probe that failed because the tmux
// server was momentarily down must not disable inserting for the app's life.
func TestCanPasteCachesOnlySuccess(t *testing.T) {
	orig := probePasteBuffers
	t.Cleanup(func() { probePasteBuffers = orig })
	results := []bool{false, true, false}
	calls := 0
	probePasteBuffers = func() bool { r := results[calls]; calls++; return r }

	h := NewExecHost()
	got := []bool{h.canPaste(), h.canPaste(), h.canPaste()}
	if got[0] || !got[1] || !got[2] {
		t.Errorf("canPaste sequence = %v, want [false true true]", got)
	}
	if calls != 2 {
		t.Errorf("probe ran %d times, want 2 (re-probe after failure, cached after success)", calls)
	}
}
