package main

import (
	"fmt"
	"strings"
	"time"

	wruntime "github.com/wailsapp/wails/v2/pkg/runtime"

	"github.com/halalgami/CodingAgentCommander/internal/imagegen"
)

// Bindings the wizard needs beyond the run itself: what can be generated with,
// what it will cost, where the base portrait comes from, and where the API key
// goes.
//
// The key rule governs this whole file: a key travels INTO Go and never comes
// back out. The webview learns only whether one is saved. There is no binding
// that returns a key, by construction rather than by discipline — an accessor
// that exists will eventually be called by something that logs its result.

// PackGenModel is one model offered for generation, with its price attached so
// the picker can show cost without a second round trip per model.
type PackGenModel struct {
	ID    string `json:"id"`
	Label string `json:"label"`
	Mode  string `json:"mode"`
	// USD is the per-image price. Known is false when the model is unpriced or
	// the price table has aged out, and the UI must then show a COUNT rather
	// than a number — a stale price presented as fact is worse than no price.
	USD   float64 `json:"usd"`
	Known bool    `json:"known"`
	// Refusal and Note come straight from the provider's catalogue. Refusal
	// tells the UI whether a rejection by this model costs the user money;
	// Note is the measured caveat shown beside it in the picker.
	Refusal string `json:"refusal,omitempty"`
	Note    string `json:"note,omitempty"`
}

// PackGenProvider is one image API the user can generate through.
type PackGenProvider struct {
	ID     string         `json:"id"`
	Models []PackGenModel `json:"models"`
	// HasKey reports only PRESENCE. The key itself never crosses this boundary.
	HasKey bool `json:"hasKey"`
}

// PackGenProviders lists what the wizard can generate with.
func (a *App) PackGenProviders() []PackGenProvider {
	now := time.Now()
	var out []PackGenProvider
	for _, p := range imagegen.Providers() {
		pv := PackGenProvider{ID: p.ID(), HasKey: packGenLoadKey(p.ID()) != ""}
		for _, m := range p.Models() {
			usd, known := imagegen.EstimateUSD(m.ID, 1, now)
			pv.Models = append(pv.Models, PackGenModel{
				ID: m.ID, Label: m.Label, Mode: string(m.Mode), USD: usd, Known: known,
				Refusal: m.Refusal, Note: m.Note,
			})
		}
		out = append(out, pv)
	}
	return out
}

// PackGenCost is an estimate for a whole run.
type PackGenCost struct {
	USD   float64 `json:"usd"`
	Known bool    `json:"known"`
	// Note is the disclosure that must accompany any figure — the table is
	// hand-maintained, so a number without its date is a claim we cannot back.
	Note string `json:"note"`
}

// PackGenEstimate quotes n images of a model. An unpriced or stale model
// returns Known=false, and the caller must show the count instead.
func (a *App) PackGenEstimate(model string, n int) PackGenCost {
	usd, ok := imagegen.EstimateUSD(model, n, time.Now())
	return PackGenCost{USD: usd, Known: ok, Note: imagegen.PriceNote()}
}

// SetImagegenKey stores an API key in the OS keychain.
//
// The key is trimmed because pasting from a dashboard reliably brings
// whitespace, and a key with a trailing newline fails authentication with a
// message that blames the key rather than the newline.
func (a *App) SetImagegenKey(providerID, key string) error {
	if _, ok := packGenLookup(providerID); !ok {
		return fmt.Errorf("unknown image provider %q", providerID)
	}
	key = strings.TrimSpace(key)
	if key == "" {
		return fmt.Errorf("the key is empty")
	}
	return imagegen.StoreKey(providerID, key)
}

// ClearImagegenKey removes a stored key. Clearing an absent key is not an
// error: the UI's Clear button must be idempotent.
func (a *App) ClearImagegenKey(providerID string) error {
	if _, ok := packGenLookup(providerID); !ok {
		return fmt.Errorf("unknown image provider %q", providerID)
	}
	return imagegen.ClearKey(providerID)
}

// PackGenCondition is a condition the wizard can offer as an extra variant.
type PackGenCondition struct {
	Name  string   `json:"name"`
	Class string   `json:"class"` // ambient | event
	Slots []string `json:"slots"`
	Label string   `json:"label"`
}

// PackGenConditions lists the conditions that can be generated as variants,
// with the slots each can actually display in. Handing the UI the matrix means
// it can grey out the combinations that would produce art that never shows,
// rather than letting the user pick one and be refused after the fact.
func (a *App) PackGenConditions() []PackGenCondition {
	var out []PackGenCondition
	for _, name := range sortedCondNames() {
		slots := packCondSlots[name]
		// Background slots are routed past the resolver, so they never appear
		// here; the matrix already omits them.
		out = append(out, PackGenCondition{
			Name:  name,
			Class: packConditions[name],
			Slots: append([]string(nil), slots...),
			Label: packGenConditionStaging[name],
		})
	}
	return out
}

// PickPackGenBase opens a native file dialog for the base portrait.
//
// The chosen file is DECODED here, not merely extension-checked, so a file that
// is not really an image is refused at the moment of picking rather than after
// the user has configured a whole run. Cancel returns ("", nil).
func (a *App) PickPackGenBase() (string, error) {
	path, err := wruntime.OpenFileDialog(a.ctx, wruntime.OpenDialogOptions{
		Title: "Choose the base portrait",
		Filters: []wruntime.FileFilter{
			{DisplayName: "Images (*.png;*.jpg;*.jpeg)", Pattern: "*.png;*.jpg;*.jpeg"},
		},
	})
	if err != nil || path == "" { // cancel -> ""
		return "", err
	}
	if _, err := packGenBaseFacts(path); err != nil {
		return "", err
	}
	return path, nil
}

// PackGenBase describes a chosen base portrait, so the wizard can show it back
// and warn about a shape that will crop badly.
type PackGenBase struct {
	Path string `json:"path"`
	W    int    `json:"w"`
	H    int    `json:"h"`
	// Warning is advisory, never blocking. The host renders art object-fit
	// cover anchored top, so a wide landscape source loses its sides; saying so
	// before six images are paid for is the entire point.
	Warning string `json:"warning,omitempty"`
}

// InspectPackGenBase validates a path the user supplied directly (typed, or
// carried over from a previous run) and returns what the wizard should show.
func (a *App) InspectPackGenBase(path string) (PackGenBase, error) {
	return packGenBaseFacts(path)
}

func packGenBaseFacts(path string) (PackGenBase, error) {
	raw, err := readFileLimited(path, packGenMaxBaseBytes)
	if err != nil {
		return PackGenBase{}, err
	}
	facts, err := packGenImageFacts(raw)
	if err != nil {
		return PackGenBase{}, fmt.Errorf("that file could not be read as a PNG or JPEG image: %w", err)
	}
	out := PackGenBase{Path: path, W: facts.W, H: facts.H}
	if facts.W > facts.H {
		out.Warning = "This is a landscape image. The companion is rendered as a tall portrait cropped from the top, so the sides will be cut off — a portrait or square source works better."
	}
	return out, nil
}
