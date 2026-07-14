package router

import (
	"os"
	"os/exec"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/halalgami/CodingAgentCommander/internal/config"
)

func requireLitellm(t *testing.T) {
	t.Helper()
	if _, err := LitellmBin(); err != nil {
		t.Skip("litellm not installed")
	}
}

func TestControllerStartHealthStop(t *testing.T) {
	requireLitellm(t)
	// Minimal routed config using a dummy key env (no real calls are made;
	// we only probe /health, which does not require a valid upstream key).
	dir := t.TempDir()
	cfgPath := filepath.Join(dir, "litellm.yaml")
	body, err := GenerateConfig([]config.Model{dummyModel()}, "sk-test", Options{})
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(cfgPath, body, 0o600); err != nil {
		t.Fatal(err)
	}
	// The config references the strip_thinking callback; litellm imports it from
	// the config dir (Start sets cmd.Dir), so it must exist or boot fails.
	if err := os.WriteFile(filepath.Join(dir, HookFile), HookSource(), 0o644); err != nil {
		t.Fatal(err)
	}

	c := NewController(0) // 0 => pick a free port
	c.ConfigPath = cfgPath
	c.Env = []string{"ZEN_KEY=dummy"}
	if err := c.Start(); err != nil {
		t.Fatalf("Start: %v", err)
	}
	t.Cleanup(func() { _ = c.Stop() })

	// Poll health for up to ~20s (litellm boots slowly).
	var healthy bool
	for i := 0; i < 100; i++ {
		if c.Health() == nil {
			healthy = true
			break
		}
		time.Sleep(200 * time.Millisecond)
	}
	if !healthy {
		t.Fatal("litellm never became healthy")
	}
}

func TestRunningReflectsLifecycle(t *testing.T) {
	c := NewController(0)
	if c.Running() {
		t.Error("new controller should not be running")
	}
	// Simulate a started controller without a real process by setting cmd via Start
	// is heavy; instead assert Running() is false initially and after Stop() on a
	// never-started controller (no panic, still false).
	if err := c.Stop(); err != nil {
		t.Errorf("Stop on unstarted: %v", err)
	}
	if c.Running() {
		t.Error("still not running after Stop")
	}
}

func TestControllerStopRunningNoRace(t *testing.T) {
	c := NewController(0)
	var wg sync.WaitGroup
	for i := 0; i < 50; i++ {
		wg.Add(2)
		go func() { defer wg.Done(); _ = c.Stop() }()
		go func() { defer wg.Done(); _ = c.Running() }()
	}
	wg.Wait()
}

func TestReapStale(t *testing.T) {
	if _, err := exec.LookPath("pkill"); err != nil {
		t.Skip("pkill not available")
	}
	// Stand in for an orphaned litellm: a shell whose command line carries a
	// unique marker mimicking the --config path ReapStale matches on. The
	// trailing `:` keeps sh from exec-replacing itself with sleep (which would
	// drop the marker from the live process's argv).
	marker := filepath.Join(t.TempDir(), "litellm.yaml")
	cmd := exec.Command("sh", "-c", "sleep 30; : # "+marker)
	if err := cmd.Start(); err != nil {
		t.Fatalf("start marker proc: %v", err)
	}
	done := make(chan error, 1)
	go func() { done <- cmd.Wait() }() // sole Wait() consumer

	ReapStale(marker)

	select {
	case <-done: // process exited => reaped
	case <-time.After(5 * time.Second):
		_ = cmd.Process.Kill()
		<-done
		t.Fatal("ReapStale did not kill the marked process")
	}

	ReapStale("") // must be a safe no-op
}

func dummyModel() config.Model {
	return config.Model{ID: "gpt-5.5", Provider: "zen", Upstream: "openai/gpt-5.5", APIBase: "https://opencode.ai/zen/v1", KeyEnv: "ZEN_KEY"}
}

func TestLitellmBinOverride(t *testing.T) {
	dir := t.TempDir()
	f := filepath.Join(dir, "litellm")
	if err := os.WriteFile(f, []byte("#!/bin/sh\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("COMMANDER_LITELLM", f)
	p, err := LitellmBin()
	if err != nil || p != f {
		t.Errorf("override: got %q err=%v, want %q", p, err, f)
	}
}

func TestLitellmBinFindsInstalled(t *testing.T) {
	// This environment has litellm installed under ~/Library/Python/*/bin even
	// though it is not on PATH — the resolver must still find it.
	os.Unsetenv("COMMANDER_LITELLM")
	p, err := LitellmBin()
	if err != nil {
		t.Skipf("litellm not installed anywhere known: %v", err)
	}
	if filepath.Base(p) != "litellm" || !isExecFile(p) {
		t.Errorf("resolved non-exec/odd path: %q", p)
	}
	t.Logf("resolved litellm at %s", p)
}
