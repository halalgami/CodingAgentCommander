// Package quota keeps a history of plan quota samples (spec §4.2 item 4):
// one jsonl file per provider per month under the ledger directory, a
// 5-minute sampler, and a linear forecast of when a window hits its limit.
package quota

import "time"

// Window is one rate-limit window as a provider reports it.
type Window struct {
	Key      string  `json:"key"`
	Label    string  `json:"label"`
	Weekly   bool    `json:"weekly"`
	Util     float64 `json:"util"` // percent 0-100
	ResetsAt string  `json:"resetsAt"`
}

// Sample is every window of one provider at one moment.
type Sample struct {
	T       time.Time `json:"t"`
	Windows []Window  `json:"w"`
}

// Retention is how long history is kept (spec: 90 days).
const Retention = 90 * 24 * time.Hour
