package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// overrideDir holds the copies this test compares against. The export deletes
// it — the tooling is not published — so in the exported tree there is nothing
// to compare and the invariant is vacuous.
const overrideDir = "scripts/_public-overrides"

// skipIfExported distinguishes "this is the published tree" from "the file is
// missing", which are the same os.Stat error but opposite meanings. Skipping on
// the absence of the whole directory is safe; skipping on a missing FILE would
// hide exactly the drift this test exists to catch.
func skipIfExported(t *testing.T) {
	t.Helper()
	if _, err := os.Stat(overrideDir); os.IsNotExist(err) {
		t.Skip("running inside the exported tree, where the override copies are not published")
	}
}

// app.go, app_test.go, App.svelte and prefsData.js were overridden until the
// sidebar companion was published (spec 2026-09-07 §7): the overrides existed
// only to cut companion code out of shared files. That code has since been
// lifted into overlay-only files the export deletes wholesale, so these four
// carry nothing private and are published verbatim — no override, and the
// parity tests that guarded them were deleted with them. Four fewer files that
// can silently drift.

// providerLabelParity guards the same drift in the store the export also
// overlays: a provider added to one label map and not the other renders as a
// raw type string in the mirror.
func TestOverrideStoreCarriesProviderLabels(t *testing.T) {
	if _, err := os.Stat(overrideDir); os.IsNotExist(err) {
		t.Skip("no override directory: this is an exported tree")
	}
	rel := filepath.Join("frontend", "src", "lib", "stores.svelte.js")
	priv, err := os.ReadFile(rel)
	if err != nil {
		t.Fatal(err)
	}
	over, err := os.ReadFile(filepath.Join(overrideDir, rel))
	if err != nil {
		t.Fatal(err)
	}
	for _, ptype := range []string{"opencode-go", "bedrock", "ollama-cloud"} {
		if !strings.Contains(string(priv), ptype) {
			t.Errorf("%s is missing provider label %q", rel, ptype)
		}
		if !strings.Contains(string(over), ptype) {
			t.Errorf("%s/%s is missing provider label %q", overrideDir, rel, ptype)
		}
	}
}

// The public CI cannot run the export tripwire: the export deliberately does
// not publish its own tooling (export-public.sh deletes itself on the way
// out), so a ci.yml carrying that job fails on every commit to the mirror. It
// did exactly that for four consecutive syncs before the override existed.
//
// Both halves are asserted, because either one alone rots quietly: the private
// file must KEEP the job (it is the leak guard) and the override must NOT have
// it (it cannot work there).
func TestOverrideCIDropsTheExportTripwire(t *testing.T) {
	skipIfExported(t)
	rel := filepath.Join(".github", "workflows", "ci.yml")
	priv, err := os.ReadFile(rel)
	if err != nil {
		t.Fatal(err)
	}
	over, err := os.ReadFile(filepath.Join(overrideDir, rel))
	if err != nil {
		t.Fatalf("no public ci.yml override: %v -- the mirror's CI will run a job whose script the export deletes", err)
	}
	if !strings.Contains(string(priv), "export-tripwire") {
		t.Error("the private ci.yml no longer runs the export tripwire, which is the leak guard")
	}
	if strings.Contains(string(over), "export-tripwire") ||
		strings.Contains(string(over), "export-public.sh") {
		t.Error("the public ci.yml override references the export tooling, which is not published")
	}
	// A job must survive, or the override is a CI file that tests nothing.
	if !strings.Contains(string(over), "runs-on:") {
		t.Error("the public ci.yml override has no jobs left")
	}
}

// The SettingsDrawer override is no longer a whole-section deletion: the public
// build keeps the Companion section (sidebar kind + pack controls) and drops
// only the avatar radio and the avatar-only controls. A partial strip drifts in
// both directions, so both are asserted.
func TestSettingsDrawerOverrideKeepsSidebarAndDropsAvatar(t *testing.T) {
	skipIfExported(t)
	rel := filepath.Join("frontend", "src", "lib", "components", "SettingsDrawer.svelte")
	priv, err := os.ReadFile(rel)
	if err != nil {
		t.Fatal(err)
	}
	over, err := os.ReadFile(filepath.Join(overrideDir, rel))
	if err != nil {
		t.Fatal(err)
	}

	// Present in both: the sidebar kind and its pack controls are public.
	keep := []string{
		`data-testid="companion-kind-panel"`,
		`data-testid="companion-kind-off"`,
		`data-testid="companion-create-pack"`,
		`data-testid="companion-pick-pack"`,
		`data-testid="companion-clear-pack"`,
		"docs/companion-pack-format.md",
	}
	for _, m := range keep {
		if !strings.Contains(string(priv), m) {
			t.Errorf("%s is missing %q", rel, m)
		}
		if !strings.Contains(string(over), m) {
			t.Errorf("%s/%s is missing %q -- the mirror would ship the section without it", overrideDir, rel, m)
		}
	}

	// Private only: the overlay has no implementation in the public build, and
	// the model label names the bundled model, which is gate vocabulary.
	drop := []string{
		`data-testid="companion-kind-avatar"`,
		`data-testid="companion-size"`,
		`data-testid="companion-opacity"`,
		`data-testid="companion-jiggle-hair"`,
		`data-testid="companion-pick-model"`,
		`data-testid="companion-reset-pos"`,
		"companionSetSize",
		"companionPickModel",
	}
	for _, m := range drop {
		if !strings.Contains(string(priv), m) {
			t.Errorf("%s no longer has %q -- this test is guarding something that moved", rel, m)
		}
		if strings.Contains(string(over), m) {
			t.Errorf("%s/%s still has %q -- it calls a binding the public build does not have", overrideDir, rel, m)
		}
	}
}

// stores.svelte.js keeps an override for one reason: five bindings that exist
// only in the private build are imported BY NAME, and a missing named import
// is a build failure rather than a silent absence. Everything else companion
// must be present in both copies.
func TestStoreOverrideKeepsSidebarActionsAndDropsOverlayBindings(t *testing.T) {
	skipIfExported(t)
	rel := filepath.Join("frontend", "src", "lib", "stores.svelte.js")
	priv, err := os.ReadFile(rel)
	if err != nil {
		t.Fatal(err)
	}
	over, err := os.ReadFile(filepath.Join(overrideDir, rel))
	if err != nil {
		t.Fatal(err)
	}
	for _, m := range []string{
		"companionSetKind", "companionLoadPack", "companionPickPack",
		"companionClearPack", "companionState", "companionPack",
	} {
		if !strings.Contains(string(priv), m) {
			t.Errorf("%s is missing %q", rel, m)
		}
		if !strings.Contains(string(over), m) {
			t.Errorf("%s/%s is missing %q", overrideDir, rel, m)
		}
	}
	for _, m := range []string{
		"SetCompanionSize", "SetCompanionOpacity", "SetCompanionJiggle",
		"ResetCompanionPos", "PickCompanionModel",
	} {
		if strings.Contains(string(over), m) {
			t.Errorf("%s/%s imports %q, which the public build's bindings do not have",
				overrideDir, rel, m)
		}
	}
}
