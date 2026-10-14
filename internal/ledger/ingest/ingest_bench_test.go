package ingest

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/halalgami/CodingAgentCommander/internal/ledger/live"
	"github.com/halalgami/CodingAgentCommander/internal/ledger/store"
	"github.com/halalgami/CodingAgentCommander/internal/repoid"
)

// BenchmarkBackfill builds ~100 MB of synthetic transcripts and backfills them.
// Guards regressions; the real-corpus number is recorded in PROJECT_STATE.
func BenchmarkBackfill(b *testing.B) {
	root := filepath.Join(b.TempDir(), "projects")
	cwd := b.TempDir()
	os.MkdirAll(filepath.Join(root, "enc"), 0o755)
	block := strings.Join(fixture(cwd), "\n") + "\n"
	pad := `{"type":"user","sessionId":"s1","message":{"role":"user","content":[{"type":"tool_result","tool_use_id":"t1","content":"` + strings.Repeat("z", 4000) + `"}]}}` + "\n"
	for i := 0; i < 200; i++ {
		var sb strings.Builder
		for sb.Len() < 500_000 {
			sb.WriteString(block)
			sb.WriteString(pad)
		}
		os.WriteFile(filepath.Join(root, "enc", fmt.Sprintf("s%03d.jsonl", i)), []byte(sb.String()), 0o600)
	}
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		st, _ := store.Create(filepath.Join(b.TempDir(), "ledger"), store.Consent{}, time.Now)
		in := New(root, live.New(nil), repoid.NewResolver())
		in.SetStore(st)
		if err := in.Backfill(context.Background()); err != nil {
			b.Fatal(err)
		}
		st.Close()
	}
}
