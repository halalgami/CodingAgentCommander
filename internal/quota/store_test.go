package quota

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"
	"time"
)

func at(s string) time.Time {
	t, err := time.ParseInLocation("2006-01-02T15:04", s, time.Local)
	if err != nil {
		panic(err)
	}
	return t
}

func smp(t time.Time, util float64) Sample {
	return Sample{T: t, Windows: []Window{{Key: "five_hour", Label: "Session", Util: util}}}
}

func TestAppendWritesMonthFile0600(t *testing.T) {
	s := Store{Dir: filepath.Join(t.TempDir(), "quota")}
	if err := s.Append("anthropic", smp(at("2026-11-15T10:00"), 12)); err != nil {
		t.Fatal(err)
	}
	fi, err := os.Stat(filepath.Join(s.Dir, "anthropic-2026-11.jsonl"))
	if err != nil {
		t.Fatal(err)
	}
	if runtime.GOOS != "windows" && fi.Mode().Perm() != 0o600 {
		t.Fatalf("mode %v", fi.Mode().Perm())
	}
}

func TestReadAcrossMonthsOrdered(t *testing.T) {
	s := Store{Dir: t.TempDir()}
	_ = s.Append("anthropic", smp(at("2026-12-01T01:00"), 20))
	_ = s.Append("anthropic", smp(at("2026-11-30T23:00"), 10))
	_ = s.Append("anthropic", smp(at("2026-10-05T10:00"), 5))
	got, err := s.Read("anthropic", at("2026-11-01T00:00"), at("2026-12-31T00:00"))
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 2 || got[0].Windows[0].Util != 10 || got[1].Windows[0].Util != 20 {
		t.Fatalf("got %+v", got)
	}
}

func TestReadSkipsCorruptLines(t *testing.T) {
	s := Store{Dir: t.TempDir()}
	_ = s.Append("anthropic", smp(at("2026-11-15T10:00"), 1))
	f, _ := os.OpenFile(filepath.Join(s.Dir, "anthropic-2026-11.jsonl"), os.O_APPEND|os.O_WRONLY, 0o600)
	_, _ = f.WriteString("{garbage\n")
	f.Close()
	_ = s.Append("anthropic", smp(at("2026-11-15T10:05"), 2))
	got, err := s.Read("anthropic", at("2026-11-01T00:00"), at("2026-12-01T00:00"))
	if err != nil || len(got) != 2 {
		t.Fatalf("got %d %v", len(got), err)
	}
}

func TestReadMissingIsEmpty(t *testing.T) {
	got, err := Store{Dir: filepath.Join(t.TempDir(), "nope")}.Read("anthropic", at("2026-11-01T00:00"), at("2026-12-01T00:00"))
	if err != nil || len(got) != 0 {
		t.Fatalf("got %v %v", got, err)
	}
}

func TestPruneDropsMonthsPastRetention(t *testing.T) {
	s := Store{Dir: t.TempDir()}
	_ = s.Append("anthropic", smp(at("2026-07-20T10:00"), 1))
	_ = s.Append("anthropic", smp(at("2026-08-20T10:00"), 1))
	_ = os.WriteFile(filepath.Join(s.Dir, "notes.txt"), []byte("x"), 0o600)
	if err := s.Prune(at("2026-11-15T10:00")); err != nil {
		t.Fatal(err)
	}
	for name, want := range map[string]bool{"anthropic-2026-07.jsonl": false, "anthropic-2026-08.jsonl": true, "notes.txt": true} {
		_, err := os.Stat(filepath.Join(s.Dir, name))
		if (err == nil) != want {
			t.Fatalf("%s exists=%v want %v", name, err == nil, want)
		}
	}
}
