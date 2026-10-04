// Package anthropic supplies the native Anthropic model catalog: the models
// this build ships knowing about, plus live discovery against the Models API
// for anything released since the build.
//
// Two sources because neither alone is enough. The built-in list is what a user
// gets before the first discovery pass lands, and it is what an offline launch
// falls back to. Live discovery then covers everything released since the build
// — which, on a long-lived install, is the common case rather than the edge one.
//
// Discovery authenticates with whatever credential is available. Native sessions
// use the Claude Code subscription's OAuth login, so there is usually no API key
// anywhere; the same keychain token that drives the usage drawer also reads
// GET /v1/models, which is why discovery no longer requires a key the user would
// have to go find.
package anthropic

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"regexp"
	"time"
)

// APIBase is the Anthropic API root. Var, not const, so tests can point at a
// local server.
var APIBase = "https://api.anthropic.com"

// Version is the anthropic-version header every request must carry.
const Version = "2023-06-01"

// OAuthBeta is the anthropic-beta value an OAuth bearer token must be presented
// with. /v1/models happens to answer without it, but the requirement is
// endpoint-dependent and undocumented per-endpoint, so every OAuth request sends
// it rather than relying on one endpoint's leniency.
const OAuthBeta = "oauth-2025-04-20"

// KeyEnv is the env var an API key is read from. Optional, and no longer the
// only way in: it is the fallback for installs that export a key, while the
// usual path is the Claude Code OAuth login.
const KeyEnv = "ANTHROPIC_API_KEY"

// Credential is a resolved way to authenticate to the Models API. OAuth tokens
// and API keys go in different headers, so the kind has to travel with the
// secret rather than being guessed from its shape.
type Credential struct {
	Token string
	// OAuth selects "Authorization: Bearer" + the beta header over "x-api-key".
	OAuth bool
}

// Empty reports that there is nothing to authenticate with.
func (c Credential) Empty() bool { return c.Token == "" }

// apply sets whichever auth headers this credential's kind requires.
func (c Credential) apply(req *http.Request) {
	if c.OAuth {
		req.Header.Set("Authorization", "Bearer "+c.Token)
		req.Header.Set("anthropic-beta", OAuthBeta)
		return
	}
	req.Header.Set("x-api-key", c.Token)
}

// Model is one native Anthropic model ready to add to the catalog.
//
// Token limits rather than prices: the Models API reports max_input_tokens and
// max_tokens but says nothing about cost, and native sessions are
// subscription-billed anyway — a per-turn dollar figure on a session that
// spends no dollars was invented precision. The context window is both real and
// reported, and it is what the session card's fullness band needs.
type Model struct {
	ID    string `json:"id"`
	Label string `json:"label"`
	// MaxInputTokens is the context window. Zero means unknown — a model
	// discovered from an API response that omitted the field.
	MaxInputTokens int `json:"maxInputTokens"`
	// MaxOutputTokens is the per-response output ceiling. Zero means unknown.
	MaxOutputTokens int `json:"maxOutputTokens"`
}

// Catalog is the built-in list of generally available models, in the order the
// picker shows them: DefaultID first, then by tier.
//
// Deliberately excludes invitation-only models: listing a model most users
// cannot call is worse than omitting it, and live discovery surfaces it for the
// credentials that do have access.
//
// Ids here are aliases, not dated snapshots. Aliases are stable and are what a
// config should pin; the API lists some models under a dated id instead, which
// Canonical reconciles so the two sources do not each contribute an entry for
// the same model.
func Catalog() []Model {
	return []Model{
		{ID: "claude-opus-5-5", Label: "Anthropic · Opus 5.5", MaxInputTokens: 1_000_000, MaxOutputTokens: 128_000},
		{ID: "claude-sonnet-5-5", Label: "Anthropic · Sonnet 5.5", MaxInputTokens: 1_000_000, MaxOutputTokens: 128_000},
		{ID: "claude-fable-5-1", Label: "Anthropic · Fable 5.1", MaxInputTokens: 1_000_000, MaxOutputTokens: 128_000},
		{ID: "claude-opus-5", Label: "Anthropic · Opus 5", MaxInputTokens: 1_000_000, MaxOutputTokens: 128_000},
		{ID: "claude-sonnet-5", Label: "Anthropic · Sonnet 5", MaxInputTokens: 1_000_000, MaxOutputTokens: 128_000},
		{ID: "claude-fable-5", Label: "Anthropic · Fable 5", MaxInputTokens: 1_000_000, MaxOutputTokens: 128_000},
		{ID: "claude-opus-4-8", Label: "Anthropic · Opus 4.8", MaxInputTokens: 1_000_000, MaxOutputTokens: 128_000},
		{ID: "claude-opus-4-7", Label: "Anthropic · Opus 4.7", MaxInputTokens: 1_000_000, MaxOutputTokens: 128_000},
		{ID: "claude-opus-4-6", Label: "Anthropic · Opus 4.6", MaxInputTokens: 1_000_000, MaxOutputTokens: 128_000},
		{ID: "claude-sonnet-4-6", Label: "Anthropic · Sonnet 4.6", MaxInputTokens: 1_000_000, MaxOutputTokens: 128_000},
		{ID: "claude-haiku-4-5", Label: "Anthropic · Haiku 4.5", MaxInputTokens: 200_000, MaxOutputTokens: 64_000},
	}
}

// DefaultID is the model a first run selects: the current flagship. Not simply
// the most expensive or the newest-sounding — it is the one Claude Code itself
// defaults to, and it must appear in Catalog or a first run writes a config
// Load rejects.
//
// Only fresh installs are affected. An existing config carries its own
// default_model, which the merge never rewrites.
const DefaultID = "claude-opus-5-5"

// CatalogRev is bumped whenever Catalog changes. A config records the revision
// it was last merged against, so each build folds its new models in exactly
// once — a model the user deleted on purpose does not reappear on every launch,
// only after an upgrade that actually changed the list.
//
// Rev 2 added Opus 5.5 / Sonnet 5.5 / Fable 5.1 and dropped the price columns.
const CatalogRev = 2

// datedSuffix matches the "-20251001" a dated snapshot id ends with.
var datedSuffix = regexp.MustCompile(`-\d{8}$`)

// Canonical reduces a dated snapshot id to its alias, so that
// "claude-haiku-4-5-20251001" from the API and "claude-haiku-4-5" from the
// built-in catalog are recognised as the same model.
//
// Without this the merge is an exact-id match and the picker ends up listing
// both, which reads as two models when it is one. The reduction is only used
// for matching — whichever id the caller already holds is the one kept, so a
// config that pinned a dated id keeps working.
func Canonical(id string) string { return datedSuffix.ReplaceAllString(id, "") }

// known looks up a built-in entry for an id discovered live, matching dated
// snapshots against their alias.
func known(id string) (Model, bool) {
	canon := Canonical(id)
	for _, m := range Catalog() {
		if m.ID == id || Canonical(m.ID) == canon {
			return m, true
		}
	}
	return Model{}, false
}

// ListModels fetches GET /v1/models with the given credential. A model already
// in Catalog keeps its built-in alias, label, and limits; anything newer takes
// the API's display_name and whatever limits the response reports.
func ListModels(ctx context.Context, cred Credential) ([]Model, error) {
	if cred.Empty() {
		return nil, fmt.Errorf("no Anthropic credential available (no Claude Code login and no %s)", KeyEnv)
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
	cred.apply(req)
	req.Header.Set("anthropic-version", Version)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("anthropic discovery: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode == http.StatusUnauthorized || resp.StatusCode == http.StatusForbidden {
		io.Copy(io.Discard, resp.Body) // drain for keep-alive reuse
		return nil, fmt.Errorf("anthropic discovery: credential rejected (%d) — run claude once so its login refreshes", resp.StatusCode)
	}
	if resp.StatusCode != http.StatusOK {
		io.Copy(io.Discard, resp.Body)
		return nil, fmt.Errorf("anthropic discovery: %s from %s", resp.Status, url)
	}
	var body struct {
		Data []struct {
			ID              string `json:"id"`
			DisplayName     string `json:"display_name"`
			MaxInputTokens  int    `json:"max_input_tokens"`
			MaxOutputTokens int    `json:"max_tokens"`
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
	if truncated && len(body.Data) == 0 {
		return nil, fmt.Errorf("anthropic discovery: paginated response with no models on the first page")
	}
	out := make([]Model, 0, len(body.Data))
	for _, d := range body.Data {
		if d.ID == "" {
			continue
		}
		if m, ok := known(d.ID); ok {
			out = append(out, m)
			continue
		}
		label := d.DisplayName
		if label == "" {
			label = d.ID
		}
		out = append(out, Model{
			ID:              d.ID,
			Label:           "Anthropic · " + label,
			MaxInputTokens:  d.MaxInputTokens,
			MaxOutputTokens: d.MaxOutputTokens,
		})
	}
	return out, nil
}
