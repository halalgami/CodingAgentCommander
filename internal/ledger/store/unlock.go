package store

// Unlock records when an achievement was first earned.
type Unlock struct {
	ID        string `json:"id"`
	At        int64  `json:"at"`
	SessionID string `json:"sessionID,omitempty"`
}
