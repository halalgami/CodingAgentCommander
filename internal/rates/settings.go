package rates

import (
	"encoding/json"
	"os"
	"path/filepath"
)

type Plan struct {
	Name       string  `json:"name"`
	MonthlyUSD float64 `json:"monthlyUSD"`
}

// Settings: Overrides keyed by exact model id or "provider:<id>"; Plans by provider id.
type Settings struct {
	Overrides map[string]Rate `json:"overrides,omitempty"`
	Plans     map[string]Plan `json:"plans,omitempty"`
	// AIModel is the model Al Asturlab's AI features use ("" = haiku).
	AIModel string `json:"aiModel,omitempty"`
}

// LoadSettings: missing → zero; corrupt → backed up to path+".corrupt", zero.
func LoadSettings(path string) Settings {
	body, err := os.ReadFile(path)
	if err != nil {
		return Settings{}
	}
	var s Settings
	if json.Unmarshal(body, &s) != nil {
		_ = os.WriteFile(path+".corrupt", body, 0o600)
		return Settings{}
	}
	return s
}

// SaveSettings writes atomically (temp + rename), 0600, dir 0700.
func SaveSettings(path string, s Settings) error {
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return err
	}
	body, err := json.MarshalIndent(s, "", "  ")
	if err != nil {
		return err
	}
	f, err := os.CreateTemp(dir, ".asturlab-*.json")
	if err != nil {
		return err
	}
	tmp := f.Name()
	if _, err = f.Write(body); err == nil {
		err = f.Chmod(0o600)
	}
	if cerr := f.Close(); err == nil {
		err = cerr
	}
	if err == nil {
		err = os.Rename(tmp, path)
	}
	if err != nil {
		_ = os.Remove(tmp)
	}
	return err
}
