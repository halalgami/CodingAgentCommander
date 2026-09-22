package main

import (
	"strings"
	"testing"

	"github.com/halalgami/CodingAgentCommander/internal/imagegen"
)

// The prompt's value is entirely in what it CONTAINS and in what order, so the
// assertions are about the four locks rather than about an exact string — a
// golden-string test here would only assert that a copy-paste succeeded.

func TestComposePackPromptCarriesAllFourLocks(t *testing.T) {
	style, ok := packGenStyleByID("whimsical")
	if !ok {
		t.Fatal("whimsical preset missing")
	}
	got := composePackPrompt(PackGenSlot{Slot: "working"}, style.Text, "")

	iIdentity := strings.Index(got, packGenIdentityLock)
	iStaging := strings.Index(got, packGenSlotStaging["working"])
	iFraming := strings.Index(got, packGenFramingLock)
	iStyle := strings.Index(got, style.Text)

	for name, idx := range map[string]int{
		"identity": iIdentity, "staging": iStaging, "framing": iFraming, "style": iStyle,
	} {
		if idx < 0 {
			t.Fatalf("%s lock missing from prompt:\n%s", name, got)
		}
	}
	// Order matters: identity first (it is the constraint everything else is
	// subject to) and style last (it applies to the whole image).
	if !(iIdentity < iStaging && iStaging < iFraming && iFraming < iStyle) {
		t.Errorf("locks out of order: identity=%d staging=%d framing=%d style=%d\n%s",
			iIdentity, iStaging, iFraming, iStyle, got)
	}
}

func TestComposePackPromptAddsConditionStaging(t *testing.T) {
	plain := composePackPrompt(PackGenSlot{Slot: "working"}, "STYLE", "")
	night := composePackPrompt(PackGenSlot{Slot: "working", When: "lateNight"}, "STYLE", "")

	if !strings.Contains(night, packGenConditionStaging["lateNight"]) {
		t.Errorf("lateNight staging missing:\n%s", night)
	}
	if strings.Contains(plain, packGenConditionStaging["lateNight"]) {
		t.Error("unconditioned slot picked up condition staging")
	}
	// A variant whose prompt equals its base is money spent on a duplicate.
	if plain == night {
		t.Error("conditioned variant has an identical prompt to its base")
	}
}

func TestComposePackPromptUserStagingOverridesDefault(t *testing.T) {
	got := composePackPrompt(PackGenSlot{Slot: "idle", Staging: "  She is juggling.  "}, "STYLE", "")
	if !strings.Contains(got, "She is juggling.") {
		t.Errorf("custom staging not used:\n%s", got)
	}
	if strings.Contains(got, packGenSlotStaging["idle"]) {
		t.Error("default staging emitted alongside the custom one")
	}
	if strings.Contains(got, "  She is juggling.  ") {
		t.Error("custom staging not trimmed")
	}
	// The locks are not user-editable and must survive an override.
	if !strings.Contains(got, packGenIdentityLock) || !strings.Contains(got, packGenFramingLock) {
		t.Error("a custom staging string displaced a lock")
	}
}

// Whitespace-only staging must fall back rather than produce a prompt with a
// hole where the subject should be.
func TestComposePackPromptBlankStagingFallsBack(t *testing.T) {
	got := composePackPrompt(PackGenSlot{Slot: "done", Staging: "   \n\t "}, "STYLE", "")
	if !strings.Contains(got, packGenSlotStaging["done"]) {
		t.Errorf("blank staging did not fall back to the default:\n%s", got)
	}
}

// Coverage guards. The slot and condition vocabularies live in companionpack.go;
// adding one there without staging text here would silently generate a prompt
// with no subject, which the model would answer with an arbitrary image.
func TestEveryGeneratedSlotHasStaging(t *testing.T) {
	for _, slot := range packGenSlotOrder {
		if !packSlotKnown(slot) {
			t.Errorf("packGenSlotOrder has %q, which is not in the pack vocabulary", slot)
		}
		if strings.TrimSpace(packGenSlotStaging[slot]) == "" {
			t.Errorf("slot %q has no default staging", slot)
		}
	}
	// The converse: every non-bg vocabulary slot should be offered, or it is
	// quietly ungeneratable.
	for _, slot := range packSlots {
		if strings.HasPrefix(slot, "bg_") {
			continue
		}
		var found bool
		for _, s := range packGenSlotOrder {
			if s == slot {
				found = true
			}
		}
		if !found {
			t.Errorf("slot %q exists in the format but cannot be generated", slot)
		}
	}
}

func TestEveryConditionHasStaging(t *testing.T) {
	for cond := range packConditions {
		if strings.TrimSpace(packGenConditionStaging[cond]) == "" {
			t.Errorf("condition %q has no staging text", cond)
		}
	}
	for cond := range packGenConditionStaging {
		if _, ok := packConditions[cond]; !ok {
			t.Errorf("staging exists for %q, which is not a real condition", cond)
		}
	}
}

func TestStylePresetsAreDistinctAndNonEmpty(t *testing.T) {
	seen := map[string]bool{}
	for _, s := range PackGenStyles {
		if s.ID == "" || s.Label == "" || strings.TrimSpace(s.Text) == "" {
			t.Errorf("style %+v has an empty field", s)
		}
		if seen[s.ID] {
			t.Errorf("duplicate style id %q", s.ID)
		}
		seen[s.ID] = true
	}
	if _, ok := packGenStyleByID("nope"); ok {
		t.Error("packGenStyleByID resolved a style that does not exist")
	}
}

func TestValidatePackGenSlots(t *testing.T) {
	idle := PackGenSlot{Slot: "idle"}

	cases := []struct {
		name    string
		slots   []PackGenSlot
		wantErr string // substring; "" means the set must be accepted
	}{
		{"minimal", []PackGenSlot{idle}, ""},
		{"full set", []PackGenSlot{
			{Slot: "idle"}, {Slot: "working"}, {Slot: "done"},
			{Slot: "awaiting"}, {Slot: "error"}, {Slot: "bored"},
		}, ""},
		{"one ambient plus one event on the same slot is fine", []PackGenSlot{
			idle, {Slot: "done"},
			{Slot: "done", When: "lateNight"}, // ambient
			{Slot: "done", When: "century"},   // event
		}, ""},
		{"empty", nil, "nothing selected"},
		{"no idle", []PackGenSlot{{Slot: "working"}}, "idle slot is required"},
		{"conditioned idle does not satisfy the requirement", []PackGenSlot{
			{Slot: "idle", When: "lateNight"},
		}, "idle slot is required"},
		{"background slot", []PackGenSlot{idle, {Slot: "bg_idle"}}, "not generated"},
		{"unknown slot", []PackGenSlot{idle, {Slot: "dancing"}}, "unknown slot"},
		{"unknown condition", []PackGenSlot{idle, {Slot: "done", When: "eclipse"}}, "unknown condition"},
		{"duplicate", []PackGenSlot{idle, {Slot: "done"}, {Slot: "done"}}, "selected twice"},
		{"two ambients on one slot", []PackGenSlot{
			idle,
			{Slot: "bored", When: "lateNight"},
			{Slot: "bored", When: "weekend"},
		}, "both be true at once"},
		// marathon is only reachable while something is running, so a marathon
		// variant of `bored` is art that can never display.
		{"condition unreachable in slot", []PackGenSlot{
			idle, {Slot: "bored", When: "marathon"},
		}, "cannot display in slot"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			err := validatePackGenSlots(tc.slots)
			if tc.wantErr == "" {
				if err != nil {
					t.Fatalf("want accepted, got %v", err)
				}
				return
			}
			if err == nil {
				t.Fatalf("want error containing %q, got nil", tc.wantErr)
			}
			if !strings.Contains(err.Error(), tc.wantErr) {
				t.Fatalf("want error containing %q, got %v", tc.wantErr, err)
			}
		})
	}
}

// The two-ambient message must name the conditions, and name them in a stable
// order — map iteration would otherwise make the text change between runs.
func TestValidateNamesConflictingConditionsStably(t *testing.T) {
	slots := []PackGenSlot{
		{Slot: "idle"},
		{Slot: "bored", When: "weekend"},
		{Slot: "bored", When: "lateNight"},
	}
	first := validatePackGenSlots(slots)
	if first == nil {
		t.Fatal("want an error")
	}
	if !strings.Contains(first.Error(), "lateNight, weekend") {
		t.Errorf("want both conditions named in sorted order, got %v", first)
	}
	for i := 0; i < 20; i++ {
		if got := validatePackGenSlots(slots); got.Error() != first.Error() {
			t.Fatalf("message is not stable:\n%v\n%v", first, got)
		}
	}
}

// Framing adherence varies far more between models than an identical-prompt
// assumption allowed for. Across two spikes and three base images, Nano Banana
// honoured "framed from the chest up" every time while FLUX.2 klein returned a
// torso-or-wider shot every time from the same words — and the host crops to a
// tall slot, so a wider composition loses the face.
func TestFramingLockIsPerModel(t *testing.T) {
	slot := PackGenSlot{Slot: "idle"}

	base := composePackPrompt(slot, "STYLE", "")
	if !strings.Contains(base, packGenFramingLock) {
		t.Error("the default framing lock is missing for an unlisted model")
	}

	klein := composePackPrompt(slot, "STYLE", "fal-ai/flux-2/klein/4b/edit")
	if strings.Contains(klein, packGenFramingLock) {
		t.Error("klein got the default framing wording, which it ignores")
	}
	if !strings.Contains(klein, packGenFramingByModel["fal-ai/flux-2/klein/4b/edit"]) {
		t.Error("klein did not get its own framing wording")
	}

	// Everything else about the prompt is model-independent: only framing is
	// overridden, so a model swap must not quietly change the character.
	for _, want := range []string{packGenIdentityLock, packGenSlotStaging["idle"], "STYLE"} {
		if !strings.Contains(klein, want) {
			t.Errorf("the per-model override dropped a lock that is not framing:\n%s", klein)
		}
	}
}

// Every model the provider offers must be reachable, and any model with its own
// framing wording must be one that actually exists — a typo'd key here would
// silently fall back to wording we measured that model ignoring.
func TestPerModelFramingKeysAreRealModels(t *testing.T) {
	offered := map[string]bool{}
	for _, p := range imagegen.Providers() {
		for _, m := range p.Models() {
			offered[m.ID] = true
		}
	}
	for id := range packGenFramingByModel {
		if !offered[id] {
			t.Errorf("framing override keyed on %q, which no provider offers", id)
		}
	}
}

// An explicit per-variant prompt is sent verbatim, so the framing override must
// not be appended to it — what the user sees in the browser is what runs.
func TestExplicitPromptIgnoresTheModelOverride(t *testing.T) {
	got := composePackPrompt(
		PackGenSlot{Slot: "idle", Prompt: "exactly this"}, "STYLE",
		"fal-ai/flux-2/klein/4b/edit")
	if got != "exactly this" {
		t.Errorf("got %q, want the verbatim prompt", got)
	}
}
