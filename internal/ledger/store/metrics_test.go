package store

import (
	"path/filepath"
	"testing"

	"github.com/halalgami/CodingAgentCommander/internal/ledger/parse"
	"github.com/halalgami/CodingAgentCommander/internal/metrics/scan"
)

func TestApplyMetricsPersistsAndReplacesToday(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "ledger")
	s, _ := Create(dir, Consent{}, clock())
	id := repoID("/repo")
	s.Apply(id, "/t/a.jsonl", Cursor{SessionID: "a"}, false, []parse.Event{turn("u1")}, 0)
	mk := func(code int) scan.Result {
		return scan.Result{Head: "h1", Files: map[string]scan.FileMetrics{"a.go": {Lang: "go", Code: code, Flagged: true}},
			Snapshot: scan.Snapshot{Day: "2026-10-11", Code: code}}
	}
	if !s.ApplyMetrics("/repo", mk(10)) || !s.ApplyMetrics("/repo", mk(12)) {
		t.Fatal("ApplyMetrics refused")
	}
	if s.ApplyMetrics("/unknown", mk(1)) {
		t.Fatal("metrics for an unknown root accepted")
	}
	s.Close()
	s2, _ := Open(dir, clock())
	defer s2.Close()
	files, snaps, ok := s2.Metrics(id.Key)
	if !ok || files["a.go"].Code != 12 || len(snaps) != 1 || snaps[0].Code != 12 {
		t.Fatalf("files=%v snaps=%v", files, snaps)
	}
	info := s2.Projects()[0]
	if info.ScanHead != "h1" || info.ScannedAt == 0 || info.Latest == nil || info.Latest.Code != 12 {
		t.Fatalf("info %+v", info)
	}
}
