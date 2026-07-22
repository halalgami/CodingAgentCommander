// Package zen discovers the models an OpenCode Zen/Go key can use, via the
// gateway's OpenAI-compatible GET /models endpoint.
package zen

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

// Model is one discovered Zen model ready to add to the catalog.
type Model struct {
	ID    string `json:"id"`
	Label string `json:"label"`
}

// ListModels fetches {apiBase}/models with the bearer key.
func ListModels(ctx context.Context, apiBase, key string) ([]Model, error) {
	if key == "" {
		return nil, fmt.Errorf("ZEN_KEY must be set in Providers first")
	}
	ctx, cancel := context.WithTimeout(ctx, 15*time.Second)
	defer cancel()
	url := strings.TrimSuffix(apiBase, "/") + "/models"
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+key)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("zen discovery: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		io.Copy(io.Discard, resp.Body) // drain for keep-alive reuse
		return nil, fmt.Errorf("zen discovery: %s from %s (check ZEN_KEY)", resp.Status, url)
	}
	var body struct {
		Data []struct {
			ID string `json:"id"`
		} `json:"data"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		return nil, fmt.Errorf("zen discovery: decode: %w", err)
	}
	out := make([]Model, 0, len(body.Data))
	for _, d := range body.Data {
		if d.ID == "" {
			continue
		}
		out = append(out, Model{ID: d.ID, Label: "Go · " + d.ID})
	}
	return out, nil
}
