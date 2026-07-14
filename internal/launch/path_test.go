package launch

import (
	"os"
	"strings"
	"testing"
)

func TestAugmentPATHAddsWithoutRemoving(t *testing.T) {
	orig := os.Getenv("PATH")
	t.Cleanup(func() { os.Setenv("PATH", orig) })

	os.Setenv("PATH", "/usr/bin:/bin")
	AugmentPATH()
	got := os.Getenv("PATH")

	if !strings.HasPrefix(got, "/usr/bin:/bin") {
		t.Fatalf("original PATH entries must stay first, got %q", got)
	}
	if !strings.Contains(got, "/opt/homebrew/bin") {
		t.Fatalf("homebrew bin missing from augmented PATH: %q", got)
	}
}

func TestAugmentPATHIdempotent(t *testing.T) {
	orig := os.Getenv("PATH")
	t.Cleanup(func() { os.Setenv("PATH", orig) })

	os.Setenv("PATH", "/usr/bin:/bin")
	AugmentPATH()
	once := os.Getenv("PATH")
	AugmentPATH()
	twice := os.Getenv("PATH")
	if once != twice {
		t.Fatalf("second augment changed PATH:\n once=%q\ntwice=%q", once, twice)
	}
}
