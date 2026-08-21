// Package anthropic supplies the native Anthropic model catalog: the models
// this build ships knowing about, plus live discovery against the Models API
// for anything released since the build.
//
// Two sources because neither alone is enough. Native sessions authenticate
// with the Claude Code subscription's OAuth, so Commander usually holds no API
// key and cannot call the Models API at all — the built-in list is what a
// subscription-only user gets, and it is why the picker stops going stale
// between releases. Live discovery then covers models newer than the build, for
// anyone who has a key stored.
package anthropic

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"
)

// APIBase is the Anthropic API root. Var, not const, so tests can point at a
// local server.
var APIBase = "https://api.anthropic.com"

// Version is the anthropic-version header every request must carry.
const Version = "2023-06-01"

// KeyEnv is the keychain ref (and env var name) the API key is read from. It is
// optional: without it Commander falls back to Catalog().
const KeyEnv = "ANTHROPIC_API_KEY"

// Model is one native Anthropic model ready to add to the catalog.
type Model struct {
	ID          string  `json:"id"`
	Label       string  `json:"label"`
	InputPrice  float64 `json:"inputPrice"`  // USD per 1M input tokens
	OutputPrice float64 `json:"outputPrice"` // USD per 1M output tokens
}

// Catalog is the built-in list of generally available models, in the order the
// picker shows them: DefaultID first, then by tier.
//
// Deliberately excludes invitation-only models (Project Glasswing's Mythos):
// listing a model most users cannot call is worse than omitting it, and live
// discovery surfaces it for the keys that do have access.
//
// Prices are the Anthropic first-party per-1M-token rates. They are here rather
// than fetched because the Models API does not report pricing. Native sessions
// are subscription-billed, so these drive the cost estimate on the card, not a
// bill — a stale price misinforms, it does not overcharge.
func Catalog() []Model {
	return []Model{
		{ID: "claude-opus-5", Label: "Anthropic · Opus 5", InputPrice: 5, OutputPrice: 25},
		{ID: "claude-fable-5", Label: "Anthropic · Fable 5", InputPrice: 10, OutputPrice: 50},
		{ID: "claude-opus-4-8", Label: "Anthropic · Opus 4.8", InputPrice: 5, OutputPrice: 25},
		{ID: "claude-opus-4-7", Label: "Anthropic · Opus 4.7", InputPrice: 5, OutputPrice: 25},
		{ID: "claude-opus-4-6", Label: "Anthropic · Opus 4.6", InputPrice: 5, OutputPrice: 25},
		{ID: "claude-sonnet-5", Label: "Anthropic · Sonnet 5", InputPrice: 3, OutputPrice: 15},
		{ID: "claude-sonnet-4-6", Label: "Anthropic · Sonnet 4.6", InputPrice: 3, OutputPrice: 15},
		{ID: "claude-haiku-4-5", Label: "Anthropic · Haiku 4.5", InputPrice: 1, OutputPrice: 5},
	}
}

// DefaultID is the model a first run selects: the current flagship, and what
// Claude Code itself defaults to. Not simply Catalog()[0] — the list is ordered
// for the picker, and the most expensive model is not the right default.
const DefaultID = "claude-opus-5"

// CatalogRev is bumped whenever Catalog changes. A config records the revision
// it was last merged against, so each build folds its new models in exactly
// once — a model the user deleted on purpose does not reappear on every launch,
// only after an upgrade that actually changed the list.
const CatalogRev = 1

// priced looks up a built-in price and label for an id discovered live.
func priced(id string) (Model, bool) {
	for _, m := range Catalog() {
		if m.ID == id {
			return m, true
		}
	}
	return Model{}, false
}

// ListModels fetches GET /v1/models. A model already in Catalog keeps its
// built-in label and price; anything newer takes the API's display_name and is
// left unpriced, since the API does not report pricing and guessing a rate is
// worse than showing none.
func ListModels(ctx context.Context, apiKey string) ([]Model, error) {
	if apiKey == "" {
		return nil, fmt.Errorf("no %s stored; the built-in catalog is used instead", KeyEnv)
	}
	ctx, cancel := context.WithTimeout(ctx, 15*time.Second)
	defer cancel()
	// One page of 100: the listing is a couple of dozen models and 100 is the
	// API's maximum page size, so paging would be dead code today. See the
	// has_more handling below for what happens if that stops being true.
	url := APIBase + "/v1/models?limit=100"
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("x-api-key", apiKey)
	req.Header.Set("anthropic-version", Version)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("anthropic discovery: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		io.Copy(io.Discard, resp.Body) // drain for keep-alive reuse
		return nil, fmt.Errorf("anthropic discovery: %s from %s (check %s)", resp.Status, url, KeyEnv)
	}
	var body struct {
		Data []struct {
			ID          string `json:"id"`
			DisplayName string `json:"display_name"`
		} `json:"data"`
		HasMore bool `json:"has_more"`
	}
	// Cap the body: this decodes a response from a host named by a mutable
	// package var, and an unbounded decode would let a wrong APIBase exhaust
	// memory.
	if err := json.NewDecoder(io.LimitReader(resp.Body, 1<<20)).Decode(&body); err != nil {
		return nil, fmt.Errorf("anthropic discovery: decode: %w", err)
	}
	// HasMore is decoded rather than ignored so the single-page assumption is
	// visible in the type. It deliberately does not error: the merge is add-only,
	// so a truncated page means fewer additions this launch, which beats none.
	truncated := body.HasMore
	out := make([]Model, 0, len(body.Data))
	if truncated && len(body.Data) == 0 {
		return nil, fmt.Errorf("anthropic discovery: paginated response with no models on the first page")
	}
	for _, d := range body.Data {
		if d.ID == "" {
			continue
		}
		if known, ok := priced(d.ID); ok {
			out = append(out, known)
			continue
		}
		label := d.DisplayName
		if label == "" {
			label = d.ID
		}
		out = append(out, Model{ID: d.ID, Label: "Anthropic · " + label})
	}
	return out, nil
}
