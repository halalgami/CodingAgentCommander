package main

// I7: nothing but a code comment ("Kept in sync with SLOT_CONDITIONS in
// pack.js") tied Go's companion vocabulary to the JS decision core's. This
// branch already contains a commit that fixed one real drift between them.
// This test reads the JS source directly and fails loudly the moment
// packSlots/packConditions/packCondSlots/packMoods disagree with
// SLOTS+BG_SLOTS/CONDITION_CLASS/SLOT_CONDITIONS (pack.js) or MOODS
// (mood.js) — parsing the JS with regexes is acceptable here precisely
// because the goal is "fail loudly on drift", not "build a JS parser".

import (
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"sort"
	"testing"
)

func readJSSource(t *testing.T, rel string) string {
	t.Helper()
	b, err := os.ReadFile(filepath.Join(".", rel))
	if err != nil {
		t.Fatalf("reading %s: %v (run go test from the repo root)", rel, err)
	}
	return string(b)
}

var quotedStringRe = regexp.MustCompile(`"([^"]*)"`)

func quotedStrings(s string) []string {
	var out []string
	for _, m := range quotedStringRe.FindAllStringSubmatch(s, -1) {
		out = append(out, m[1])
	}
	return out
}

// jsStringArray extracts `export const NAME = Object.freeze([...]);` and
// returns the quoted string literals, in source order.
func jsStringArray(t *testing.T, src, name string) []string {
	t.Helper()
	re := regexp.MustCompile(`export const ` + name + ` = Object\.freeze\(\[([^\]]*)\]\);`)
	m := re.FindStringSubmatch(src)
	if m == nil {
		t.Fatalf("could not find `export const %s = Object.freeze([...])` in source", name)
	}
	return quotedStrings(m[1])
}

// jsStringMap extracts `export const NAME = Object.freeze({ key: "value", ... });`.
func jsStringMap(t *testing.T, src, name string) map[string]string {
	t.Helper()
	re := regexp.MustCompile(`(?s)export const ` + name + ` = Object\.freeze\(\{(.*?)\}\);`)
	m := re.FindStringSubmatch(src)
	if m == nil {
		t.Fatalf("could not find `export const %s = Object.freeze({...})` in source", name)
	}
	entryRe := regexp.MustCompile(`(\w+):\s*"(\w+)"`)
	out := map[string]string{}
	for _, e := range entryRe.FindAllStringSubmatch(m[1], -1) {
		out[e[1]] = e[2]
	}
	return out
}

// jsSlotToConditions extracts SLOT_CONDITIONS — `slot: Object.freeze([...]),`
// per entry — keyed by slot, valued by its list of condition names.
func jsSlotToConditions(t *testing.T, src string) map[string][]string {
	t.Helper()
	re := regexp.MustCompile(`(?s)export const SLOT_CONDITIONS = Object\.freeze\(\{(.*?)\n\}\);`)
	m := re.FindStringSubmatch(src)
	if m == nil {
		t.Fatal("could not find `export const SLOT_CONDITIONS = Object.freeze({...})` in source")
	}
	entryRe := regexp.MustCompile(`(\w+):\s*Object\.freeze\(\[([^\]]*)\]\)`)
	out := map[string][]string{}
	for _, e := range entryRe.FindAllStringSubmatch(m[1], -1) {
		out[e[1]] = quotedStrings(e[2])
	}
	return out
}

func sortedCopy(in []string) []string {
	out := append([]string(nil), in...)
	sort.Strings(out)
	return out
}

func TestPackVocabularySyncedWithJS(t *testing.T) {
	packJS := readJSSource(t, "frontend/src/lib/companion/pack.js")
	moodJS := readJSSource(t, "frontend/src/lib/companion/mood.js")

	// packSlots is authoring/display order: SLOTS then BG_SLOTS (packSlots's
	// own comment says "six foreground, four background", in that order).
	wantSlots := append(jsStringArray(t, packJS, "SLOTS"), jsStringArray(t, packJS, "BG_SLOTS")...)
	if !slices.Equal(packSlots, wantSlots) {
		t.Errorf("packSlots = %v, want SLOTS+BG_SLOTS from pack.js = %v", packSlots, wantSlots)
	}

	// packConditions vs CONDITION_CLASS: identical map.
	wantConditions := jsStringMap(t, packJS, "CONDITION_CLASS")
	for name, class := range wantConditions {
		if got, ok := packConditions[name]; !ok || got != class {
			t.Errorf("packConditions[%q] = %q, pack.js's CONDITION_CLASS says %q", name, got, class)
		}
	}
	for name, class := range packConditions {
		if wantConditions[name] != class {
			t.Errorf("packConditions has %q: %q, absent (or different) in pack.js's CONDITION_CLASS", name, class)
		}
	}

	// packCondSlots (keyed by condition) vs SLOT_CONDITIONS (keyed by slot):
	// transposes of each other. Invert pack.js's version and compare sets —
	// order is authoring order on each side and carries no meaning.
	slotConditions := jsSlotToConditions(t, packJS)
	wantCondSlots := map[string][]string{}
	for slot, conds := range slotConditions {
		for _, cond := range conds {
			wantCondSlots[cond] = append(wantCondSlots[cond], slot)
		}
	}
	for cond, slots := range wantCondSlots {
		got := sortedCopy(packCondSlots[cond])
		want := sortedCopy(slots)
		if !slices.Equal(got, want) {
			t.Errorf("packCondSlots[%q] = %v, pack.js's SLOT_CONDITIONS implies %v", cond, got, want)
		}
	}
	for cond, slots := range packCondSlots {
		got := sortedCopy(slots)
		want := sortedCopy(wantCondSlots[cond])
		if !slices.Equal(got, want) {
			t.Errorf("packCondSlots[%q] = %v, pack.js's SLOT_CONDITIONS implies %v", cond, got, want)
		}
	}

	// packMoods vs MOODS.
	wantMoods := jsStringArray(t, moodJS, "MOODS")
	for _, mood := range wantMoods {
		if !packMoods[mood] {
			t.Errorf("packMoods is missing %q, present in mood.js's MOODS", mood)
		}
	}
	wantSet := map[string]bool{}
	for _, m := range wantMoods {
		wantSet[m] = true
	}
	for mood := range packMoods {
		if !wantSet[mood] {
			t.Errorf("packMoods has %q, which mood.js's MOODS does not", mood)
		}
	}
}
