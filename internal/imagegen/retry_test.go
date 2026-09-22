package imagegen

import (
	"context"
	"fmt"
	"image/color"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

// timings overrides the retry knobs for one test and restores them afterwards,
// so a retry test runs in milliseconds instead of minutes.
func timings(t *testing.T, base, req, fetch time.Duration) {
	t.Helper()
	oldBase, oldReq, oldFetch := retryBase, requestTimeout, fetchTimeout
	retryBase, requestTimeout, fetchTimeout = base, req, fetch
	t.Cleanup(func() { retryBase, requestTimeout, fetchTimeout = oldBase, oldReq, oldFetch })
}

// fastRetries is the common case: negligible backoff, short deadlines.
func fastRetries(t *testing.T) {
	t.Helper()
	timings(t, time.Millisecond, 2*time.Second, 2*time.Second)
}

func TestSubmitIsNeverRetried(t *testing.T) {
	fastRetries(t)
	var posts int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&posts, 1)
		http.Error(w, "upstream exploded", http.StatusInternalServerError)
	}))
	defer srv.Close()
	withFalBase(t, srv.URL)

	f := &Fal{Client: srv.Client()}
	_, err := f.Generate(context.Background(), "k", Request{
		Mode: ModeEdit, Base: solidJPEG(t, color.White), BaseMIME: "image/jpeg", Prompt: "p",
	})
	if err == nil {
		t.Fatal("expected an error on 500")
	}
	// The server may have accepted and billed the job before the response
	// failed. A second submit is a second charge for one image.
	if n := atomic.LoadInt32(&posts); n != 1 {
		t.Fatalf("submit was sent %d times; it must be sent exactly once, retrying it double-bills", n)
	}
}

func TestSubmitIsNotRetriedOnTransportFailure(t *testing.T) {
	fastRetries(t)
	var posts int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&posts, 1)
		// Hang up mid-response: the client sees a transport error, but the
		// server has already taken the job.
		hj, ok := w.(http.Hijacker)
		if !ok {
			t.Error("test server does not support hijacking")
			return
		}
		conn, _, err := hj.Hijack()
		if err != nil {
			t.Errorf("hijack: %v", err)
			return
		}
		conn.Close()
	}))
	defer srv.Close()
	withFalBase(t, srv.URL)

	f := &Fal{Client: srv.Client()}
	if _, err := f.Generate(context.Background(), "k", Request{
		Mode: ModeEdit, Base: solidJPEG(t, color.White), BaseMIME: "image/jpeg", Prompt: "p",
	}); err == nil {
		t.Fatal("expected a transport error")
	}
	if n := atomic.LoadInt32(&posts); n != 1 {
		t.Fatalf("submit was sent %d times after a transport failure; want 1", n)
	}
}

func TestResultFetchIsRetried(t *testing.T) {
	fastRetries(t)
	art := solidJPEG(t, color.RGBA{R: 120, G: 160, B: 200, A: 255})

	var gets int32
	mux := http.NewServeMux()
	mux.HandleFunc("/result.jpg", func(w http.ResponseWriter, r *http.Request) {
		// Fails twice, then succeeds. A GET of a finished artifact is
		// idempotent and free, so retrying it costs nothing.
		if atomic.AddInt32(&gets, 1) < 3 {
			http.Error(w, "not ready", http.StatusBadGateway)
			return
		}
		w.Write(art)
	})
	var srv *httptest.Server
	mux.HandleFunc("/"+FalDefaultModel, func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`{"images":[{"url":"` + srv.URL + `/result.jpg"}]}`))
	})
	srv = httptest.NewServer(mux)
	defer srv.Close()
	withFalBase(t, srv.URL)

	f := &Fal{Client: srv.Client()}
	res, err := f.Generate(context.Background(), "k", Request{
		Mode: ModeEdit, Base: solidJPEG(t, color.White), BaseMIME: "image/jpeg", Prompt: "p",
	})
	if err != nil {
		t.Fatalf("Generate should have recovered by the third fetch: %v", err)
	}
	if n := atomic.LoadInt32(&gets); n != 3 {
		t.Errorf("result fetched %d times, want 3", n)
	}
	if len(res.Image) == 0 {
		t.Error("recovered fetch returned no bytes")
	}
}

func TestResultFetchGivesUp(t *testing.T) {
	fastRetries(t)
	var gets int32
	mux := http.NewServeMux()
	mux.HandleFunc("/result.jpg", func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&gets, 1)
		http.Error(w, "gone", http.StatusBadGateway)
	})
	var srv *httptest.Server
	mux.HandleFunc("/"+FalDefaultModel, func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`{"images":[{"url":"` + srv.URL + `/result.jpg"}]}`))
	})
	srv = httptest.NewServer(mux)
	defer srv.Close()
	withFalBase(t, srv.URL)

	f := &Fal{Client: srv.Client()}
	_, err := f.Generate(context.Background(), "k", Request{
		Mode: ModeEdit, Base: solidJPEG(t, color.White), BaseMIME: "image/jpeg", Prompt: "p",
	})
	if err == nil {
		t.Fatal("expected an error after exhausting retries")
	}
	if n := atomic.LoadInt32(&gets); n != fetchAttempts {
		t.Errorf("result fetched %d times, want %d", n, fetchAttempts)
	}
	if !strings.Contains(err.Error(), fmt.Sprintf("%d attempts", fetchAttempts)) {
		t.Errorf("error does not say how many attempts were made: %v", err)
	}
}

func TestSubmitHasADeadline(t *testing.T) {
	timings(t, time.Millisecond, 50*time.Millisecond, 50*time.Millisecond)

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(2 * time.Second)
	}))
	defer srv.Close()
	withFalBase(t, srv.URL)

	f := &Fal{Client: srv.Client()}
	start := time.Now()
	_, err := f.Generate(context.Background(), "k", Request{
		Mode: ModeEdit, Base: solidJPEG(t, color.White), BaseMIME: "image/jpeg", Prompt: "p",
	})
	if err == nil {
		t.Fatal("a wedged provider must not hang the caller")
	}
	if time.Since(start) > time.Second {
		t.Errorf("waited %v; the per-request timeout did not apply", time.Since(start))
	}
}

func TestRetryStopsOnCancellation(t *testing.T) {
	// A long backoff so the test can prove cancellation cuts it short.
	timings(t, 300*time.Millisecond, 2*time.Second, 2*time.Second)

	mux := http.NewServeMux()
	mux.HandleFunc("/result.jpg", func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "not ready", http.StatusBadGateway)
	})
	var srv *httptest.Server
	mux.HandleFunc("/"+FalDefaultModel, func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`{"images":[{"url":"` + srv.URL + `/result.jpg"}]}`))
	})
	srv = httptest.NewServer(mux)
	defer srv.Close()
	withFalBase(t, srv.URL)

	ctx, cancel := context.WithCancel(context.Background())
	go func() { time.Sleep(50 * time.Millisecond); cancel() }()

	f := &Fal{Client: srv.Client()}
	start := time.Now()
	if _, err := f.Generate(ctx, "k", Request{
		Mode: ModeEdit, Base: solidJPEG(t, color.White), BaseMIME: "image/jpeg", Prompt: "p",
	}); err == nil {
		t.Fatal("expected an error after cancellation")
	}
	// Two full 300ms backoffs would be 600ms; cancellation must cut the wait.
	if time.Since(start) > 400*time.Millisecond {
		t.Errorf("backoff slept through cancellation: %v", time.Since(start))
	}
}
