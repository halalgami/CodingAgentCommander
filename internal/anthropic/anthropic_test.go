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
		// A zero price would read as "free" on the session card. Live-discovered
		// models are allowed to be unpriced; built-in ones are not.
		if m.InputPrice <= 0 || m.OutputPrice <= 0 {
			t.Errorf("%s priced %v/%v; built-in entries must carry real prices", m.ID, m.InputPrice, m.OutputPrice)
		}
	}
	if !seen[DefaultID] {
		t.Errorf("DefaultID %q is not in Catalog; a first run would write a config Load rejects", DefaultID)
	}
	if Catalog()[0].ID != DefaultID {
		t.Errorf("Catalog is ordered %q first but DefaultID is %q", Catalog()[0].ID, DefaultID)
	}
}

func TestListModelsMergesBuiltInPricing(t *testing.T) {
	var gotKey, gotVersion string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotKey = r.Header.Get("x-api-key")
		gotVersion = r.Header.Get("anthropic-version")
		w.Write([]byte(`{"data":[
			{"id":"claude-opus-5","display_name":"Claude Opus 5"},
			{"id":"claude-nova-9","display_name":"Claude Nova 9"},
			{"id":"","display_name":"junk"}
		]}`))
	}))
	defer srv.Close()
	old := APIBase
	APIBase = srv.URL
	defer func() { APIBase = old }()

	got, err := ListModels(context.Background(), "sk-test")
	if err != nil {
		t.Fatalf("ListModels: %v", err)
	}
	if gotKey != "sk-test" || gotVersion != Version {
		t.Errorf("headers: x-api-key=%q anthropic-version=%q", gotKey, gotVersion)
	}
	if len(got) != 2 {
		t.Fatalf("expected the blank id to be dropped, got %+v", got)
	}
	// A known id keeps the built-in label and price rather than the API's name.
	if got[0].ID != "claude-opus-5" || got[0].InputPrice != 5 || got[0].Label != "Anthropic · Opus 5" {
		t.Errorf("known model not priced from the catalog: %+v", got[0])
	}
	// An unknown id takes display_name and stays unpriced.
	if got[1].ID != "claude-nova-9" || got[1].Label != "Anthropic · Claude Nova 9" {
		t.Errorf("unknown model label: %+v", got[1])
	}
	if got[1].InputPrice != 0 || got[1].OutputPrice != 0 {
		t.Errorf("unknown model should be unpriced, got %+v", got[1])
	}
}

func TestListModelsWithoutKey(t *testing.T) {
	if _, err := ListModels(context.Background(), ""); err == nil {
		t.Fatal("expected an error with no key")
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

	if _, err := ListModels(context.Background(), "sk-bad"); err == nil {
		t.Fatal("expected an error on 401")
	}
}
