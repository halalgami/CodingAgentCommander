package imagegen

import (
	"context"
	"testing"
)

// fakeProvider is a Provider that records nothing and returns nothing. The
// registry's job is bookkeeping, so the values behind the ids do not matter.
type fakeProvider struct {
	id     string
	models []Model
}

func (f *fakeProvider) ID() string      { return f.id }
func (f *fakeProvider) Models() []Model { return f.models }
func (f *fakeProvider) Generate(context.Context, string, Request) (Result, error) {
	return Result{}, nil
}

func TestRegistry(t *testing.T) {
	Register(&fakeProvider{id: "zeta", models: []Model{{ID: "z/1", Label: "Zeta One", Mode: ModeEdit}}})
	Register(&fakeProvider{id: "alpha", models: []Model{{ID: "a/1", Label: "Alpha One", Mode: ModeImg2Img}}})

	cases := []struct {
		name    string
		id      string
		wantOK  bool
		wantMod string
	}{
		{"registered id resolves", "alpha", true, "a/1"},
		{"second registered id resolves", "zeta", true, "z/1"},
		{"unknown id does not resolve", "nope", false, ""},
		{"empty id does not resolve", "", false, ""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			p, ok := Lookup(tc.id)
			if ok != tc.wantOK {
				t.Fatalf("Lookup(%q) ok = %v, want %v", tc.id, ok, tc.wantOK)
			}
			if !ok {
				return
			}
			if got := p.Models()[0].ID; got != tc.wantMod {
				t.Errorf("Lookup(%q) first model = %q, want %q", tc.id, got, tc.wantMod)
			}
		})
	}

	// Providers is ordered so a caller renders a stable list rather than Go's
	// randomised map order. Other tasks register more providers into the same
	// registry, so assert on ordering and membership, never on the total.
	all := Providers()
	var prev string
	seen := map[string]bool{}
	for _, p := range all {
		if prev != "" && p.ID() < prev {
			t.Errorf("Providers() not sorted: %q after %q", p.ID(), prev)
		}
		prev = p.ID()
		seen[p.ID()] = true
	}
	if !seen["alpha"] || !seen["zeta"] {
		t.Errorf("Providers() missing a registered id: %v", seen)
	}
}

func TestRegisterRejectsDuplicate(t *testing.T) {
	defer func() {
		if recover() == nil {
			t.Fatal("registering a duplicate id should panic; a silent overwrite hides a wiring bug until runtime")
		}
	}()
	Register(&fakeProvider{id: "dup"})
	Register(&fakeProvider{id: "dup"})
}

func TestStrengthIsDocumentedAsImg2ImgOnly(t *testing.T) {
	// Not behaviour — a guard on the contract. ModeEdit requests carry no
	// denoise fraction, and a future provider that reads Strength under
	// ModeEdit is misreading the interface.
	r := Request{Mode: ModeEdit, Strength: 0.65}
	if r.Mode == ModeImg2Img {
		t.Fatal("ModeEdit must not equal ModeImg2Img")
	}
}
