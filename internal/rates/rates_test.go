package rates

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"
	"time"

	"github.com/halalgami/CodingAgentCommander/internal/ledger/store"
)

func TestStaleAfter60Days(t *testing.T) {
	if Stale(PricedAt().Add(59 * 24 * time.Hour)) {
		t.Fatal("59 days stale")
	}
	if !Stale(PricedAt().Add(61 * 24 * time.Hour)) {
		t.Fatal("61 days fresh")
	}
}

func TestRateLookupOrder(t *testing.T) {
	b := NewBook(Settings{Overrides: map[string]Rate{
		"claude-opus-5-5":    {In: 1},
		"provider:anthropic": {In: 2},
	}})
	if r, ok := b.Rate("claude-opus-5-5", "anthropic"); !ok || r.In != 1 {
		t.Fatalf("model override: %+v %v", r, ok)
	}
	if r, ok := b.Rate("claude-sonnet-5-5", "anthropic"); !ok || r.In != 2 {
		t.Fatalf("provider override: %+v %v", r, ok)
	}
	if r, ok := NewBook(Settings{}).Rate("claude-sonnet-5-5", "anthropic"); !ok || r.In != 2 || r.Out != 10 {
		t.Fatalf("table: %+v %v", r, ok)
	}
}

func TestDatedIDMatchesPrefix(t *testing.T) {
	b := NewBook(Settings{})
	r, ok := b.Rate("claude-opus-5-5-20261001", "anthropic")
	if !ok || r.In != 4 {
		t.Fatalf("dated: %+v %v", r, ok)
	}
	if r, ok := b.Rate("us.anthropic.claude-opus-5-5", "bedrock"); !ok || r.In != 4 {
		t.Fatalf("bedrock: %+v %v", r, ok)
	}
	// opus-5 must not swallow opus-5-5
	if r, _ := b.Rate("claude-opus-5", "anthropic"); r.In != 5 {
		t.Fatalf("opus-5: %+v", r)
	}
}

func TestUnknownModelHasNoRate(t *testing.T) {
	if _, ok := NewBook(Settings{}).Rate("glm-5.3", "ollama-cloud"); ok {
		t.Fatal("unknown model priced")
	}
}

func TestCostPerMillion(t *testing.T) {
	got := Rate{In: 3, Out: 15}.Cost(store.TokenCounts{In: 1e6, Out: 2e6})
	if got != 33 {
		t.Fatalf("cost %v", got)
	}
}

func TestSettingsRoundTrip(t *testing.T) {
	p := filepath.Join(t.TempDir(), "sub", "asturlab.json")
	want := Settings{Overrides: map[string]Rate{"x": {In: 1, Out: 2, CacheRead: 3, CacheWrite: 4}},
		Plans: map[string]Plan{"anthropic": {Name: "Max", MonthlyUSD: 200}}}
	if err := SaveSettings(p, want); err != nil {
		t.Fatal(err)
	}
	got := LoadSettings(p)
	if got.Overrides["x"] != want.Overrides["x"] || got.Plans["anthropic"] != want.Plans["anthropic"] {
		t.Fatalf("round trip %+v", got)
	}
	fi, _ := os.Stat(p)
	if runtime.GOOS != "windows" && fi.Mode().Perm() != 0o600 {
		t.Fatalf("mode %v", fi.Mode().Perm())
	}
	if s := LoadSettings(filepath.Join(t.TempDir(), "missing.json")); len(s.Plans) != 0 {
		t.Fatal("missing file not empty")
	}
}

func TestSettingsCorruptBacksUp(t *testing.T) {
	p := filepath.Join(t.TempDir(), "asturlab.json")
	os.WriteFile(p, []byte("{nope"), 0o600)
	if s := LoadSettings(p); len(s.Plans)+len(s.Overrides) != 0 {
		t.Fatal("corrupt not empty")
	}
	if b, err := os.ReadFile(p + ".corrupt"); err != nil || string(b) != "{nope" {
		t.Fatalf("backup %q %v", b, err)
	}
}
