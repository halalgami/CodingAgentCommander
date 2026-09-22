package transcripts

import (
	"os"
	"path/filepath"
	"testing"
)

func biggest(t testing.TB) string {
	home, _ := os.UserHomeDir()
	var best string
	var bestSize int64
	filepath.Walk(filepath.Join(home, ".claude", "projects"), func(p string, fi os.FileInfo, err error) error {
		if err == nil && !fi.IsDir() && filepath.Ext(p) == ".jsonl" && fi.Size() > bestSize {
			best, bestSize = p, fi.Size()
		}
		return nil
	})
	if best == "" {
		t.Skip("no transcripts")
	}
	return best
}

func BenchmarkOldTwoPass(b *testing.B) {
	p := biggest(b)
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		oldStats(p)
		oldStats(p)
	}
}

func BenchmarkNewSinglePass(b *testing.B) {
	p := biggest(b)
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		Scan(p, Progress{})
	}
}

func BenchmarkNewIncremental(b *testing.B) {
	p := biggest(b)
	warm, _ := Scan(p, Progress{})
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		Scan(p, warm)
	}
}
