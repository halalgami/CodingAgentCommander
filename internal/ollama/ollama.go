// Package ollama discovers the models Ollama Cloud serves, via the native
// GET /api/tags endpoint.
//
// The listing is anonymous: ollama.com answers /api/tags without an API key, so
// discovery works before the user has stored one. That is convenient and it is
// also a gap — nothing here validates the key, so a wrong or revoked key first
// surfaces as a 401 from the proxy at turn time.
//
// There is no built-in catalog. Ollama retires its cloud lineup on a scale of
// weeks (an entire generation went away on 2026-07-15), so a hardcoded list
// would ship stale; live discovery is the only listing this package offers.
package ollama

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

// Model is one discovered Ollama Cloud model ready to add to the catalog. ID is
// the upstream model name exactly as Ollama reports it, tag and all.
type Model struct {
	ID    string `json:"id"`
	Label string `json:"label"`
}

// ErrKeyRejected reports that the upstream refused the API key.
var ErrKeyRejected = errors.New("API key rejected")

// verifyModel is a model name that cannot exist upstream. VerifyKey names it on
// purpose: Ollama checks authorization BEFORE it looks the model up, so a
// request for a nonexistent model separates the two answers cleanly — 401 means
// the key is bad, 404 means the key is good and only the model was missing —
// without running a generation or spending any quota.
const verifyModel = "__commander_key_check__"

// VerifyKey reports whether the stored key is accepted by Ollama Cloud.
//
// This exists because discovery is anonymous: /api/tags answers without a key,
// so a successful listing says nothing about whether the key works. Without
// this check the first sign of a wrong or revoked key is a 401 from the proxy
// mid-turn, inside a session the user has already opened.
//
// Only an explicit 401/403 returns ErrKeyRejected. Anything else — including a
// 5xx or a transport failure — is inconclusive and returns nil: refusing to
// launch because ollama.com had a bad minute would be worse than the problem
// this guards against.
func VerifyKey(ctx context.Context, apiBase, key string) error {
	if key == "" {
		return ErrKeyRejected
	}
	ctx, cancel := context.WithTimeout(ctx, 15*time.Second)
	defer cancel()
	url := strings.TrimSuffix(apiBase, "/") + "/api/chat"
	body := `{"model":"` + verifyModel + `","messages":[],"stream":false}`
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, strings.NewReader(body))
	if err != nil {
		return nil
	}
	req.Header.Set("Authorization", "Bearer "+key)
	req.Header.Set("Content-Type", "application/json")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil // inconclusive, not a rejection
	}
	defer resp.Body.Close()
	io.Copy(io.Discard, resp.Body) // drain for keep-alive reuse
	if resp.StatusCode == http.StatusUnauthorized || resp.StatusCode == http.StatusForbidden {
		return ErrKeyRejected
	}
	return nil
}

// ListModels fetches {apiBase}/api/tags. No Authorization header: the endpoint
// is anonymous, and sending a key would imply a validation this does not do.
func ListModels(ctx context.Context, apiBase string) ([]Model, error) {
	ctx, cancel := context.WithTimeout(ctx, 15*time.Second)
	defer cancel()
	url := strings.TrimSuffix(apiBase, "/") + "/api/tags"
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("ollama discovery: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		io.Copy(io.Discard, resp.Body) // drain for keep-alive reuse
		return nil, fmt.Errorf("ollama discovery: %s from %s", resp.Status, url)
	}
	var body struct {
		Models []struct {
			Name string `json:"name"`
		} `json:"models"`
	}
	// Cap the body: apiBase is user-supplied, and an unbounded decode would let
	// a wrong host exhaust memory.
	if err := json.NewDecoder(io.LimitReader(resp.Body, 1<<20)).Decode(&body); err != nil {
		return nil, fmt.Errorf("ollama discovery: decode: %w", err)
	}
	out := make([]Model, 0, len(body.Models))
	for _, m := range body.Models {
		if m.Name == "" {
			continue
		}
		out = append(out, Model{ID: m.Name, Label: "Ollama · " + m.Name})
	}
	return out, nil
}
