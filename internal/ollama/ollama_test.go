package ollama

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

const tagsBody = `{"models":[
  {"name":"glm-5.3","model":"glm-5.3","size":0},
  {"name":"gpt-oss:120b","model":"gpt-oss:120b","size":0},
  {"name":"","model":""}
]}`

func TestListModels(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/tags" {
			t.Errorf("path = %s, want /api/tags", r.URL.Path)
		}
		if got := r.Header.Get("Authorization"); got != "" {
			t.Errorf("listing must be anonymous, got Authorization %q", got)
		}
		w.Write([]byte(tagsBody))
	}))
	defer srv.Close()

	ms, err := ListModels(context.Background(), srv.URL)
	if err != nil {
		t.Fatal(err)
	}
	// The blank-name entry is dropped, so two survive.
	if len(ms) != 2 {
		t.Fatalf("got %d models: %+v", len(ms), ms)
	}
	if ms[0].ID != "glm-5.3" || ms[0].Label != "Ollama · glm-5.3" {
		t.Errorf("first = %+v", ms[0])
	}
	// Colons in a tag survive verbatim: the name is the upstream model name.
	if ms[1].ID != "gpt-oss:120b" {
		t.Errorf("second id = %q, want gpt-oss:120b", ms[1].ID)
	}
}

func TestListModelsTrailingSlashBase(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/tags" {
			t.Errorf("path = %s, want /api/tags", r.URL.Path)
		}
		w.Write([]byte(`{"models":[{"name":"glm-5.3"}]}`))
	}))
	defer srv.Close()

	if _, err := ListModels(context.Background(), srv.URL+"/"); err != nil {
		t.Fatal(err)
	}
}

func TestListModelsNon200(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusServiceUnavailable)
		w.Write([]byte("upstream down"))
	}))
	defer srv.Close()

	_, err := ListModels(context.Background(), srv.URL)
	if err == nil {
		t.Fatal("non-200 must error")
	}
	// The URL belongs in the message: a wrong api_base is the likely cause and
	// the status alone does not reveal it.
	if !strings.Contains(err.Error(), srv.URL) {
		t.Errorf("error must name the URL, got %v", err)
	}
}

func TestListModelsBodyCapped(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`{"models":[{"name":"` + strings.Repeat("x", 2<<20) + `"}]}`))
	}))
	defer srv.Close()

	// A 2 MB name exceeds the 1 MB read cap, so the decode must fail rather
	// than allocate whatever a wrong api_base decides to return.
	if _, err := ListModels(context.Background(), srv.URL); err == nil {
		t.Fatal("oversized body must error")
	}
}

func TestListModelsCancelledContext(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`{"models":[]}`))
	}))
	defer srv.Close()

	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := ListModels(ctx, srv.URL); err == nil {
		t.Fatal("cancelled context must error")
	}
}

func TestVerifyKey(t *testing.T) {
	// Mirrors the live behaviour probed against ollama.com: authorization is
	// checked before the model is looked up, so a bad key is 401 and a good key
	// reaches the missing model and gets 404.
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/chat" {
			t.Errorf("path = %s, want /api/chat", r.URL.Path)
		}
		if r.Header.Get("Authorization") != "Bearer good" {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		w.WriteHeader(http.StatusNotFound)
	}))
	defer srv.Close()

	if err := VerifyKey(context.Background(), srv.URL, "good"); err != nil {
		t.Errorf("good key: %v, want nil", err)
	}
	if err := VerifyKey(context.Background(), srv.URL, "bad"); !errors.Is(err, ErrKeyRejected) {
		t.Errorf("bad key: %v, want ErrKeyRejected", err)
	}
	if err := VerifyKey(context.Background(), srv.URL, ""); !errors.Is(err, ErrKeyRejected) {
		t.Errorf("empty key: %v, want ErrKeyRejected", err)
	}
}

func TestVerifyKeyInconclusiveIsNotRejection(t *testing.T) {
	// A bad minute upstream must not be reported as a bad key: refusing to
	// launch over a 500 would be worse than the problem VerifyKey guards.
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer srv.Close()
	if err := VerifyKey(context.Background(), srv.URL, "good"); err != nil {
		t.Errorf("5xx must be inconclusive, got %v", err)
	}

	// Same for a host that does not answer at all.
	srv.Close()
	if err := VerifyKey(context.Background(), srv.URL, "good"); err != nil {
		t.Errorf("transport failure must be inconclusive, got %v", err)
	}
}
