package imagegen

import (
	"strings"
	"testing"

	"github.com/halalgami/CodingAgentCommander/internal/secrets"
	"github.com/zalando/go-keyring"
)

func TestKeyRefIsNamespaced(t *testing.T) {
	ref := KeyRef("fal")
	if ref != "imagegen.fal" {
		t.Fatalf("KeyRef(\"fal\") = %q, want imagegen.fal", ref)
	}
	// The keys this app already stores are bare environment-variable names in
	// one flat namespace. An unprefixed ref could collide with one of them.
	if !strings.HasPrefix(ref, KeyRefPrefix) {
		t.Errorf("%q is not namespaced", ref)
	}
	if ref == "fal" || ref == strings.ToUpper(ref) {
		t.Errorf("%q looks like an environment-variable ref", ref)
	}
}

func TestKeyRoundTrip(t *testing.T) {
	keyring.MockInit() // in-memory keychain; a real one prompts on the release machine

	if err := StoreKey("fal", "fal-secret-value"); err != nil {
		t.Fatalf("StoreKey: %v", err)
	}
	if got := LoadKey("fal"); got != "fal-secret-value" {
		t.Errorf("LoadKey = %q, want fal-secret-value", got)
	}
	// Stored under the namespaced ref and nowhere else.
	if v, err := secrets.Get("imagegen.fal"); err != nil || v != "fal-secret-value" {
		t.Errorf("secrets.Get(\"imagegen.fal\") = %q, %v", v, err)
	}
	if _, err := secrets.Get("fal"); err == nil {
		t.Error("a bare \"fal\" ref was written; that namespace belongs to the model provider keys")
	}

	if err := ClearKey("fal"); err != nil {
		t.Fatalf("ClearKey: %v", err)
	}
	// A missing key is not an error — the caller's job is to prompt for one.
	if got := LoadKey("fal"); got != "" {
		t.Errorf("LoadKey after ClearKey = %q, want empty", got)
	}
}

func TestStoreKeyRejectsEmpties(t *testing.T) {
	keyring.MockInit()
	if err := StoreKey("", "x"); err == nil {
		t.Error("expected an error with no provider id")
	}
	if err := StoreKey("fal", ""); err == nil {
		t.Error("expected an error storing an empty key; it would read back as 'a key is set'")
	}
}
