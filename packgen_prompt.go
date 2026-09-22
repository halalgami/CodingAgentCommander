package main

import (
	"fmt"
	"sort"
	"strings"
)

// Prompt composition for generated packs.
//
// The spike (docs/superpowers/specs/spike-2026-08-30/) established that a set of
// images reads as ONE character only when four things are locked, and that the
// locks matter in different ways:
//
//   - identity  names the specific features to hold. A generic "keep her the
//     same" is measurably weaker than naming face, skin tone and garments.
//   - staging   per-slot, and it AMPLIFIES the state rather than merely varying
//     it. This is the only lock the user edits per slot.
//   - framing   FUNCTIONAL, not taste. The sidebar renders art object-fit:cover
//     anchored top in a ~300x560 slot, so a full-body composition is cropped to
//     head-and-shoulders. Spike round 2 lost an entire armchair slouch this way:
//     the pose that carried the meaning was below the crop.
//   - style     one register, applied VERBATIM to every slot. This is the lock
//     that makes six images a set instead of six pictures, which is why it is a
//     preset rather than a free-text field.
//
// Star topology: every slot derives from the BASE image, never from a previously
// generated slot. Chaining edits is the documented way drift accumulates.

// PackGenStyle is a shared art register. Only "whimsical" is validated by the
// spike; the other two are the same template with a different register string
// and are offered as alternatives, not as equals.
type PackGenStyle struct {
	ID    string `json:"id"`
	Label string `json:"label"`
	Text  string `json:"text"`
}

var PackGenStyles = []PackGenStyle{
	{
		ID:    "whimsical",
		Label: "Whimsical illustration",
		Text: "Expressive, exaggerated, whimsical and characterful — a warm stylised " +
			"character illustration rather than a realistic photograph. Playful, " +
			"charming, a little theatrical. Strong readable emotion.",
	},
	{
		ID:    "painterly",
		Label: "Soft painterly",
		Text: "Soft painterly digital illustration with visible brushwork and warm " +
			"muted colour. Gentle, expressive, unhurried. Clear readable emotion.",
	},
	{
		ID:    "semireal",
		Label: "Semi-real",
		Text: "Semi-realistic stylised portrait, cinematic lighting, lightly " +
			"illustrated rather than photographic. Grounded but expressive.",
	},
}

func packGenStyleByID(id string) (PackGenStyle, bool) {
	for _, s := range PackGenStyles {
		if s.ID == id {
			return s, true
		}
	}
	return PackGenStyle{}, false
}

// packGenIdentityLock is prepended to every prompt. The phrasing names features
// explicitly because that measurably outperforms a generic instruction, and it
// is deliberately not editable: it is the lock, not the content.
const packGenIdentityLock = "Keep the face, facial features, skin tone, hair and clothing of the " +
	"person in the reference image completely unchanged and identical. It must " +
	"clearly be the same person."

// packGenFramingLock exists because of how the host crops, not because of taste.
//
// "Centred" is in here because of a real generated pack: the model composes
// each image independently, so one slot placed the subject right of centre to
// make room for a cat and the next centred her. The host cover-crops to a tall
// slot, so an off-centre subject reads as badly framed, and focusX can only
// correct it after the fact — by hand, per variant. Asking for it up front is
// free; correcting it afterwards is not.
const packGenFramingLock = "Vertical portrait composition, upper body only, head and shoulders " +
	"filling most of the frame, framed from the chest up. The person must be " +
	"centred horizontally in the frame, with their face in the upper middle."

// packGenSlotStaging is the editable per-slot default. Each amplifies its state
// with a setting rather than only describing an expression — spike round 3 found
// that a set with staged backgrounds reads as scenes in one story, while the same
// set photoreal and unstaged read as the subject teleporting between rooms.
var packGenSlotStaging = map[string]string{
	"idle": "She waits patiently with a faint amused half-smile, chin resting on one hand. " +
		"Behind her, a cosy window seat with rain running down the glass, a sleeping cat " +
		"and trailing houseplants.",
	"working": "She is hunched forward in fierce concentration, eyes narrowed, lit from below " +
		"by a bright screen. Behind her, a dark room full of floating glowing panels and " +
		"cascading code.",
	"done": "She is beaming with delight, head lifted, eyes bright and triumphant. Behind her, " +
		"warm golden light, drifting confetti and a few tiny celebratory sparks.",
	"awaiting": "She leans in expectantly, both eyebrows raised high, making direct eye contact. " +
		"Behind her, an oversized ornate clock and slowly drifting question marks.",
	"error": "She is alarmed, brow furrowed hard, mouth tight. Behind her, spinning red alarm " +
		"lights, drifting smoke and a shower of tiny sparks.",
	"bored": "She is thoroughly fed up, cheek squashed against one propped-up hand, eyes rolled " +
		"away. Behind her, a dim dusty room, a tumbleweed and a clock whose hands have " +
		"barely moved.",
}

// packGenConditionStaging adds a phrase for a condition-gated variant, so a
// lateNight variant of a slot is visibly the night version rather than a
// near-duplicate that wastes an image.
var packGenConditionStaging = map[string]string{
	"lateNight":     "It is the middle of the night: dim warm lamplight, everything else dark, she is tired.",
	"weekend":       "It is a lazy weekend: relaxed, unhurried, soft daylight.",
	"freshInstall":  "Everything is brand new and unfamiliar, a sense of first-day curiosity.",
	"firstRunOfDay": "It is early morning, first light, a fresh start to the day.",
	"marathon":      "This has been going on for hours; the light has changed and she has settled in.",
	"longIdle":      "Nothing has happened for a long time; stillness and dust in the air.",
	"century":       "A milestone moment worth marking, quietly celebratory.",
	"streak":        "One thing after another in quick succession, a sense of momentum.",
}

// packGenSlotOrder is the order slots are generated and presented in. It follows
// the format's own foreground list; bg_* is deliberately absent because no host
// renders it and generating it would spend money on invisible art.
var packGenSlotOrder = []string{"idle", "working", "done", "awaiting", "error", "bored"}

// PackGenSlot is one thing to generate: a slot, optionally gated by a condition.
// The json tags are load-bearing, not decoration. These two types cross a
// Wails binding, and Go marshals an untagged field under its EXACT Go name —
// so without them the frontend receives {"Slot":…} while reading `.slot` and
// every field is undefined. It went unnoticed because the asymmetry hides it:
// encoding/json matches keys case-INSENSITIVELY when unmarshalling, so the
// inbound direction (StartPackGen's request) worked by luck while the outbound
// direction was silently empty.
type PackGenSlot struct {
	Slot    string `json:"slot"`    // must be one of packGenSlotOrder
	When    string `json:"when"`    // "" for the base variant, else a condition from packCondSlots
	Staging string `json:"staging"` // editable; defaults from packGenSlotStaging
	// Prompt, when set, is sent VERBATIM instead of being composed from the
	// staging text and the style. Regeneration uses it so the prompt shown in
	// the browser — the one that produced the image being replaced — is exactly
	// the one that runs, editable and without silent recomposition.
	Prompt string `json:"prompt"`
}

// packGenFramingByModel overrides the framing lock for models that measurably
// ignore the default wording.
//
// Not a taste adjustment. Across two spikes and three base images, Nano Banana
// honoured "head and shoulders, framed from the chest up" every time, while
// FLUX.2 klein returned a torso-or-wider shot every time from the identical
// instruction. The host crops to a tall slot, so a wider composition loses the
// face — the thing the whole feature is about. Blunter, more imperative wording
// is what klein responds to.
//
// Keyed by model id here rather than in internal/imagegen because prompt
// wording is companion-domain: imagegen knows transport, not what to say.
var packGenFramingByModel = map[string]string{
	"fal-ai/flux-2/klein/4b/edit": "EXTREME CLOSE-UP PORTRAIT. Crop tightly to the head and " +
		"shoulders only. The face must fill the upper half of the frame. Do not " +
		"show the waist, hips or legs. Vertical portrait orientation, subject " +
		"centred horizontally.",
}

func packGenFramingFor(model string) string {
	if s, ok := packGenFramingByModel[model]; ok {
		return s
	}
	return packGenFramingLock
}

// composePackPrompt assembles the four locks in order. The style text is passed
// in rather than looked up so the caller proves it resolved a real preset once,
// and every slot in a run then receives the identical string.
//
// The model id is passed because one of the locks is model-dependent: framing
// adherence varies far more between models than the identical-prompt assumption
// allowed for.
func composePackPrompt(s PackGenSlot, styleText, model string) string {
	if p := strings.TrimSpace(s.Prompt); p != "" {
		return p
	}
	staging := strings.TrimSpace(s.Staging)
	if staging == "" {
		staging = packGenSlotStaging[s.Slot]
	}
	parts := []string{packGenIdentityLock, staging}
	if cond := packGenConditionStaging[s.When]; s.When != "" && cond != "" {
		parts = append(parts, cond)
	}
	parts = append(parts, packGenFramingFor(model), styleText)
	return strings.Join(parts, " ")
}

// validatePackGenSlots checks a requested set BEFORE anything is generated, so a
// pack that cannot load cleanly is refused rather than paid for.
//
// Two of these rules exist because the loader warns about them and a warning on
// art the user just bought is a bad experience:
//
//   - the format requires `idle`, and a pack without it warns on every load.
//   - a slot carrying two DIFFERENT ambient conditions warns, because both can
//     hold at once and the resolver cannot say which was meant.
func validatePackGenSlots(slots []PackGenSlot) error {
	if len(slots) == 0 {
		return fmt.Errorf("nothing selected to generate")
	}
	var hasPlainIdle bool
	ambient := map[string]map[string]bool{} // slot -> set of ambient conditions
	seen := map[string]bool{}

	for _, s := range slots {
		if !packSlotKnown(s.Slot) {
			return fmt.Errorf("unknown slot %q", s.Slot)
		}
		if strings.HasPrefix(s.Slot, "bg_") {
			return fmt.Errorf("slot %q is not generated: no current layout renders the background slots", s.Slot)
		}
		key := s.Slot + "|" + s.When
		if seen[key] {
			return fmt.Errorf("slot %q is selected twice with the same condition", s.Slot)
		}
		seen[key] = true

		if s.When == "" {
			if s.Slot == "idle" {
				hasPlainIdle = true
			}
			continue
		}
		class, ok := packConditions[s.When]
		if !ok {
			return fmt.Errorf("unknown condition %q", s.When)
		}
		if !packCondAllowed(s.When, s.Slot) {
			return fmt.Errorf("condition %q cannot display in slot %q", s.When, s.Slot)
		}
		if class == "ambient" {
			if ambient[s.Slot] == nil {
				ambient[s.Slot] = map[string]bool{}
			}
			ambient[s.Slot][s.When] = true
		}
	}

	if !hasPlainIdle {
		return fmt.Errorf("the idle slot is required: a pack without it warns on every load")
	}
	for slot, set := range ambient {
		if len(set) > 1 {
			names := make([]string, 0, len(set))
			for n := range set {
				names = append(names, n)
			}
			sort.Strings(names)
			return fmt.Errorf("slot %q has two conditions that can both be true at once (%s); pick one",
				slot, strings.Join(names, ", "))
		}
	}
	return nil
}

// validateRegenSlots is validatePackGenSlots minus the "must include idle"
// rule. A regeneration replaces part of a pack that already has an idle slot,
// and demanding one here would mean re-buying the idle image to fix any other
// scene — which is the exact waste per-variant regeneration exists to avoid.
//
// Every other rule still applies: the slot must exist, background slots are not
// generated, a condition must be able to display in its slot, and one slot may
// not carry two ambient conditions.
func validateRegenSlots(slots []PackGenSlot) error {
	if len(slots) == 0 {
		return fmt.Errorf("nothing selected to regenerate")
	}
	// Reuse the full validator by lending it the idle slot it insists on, then
	// discard the borrowed entry. Duplicating the rules would let the two drift,
	// and the drift would only ever be discovered by paying for bad art.
	probe := append([]PackGenSlot{{Slot: "idle"}}, slots...)
	for _, s := range slots {
		if s.Slot == "idle" && s.When == "" {
			probe = slots // already present; the borrowed one would be a duplicate
			break
		}
	}
	return validatePackGenSlots(probe)
}
