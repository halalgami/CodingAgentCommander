package main

import "testing"

func TestDoubleClickAction(t *testing.T) {
	for raw, want := range map[string]string{
		"Maximize": "zoom", "Minimize": "minimize", "None": "none",
		"": "zoom", "\n": "zoom", "Minimize\n": "minimize", "banana": "zoom",
	} {
		if got := doubleClickAction(raw); got != want {
			t.Errorf("doubleClickAction(%q) = %q, want %q", raw, got, want)
		}
	}
}
