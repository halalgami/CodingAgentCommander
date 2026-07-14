package secrets

import (
	"testing"

	"github.com/zalando/go-keyring"
)

func TestRoundTrip(t *testing.T) {
	keyring.MockInit() // in-memory keychain, no real OS access
	if err := Set("ZEN_KEY", "sk-zen-123"); err != nil {
		t.Fatalf("Set: %v", err)
	}
	got, err := Get("ZEN_KEY")
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if got != "sk-zen-123" {
		t.Errorf("Get = %q, want sk-zen-123", got)
	}
	if err := Delete("ZEN_KEY"); err != nil {
		t.Fatalf("Delete: %v", err)
	}
	if _, err := Get("ZEN_KEY"); err == nil {
		t.Error("expected error after Delete")
	}
}
