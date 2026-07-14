package transcripts

import (
	"os"
	"path/filepath"
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
