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
