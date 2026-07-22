package zen

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestListModels(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/models" {
			t.Errorf("path = %s", r.URL.Path)
		}
		if r.Header.Get("Authorization") != "Bearer k123" {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		w.Write([]byte(`{"data":[{"id":"glm-5.2"},{"id":"kimi-k2.7-code"}]}`))
	}))
	defer srv.Close()

	ms, err := ListModels(context.Background(), srv.URL, "k123")
	if err != nil {
		t.Fatal(err)
	}
	if len(ms) != 2 || ms[0].ID != "glm-5.2" || ms[0].Label != "Go · glm-5.2" {
		t.Fatalf("got %+v", ms)
	}
	if _, err := ListModels(context.Background(), srv.URL, "wrong"); err == nil {
		t.Fatal("bad key must error")
	}
	if _, err := ListModels(context.Background(), srv.URL, ""); err == nil {
		t.Fatal("empty key must error before any request")
	}
}
