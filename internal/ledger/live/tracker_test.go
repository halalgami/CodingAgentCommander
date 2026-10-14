package live

import (
	"sync"
	"testing"
	"time"

	"github.com/halalgami/CodingAgentCommander/internal/ledger/parse"
)

func TestTrackerApplyHookSnapshotExpire(t *testing.T) {
	var mu sync.Mutex
	var changes []Session
	tr := New(func(s Session) { mu.Lock(); changes = append(changes, s); mu.Unlock() })

	tr.Apply([]parse.Event{{Kind: parse.Prompt, At: 1000, SessionID: "a"}, {Kind: parse.Prompt, At: 1000, SessionID: "b"}}, true)
	tr.Hook("a", HookStop, 2000)
	tr.Hook("unknown", HookStop, 2000) // ignored: never seen

	if s, _ := tr.Get("a"); s.State != Finished || !s.External {
		t.Fatalf("a = %+v", s)
	}
	if len(tr.Snapshot()) != 2 {
		t.Fatal("snapshot size")
	}
	mu.Lock()
	n := len(changes)
	mu.Unlock()
	if n != 3 {
		t.Fatalf("changes = %d, want 3 (a, b, a-stop)", n)
	}
	tr.Expire(1000+int64(16*time.Minute/time.Millisecond), 15*time.Minute)
	if len(tr.Snapshot()) != 0 {
		t.Fatal("external sessions idle past the window must expire")
	}
}

func TestTrackerCommanderSessionsNeverExpire(t *testing.T) {
	tr := New(nil)
	tr.Apply([]parse.Event{{Kind: parse.Prompt, At: 1, SessionID: "c"}}, false)
	tr.Expire(1<<40, time.Minute)
	if _, ok := tr.Get("c"); !ok {
		t.Fatal("commander session expired")
	}
}
