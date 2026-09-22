package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/halalgami/CodingAgentCommander/internal/imagegen"
)

// The key boundary is the point of this file: a key goes INTO Go and never
// comes back. This walks every exported binding's marshalled output looking for
// the secret, which is stronger than asserting field-by-field — a field added
// later would be covered without anyone remembering to extend the test.
func TestNoBindingEverReturnsTheKey(t *testing.T) {
	f := newPackGenFixture(t)
	const secret = "sk-super-secret-key-value-9999"

	// The fixture's seam returns a fixed key; point it at our sentinel so any
	// leak is unambiguous.
	orig := packGenLoadKey
	packGenLoadKey = func(string) string { return secret }
	t.Cleanup(func() { packGenLoadKey = orig })

	if _, err := f.app.StartPackGen(f.request()); err != nil {
		t.Fatal(err)
	}
	f.waitDone(t)

	payloads := map[string]any{
		"PackGenProviders":  f.app.PackGenProviders(),
		"PackGenConditions": f.app.PackGenConditions(),
		"PackGenStyles":     f.app.PackGenStyles(),
		"PackGenSlots":      f.app.PackGenSlots(),
		"PackGenStatus":     f.app.PackGenStatus(),
		"PackGenEstimate":   f.app.PackGenEstimate("fake/model", 6),
	}
	for name, v := range payloads {
		b, err := json.Marshal(v)
		if err != nil {
			t.Fatalf("%s: %v", name, err)
		}
		if strings.Contains(string(b), secret) {
			t.Errorf("%s leaks the API key across the binding:\n%s", name, b)
		}
	}
}

func TestPackGenProvidersReportsKeyPresenceOnly(t *testing.T) {
	f := newPackGenFixture(t)
	// The fake is installed through the lookup seam, so it is not in the
	// package registry that PackGenProviders walks; the real fal provider is.
	got := f.app.PackGenProviders()
	if len(got) == 0 {
		t.Fatal("no providers registered")
	}
	var fal *PackGenProvider
	for i := range got {
		if got[i].ID == "fal" {
			fal = &got[i]
		}
	}
	if fal == nil {
		t.Fatal("the fal provider is not listed")
	}
	if len(fal.Models) == 0 {
		t.Error("fal offers no models")
	}
	// Priced models must carry a price; unpriced ones must say so rather than
	// showing 0.00, which reads as free.
	for _, m := range fal.Models {
		if m.ID == "" || m.Label == "" {
			t.Errorf("incomplete model: %+v", m)
		}
		if m.Known && m.USD <= 0 {
			t.Errorf("model %q claims a known price of %v", m.ID, m.USD)
		}
		if !m.Known && m.USD != 0 {
			t.Errorf("model %q has an unknown price but a non-zero figure %v", m.ID, m.USD)
		}
	}
}

func TestPackGenEstimateScalesAndDisclosesItsDate(t *testing.T) {
	f := newPackGenFixture(t)
	one := f.app.PackGenEstimate(imagegen.FalDefaultModel, 1)
	six := f.app.PackGenEstimate(imagegen.FalDefaultModel, 6)
	if !one.Known || !six.Known {
		t.Skip("the price table has aged out; StaleAt is doing its job")
	}
	if six.USD <= one.USD {
		t.Errorf("six images (%v) do not cost more than one (%v)", six.USD, one.USD)
	}
	// A hand-maintained figure without its date is a claim we cannot back.
	if !strings.Contains(one.Note, "prices as of") {
		t.Errorf("estimate carries no disclosure: %q", one.Note)
	}
	// An unpriced model must report Known=false rather than a confident zero.
	if got := f.app.PackGenEstimate("nobody/such-model", 6); got.Known || got.USD != 0 {
		t.Errorf("unpriced model quoted as %+v", got)
	}
}

func TestSetImagegenKeyValidates(t *testing.T) {
	f := newPackGenFixture(t)
	if err := f.app.SetImagegenKey("nope", "x"); err == nil {
		t.Error("stored a key for a provider that does not exist")
	}
	// Whitespace-only is empty. A key pasted from a dashboard reliably arrives
	// with a trailing newline, and an all-whitespace value means the paste
	// failed, not that the user wants a blank key.
	for _, k := range []string{"", "   ", "\n\t "} {
		if err := f.app.SetImagegenKey("fake", k); err == nil {
			t.Errorf("accepted %q as a key", k)
		}
	}
	if err := f.app.ClearImagegenKey("nope"); err == nil {
		t.Error("cleared a key for a provider that does not exist")
	}
}

// --- base portrait ---------------------------------------------------------

func TestInspectPackGenBase(t *testing.T) {
	f := newPackGenFixture(t)
	dir := t.TempDir()

	portrait := filepath.Join(dir, "tall.png")
	if err := os.WriteFile(portrait, testPNG(t, 40, 60, 3, false), 0o644); err != nil {
		t.Fatal(err)
	}
	got, err := f.app.InspectPackGenBase(portrait)
	if err != nil {
		t.Fatal(err)
	}
	if got.W != 40 || got.H != 60 {
		t.Errorf("dimensions = %dx%d", got.W, got.H)
	}
	if got.Warning != "" {
		t.Errorf("a portrait source warned: %q", got.Warning)
	}

	// Landscape must warn but still be usable — the host crops from the top, so
	// the sides are lost, and saying so before six images are paid for is the
	// whole point. It is advice, not a refusal.
	wide := filepath.Join(dir, "wide.png")
	if err := os.WriteFile(wide, testPNG(t, 80, 20, 4, false), 0o644); err != nil {
		t.Fatal(err)
	}
	got, err = f.app.InspectPackGenBase(wide)
	if err != nil {
		t.Fatal("a landscape base must be allowed, not refused: " + err.Error())
	}
	if !strings.Contains(got.Warning, "landscape") {
		t.Errorf("landscape source did not warn: %q", got.Warning)
	}
}

func TestInspectPackGenBaseRefusals(t *testing.T) {
	f := newPackGenFixture(t)
	dir := t.TempDir()

	notImage := filepath.Join(dir, "notes.png")
	if err := os.WriteFile(notImage, []byte("just some prose"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := f.app.InspectPackGenBase(notImage); err == nil ||
		!strings.Contains(err.Error(), "PNG or JPEG") {
		t.Errorf("got %v", err)
	}
	if _, err := f.app.InspectPackGenBase(filepath.Join(dir, "absent.png")); err == nil {
		t.Error("accepted a missing file")
	}
	if _, err := f.app.InspectPackGenBase(dir); err == nil {
		t.Error("accepted a directory as a base portrait")
	}
}

// A base file is read into memory whole before anything decodes it, so the
// pixel bound does not help — the byte cap is what stops a huge file. The
// oversized file must be REFUSED, not silently truncated to a prefix that then
// fails to decode with a message blaming the user's image.
func TestOversizedBaseIsRefusedNotTruncated(t *testing.T) {
	path := filepath.Join(t.TempDir(), "huge.png")
	if err := os.WriteFile(path, make([]byte, 4096), 0o644); err != nil {
		t.Fatal(err)
	}
	_, err := readFileLimited(path, 1024)
	if err == nil {
		t.Fatal("want an error for a file over the cap")
	}
	if !strings.Contains(err.Error(), "larger than any portrait") {
		t.Errorf("unhelpful error: %v", err)
	}
	// Under the cap still reads fine.
	if got, err := readFileLimited(path, 8192); err != nil || len(got) != 4096 {
		t.Errorf("got %d bytes, %v", len(got), err)
	}
}

// --- the condition matrix --------------------------------------------------

// The UI greys out combinations from this matrix. If it disagreed with the
// validator, the wizard would offer a choice that is then refused — or worse,
// hide one that is legal.
func TestPackGenConditionsMatchTheValidator(t *testing.T) {
	f := newPackGenFixture(t)
	got := f.app.PackGenConditions()
	if len(got) != len(packConditions) {
		t.Fatalf("got %d conditions, want %d", len(got), len(packConditions))
	}
	for _, c := range got {
		if c.Class != "ambient" && c.Class != "event" {
			t.Errorf("condition %q has class %q", c.Name, c.Class)
		}
		if c.Label == "" {
			t.Errorf("condition %q has no staging text to show", c.Name)
		}
		for _, slot := range c.Slots {
			// Every advertised pairing must be one the validator accepts.
			err := validatePackGenSlots([]PackGenSlot{{Slot: "idle"}, {Slot: slot, When: c.Name}})
			if err != nil && !strings.Contains(err.Error(), "selected twice") {
				t.Errorf("UI offers %s/%s but the validator refuses it: %v", c.Name, slot, err)
			}
		}
		// And the converse: a slot NOT advertised must be one the validator
		// would refuse, or the wizard is hiding a legal choice.
		for _, slot := range packGenSlotOrder {
			var offered bool
			for _, s := range c.Slots {
				if s == slot {
					offered = true
				}
			}
			if offered {
				continue
			}
			if packCondAllowed(c.Name, slot) {
				t.Errorf("condition %q can display in %q but the wizard does not offer it", c.Name, slot)
			}
		}
	}
}

func TestPackGenSlotsCarryTheirDefaultStaging(t *testing.T) {
	f := newPackGenFixture(t)
	got := f.app.PackGenSlots()
	if len(got) != len(packGenSlotOrder) {
		t.Fatalf("got %d slots, want %d", len(got), len(packGenSlotOrder))
	}
	for i, s := range got {
		if s.Slot != packGenSlotOrder[i] {
			t.Errorf("slot %d = %q, want %q", i, s.Slot, packGenSlotOrder[i])
		}
		if strings.TrimSpace(s.Staging) == "" {
			t.Errorf("slot %q arrives with no staging text to edit", s.Slot)
		}
	}
}
