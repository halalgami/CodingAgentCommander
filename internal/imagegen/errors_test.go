package imagegen

import (
	"context"
	"errors"
	"fmt"
	"image/color"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
)

const secretKey = "fal-KEY-8f3c9a11-DO-NOT-LEAK"

func TestWrapErrDropsTheURL(t *testing.T) {
	// This is the exact shape net/http returns on a transport failure.
	raw := &url.Error{
		Op:  "Post",
		URL: "https://fal.run/tok/" + secretKey + "/fal-ai/nano-banana-2/edit",
		Err: errors.New("dial tcp 127.0.0.1:1: connect: connection refused"),
	}
	if !strings.Contains(raw.Error(), secretKey) {
		t.Fatal("premise broken: a bare *url.Error is supposed to stringify the whole URL")
	}
	got := wrapErr("fal: submit", raw)
	if strings.Contains(got.Error(), secretKey) {
		t.Errorf("wrapped error still carries the credential: %v", got)
	}
	if !strings.Contains(got.Error(), "fal: submit") {
		t.Errorf("wrapped error lost the operation: %v", got)
	}
	if !strings.Contains(got.Error(), "connection refused") {
		t.Errorf("wrapped error lost the cause, so it is undiagnosable: %v", got)
	}
}

func TestWrapErrHandlesAWrappedURLError(t *testing.T) {
	// errors.As, not a type assertion: the transport error may already be
	// wrapped by the time it reaches us.
	inner := &url.Error{Op: "Get", URL: "https://x/" + secretKey, Err: errors.New("EOF")}
	got := wrapErr("fal: fetch result", fmt.Errorf("layer: %w", inner))
	if strings.Contains(got.Error(), secretKey) {
		t.Errorf("credential survived a wrapped url.Error: %v", got)
	}
}

func TestTransportErrorNeverContainsTheKey(t *testing.T) {
	// End to end: a base URL with the credential in the path, and a server that
	// is closed so the request fails at the transport.
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}))
	client := srv.Client()
	base := srv.URL + "/tok/" + secretKey
	srv.Close()
	withFalBase(t, base)

	f := &Fal{Client: client}
	_, err := f.Generate(context.Background(), secretKey, Request{
		Mode: ModeEdit, Base: solidJPEG(t, color.White), BaseMIME: "image/jpeg", Prompt: "p",
	})
	if err == nil {
		t.Fatal("expected a transport error against a closed server")
	}
	if strings.Contains(err.Error(), secretKey) {
		t.Fatalf("API key leaked into the error string: %v", err)
	}
}

func TestErrorBodyIsRedacted(t *testing.T) {
	// Some APIs echo request context into an error body. Anything quoted into
	// an error is redacted against the key first.
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusForbidden)
		fmt.Fprintf(w, `{"detail":"key %s is not authorised"}`, secretKey)
	}))
	defer srv.Close()
	withFalBase(t, srv.URL)

	f := &Fal{Client: srv.Client()}
	_, err := f.Generate(context.Background(), secretKey, Request{
		Mode: ModeEdit, Base: solidJPEG(t, color.White), BaseMIME: "image/jpeg", Prompt: "p",
	})
	if err == nil {
		t.Fatal("expected an error on 403")
	}
	if strings.Contains(err.Error(), secretKey) {
		t.Fatalf("API key echoed by the provider leaked into the error: %v", err)
	}
	if !strings.Contains(err.Error(), "403") {
		t.Errorf("redaction removed the diagnosis too: %v", err)
	}
}

func TestBodyForErrorTrims(t *testing.T) {
	long := strings.Repeat("x", 500)
	got := bodyForError([]byte(long), "")
	if len(got) > 220 {
		t.Errorf("body not trimmed: %d chars", len(got))
	}
	if bodyForError([]byte("abc"+secretKey+"def"), secretKey) != "abc[redacted]def" {
		t.Errorf("bodyForError did not redact: %q", bodyForError([]byte("abc"+secretKey+"def"), secretKey))
	}
	if bodyForError([]byte("plain"), "") != "plain" {
		t.Error("an empty key must not redact everything")
	}
}
