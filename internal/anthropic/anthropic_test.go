package anthropic

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestCatalogIsWellFormed(t *testing.T) {
	seen := map[string]bool{}
	for _, m := range Catalog() {
		if seen[m.ID] {
			t.Errorf("duplicate id %q", m.ID)
		}
		seen[m.ID] = true
		if !strings.HasPrefix(m.ID, "claude-") {
			t.Errorf("id %q does not look like a model id", m.ID)
		}
		if m.Label == "" {
			t.Errorf("%s has no label", m.ID)
		}
		// A zero context window would make the session card's fullness band
		// divide by nothing. Live-discovered models may be unknown; built-in
		// ones are not.
		if m.MaxInputTokens <= 0 || m.MaxOutputTokens <= 0 {
			t.Errorf("%s has limits %d/%d; built-in entries must carry real ones",
				m.ID, m.MaxInputTokens, m.MaxOutputTokens)
		}
	}
	if !seen[DefaultID] {
		t.Errorf("DefaultID %q is not in Catalog; a first run would write a config Load rejects", DefaultID)
	}
	if Catalog()[0].ID != DefaultID {
		t.Errorf("Catalog is ordered %q first but DefaultID is %q", Catalog()[0].ID, DefaultID)
	}
}

// Catalog ids must be aliases. A dated id here would collide with the same
// model's alias the moment someone added one, and dated ids go stale in configs.
func TestCatalogUsesAliasesNotDatedSnapshots(t *testing.T) {
	for _, m := range Catalog() {
		if Canonical(m.ID) != m.ID {
			t.Errorf("%s is a dated snapshot id; the catalog must carry the alias %q", m.ID, Canonical(m.ID))
		}
	}
}

func TestCanonicalStripsOnlyDatedSuffixes(t *testing.T) {
	cases := map[string]string{
		"claude-haiku-4-5-20251001": "claude-haiku-4-5",
		"claude-haiku-4-5":          "claude-haiku-4-5",
		"claude-opus-5-5":           "claude-opus-5-5",
		"claude-opus-4-8":           "claude-opus-4-8", // trailing "-8" is not a date
	}
	for in, want := range cases {
		if got := Canonical(in); got != want {
			t.Errorf("Canonical(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestListModelsMergesBuiltInEntries(t *testing.T) {
	var gotKey, gotVersion, gotAuth string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotKey = r.Header.Get("x-api-key")
		gotAuth = r.Header.Get("Authorization")
		gotVersion = r.Header.Get("anthropic-version")
		w.Write([]byte(`{"data":[
			{"id":"claude-opus-5-5","display_name":"Claude Opus 5.5","max_input_tokens":1000000,"max_tokens":128000},
			{"id":"claude-nova-9","display_name":"Claude Nova 9","max_input_tokens":2000000,"max_tokens":256000},
			{"id":"","display_name":"junk"}
		]}`))
	}))
	defer srv.Close()
	old := APIBase
	APIBase = srv.URL
	defer func() { APIBase = old }()

	got, err := ListModels(context.Background(), Credential{Token: "sk-test"})
	if err != nil {
		t.Fatalf("ListModels: %v", err)
	}
	if gotKey != "sk-test" || gotVersion != Version {
		t.Errorf("headers: x-api-key=%q anthropic-version=%q", gotKey, gotVersion)
	}
	if gotAuth != "" {
		t.Errorf("an API key must not be sent as a bearer token, got Authorization=%q", gotAuth)
	}
	if len(got) != 2 {
		t.Fatalf("expected the blank id to be dropped, got %+v", got)
	}
	// A known id keeps the built-in label rather than the API's display name.
	if got[0].ID != "claude-opus-5-5" || got[0].Label != "Anthropic · Opus 5.5" {
		t.Errorf("known model not labelled from the catalog: %+v", got[0])
	}
	// An unknown id takes display_name and the API's reported limits.
	if got[1].ID != "claude-nova-9" || got[1].Label != "Anthropic · Claude Nova 9" {
		t.Errorf("unknown model label: %+v", got[1])
	}
	if got[1].MaxInputTokens != 2_000_000 || got[1].MaxOutputTokens != 256_000 {
		t.Errorf("unknown model should take the API's limits, got %+v", got[1])
	}
}

// The subscription OAuth login is the usual credential, and it goes in a
// different header than an API key. Sending it as x-api-key is a 401.
func TestListModelsSendsOAuthTokenAsBearer(t *testing.T) {
	var gotAuth, gotKey, gotBeta string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotAuth = r.Header.Get("Authorization")
		gotKey = r.Header.Get("x-api-key")
		gotBeta = r.Header.Get("anthropic-beta")
		w.Write([]byte(`{"data":[{"id":"claude-opus-5-5","display_name":"Claude Opus 5.5"}]}`))
	}))
	defer srv.Close()
	old := APIBase
	APIBase = srv.URL
	defer func() { APIBase = old }()

	if _, err := ListModels(context.Background(), Credential{Token: "oat-abc", OAuth: true}); err != nil {
		t.Fatalf("ListModels: %v", err)
	}
	if gotAuth != "Bearer oat-abc" {
		t.Errorf("Authorization = %q, want %q", gotAuth, "Bearer oat-abc")
	}
	if gotKey != "" {
		t.Errorf("an OAuth token must not also be sent as x-api-key, got %q", gotKey)
	}
	if gotBeta != OAuthBeta {
		t.Errorf("anthropic-beta = %q, want %q", gotBeta, OAuthBeta)
	}
}

// A dated id from the API and its alias in the catalog are one model. Returning
// the alias keeps the picker from listing both.
func TestListModelsFoldsDatedIDsOntoTheirAlias(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`{"data":[
			{"id":"claude-haiku-4-5-20251001","display_name":"Claude Haiku 4.5","max_input_tokens":200000,"max_tokens":64000}
		]}`))
	}))
	defer srv.Close()
	old := APIBase
	APIBase = srv.URL
	defer func() { APIBase = old }()

	got, err := ListModels(context.Background(), Credential{Token: "sk-test"})
	if err != nil {
		t.Fatalf("ListModels: %v", err)
	}
	if len(got) != 1 {
		t.Fatalf("expected one model, got %+v", got)
	}
	if got[0].ID != "claude-haiku-4-5" {
		t.Errorf("dated id was not folded onto its alias: got %q", got[0].ID)
	}
	if got[0].Label != "Anthropic · Haiku 4.5" {
		t.Errorf("folded model should keep the catalog label, got %q", got[0].Label)
	}
}

func TestListModelsWithoutCredential(t *testing.T) {
	if _, err := ListModels(context.Background(), Credential{}); err == nil {
		t.Fatal("expected an error with no credential")
	}
}

func TestListModelsSurfacesHTTPError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "nope", http.StatusUnauthorized)
	}))
	defer srv.Close()
	old := APIBase
	APIBase = srv.URL
	defer func() { APIBase = old }()

	_, err := ListModels(context.Background(), Credential{Token: "sk-bad"})
	if err == nil {
		t.Fatal("expected an error on 401")
	}
	// A rejected credential is the one failure a user can act on, so it must say
	// so rather than reporting a bare status line.
	if !strings.Contains(err.Error(), "rejected") {
		t.Errorf("401 should name the credential as the problem, got %v", err)
	}
}
