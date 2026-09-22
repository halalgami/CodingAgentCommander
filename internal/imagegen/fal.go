package imagegen

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"image"
	_ "image/jpeg" // registers the JPEG decoder used by image.Decode
	_ "image/png"  // registers the PNG decoder used by image.Decode
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

// FalBase is the root of fal's synchronous run endpoint. Var, not const, so
// tests can point at a local server — same idiom as internal/anthropic's
// APIBase.
var FalBase = "https://fal.run"

// FalDefaultModel is the model id this build runs. fal's catalogue churns
// quickly, so callers may override it via Fal.Model rather than editing code.
const FalDefaultModel = "fal-ai/nano-banana-2/edit"

// maxBodyBytes caps every response read. A request body carries a base64 image
// and a response may carry another, so the cap is generous; the point is that
// it exists, since these bodies come from a host named by a mutable package var.
const maxBodyBytes = 32 << 20

// Timeouts and retry policy.
//
// The submit is NEVER retried. fal may have accepted — and started billing —
// the job before the response failed, so retrying a 5xx or a dropped connection
// buys a second image for the same request. Only the result fetch is retried:
// a GET of a finished artifact is idempotent and free.
//
// All three are vars so tests can shrink them.
var (
	// requestTimeout bounds one submit. Instruction edits on this model run
	// tens of seconds; the spike used a 300s ceiling and never approached it.
	requestTimeout = 5 * time.Minute
	// fetchTimeout bounds one attempt at downloading a result.
	fetchTimeout = 2 * time.Minute
	// retryBase is the first backoff; it doubles per attempt.
	retryBase = 500 * time.Millisecond
)

// fetchAttempts is how many times a result download is tried in total.
const fetchAttempts = 3

// Fal is the fal.ai adapter. Stateless apart from configuration: the key is
// passed per call.
type Fal struct {
	Model  string       // defaults to FalDefaultModel
	Client *http.Client // defaults to a plain client; per-call deadlines come from ctx
}

func init() { Register(&Fal{}) }

// ID implements Provider.
func (f *Fal) ID() string { return "fal" }

// falModel is one endpoint plus the facts about how to talk to it.
//
// ImageField and ImageArray are the dangerous pair. fal's endpoints disagree
// about how the input image is passed — some take `image_urls` as an array,
// some take `image_url` as a bare string, others use `reference_image_urls` —
// and sending the wrong shape does NOT produce an error. The field is simply
// unrecognised, the model generates from the prompt alone, and the user is
// billed for a picture of nobody. Every entry here was verified against fal's
// published OpenAPI schema for that endpoint.
type falModel struct {
	ID         string
	Label      string
	ImageField string
	ImageArray bool
	Refusal    string
	Note       string
	// NumImages is false for endpoints whose schema has no such field. Sending
	// one is harmless, but its absence is worth recording where it was checked.
	NumImages bool
}

// falCatalogue is the set of endpoints this build offers.
//
// Deliberately short. Every model here has been run against real base images
// through the actual prompt template, because the only claim that matters —
// does identity survive several independent edits from one base — is one no
// vendor benchmarks and every vendor asserts.
var falCatalogue = []falModel{
	{
		ID:         FalDefaultModel,
		Label:      "fal.ai · Nano Banana 2 (edit)",
		ImageField: "image_urls",
		ImageArray: true,
		NumImages:  true,
		// Exposes safety_tolerance rather than enable_safety_checker, and
		// refused nothing across 10 images on two different base portraits.
		Refusal: RefusalUnknown,
		Note:    "Best identity and framing in testing. Slower.",
	},
	{
		ID:         "fal-ai/flux-2/klein/4b/edit",
		Label:      "fal.ai · FLUX.2 klein 4B (edit)",
		ImageField: "image_urls",
		ImageArray: true,
		NumImages:  true,
		// Measured: 3 all-black frames out of 18 across three base images, all
		// on entirely benign prompts, and fal bills for every one of them.
		Refusal: RefusalBlackImage,
		Note:    "8x cheaper and ~5x faster. Around 1 in 6 images comes back rejected by fal's safety filter — and those are still charged.",
	},
}

func falModelByID(id string) (falModel, bool) {
	for _, m := range falCatalogue {
		if m.ID == id {
			return m, true
		}
	}
	return falModel{}, false
}

// Models implements Provider.
func (f *Fal) Models() []Model {
	out := make([]Model, 0, len(falCatalogue))
	for _, m := range falCatalogue {
		out = append(out, Model{
			ID: m.ID, Label: m.Label, Mode: ModeEdit,
			Refusal: m.Refusal, Note: m.Note,
		})
	}
	return out
}

// resolve picks the model for a request: the request's choice, else this
// provider's override, else the default.
func (f *Fal) resolve(want string) (falModel, error) {
	for _, id := range []string{want, f.Model, FalDefaultModel} {
		if id == "" {
			continue
		}
		if m, ok := falModelByID(id); ok {
			return m, nil
		}
		// An unknown id is refused rather than passed through. fal answers an
		// unrecognised path with 404, but a KNOWN path whose request shape we
		// have not verified would succeed and quietly bill for an image
		// generated without the input.
		return falModel{}, fmt.Errorf("fal: unknown model %q", id)
	}
	return falModel{}, fmt.Errorf("fal: no model configured")
}

func (f *Fal) model() string {
	if f.Model != "" {
		return f.Model
	}
	return FalDefaultModel
}

func (f *Fal) client() *http.Client {
	if f.Client != nil {
		return f.Client
	}
	return http.DefaultClient
}

// falBody builds the request as a map rather than a struct, because the KEY
// the input image travels under differs per endpoint and a struct tag cannot
// vary at runtime. Getting it wrong is silent and billable, so the field name
// comes from the verified catalogue entry, never from a literal here.
func falBody(m falModel, r Request, mime string) ([]byte, error) {
	body := map[string]any{"prompt": r.Prompt}

	uri := dataURI(mime, r.Base)
	if m.ImageArray {
		body[m.ImageField] = []string{uri}
	} else {
		body[m.ImageField] = uri
	}
	if m.NumImages {
		body["num_images"] = 1
	}
	// fal's own description of this flag warns that the default "may affect
	// generation quality", so it is always sent explicitly. Unknown fields are
	// ignored by endpoints that do not define it.
	body["limit_generations"] = false
	// Content filtering is disabled at the source. The two endpoints in the
	// catalogue name this differently — the flux family takes
	// enable_safety_checker (bool), nano-banana takes safety_tolerance (a
	// "1".."6" string, 6 being the most permissive) — and an endpoint that does
	// not define one of these simply ignores it, so both are sent unconditionally.
	// With the checker off, fal stops returning has_nsfw_concepts and stops
	// substituting the black-frame refusal, so the result is whatever the model
	// actually produced.
	body["enable_safety_checker"] = false
	body["safety_tolerance"] = "6"
	if p := seedPtr(r.Seed); p != nil {
		body["seed"] = *p
	}
	return json.Marshal(body)
}

type falResponse struct {
	Images []struct {
		URL         string `json:"url"`
		ContentType string `json:"content_type"`
	} `json:"images"`
	// HasNSFWConcepts is fal's moderation signal on this family of endpoints:
	// one bool per returned image, HTTP 200 either way.
	HasNSFWConcepts []bool `json:"has_nsfw_concepts"`
}

// Generate implements Provider.
func (f *Fal) Generate(ctx context.Context, key string, r Request) (Result, error) {
	if key == "" {
		return Result{}, fmt.Errorf("fal: no API key")
	}
	if len(r.Base) == 0 {
		return Result{}, fmt.Errorf("fal: no input image")
	}
	m, err := f.resolve(r.Model)
	if err != nil {
		return Result{}, err
	}
	mime := r.BaseMIME
	if mime == "" {
		mime = http.DetectContentType(r.Base)
	}
	// Shrink BEFORE encoding: several fal endpoints bill per megapixel and
	// round up, so the input's size sets the output's price. Skipped when the
	// image is already under budget, so a small base costs nothing extra here.
	scaled, scaledMIME, err := downscaleToBudget(r.Base)
	if err != nil {
		return Result{}, fmt.Errorf("fal: preparing the input image: %w", err)
	}
	r.Base = scaled
	if scaledMIME != "" {
		mime = scaledMIME
	}
	body, err := falBody(m, r, mime)
	if err != nil {
		return Result{}, fmt.Errorf("fal: encode request: %w", err)
	}

	status, raw, err := f.submit(ctx, key, m.ID, body)
	if err != nil {
		return Result{}, err
	}
	if status/100 != 2 {
		return Result{}, fmt.Errorf("fal: submit: HTTP %d: %s", status, bodyForError(raw, key))
	}

	// PAST THIS LINE THE USER HAS BEEN CHARGED. The submit returned 2xx, so fal
	// generated the image; everything that can still go wrong is our failure to
	// collect it, not their failure to produce it. Each of these returns
	// therefore carries Billed, or the run reports a spend lower than the bill.
	billed := ErrorResult(m.ID, r.Seed)

	var fr falResponse
	if err := json.Unmarshal(raw, &fr); err != nil {
		return billed, fmt.Errorf("fal: decode response: %w", err)
	}
	if len(fr.Images) == 0 {
		return billed, fmt.Errorf("fal: response carried no images")
	}

	img, err := f.fetchImage(ctx, fr.Images[0].URL)
	if err != nil {
		return billed, err
	}

	out := Result{
		Image:  img,
		MIME:   http.DetectContentType(img),
		Seed:   r.Seed,
		Model:  m.ID,
		Billed: true,
	}
	// The moderation flag is intentionally NOT acted on: with the safety
	// checker disabled on the request, content fal would have flagged is kept
	// rather than dropped. has_nsfw_concepts is still decoded (fal may set it
	// regardless) but no longer turns into a FinishContentFiltered that the
	// pack pipeline treats as a failed slot.

	luma, err := meanLuminance(img)
	if err != nil {
		return billed, fmt.Errorf("fal: inspect result: %v", err)
	}
	if luma < blackLuma {
		// Image is deliberately dropped: a black frame that reaches a caller is
		// a black frame that eventually gets written to disk.
		return Result{Seed: r.Seed, Model: m.ID, FinishReason: FinishBlackImage, Billed: true}, ErrBlackImage
	}
	return out, nil
}

// submit posts the payload once. It is never retried: the server may have
// accepted and started billing the job before the response failed.
func (f *Fal) submit(ctx context.Context, key, modelID string, body []byte) (int, []byte, error) {
	ctx, cancel := context.WithTimeout(ctx, requestTimeout)
	defer cancel()
	endpoint := strings.TrimSuffix(FalBase, "/") + "/" + modelID
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(body))
	if err != nil {
		return 0, nil, fmt.Errorf("fal: submit: build request: %v", err)
	}
	req.Header.Set("Authorization", "Key "+key)
	req.Header.Set("Content-Type", "application/json")
	resp, err := f.client().Do(req)
	if err != nil {
		return 0, nil, wrapErr("fal: submit", err)
	}
	defer resp.Body.Close()
	raw, err := io.ReadAll(io.LimitReader(resp.Body, maxBodyBytes))
	if err != nil {
		return resp.StatusCode, nil, wrapErr("fal: submit: read body", err)
	}
	return resp.StatusCode, raw, nil
}

// fetchImage resolves the url a result carries. fal returns either an https URL
// or an inline data: URI depending on size, so both must work.
func (f *Fal) fetchImage(ctx context.Context, ref string) ([]byte, error) {
	if strings.HasPrefix(ref, "data:") {
		i := strings.Index(ref, ",")
		if i < 0 {
			return nil, fmt.Errorf("fal: malformed data URI in result")
		}
		raw, err := base64.StdEncoding.DecodeString(ref[i+1:])
		if err != nil {
			return nil, fmt.Errorf("fal: decode result data URI: %v", err)
		}
		return raw, nil
	}
	if ref == "" {
		return nil, fmt.Errorf("fal: result carried an empty url")
	}
	return f.fetchWithRetry(ctx, ref)
}

// fetchWithRetry downloads a finished result, retrying on failure with an
// exponential backoff that a cancelled context cuts short.
func (f *Fal) fetchWithRetry(ctx context.Context, ref string) ([]byte, error) {
	var last error
	for attempt := 1; attempt <= fetchAttempts; attempt++ {
		if attempt > 1 {
			select {
			case <-ctx.Done():
				return nil, fmt.Errorf("fal: fetch result: %v", ctx.Err())
			case <-time.After(retryBase << uint(attempt-2)):
			}
		}
		attemptCtx, cancel := context.WithTimeout(ctx, fetchTimeout)
		raw, err := f.get(attemptCtx, ref)
		cancel()
		if err == nil {
			return raw, nil
		}
		last = err
		// A cancelled parent context will not recover on the next attempt.
		if ctx.Err() != nil {
			return nil, fmt.Errorf("fal: fetch result: %v", ctx.Err())
		}
	}
	return nil, fmt.Errorf("%v (after %d attempts)", last, fetchAttempts)
}

func (f *Fal) get(ctx context.Context, ref string) ([]byte, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, ref, nil)
	if err != nil {
		return nil, fmt.Errorf("fal: fetch result: build request: %v", err)
	}
	resp, err := f.client().Do(req)
	if err != nil {
		return nil, wrapErr("fal: fetch result", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode/100 != 2 {
		io.Copy(io.Discard, io.LimitReader(resp.Body, 4096))
		return nil, fmt.Errorf("fal: fetch result: HTTP %d", resp.StatusCode)
	}
	raw, err := io.ReadAll(io.LimitReader(resp.Body, maxBodyBytes))
	if err != nil {
		return nil, wrapErr("fal: fetch result: read body", err)
	}
	return raw, nil
}

func dataURI(mime string, raw []byte) string {
	return "data:" + mime + ";base64," + base64.StdEncoding.EncodeToString(raw)
}

func seedPtr(s int64) *int64 {
	if s == 0 {
		return nil
	}
	return &s
}

// FinishReason values this package produces.
const (
	// FinishContentFiltered means the provider flagged the output. The bytes
	// are real; the caller must not keep them.
	FinishContentFiltered = "content_filtered"
	// FinishBlackImage means the output was an all-black frame.
	FinishBlackImage = "black_image"
)

// ErrBlackImage reports an all-black result. This is how a silent safety
// rejection arrives on this endpoint: HTTP 200, a well-formed response, and a
// black frame. Nothing in the body says so, so luminance is the only signal.
var ErrBlackImage = errors.New("provider returned an all-black image")

// blackLuma is the mean-luminance cutoff, on a 0..255 scale. Established by the
// spike, which used the same test to catch exactly this failure.
const blackLuma = 4.0

// A per-side cap alone is not a bound on memory: 16384 on each side is still
// 16384*16384*4 = ~1 GiB once decoded. So the real guard is the pixel COUNT,
// with the per-side cap kept only to reject absurd aspect ratios early.
//
// 40 megapixels is ~160 MiB decoded and roughly 40x the largest image any
// current provider returns for this use, so it rejects a decompression bomb
// without being a limit anything legitimate can reach.
const (
	maxDim    = 16384
	maxPixels = 40 << 20 // 40 Mpx; ~160 MiB as RGBA
)

// meanLuminance decodes raw and returns the mean luminance of a sampled grid.
// DecodeConfig runs first because it reads only the header, so a hostile or
// corrupt size is rejected before anything large is allocated.
func meanLuminance(raw []byte) (float64, error) {
	cfg, _, err := image.DecodeConfig(bytes.NewReader(raw))
	if err != nil {
		return 0, fmt.Errorf("decode header: %v", err)
	}
	if cfg.Width <= 0 || cfg.Height <= 0 || cfg.Width > maxDim || cfg.Height > maxDim {
		return 0, fmt.Errorf("implausible image dimensions %dx%d", cfg.Width, cfg.Height)
	}
	if px := int64(cfg.Width) * int64(cfg.Height); px > maxPixels {
		return 0, fmt.Errorf("image too large to decode: %d pixels (max %d)", px, maxPixels)
	}
	img, _, err := image.Decode(bytes.NewReader(raw))
	if err != nil {
		return 0, fmt.Errorf("decode: %v", err)
	}
	b := img.Bounds()
	if b.Dx() == 0 || b.Dy() == 0 {
		return 0, fmt.Errorf("image has no pixels")
	}
	// A sampled grid, not every pixel: this statistic only has to separate
	// "black" from "not black", and a full pass over a megapixel is wasted work.
	const grid = 64
	var sum float64
	for gy := 0; gy < grid; gy++ {
		for gx := 0; gx < grid; gx++ {
			x := b.Min.X + (b.Dx()*gx)/grid
			y := b.Min.Y + (b.Dy()*gy)/grid
			r, g, bl, _ := img.At(x, y).RGBA()
			// RGBA returns alpha-premultiplied 16-bit values; /257 rescales to
			// the 0..255 range blackLuma is expressed in.
			sum += (0.299*float64(r) + 0.587*float64(g) + 0.114*float64(bl)) / 257
		}
	}
	return sum / float64(grid*grid), nil
}

// wrapErr attaches an operation to a transport error and, critically, discards
// the URL. A bare *url.Error stringifies as
//
//	Post "https://host/path?token=SECRET": dial tcp: connection refused
//
// so returning one unmodified is how a credential in a URL reaches a log or an
// error shown to the user. errors.As rather than a type assertion, because the
// transport error may already be wrapped by the time it arrives.
func wrapErr(op string, err error) error {
	var ue *url.Error
	if errors.As(err, &ue) {
		return fmt.Errorf("%s: %s: %v", op, ue.Op, ue.Err)
	}
	return fmt.Errorf("%s: %v", op, err)
}

// bodyForError prepares a provider response body for inclusion in an error:
// trimmed, because bodies can be pages long, and redacted against the key,
// because some APIs echo request context back in their error prose.
func bodyForError(raw []byte, key string) string {
	s := strings.TrimSpace(string(raw))
	if key != "" {
		s = strings.ReplaceAll(s, key, "[redacted]")
	}
	if len(s) > 200 {
		return s[:200] + "…"
	}
	return s
}
