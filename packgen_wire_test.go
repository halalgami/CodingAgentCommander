package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"testing"
)

// The wire contract between Go and the webview.
//
// This file exists because of a real bug that shipped past both suites. The Go
// tests used Go structs and the frontend tests used hand-written fixtures with
// lowercase keys, so NEITHER ever looked at the bytes that actually cross the
// binding. PackGenStyle and PackGenSlot were unexported until they were
// exported *for* the binding — which gave them visibility but no json tags, so
// they marshalled as {"Slot":…,"Staging":…} while the wizard read `.slot` and
// `.staging`. Every field arrived undefined: the style picker was blank and
// Generate could never enable, because the slot names the blocker checks were
// all the string "undefined".
//
// The asymmetry is what hid it. encoding/json matches keys case-INSENSITIVELY
// when UNmarshalling, so the inbound direction (a request from the wizard)
// worked by luck the whole time while the outbound direction was empty.

// wireKeys marshals v and returns its top-level object keys. For a slice, it
// reads the first element — every element of a homogeneous slice shares a
// shape, so one is enough.
func wireKeys(t *testing.T, v any) []string {
	t.Helper()
	b, err := json.Marshal(v)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	trimmed := strings.TrimSpace(string(b))
	if strings.HasPrefix(trimmed, "[") {
		var arr []json.RawMessage
		if err := json.Unmarshal(b, &arr); err != nil {
			t.Fatalf("unmarshal array: %v", err)
		}
		if len(arr) == 0 {
			t.Fatal("empty payload: nothing to check a wire shape against")
		}
		b = arr[0]
	}
	var obj map[string]json.RawMessage
	if err := json.Unmarshal(b, &obj); err != nil {
		t.Fatalf("unmarshal object: %v", err)
	}
	keys := make([]string, 0, len(obj))
	for k := range obj {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}

// Every key the wizard reads must exist on the wire, spelled exactly as the
// JavaScript spells it. Asserting the FULL key set rather than a subset also
// catches a field renamed in Go without the frontend following.
func TestBindingPayloadsUseTheKeysTheWizardReads(t *testing.T) {
	f := newPackGenFixture(t)
	if _, err := f.app.StartPackGen(f.request()); err != nil {
		t.Fatal(err)
	}
	f.waitDone(t)

	cases := []struct {
		name string
		got  []string
		want []string
	}{
		{"PackGenStyles", wireKeys(t, f.app.PackGenStyles()),
			[]string{"id", "label", "text"}},
		{"PackGenSlots", wireKeys(t, f.app.PackGenSlots()),
			[]string{"prompt", "slot", "staging", "when"}},
		{"PackGenProviders", wireKeys(t, f.app.PackGenProviders()),
			[]string{"hasKey", "id", "models"}},
		{"PackGenConditions", wireKeys(t, f.app.PackGenConditions()),
			[]string{"class", "label", "name", "slots"}},
		{"PackGenEstimate", wireKeys(t, f.app.PackGenEstimate("fake/model", 6)),
			[]string{"known", "note", "usd"}},
		{"PackGenStatus", wireKeys(t, f.app.PackGenStatus()),
			[]string{"billed", "done", "hasArt", "id", "items", "name", "saved", "status", "total"}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if strings.Join(tc.got, ",") != strings.Join(tc.want, ",") {
				t.Errorf("wire keys are\n  %v\nbut the wizard reads\n  %v", tc.got, tc.want)
			}
			// A capitalised key is the specific failure this file exists for.
			for _, k := range tc.got {
				if k != "" && k[0] >= 'A' && k[0] <= 'Z' {
					t.Errorf("key %q is capitalised — the Go field has no json tag", k)
				}
			}
		})
	}
}

// Nested payloads matter just as much: the model picker reads m.label and
// m.usd off each entry of provider.models.
func TestNestedModelPayloadUsesLowercaseKeys(t *testing.T) {
	f := newPackGenFixture(t)
	b, err := json.Marshal(f.app.PackGenProviders())
	if err != nil {
		t.Fatal(err)
	}
	var provs []struct {
		Models []map[string]json.RawMessage `json:"models"`
	}
	if err := json.Unmarshal(b, &provs); err != nil {
		t.Fatal(err)
	}
	var checked int
	for _, p := range provs {
		for _, m := range p.Models {
			checked++
			for _, want := range []string{"id", "label", "mode", "usd", "known"} {
				if _, ok := m[want]; !ok {
					got := make([]string, 0, len(m))
					for k := range m {
						got = append(got, k)
					}
					sort.Strings(got)
					t.Fatalf("model payload is missing %q; it has %v", want, got)
				}
			}
		}
	}
	if checked == 0 {
		t.Fatal("no models to check")
	}
}

// The run items drive the review grid, including the preview URL.
func TestRunItemPayloadUsesLowercaseKeys(t *testing.T) {
	f := newPackGenFixture(t)
	if _, err := f.app.StartPackGen(f.request()); err != nil {
		t.Fatal(err)
	}
	f.waitDone(t)

	b, err := json.Marshal(f.app.PackGenStatus())
	if err != nil {
		t.Fatal(err)
	}
	var st struct {
		Items []map[string]json.RawMessage `json:"items"`
	}
	if err := json.Unmarshal(b, &st); err != nil {
		t.Fatal(err)
	}
	if len(st.Items) == 0 {
		t.Fatal("no items")
	}
	for _, want := range []string{"slot", "when", "status", "preview"} {
		if _, ok := st.Items[0][want]; !ok {
			t.Errorf("item payload is missing %q: %v", want, st.Items[0])
		}
	}
}

// The strongest guard available without running a browser: read the wizard's
// own source and confirm every property it reads off a Go payload is a key Go
// actually emits. A field renamed on either side fails here.
func TestWizardSourceOnlyReadsKeysGoEmits(t *testing.T) {
	f := newPackGenFixture(t)

	emitted := map[string]bool{}
	for _, keys := range [][]string{
		wireKeys(t, f.app.PackGenStyles()),
		wireKeys(t, f.app.PackGenSlots()),
		wireKeys(t, f.app.PackGenProviders()),
		wireKeys(t, f.app.PackGenConditions()),
		wireKeys(t, f.app.PackGenEstimate("fake/model", 1)),
		wireKeys(t, f.app.PackGenStatus()),
		{"id", "label", "mode", "usd", "known"},        // provider.models[]
		{"slot", "when", "status", "error", "preview"}, // state.items[]
		{"path", "w", "h", "warning"},                  // PackGenBase
	} {
		for _, k := range keys {
			emitted[k] = true
		}
	}

	// Index the emitted keys by their lowercased form, so a JS access can be
	// matched against Go's spelling regardless of how the JS spells it.
	byLower := map[string]string{}
	for k := range emitted {
		byLower[strings.ToLower(k)] = k
	}

	// Scrape every property access in the wizard's source and flag any that is
	// a MIS-CASED version of a key Go emits — `.Slot` where Go sends `slot`.
	//
	// Deliberately not "flag any property Go does not emit": these files also
	// touch plain JS objects and DOM nodes, so that rule is all false
	// positives. Matching only on a case-insensitive collision targets exactly
	// the bug this file exists for, and needs no list anyone has to maintain.
	files := []string{
		"src/lib/companion/packgen.svelte.js",
		"src/lib/companion/packgen-logic.js",
		"src/lib/companion/PackGenPanel.svelte",
	}
	// No whitespace after the dot: a property access never has one, but English
	// prose does — "The cost line. Known=false…" in a doc comment matched the
	// looser pattern and reported a bug that was not there.
	prop := regexp.MustCompile(`\.([A-Za-z_$][\w$]*)`)

	for _, file := range files {
		src, err := os.ReadFile(filepath.Join("frontend", file))
		if err != nil {
			t.Fatalf("%s: %v", file, err)
		}
		for _, m := range prop.FindAllStringSubmatch(stripComments(string(src)), -1) {
			read := m[1]
			goKey, collides := byLower[strings.ToLower(read)]
			if collides && goKey != read {
				t.Errorf("%s reads .%s but Go emits %q — the field's json tag and the JS disagree",
					file, read, goKey)
			}
		}
	}

	// And confirm the scrape is actually looking at something: these are the
	// keys whose absence produced the original bug, so if the wizard stops
	// reading them the guard above has quietly stopped guarding.
	panel, err := os.ReadFile(filepath.Join("frontend", "src/lib/companion/PackGenPanel.svelte"))
	if err != nil {
		t.Fatal(err)
	}
	logic, err := os.ReadFile(filepath.Join("frontend", "src/lib/companion/packgen-logic.js"))
	if err != nil {
		t.Fatal(err)
	}
	both := string(panel) + string(logic)
	for _, want := range []string{".slot", ".staging", ".label", ".hasKey", ".known", ".preview"} {
		if !strings.Contains(both, want) {
			t.Errorf("the wizard no longer reads %s; this test is no longer covering the payload", want)
		}
	}
}

// stripComments removes // line, /* */ block and <!-- --> markup comments, so
// the property scan reads code rather than the prose describing it. Crude but
// sufficient: it only has to keep English sentences out of a regex, not parse
// JavaScript.
var (
	reLineComment  = regexp.MustCompile(`(?m)//.*$`)
	reBlockComment = regexp.MustCompile(`(?s)/\*.*?\*/`)
	reHTMLComment  = regexp.MustCompile(`(?s)<!--.*?-->`)
)

func stripComments(s string) string {
	s = reBlockComment.ReplaceAllString(s, " ")
	s = reHTMLComment.ReplaceAllString(s, " ")
	return reLineComment.ReplaceAllString(s, " ")
}
