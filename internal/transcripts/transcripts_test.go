package transcripts

import (
	"os"
	"path/filepath"
	"strconv"
	"testing"
)

func TestContextTokensUsesLastAssistantUsage(t *testing.T) {
	got, err := ContextTokens("testdata/sample.jsonl")
	if err != nil {
		t.Fatalf("ContextTokens: %v", err)
	}
	// last assistant usage: 2 + 500 + 34 = 536
	if got != 536 {
		t.Errorf("ContextTokens = %d, want 536", got)
	}
}

func TestContextTokensEmptyFile(t *testing.T) {
	if _, err := ContextTokens("testdata/does-not-exist.jsonl"); err == nil {
		t.Fatal("expected error for missing file")
	}
}

func TestTurnCount(t *testing.T) {
	n, err := TurnCount("testdata/sample.jsonl")
	if err != nil {
		t.Fatalf("TurnCount: %v", err)
	}
	if n != 2 { // sample.jsonl has two assistant messages
		t.Errorf("TurnCount = %d, want 2", n)
	}
}

func TestEncodeCwd(t *testing.T) {
	got := EncodeCwd("/Users/x/.config/My.App")
	want := "-Users-x--config-My-App"
	if got != want {
		t.Errorf("EncodeCwd = %q, want %q", got, want)
	}
}

func TestStatsForCwd(t *testing.T) {
	root := t.TempDir()
	cwd := "/tmp/projA"
	dir := filepath.Join(root, EncodeCwd(cwd))
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	// copy the fixture in as the session transcript
	b, _ := os.ReadFile("testdata/sample.jsonl")
	if err := os.WriteFile(filepath.Join(dir, "s1.jsonl"), b, 0o644); err != nil {
		t.Fatal(err)
	}
	ctx, turns, path, err := StatsForCwd(root, cwd)
	if err != nil {
		t.Fatalf("StatsForCwd: %v", err)
	}
	if ctx != 536 || turns != 2 || filepath.Base(path) != "s1.jsonl" {
		t.Errorf("got ctx=%d turns=%d path=%s", ctx, turns, path)
	}
}

// The whole point of carrying Progress forward is that an APPENDED file costs
// only its tail — but only if the tail-read answer matches the full re-read.
// An active session appends between every poll, so this is the common path, not
// an edge case.
func TestScanResumeMatchesFullRescan(t *testing.T) {
	path := filepath.Join(t.TempDir(), "s.jsonl")
	line := func(in int) string {
		return `{"type":"assistant","message":{"usage":{"input_tokens":` +
			strconv.Itoa(in) + `,"cache_read_input_tokens":1,"cache_creation_input_tokens":2}}}` + "\n"
	}
	noise := `{"type":"user","message":{"content":"a word about an assistant"}}` + "\n"

	if err := os.WriteFile(path, []byte(line(10)+noise+line(20)), 0o600); err != nil {
		t.Fatal(err)
	}
	warm, err := Scan(path, Progress{})
	if err != nil {
		t.Fatal(err)
	}
	if warm.Turns != 2 || warm.Ctx != 23 {
		t.Fatalf("cold scan: turns=%d ctx=%d, want 2 and 23", warm.Turns, warm.Ctx)
	}

	// Append, then compare resumed against a scan that read the whole file.
	f, err := os.OpenFile(path, os.O_APPEND|os.O_WRONLY, 0o600)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := f.WriteString(noise + line(30)); err != nil {
		t.Fatal(err)
	}
	f.Close()

	resumed, err := Scan(path, warm)
	if err != nil {
		t.Fatal(err)
	}
	full, err := Scan(path, Progress{})
	if err != nil {
		t.Fatal(err)
	}
	if resumed.Turns != full.Turns || resumed.Ctx != full.Ctx {
		t.Fatalf("resumed turns=%d ctx=%d, full turns=%d ctx=%d",
			resumed.Turns, resumed.Ctx, full.Turns, full.Ctx)
	}
	if resumed.Offset != full.Offset {
		t.Fatalf("resumed offset %d, full %d", resumed.Offset, full.Offset)
	}
}

// A poll can land while Claude Code is mid-write. Half a line is not a turn,
// and consuming it would both miscount and skip the real line when it lands.
func TestScanLeavesAPartialTrailingLineForNextTime(t *testing.T) {
	path := filepath.Join(t.TempDir(), "s.jsonl")
	complete := `{"type":"assistant","message":{"usage":{"input_tokens":5,"cache_read_input_tokens":0,"cache_creation_input_tokens":0}}}` + "\n"
	partial := `{"type":"assistant","message":{"usage":{"input_toke`

	if err := os.WriteFile(path, []byte(complete+partial), 0o600); err != nil {
		t.Fatal(err)
	}
	p, err := Scan(path, Progress{})
	if err != nil {
		t.Fatal(err)
	}
	if p.Turns != 1 {
		t.Fatalf("counted %d turns, want 1: the partial tail was consumed", p.Turns)
	}
	if p.Offset != int64(len(complete)) {
		t.Fatalf("offset %d, want %d: the partial tail must stay unconsumed", p.Offset, len(complete))
	}

	// Finish the line; the next scan must pick it up whole.
	f, _ := os.OpenFile(path, os.O_APPEND|os.O_WRONLY, 0o600)
	f.WriteString(`ns":7,"cache_read_input_tokens":0,"cache_creation_input_tokens":0}}}` + "\n")
	f.Close()

	p2, err := Scan(path, p)
	if err != nil {
		t.Fatal(err)
	}
	if p2.Turns != 2 || p2.Ctx != 7 {
		t.Fatalf("after completion: turns=%d ctx=%d, want 2 and 7", p2.Turns, p2.Ctx)
	}
}

// A transcript that SHRANK was rewritten, not appended to, so the bytes behind
// the cached offset are no longer the bytes that were counted.
func TestScanRestartsWhenTheFileShrank(t *testing.T) {
	path := filepath.Join(t.TempDir(), "s.jsonl")
	line := `{"type":"assistant","message":{"usage":{"input_tokens":9,"cache_read_input_tokens":0,"cache_creation_input_tokens":0}}}` + "\n"
	if err := os.WriteFile(path, []byte(line+line+line), 0o600); err != nil {
		t.Fatal(err)
	}
	warm, _ := Scan(path, Progress{})
	if warm.Turns != 3 {
		t.Fatalf("turns=%d, want 3", warm.Turns)
	}
	if err := os.WriteFile(path, []byte(line), 0o600); err != nil {
		t.Fatal(err)
	}
	p, err := Scan(path, warm)
	if err != nil {
		t.Fatal(err)
	}
	if p.Turns != 1 {
		t.Fatalf("turns=%d after truncation, want 1: stale progress was reused", p.Turns)
	}
}
