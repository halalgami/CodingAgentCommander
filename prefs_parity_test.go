package main

import (
	"os"
	"regexp"
	"sort"
	"strings"
	"testing"
)

// frontend/src/lib/prefsData.js has a public-override copy, and its own comment
// says both must carry identical DEFAULTS — but nothing enforced it. The two
// files drift silently: the export deletes the private copy and overlays the
// public one, its content gate looks only for specific vocabulary, and its
// build gate compiles Go. A key added to one and not the other would ship a
// public build whose prefs quietly differ, with no failure anywhere.
//
// This file is deliberately free of feature vocabulary so it SURVIVES the
// export — the invariant it guards matters just as much in the public build,
// and a guard that gets stripped from the tree it protects is no guard.
//
// Found while adding a key that had to be patched into both copies by hand.

var defaultsBlock = regexp.MustCompile(`(?s)export const DEFAULTS = Object\.freeze\(\{(.*?)\}\);`)

// prefKeys returns the pref names declared in a prefsData.js, with comments
// stripped so prose about a key is not mistaken for one.
func prefKeys(t *testing.T, path string) []string {
	t.Helper()
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("%s: %v", path, err)
	}
	m := defaultsBlock.FindStringSubmatch(string(raw))
	if m == nil {
		t.Fatalf("%s: no DEFAULTS block found — this test can no longer see what it guards", path)
	}
	body := regexp.MustCompile(`(?m)//.*$`).ReplaceAllString(m[1], "")

	var keys []string
	for _, km := range regexp.MustCompile(`([A-Za-z_$][\w$]*)\s*:`).FindAllStringSubmatch(body, -1) {
		keys = append(keys, km[1])
	}
	sort.Strings(keys)
	return keys
}

// overrideDir holds the copies this test compares against. The export deletes
// it — the tooling is not published — so in the exported tree there is nothing
// to compare and the invariant is vacuous.
const overrideDir = "scripts/_public-overrides"

const (
	privatePrefs = "frontend/src/lib/prefsData.js"
	publicPrefs  = overrideDir + "/frontend/src/lib/prefsData.js"
)

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

func TestPublicAndPrivatePrefsDeclareTheSameKeys(t *testing.T) {
	skipIfExported(t)
	private, public := privatePrefs, publicPrefs
	priv := prefKeys(t, private)
	pub := prefKeys(t, public)

	if len(priv) == 0 {
		t.Fatal("no keys parsed from the private prefs; the regex has gone stale")
	}
	if strings.Join(priv, ",") != strings.Join(pub, ",") {
		t.Errorf("prefs keys have drifted between the two copies\n private: %v\n  public: %v\n"+
			"Both files must declare the same DEFAULTS — see the note in %s.", priv, pub, private)
	}
}

// The override is a full replacement, not a patch, so a stale one silently
// reverts whatever the private file gained. Comparing the DEFAULTS values too
// catches a key that exists in both but was given a different default.
func TestPublicAndPrivatePrefsAgreeOnValues(t *testing.T) {
	skipIfExported(t)
	values := func(path string) map[string]string {
		raw, err := os.ReadFile(path)
		if err != nil {
			t.Fatalf("%s: %v", path, err)
		}
		m := defaultsBlock.FindStringSubmatch(string(raw))
		if m == nil {
			t.Fatalf("%s: no DEFAULTS block", path)
		}
		body := regexp.MustCompile(`(?m)//.*$`).ReplaceAllString(m[1], "")
		out := map[string]string{}
		for _, kv := range regexp.MustCompile(`([A-Za-z_$][\w$]*)\s*:\s*([^,\n}]+)`).FindAllStringSubmatch(body, -1) {
			out[kv[1]] = strings.TrimSpace(kv[2])
		}
		return out
	}
	priv := values(privatePrefs)
	pub := values(publicPrefs)

	for k, want := range priv {
		if got, ok := pub[k]; !ok {
			t.Errorf("public prefs is missing %q", k)
		} else if got != want {
			t.Errorf("default for %q differs: private %s, public %s", k, want, got)
		}
	}
}
