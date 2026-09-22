package transcripts

import (
	"bufio"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

// oldScan is the pre-rewrite implementation, verbatim, kept only as the oracle
// for the parity test below.
func oldStats(path string) (int, int, bool) {
	f, _ := os.Open(path)
	defer f.Close()
	last, n, found := 0, 0, false
	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 0, 1024*1024), 16*1024*1024)
	for sc.Scan() {
		var l line
		if json.Unmarshal(sc.Bytes(), &l) != nil {
			continue
		}
		if l.Type == "assistant" {
			n++
			if l.Message.Usage != nil {
				u := l.Message.Usage
				last = u.InputTokens + u.CacheReadInputTokens + u.CacheCreationInputTokens
				found = true
			}
		}
	}
	return last, n, found
}

// The prefilter and the single pass must not change a single answer. Run against
// every real transcript on this machine, not a fixture.
func TestScanMatchesOldImplementationOnRealTranscripts(t *testing.T) {
	home, _ := os.UserHomeDir()
	root := filepath.Join(home, ".claude", "projects")
	var files []string
	filepath.Walk(root, func(p string, fi os.FileInfo, err error) error {
		if err == nil && !fi.IsDir() && filepath.Ext(p) == ".jsonl" {
			files = append(files, p)
		}
		return nil
	})
	if len(files) == 0 {
		t.Skip("no transcripts on this machine")
	}
	for _, p := range files {
		wantCtx, wantTurns, wantFound := oldStats(p)
		got, err := Scan(p, Progress{})
		if err != nil {
			t.Errorf("%s: %v", p, err)
			continue
		}
		if got.Ctx != wantCtx || got.Turns != wantTurns || got.Found != wantFound {
			t.Errorf("%s: got ctx=%d turns=%d found=%v, want ctx=%d turns=%d found=%v",
				p, got.Ctx, got.Turns, got.Found, wantCtx, wantTurns, wantFound)
		}
	}
	t.Logf("compared %d real transcripts", len(files))
}
