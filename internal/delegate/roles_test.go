package delegate

import (
	"strings"
	"testing"
)

func TestRolesDefaults(t *testing.T) {
	r, err := RoleByName("scout")
	if err != nil {
		t.Fatal(err)
	}
	if r.Model != "ollama-glm-5.3" {
		t.Errorf("scout model = %q, want ollama-glm-5.3", r.Model)
	}
	// glm was the fastest correct arm in the four-arm measurement (spec §4.4).
	if r.Effort != "low" {
		t.Errorf("effort = %q, want low: default effort doubled token use (spec §4.9)", r.Effort)
	}
	// glm took 246s on the hardest task. A 120s deadline would kill healthy work.
	if r.TimeoutSec < 300 {
		t.Errorf("timeout = %d, want >= 300: measured tail is 246s (spec §4.4)", r.TimeoutSec)
	}
	if len(r.Tools) == 0 {
		t.Error("empty Tools would leave the worker with zero tools (spec §4.6)")
	}
}

func TestRolesExceedProxyRequestTimeout(t *testing.T) {
	// ProxyRequestTimeout (proxy.go) must sit strictly below every role's
	// TimeoutSec, or request_timeout becomes dead config: LiteLLM would never
	// be the one to time out an upstream stall, our own kill always wins
	// first, and a genuine upstream error gets misattributed to "worker
	// exceeded its deadline" instead of surfacing as an upstream error.
	for _, r := range Roles() {
		if r.TimeoutSec <= ProxyRequestTimeout {
			t.Errorf("role %q: TimeoutSec %d must be > ProxyRequestTimeout %d", r.Name, r.TimeoutSec, ProxyRequestTimeout)
		}
	}
}

func TestRoleByNameUnknown(t *testing.T) {
	_, err := RoleByName("nope")
	if err == nil {
		t.Fatal("want an error for an unknown role")
	}
	if got := err.Error(); !strings.HasPrefix(got, "refusing to") || !strings.Contains(got, "scout") {
		t.Errorf("error = %q, want a `refusing to` message listing valid roles", got)
	}
}
