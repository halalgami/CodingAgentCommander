package imagegen

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"image/color"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
)

// serveResult stands up a server whose single endpoint returns the given JSON
// body for a submit.
func serveResult(t *testing.T, body string) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(body))
	}))
	t.Cleanup(srv.Close)
	withFalBase(t, srv.URL)
	return srv
}

func TestAllBlackResultIsAFailure(t *testing.T) {
	black := solidJPEG(t, color.Black)
	srv := serveResult(t, `{"images":[{"url":"data:image/jpeg;base64,`+
		base64.StdEncoding.EncodeToString(black)+`"}]}`)

	f := &Fal{Client: srv.Client()}
	res, err := f.Generate(context.Background(), "k", Request{
		Mode: ModeEdit, Base: solidJPEG(t, color.White), BaseMIME: "image/jpeg", Prompt: "p",
	})
	if err == nil {
		t.Fatal("an all-black image is a silent safety rejection and must not be returned as a success")
	}
	if !errors.Is(err, ErrBlackImage) {
		t.Errorf("error is not ErrBlackImage, so a caller cannot distinguish it: %v", err)
	}
	if res.Image != nil {
		t.Error("Result.Image must be nil on a black result; anything else can be saved by accident")
	}
	if res.FinishReason != FinishBlackImage {
		t.Errorf("FinishReason = %q, want %q", res.FinishReason, FinishBlackImage)
	}
}

func TestNonBlackResultIsNotFlaggedAsBlack(t *testing.T) {
	// A dark but legitimate image must survive. The threshold separates "the
	// provider returned nothing" from "the scene is dim".
	dim := solidJPEG(t, color.RGBA{R: 18, G: 22, B: 30, A: 255})
	srv := serveResult(t, `{"images":[{"url":"data:image/jpeg;base64,`+
		base64.StdEncoding.EncodeToString(dim)+`"}]}`)

	f := &Fal{Client: srv.Client()}
	res, err := f.Generate(context.Background(), "k", Request{
		Mode: ModeEdit, Base: solidJPEG(t, color.White), BaseMIME: "image/jpeg", Prompt: "p",
	})
	if err != nil {
		t.Fatalf("a dim image is not a black image: %v", err)
	}
	if len(res.Image) == 0 {
		t.Error("dim image was dropped")
	}
}

func TestModerationFlagIsNotActedOn(t *testing.T) {
	// Content filtering is disabled at the source, so even if fal sets
	// has_nsfw_concepts the result is kept verbatim: no FinishContentFiltered,
	// which is what the pack pipeline would treat as a failed, discarded slot.
	art := solidJPEG(t, color.RGBA{R: 190, G: 140, B: 90, A: 255})
	srv := serveResult(t, `{"images":[{"url":"data:image/jpeg;base64,`+
		base64.StdEncoding.EncodeToString(art)+`"}],"has_nsfw_concepts":[true]}`)

	f := &Fal{Client: srv.Client()}
	res, err := f.Generate(context.Background(), "k", Request{
		Mode: ModeEdit, Base: solidJPEG(t, color.White), BaseMIME: "image/jpeg", Prompt: "p",
	})
	if err != nil {
		t.Fatalf("a moderation flag is not an error: %v", err)
	}
	if res.FinishReason != "" {
		t.Errorf("FinishReason = %q, want empty: flagged content is kept, not filtered", res.FinishReason)
	}
	if len(res.Image) == 0 {
		t.Error("flagged image bytes should still be returned")
	}
}

// TestFalRequestDisablesSafety asserts the outgoing request carries the
// permissive flags for both endpoint families, so fal does not moderate at all.
func TestFalRequestDisablesSafety(t *testing.T) {
	art := solidJPEG(t, color.RGBA{R: 190, G: 140, B: 90, A: 255})
	var gotBody map[string]any
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		raw, _ := io.ReadAll(req.Body)
		_ = json.Unmarshal(raw, &gotBody)
		w.Write([]byte(`{"images":[{"url":"data:image/jpeg;base64,` +
			base64.StdEncoding.EncodeToString(art) + `"}]}`))
	}))
	t.Cleanup(srv.Close)
	withFalBase(t, srv.URL)

	f := &Fal{Client: srv.Client()}
	if _, err := f.Generate(context.Background(), "k", Request{
		Mode: ModeEdit, Base: solidJPEG(t, color.White), BaseMIME: "image/jpeg", Prompt: "p",
	}); err != nil {
		t.Fatalf("Generate: %v", err)
	}
	if v, ok := gotBody["enable_safety_checker"].(bool); !ok || v {
		t.Errorf("enable_safety_checker = %v, want false", gotBody["enable_safety_checker"])
	}
	if v, _ := gotBody["safety_tolerance"].(string); v != "6" {
		t.Errorf("safety_tolerance = %q, want \"6\"", gotBody["safety_tolerance"])
	}
}

func TestUnflaggedResultHasEmptyFinishReason(t *testing.T) {
	art := solidJPEG(t, color.RGBA{R: 190, G: 140, B: 90, A: 255})
	srv := serveResult(t, `{"images":[{"url":"data:image/jpeg;base64,`+
		base64.StdEncoding.EncodeToString(art)+`"}],"has_nsfw_concepts":[false]}`)

	f := &Fal{Client: srv.Client()}
	res, err := f.Generate(context.Background(), "k", Request{
		Mode: ModeEdit, Base: solidJPEG(t, color.White), BaseMIME: "image/jpeg", Prompt: "p",
	})
	if err != nil {
		t.Fatalf("Generate: %v", err)
	}
	if res.FinishReason != "" {
		t.Errorf("FinishReason = %q, want empty when the flag is false", res.FinishReason)
	}
}

func TestUndecodableResultIsRejected(t *testing.T) {
	srv := serveResult(t, `{"images":[{"url":"data:image/jpeg;base64,`+
		base64.StdEncoding.EncodeToString([]byte("not an image at all"))+`"}]}`)

	f := &Fal{Client: srv.Client()}
	if _, err := f.Generate(context.Background(), "k", Request{
		Mode: ModeEdit, Base: solidJPEG(t, color.White), BaseMIME: "image/jpeg", Prompt: "p",
	}); err == nil {
		t.Fatal("bytes that do not decode as an image must not be returned as a result")
	}
}
