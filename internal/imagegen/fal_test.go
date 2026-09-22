package imagegen

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"hash/crc32"
	"image"
	"image/color"
	"image/draw"
	"image/jpeg"
	"net/http"
	"net/http/httptest"
	"sort"
	"strings"
	"testing"
)

// solidJPEG returns a small JPEG filled with one colour. Used both as the input
// image and, in later tests, as a stand-in for a provider's output.
func solidJPEG(t *testing.T, c color.Color) []byte {
	t.Helper()
	img := image.NewRGBA(image.Rect(0, 0, 32, 32))
	draw.Draw(img, img.Bounds(), &image.Uniform{C: c}, image.Point{}, draw.Src)
	var buf bytes.Buffer
	if err := jpeg.Encode(&buf, img, nil); err != nil {
		t.Fatalf("encode: %v", err)
	}
	return buf.Bytes()
}

// withFalBase points the adapter at a test server for the duration of a test.
func withFalBase(t *testing.T, base string) {
	t.Helper()
	old := FalBase
	FalBase = base
	t.Cleanup(func() { FalBase = old })
}

func TestFalSendsTheValidatedPayload(t *testing.T) {
	art := solidJPEG(t, color.RGBA{R: 200, G: 120, B: 60, A: 255})

	var gotAuth, gotCT, gotPath string
	var gotBody map[string]any
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotAuth = r.Header.Get("Authorization")
		gotCT = r.Header.Get("Content-Type")
		gotPath = r.URL.Path
		if err := json.NewDecoder(r.Body).Decode(&gotBody); err != nil {
			t.Errorf("decode request body: %v", err)
		}
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"images":[{"url":"data:image/jpeg;base64,` +
			base64.StdEncoding.EncodeToString(art) + `"}]}`))
	}))
	defer srv.Close()
	withFalBase(t, srv.URL)

	f := &Fal{Client: srv.Client()}
	res, err := f.Generate(context.Background(), "fal-secret-123", Request{
		Mode:     ModeEdit,
		Base:     solidJPEG(t, color.RGBA{R: 10, G: 90, B: 200, A: 255}),
		BaseMIME: "image/jpeg",
		Prompt:   "keep her face unchanged; she is beaming with delight",
		Seed:     4242,
	})
	if err != nil {
		t.Fatalf("Generate: %v", err)
	}

	if gotAuth != "Key fal-secret-123" {
		t.Errorf("Authorization = %q, want %q", gotAuth, "Key fal-secret-123")
	}
	if gotCT != "application/json" {
		t.Errorf("Content-Type = %q", gotCT)
	}
	if gotPath != "/"+FalDefaultModel {
		t.Errorf("path = %q, want /%s", gotPath, FalDefaultModel)
	}
	if got := gotBody["prompt"]; got != "keep her face unchanged; she is beaming with delight" {
		t.Errorf("prompt = %v", got)
	}
	// image_urls is a LIST even for one image. fal's flux endpoints take the
	// singular image_url; sending that shape here is accepted and the input
	// image is silently ignored.
	urls, ok := gotBody["image_urls"].([]any)
	if !ok {
		t.Fatalf("image_urls is %T, must be a JSON array", gotBody["image_urls"])
	}
	if len(urls) != 1 {
		t.Fatalf("image_urls has %d entries, want 1", len(urls))
	}
	if s, _ := urls[0].(string); !strings.HasPrefix(s, "data:image/jpeg;base64,") {
		t.Errorf("image_urls[0] is not a jpeg data URI: %.40q", s)
	}
	if _, singular := gotBody["image_url"]; singular {
		t.Error("sent image_url as well as image_urls; the singular key belongs to a different family of endpoints")
	}
	if got := gotBody["num_images"]; got != float64(1) {
		t.Errorf("num_images = %v, want 1", got)
	}
	// fal's own description of this flag warns the default "may affect
	// generation quality", so it is sent explicitly rather than defaulted.
	lim, present := gotBody["limit_generations"]
	if !present {
		t.Error("limit_generations was not sent")
	}
	if lim != false {
		t.Errorf("limit_generations = %v, want false", lim)
	}
	if got := gotBody["seed"]; got != float64(4242) {
		t.Errorf("seed = %v, want 4242", got)
	}

	// A data: result URI is decoded inline; no second request is made.
	if !bytes.Equal(res.Image, art) {
		t.Errorf("image bytes not returned intact: got %d bytes, want %d", len(res.Image), len(art))
	}
	if res.MIME != "image/jpeg" {
		t.Errorf("MIME = %q, want image/jpeg (sniffed from the bytes)", res.MIME)
	}
	if res.Seed != 4242 {
		t.Errorf("Seed = %d, want the request's 4242 echoed back", res.Seed)
	}
	if res.Model != FalDefaultModel {
		t.Errorf("Model = %q", res.Model)
	}
	if res.FinishReason != "" {
		t.Errorf("FinishReason = %q, want empty on success", res.FinishReason)
	}
}

func TestFalFetchesAnHTTPSResultURL(t *testing.T) {
	art := solidJPEG(t, color.RGBA{R: 30, G: 180, B: 90, A: 255})

	var fetched int
	mux := http.NewServeMux()
	mux.HandleFunc("/result.jpg", func(w http.ResponseWriter, r *http.Request) {
		fetched++
		w.Header().Set("Content-Type", "image/jpeg")
		w.Write(art)
	})
	var srv *httptest.Server
	mux.HandleFunc("/"+FalDefaultModel, func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`{"images":[{"url":"` + srv.URL + `/result.jpg","content_type":"image/jpeg"}]}`))
	})
	srv = httptest.NewServer(mux)
	defer srv.Close()
	withFalBase(t, srv.URL)

	f := &Fal{Client: srv.Client()}
	res, err := f.Generate(context.Background(), "k", Request{
		Mode: ModeEdit, Base: solidJPEG(t, color.White), BaseMIME: "image/jpeg", Prompt: "p",
	})
	if err != nil {
		t.Fatalf("Generate: %v", err)
	}
	if fetched != 1 {
		t.Errorf("result fetched %d times, want 1", fetched)
	}
	if !bytes.Equal(res.Image, art) {
		t.Error("fetched image bytes do not match what the server served")
	}
}

func TestFalWrapsNon2xx(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
		w.Write([]byte(`{"detail":"Unauthorized"}`))
	}))
	defer srv.Close()
	withFalBase(t, srv.URL)

	f := &Fal{Client: srv.Client()}
	_, err := f.Generate(context.Background(), "bad-key", Request{
		Mode: ModeEdit, Base: solidJPEG(t, color.White), BaseMIME: "image/jpeg", Prompt: "p",
	})
	if err == nil {
		t.Fatal("expected an error on 401")
	}
	if !strings.Contains(err.Error(), "fal: submit") {
		t.Errorf("error is not wrapped with an operation: %v", err)
	}
	if !strings.Contains(err.Error(), "401") {
		t.Errorf("error drops the status code: %v", err)
	}
}

func TestFalRejectsEmptyInputs(t *testing.T) {
	f := &Fal{}
	if _, err := f.Generate(context.Background(), "", Request{Base: []byte{1}}); err == nil {
		t.Error("expected an error with no key")
	}
	if _, err := f.Generate(context.Background(), "k", Request{}); err == nil {
		t.Error("expected an error with no base image")
	}
}

func TestFalIsRegistered(t *testing.T) {
	p, ok := Lookup("fal")
	if !ok {
		t.Fatal(`Lookup("fal") failed; the adapter should register itself`)
	}
	models := p.Models()
	if len(models) == 0 || models[0].ID != FalDefaultModel {
		t.Fatalf("Models() = %+v, want the default model first", models)
	}
	if models[0].Mode != ModeEdit {
		t.Errorf("model mode = %q, want %q", models[0].Mode, ModeEdit)
	}
}

// A per-side cap is not a memory bound on its own: 16384x16384 passes it and is
// still ~1 GiB once decoded. The pixel-count guard is what actually stops a
// decompression bomb, and it must reject BEFORE image.Decode allocates.
func TestRejectsBombBeforeDecoding(t *testing.T) {
	// A PNG header claiming 16384x16384 — within the per-side cap, far over the
	// pixel budget. Built by hand so the file stays tiny; only the header is read.
	var b bytes.Buffer
	b.Write([]byte{0x89, 'P', 'N', 'G', '\r', '\n', 0x1a, '\n'})
	ihdr := []byte{
		0, 0, 0, 13, 'I', 'H', 'D', 'R',
		0, 0, 0x40, 0, // width  16384
		0, 0, 0x40, 0, // height 16384
		8, 6, 0, 0, 0,
	}
	crc := crc32.ChecksumIEEE(ihdr[4:])
	b.Write(ihdr)
	b.Write([]byte{byte(crc >> 24), byte(crc >> 16), byte(crc >> 8), byte(crc)})

	if _, err := meanLuminance(b.Bytes()); err == nil {
		t.Fatal("a 16384x16384 image must be rejected on pixel count, not decoded")
	} else if !strings.Contains(err.Error(), "too large") {
		t.Fatalf("want a pixel-count rejection, got %v", err)
	}
}

// --- multi-model routing ---------------------------------------------------

// falCapture stands up a server that records the path and body of one call and
// answers with a usable image.
func falCapture(t *testing.T, art []byte) (path *string, body *map[string]any) {
	t.Helper()
	var gotPath string
	var gotBody map[string]any
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		_ = json.NewDecoder(r.Body).Decode(&gotBody)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"images":[{"url":"data:image/jpeg;base64,` +
			base64.StdEncoding.EncodeToString(art) + `"}]}`))
	}))
	t.Cleanup(srv.Close)
	withFalBase(t, srv.URL)
	return &gotPath, &gotBody
}

// The model on the REQUEST must decide the endpoint. Before Request.Model
// existed, every call went to the provider's default and the model the user
// picked in the UI was silently ignored — a bug only a second model exposed.
func TestRequestModelDecidesTheEndpoint(t *testing.T) {
	art := solidJPEG(t, color.RGBA{R: 180, G: 180, B: 90, A: 255})
	for _, want := range []string{FalDefaultModel, "fal-ai/flux-2/klein/4b/edit"} {
		t.Run(want, func(t *testing.T) {
			path, _ := falCapture(t, art)
			f := &Fal{}
			res, err := f.Generate(context.Background(), "k",
				Request{Mode: ModeEdit, Model: want, Base: art, Prompt: "p"})
			if err != nil {
				t.Fatal(err)
			}
			if *path != "/"+want {
				t.Errorf("posted to %q, want /%s", *path, want)
			}
			// And the Result must name the model that actually ran, because it
			// is written into the pack's provenance.
			if res.Model != want {
				t.Errorf("Result.Model = %q, want %q", res.Model, want)
			}
		})
	}
}

func TestEmptyRequestModelFallsBackToTheDefault(t *testing.T) {
	art := solidJPEG(t, color.RGBA{R: 90, G: 160, B: 200, A: 255})
	path, _ := falCapture(t, art)
	f := &Fal{}
	if _, err := f.Generate(context.Background(), "k",
		Request{Mode: ModeEdit, Base: art, Prompt: "p"}); err != nil {
		t.Fatal(err)
	}
	if *path != "/"+FalDefaultModel {
		t.Errorf("posted to %q, want the default", *path)
	}
}

// An unknown model is refused BEFORE the request goes out. fal answers an
// unrecognised path with 404, but a real path whose request shape has not been
// verified would succeed — and bill — for an image generated with no input.
func TestUnknownModelIsRefusedBeforeSpending(t *testing.T) {
	art := solidJPEG(t, color.RGBA{R: 10, G: 200, B: 10, A: 255})
	var called bool
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		called = true
	}))
	defer srv.Close()
	withFalBase(t, srv.URL)

	f := &Fal{}
	_, err := f.Generate(context.Background(), "k",
		Request{Mode: ModeEdit, Model: "fal-ai/not-in-our-catalogue", Base: art, Prompt: "p"})
	if err == nil {
		t.Fatal("want an error for an unverified model")
	}
	if !strings.Contains(err.Error(), "unknown model") {
		t.Errorf("unhelpful error: %v", err)
	}
	if called {
		t.Error("the request was sent anyway, so an unverified model could still be billed")
	}
}

// The input image's FIELD NAME differs per endpoint, and sending the wrong one
// is not an error — the field is ignored, the model generates from the prompt
// alone, and the user pays for a picture of nobody. So the name must come from
// the catalogue on every call.
func TestImageFieldNameComesFromTheCatalogue(t *testing.T) {
	art := solidJPEG(t, color.RGBA{R: 30, G: 30, B: 200, A: 255})
	for _, m := range falCatalogue {
		t.Run(m.ID, func(t *testing.T) {
			_, body := falCapture(t, art)
			f := &Fal{}
			if _, err := f.Generate(context.Background(), "k",
				Request{Mode: ModeEdit, Model: m.ID, Base: art, Prompt: "p"}); err != nil {
				t.Fatal(err)
			}
			v, ok := (*body)[m.ImageField]
			if !ok {
				keys := make([]string, 0, len(*body))
				for k := range *body {
					keys = append(keys, k)
				}
				sort.Strings(keys)
				t.Fatalf("no %q in the payload; sent %v", m.ImageField, keys)
			}
			_, isArray := v.([]any)
			if isArray != m.ImageArray {
				t.Errorf("%s: sent array=%v, catalogue says array=%v",
					m.ImageField, isArray, m.ImageArray)
			}
			// The image must actually be in there, not an empty container.
			if m.ImageArray {
				if arr, _ := v.([]any); len(arr) != 1 {
					t.Errorf("array had %d entries, want 1", len(arr))
				}
			} else if s, _ := v.(string); !strings.HasPrefix(s, "data:") {
				t.Errorf("singular field is not a data URI: %.20q", s)
			}
		})
	}
}

// Every catalogue entry has to be complete, or a model ships that cannot work.
func TestCatalogueEntriesAreComplete(t *testing.T) {
	seen := map[string]bool{}
	for _, m := range falCatalogue {
		if m.ID == "" || m.Label == "" || m.ImageField == "" {
			t.Errorf("incomplete entry: %+v", m)
		}
		if seen[m.ID] {
			t.Errorf("duplicate model id %q", m.ID)
		}
		seen[m.ID] = true
		switch m.Refusal {
		case RefusalBlackImage, RefusalPromptError, RefusalUnknown:
		default:
			t.Errorf("%s has refusal mode %q, which is not one of the three", m.ID, m.Refusal)
		}
		// A model that returns a BILLED refusal must warn about it: the user is
		// paying for those, and silence is how that becomes a surprise.
		if m.Refusal == RefusalBlackImage && m.Note == "" {
			t.Errorf("%s refuses by billed black image but carries no warning", m.ID)
		}
	}
	// Models() must expose all of them, with the traits attached.
	f := &Fal{}
	got := f.Models()
	if len(got) != len(falCatalogue) {
		t.Fatalf("Models() returned %d, catalogue has %d", len(got), len(falCatalogue))
	}
	for _, m := range got {
		if m.Mode != ModeEdit {
			t.Errorf("%s: mode %q — every model here is an instruction editor", m.ID, m.Mode)
		}
		if m.Refusal == "" {
			t.Errorf("%s: refusal mode not surfaced to callers", m.ID)
		}
	}
}

// Both models currently in the catalogue happen to use `image_urls`, so a
// hardcoded field name passes every test that goes through Generate. That is a
// coincidence of today's catalogue, not a property of the code — so the mapping
// is tested directly, with the shapes the next models actually need
// (Kontext takes a singular `image_url`, HiDream `reference_image_urls`).
func TestFalBodyHonoursAnyImageFieldShape(t *testing.T) {
	art := solidJPEG(t, color.RGBA{R: 1, G: 2, B: 3, A: 255})
	req := Request{Mode: ModeEdit, Base: art, Prompt: "p"}

	cases := []struct {
		name  string
		model falModel
	}{
		{"array", falModel{ID: "x", ImageField: "image_urls", ImageArray: true}},
		{"singular", falModel{ID: "x", ImageField: "image_url", ImageArray: false}},
		{"reference array", falModel{ID: "x", ImageField: "reference_image_urls", ImageArray: true}},
		{"input array", falModel{ID: "x", ImageField: "input_image_urls", ImageArray: true}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			raw, err := falBody(tc.model, req, "image/jpeg")
			if err != nil {
				t.Fatal(err)
			}
			var body map[string]any
			if err := json.Unmarshal(raw, &body); err != nil {
				t.Fatal(err)
			}
			v, ok := body[tc.model.ImageField]
			if !ok {
				t.Fatalf("no %q in %s", tc.model.ImageField, raw)
			}
			if arr, isArr := v.([]any); tc.model.ImageArray {
				if !isArr || len(arr) != 1 {
					t.Errorf("want a 1-element array, got %T", v)
				}
			} else if s, isStr := v.(string); !isStr || !strings.HasPrefix(s, "data:") {
				t.Errorf("want a bare data URI, got %T", v)
			}
			// And the image must appear under EXACTLY one key: an extra
			// `image_urls` alongside a singular `image_url` is how an endpoint
			// silently ignores the input and bills anyway.
			var found int
			for _, k := range []string{"image_url", "image_urls", "reference_image_urls", "input_image_urls"} {
				if _, ok := body[k]; ok {
					found++
				}
			}
			if found != 1 {
				t.Errorf("the image appears under %d keys; exactly one is correct: %s", found, raw)
			}
		})
	}
}

// num_images is absent from some endpoints' schemas. Sending it is harmless,
// but the flag records where that was checked, so it must be respected.
func TestFalBodyOmitsNumImagesWhenTheEndpointHasNone(t *testing.T) {
	art := solidJPEG(t, color.RGBA{R: 9, G: 9, B: 9, A: 255})
	req := Request{Mode: ModeEdit, Base: art, Prompt: "p"}

	with, _ := falBody(falModel{ImageField: "image_urls", ImageArray: true, NumImages: true}, req, "image/jpeg")
	without, _ := falBody(falModel{ImageField: "image_urls", ImageArray: true, NumImages: false}, req, "image/jpeg")

	var a, b map[string]any
	_ = json.Unmarshal(with, &a)
	_ = json.Unmarshal(without, &b)
	if _, ok := a["num_images"]; !ok {
		t.Error("num_images missing when the endpoint accepts it")
	}
	if _, ok := b["num_images"]; ok {
		t.Error("num_images sent to an endpoint whose schema has no such field")
	}
}

// --- what the provider says it charged for --------------------------------

// Result.Billed exists because "an error means no charge" is false. Once the
// submit returns 2xx fal has generated the image and billed for it; everything
// that can still fail is our failure to COLLECT it. The caller cannot tell
// those apart from the error, so the provider has to say.
func TestBilledIsTrueForEveryFailureAfterTheSubmitSucceeds(t *testing.T) {
	art := solidJPEG(t, color.RGBA{R: 70, G: 70, B: 70, A: 255})

	cases := []struct {
		name string
		body string // the 200 response fal returns
	}{
		{"body will not decode", `{"images":[`},
		{"no images in the response", `{"images":[]}`},
		{"result download fails", `{"images":[{"url":"http://127.0.0.1:1/gone.jpg"}]}`},
		{"result is not a decodable image", `{"images":[{"url":"data:image/jpeg;base64,` +
			base64.StdEncoding.EncodeToString([]byte("not an image")) + `"}]}`},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				_, _ = w.Write([]byte(tc.body))
			}))
			defer srv.Close()
			withFalBase(t, srv.URL)

			f := &Fal{}
			res, err := f.Generate(context.Background(), "k",
				Request{Mode: ModeEdit, Base: art, Prompt: "p"})
			if err == nil {
				t.Fatal("want an error")
			}
			if !res.Billed {
				t.Error("reported as free; the submit returned 2xx, so fal generated and charged")
			}
			if res.Model == "" {
				t.Error("a billed failure must still name the model that was charged for")
			}
		})
	}
}

// The converse: a failure BEFORE the submit lands produced nothing, and
// counting it would over-report the user's spend.
func TestBilledIsFalseWhenNothingWasGenerated(t *testing.T) {
	art := solidJPEG(t, color.RGBA{R: 20, G: 90, B: 20, A: 255})

	t.Run("submit refused", func(t *testing.T) {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusUnauthorized)
			_, _ = w.Write([]byte(`{"detail":"invalid key credentials"}`))
		}))
		defer srv.Close()
		withFalBase(t, srv.URL)

		res, err := (&Fal{}).Generate(context.Background(), "k",
			Request{Mode: ModeEdit, Base: art, Prompt: "p"})
		if err == nil {
			t.Fatal("want an error")
		}
		if res.Billed {
			t.Error("a rejected submit was counted as billed")
		}
	})

	t.Run("rejected before the request is built", func(t *testing.T) {
		res, err := (&Fal{}).Generate(context.Background(), "k",
			Request{Mode: ModeEdit, Model: "fal-ai/not-real", Base: art, Prompt: "p"})
		if err == nil {
			t.Fatal("want an error")
		}
		if res.Billed {
			t.Error("an unknown model was counted as billed")
		}
	})
}

// A black frame is the case that started this: HTTP 200, real bytes, a real
// charge, and no image the caller may keep.
func TestBlackImageIsBilled(t *testing.T) {
	black := solidJPEG(t, color.RGBA{A: 255})
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"images":[{"url":"data:image/jpeg;base64,` +
			base64.StdEncoding.EncodeToString(black) + `"}]}`))
	}))
	defer srv.Close()
	withFalBase(t, srv.URL)

	res, err := (&Fal{}).Generate(context.Background(), "k",
		Request{Mode: ModeEdit, Base: solidJPEG(t, color.RGBA{R: 200, A: 255}), Prompt: "p"})
	if !errors.Is(err, ErrBlackImage) {
		t.Fatalf("err = %v, want ErrBlackImage", err)
	}
	if !res.Billed {
		t.Error("the black frame was reported as free")
	}
	if len(res.Image) != 0 {
		t.Error("the black frame reached the caller")
	}
}
