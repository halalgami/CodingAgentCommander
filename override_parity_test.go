package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// sharedBindings are methods that must exist in BOTH the private app.go and the
// override copy. The export replaces app.go wholesale, so a binding added only
// to the private file yields a mirror without the feature — and neither export
// gate notices: the grep gate looks for leaked strings, and the build gate
// compiles happily because the absence is additive.
//
// This is a curated list rather than a full set comparison: the override
// legitimately omits methods that the export strips, and enumerating those here
// would put their vocabulary into a file that survives the export.
var sharedBindings = []string{
	"func (a *App) DiscoverOllamaModels()",
	"func (a *App) DiscoverZenModels()",
	"func (a *App) ListProviders()",
	"func (a *App) AddProvider(",
	"func (a *App) AddModel(",
	"func (a *App) KeyStatus()",
}

func TestOverrideCarriesSharedBindings(t *testing.T) {
	// Skip on the absence of the override DIRECTORY, not a file: in the exported
	// tree the directory is gone, and a missing file there would mean something
	// different (a deleted override) than it does here.
	if _, err := os.Stat(overrideDir); os.IsNotExist(err) {
		t.Skip("no override directory: this is an exported tree")
	}
	priv, err := os.ReadFile("app.go")
	if err != nil {
		t.Fatal(err)
	}
	over, err := os.ReadFile(filepath.Join(overrideDir, "app.go"))
	if err != nil {
		t.Fatal(err)
	}
	for _, sig := range sharedBindings {
		if !strings.Contains(string(priv), sig) {
			t.Errorf("app.go is missing %q", sig)
		}
		if !strings.Contains(string(over), sig) {
			t.Errorf("%s/app.go is missing %q -- the export would ship a mirror without it", overrideDir, sig)
		}
	}
}

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

// sharedAppSvelteMounts are markers that must exist in BOTH copies of
// App.svelte. Same failure mode as sharedBindings: the export replaces the
// file wholesale, so a component mounted only in the private copy yields a
// mirror where the feature is unreachable — and no gate notices, because an
// absence compiles and greps clean.
var sharedAppSvelteMounts = []string{
	`{#if app.drawer === "docview"}<DocViewer />{/if}`,
	`import DocViewer from "./lib/components/DocViewer.svelte";`,
	// The doc-viewer Playwright spec SURVIVES the export and drives the viewer
	// through this seam, so a mirror missing it has a failing test suite rather
	// than a missing feature. Same class as the mounts above.
	`window.__openDoc = openDoc;`,
	// The index drawer (Task 11): its own mount, its own import, and the same
	// class of window seam the spec drives it through.
	`{#if app.drawer === "docs"}<DocsDrawer />{/if}`,
	`import DocsDrawer from "./lib/components/DocsDrawer.svelte";`,
	`window.__openDocsList = openDocsList;`,
}

func TestOverrideAppSvelteCarriesSharedMounts(t *testing.T) {
	skipIfExported(t)
	rel := filepath.Join("frontend", "src", "App.svelte")
	priv, err := os.ReadFile(rel)
	if err != nil {
		t.Fatal(err)
	}
	over, err := os.ReadFile(filepath.Join(overrideDir, rel))
	if err != nil {
		t.Fatal(err)
	}
	for _, marker := range sharedAppSvelteMounts {
		if !strings.Contains(string(priv), marker) {
			t.Errorf("%s is missing %q", rel, marker)
		}
		if !strings.Contains(string(over), marker) {
			t.Errorf("%s/%s is missing %q -- the export would ship a mirror without it", overrideDir, rel, marker)
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
