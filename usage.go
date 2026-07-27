package main

import (
	"encoding/json"
	"fmt"
	"net/http"
	"sort"
	"strings"
	"time"
)

// PlanUsage is the subscription rate-limit picture Claude Code shows in its
// /usage screen, fetched from the same OAuth endpoint with the login Claude
// Code already keeps in the macOS keychain.
type PlanUsage struct {
	Windows   []UsageWindow `json:"windows"`
	FetchedAt string        `json:"fetchedAt"` // RFC3339
}

// UsageWindow is one rate-limit window (5-hour session, weekly, per-model weekly).
type UsageWindow struct {
	Key         string  `json:"key"`
	Label       string  `json:"label"`
	Weekly      bool    `json:"weekly"`
	Utilization float64 `json:"utilization"` // percent 0-100
	ResetsAt    string  `json:"resetsAt"`    // RFC3339, may be empty
}

// windowLabels maps known endpoint keys to display labels; unknown keys fall
// back to a prettified key so schema drift degrades gracefully.
var windowLabels = map[string]string{
	"five_hour":        "Current session",
	"seven_day":        "All models",
	"seven_day_opus":   "Opus",
	"seven_day_oauth":  "All models",
	"seven_day_sonnet": "Sonnet",
	"seven_day_fable":  "Fable",
	"seven_day_haiku":  "Haiku",
}

// claudeOAuthToken reads Claude Code's OAuth access token from wherever the CLI
// stores it: the macOS keychain (usage_darwin.go) or ~/.claude/.credentials.json
// on other platforms (usage_other.go). Both hand the raw JSON to
// parseClaudeCredential.

// parseClaudeCredential extracts the OAuth access token from Claude Code's
// credential JSON. The shape is identical whether the blob came from the macOS
// keychain or the on-disk .credentials.json.
func parseClaudeCredential(data string) (string, error) {
	var blob struct {
		ClaudeAiOauth struct {
			AccessToken string `json:"accessToken"`
			ExpiresAt   int64  `json:"expiresAt"`
		} `json:"claudeAiOauth"`
	}
	if err := json.Unmarshal([]byte(strings.TrimSpace(data)), &blob); err != nil ||
		blob.ClaudeAiOauth.AccessToken == "" {
		return "", fmt.Errorf("unrecognized Claude Code credential format")
	}
	return blob.ClaudeAiOauth.AccessToken, nil
}

// PlanUsage fetches current plan rate-limit utilization. Never cached: the
// drawer polls it explicitly.
func (a *App) PlanUsage() (PlanUsage, error) {
	token, err := claudeOAuthToken()
	if err != nil {
		return PlanUsage{}, err
	}

	req, _ := http.NewRequest("GET", "https://api.anthropic.com/api/oauth/usage", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("anthropic-beta", "oauth-2025-04-20")
	req.Header.Set("Content-Type", "application/json")
	client := &http.Client{Timeout: 8 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return PlanUsage{}, fmt.Errorf("usage fetch failed: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode == http.StatusUnauthorized || resp.StatusCode == http.StatusForbidden {
		return PlanUsage{}, fmt.Errorf("Claude login token rejected (%d) — run claude once so it refreshes, then retry", resp.StatusCode)
	}
	if resp.StatusCode != http.StatusOK {
		return PlanUsage{}, fmt.Errorf("usage endpoint returned %d", resp.StatusCode)
	}

	// Tolerant parse: any top-level object holding utilization/resets_at is a
	// window; everything else is ignored.
	var raw map[string]json.RawMessage
	if err := json.NewDecoder(resp.Body).Decode(&raw); err != nil {
		return PlanUsage{}, fmt.Errorf("usage response unreadable: %w", err)
	}
	var windows []UsageWindow
	for key, v := range raw {
		var w struct {
			Utilization *float64 `json:"utilization"`
			ResetsAt    string   `json:"resets_at"`
		}
		if json.Unmarshal(v, &w) != nil || w.Utilization == nil {
			continue
		}
		label, known := windowLabels[key]
		if !known {
			label = strings.ReplaceAll(strings.TrimPrefix(key, "seven_day_"), "_", " ")
			if label != "" {
				label = strings.ToUpper(label[:1]) + label[1:]
			}
		}
		windows = append(windows, UsageWindow{
			Key: key, Label: label, Weekly: strings.HasPrefix(key, "seven_day"),
			Utilization: *w.Utilization, ResetsAt: w.ResetsAt,
		})
	}
	if len(windows) == 0 {
		return PlanUsage{}, fmt.Errorf("no usage windows in response (endpoint schema changed?)")
	}
	// Session window first, then weekly aggregate, then per-model by key.
	rank := func(w UsageWindow) int {
		switch {
		case !w.Weekly:
			return 0
		case w.Key == "seven_day" || w.Key == "seven_day_oauth":
			return 1
		default:
			return 2
		}
	}
	sort.Slice(windows, func(i, j int) bool {
		if r1, r2 := rank(windows[i]), rank(windows[j]); r1 != r2 {
			return r1 < r2
		}
		return windows[i].Key < windows[j].Key
	})
	return PlanUsage{Windows: windows, FetchedAt: time.Now().Format(time.RFC3339)}, nil
}
