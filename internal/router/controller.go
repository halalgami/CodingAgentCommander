package router

import (
	"errors"
	"fmt"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"sync"
	"time"
)

// Controller runs a local LiteLLM proxy process.
type Controller struct {
	Port       int      // 0 => choose a free port on Start
	ConfigPath string   // path to a generated config.yaml
	Env        []string // extra env (e.g. provider keys) for the litellm process
	mu         sync.Mutex
	cmd        *exec.Cmd
	running    bool
}

// NewController returns a controller bound to port (0 = auto).
func NewController(port int) *Controller { return &Controller{Port: port} }

func isExecFile(p string) bool {
	fi, err := os.Stat(p)
	return err == nil && !fi.IsDir() && fi.Mode()&0o111 != 0
}

// LitellmBin locates the litellm executable. macOS GUI apps (and terminals
// without pip-user bins on PATH) can't find pip --user installs, so beyond
// PATH we probe the common install locations. Override with COMMANDER_LITELLM.
func LitellmBin() (string, error) {
	if p := os.Getenv("COMMANDER_LITELLM"); p != "" {
		if isExecFile(p) {
			return p, nil
		}
		return "", fmt.Errorf("COMMANDER_LITELLM=%s is not an executable file", p)
	}
	if p, err := exec.LookPath("litellm"); err == nil {
		return p, nil
	}
	home, _ := os.UserHomeDir()
	var candidates []string
	// pip --user on macOS: ~/Library/Python/<X.Y>/bin/litellm
	matches, _ := filepath.Glob(filepath.Join(home, "Library", "Python", "*", "bin", "litellm"))
	candidates = append(candidates, matches...)
	candidates = append(candidates,
		filepath.Join(home, ".local", "bin", "litellm"), // pip --user on Linux/mac
		"/opt/homebrew/bin/litellm",
		"/usr/local/bin/litellm",
	)
	for _, c := range candidates {
		if isExecFile(c) {
			return c, nil
		}
	}
	return "", fmt.Errorf("litellm not found on PATH or common locations; install with `pip install 'litellm[proxy]'` or set COMMANDER_LITELLM=/path/to/litellm")
}

// ReapStale best-effort kills any litellm process whose command line references
// configPath — an orphan left when the GUI exited uncleanly (force-quit/crash)
// so its OnShutdown never ran to Stop() the child. It matches on the --config
// argument, so it never touches an unrelated litellm serving a different config.
// No-op on an empty path or where pkill is absent (e.g. Windows).
func ReapStale(configPath string) {
	if configPath == "" {
		return
	}
	_ = exec.Command("pkill", "-f", configPath).Run()
}

// Running reports whether the proxy process has been started and not stopped.
func (c *Controller) Running() bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.running
}

func freePort() (int, error) {
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return 0, err
	}
	defer l.Close()
	return l.Addr().(*net.TCPAddr).Port, nil
}

// Start launches `litellm --config <ConfigPath> --port <Port>`.
func (c *Controller) Start() error {
	if c.Port == 0 {
		p, err := freePort()
		if err != nil {
			return fmt.Errorf("pick port: %w", err)
		}
		c.Port = p
	}
	bin, err := LitellmBin()
	if err != nil {
		return err
	}
	cmd := exec.Command(bin, "--config", c.ConfigPath, "--port", fmt.Sprintf("%d", c.Port))
	cmd.Env = append(os.Environ(), c.Env...)
	// Run from the config dir so the strip_thinking callback module (written
	// alongside the yaml) is importable by litellm.
	cmd.Dir = filepath.Dir(c.ConfigPath)
	cmd.Stdout = os.Stderr // litellm logs to our stderr; keeps output visible
	cmd.Stderr = os.Stderr
	if err := cmd.Start(); err != nil {
		return fmt.Errorf("start litellm: %w", err)
	}
	c.mu.Lock()
	c.cmd = cmd
	c.running = true
	c.mu.Unlock()
	return nil
}

// Health returns nil when the proxy answers /health/liveliness with 2xx.
//
// LiteLLM's /health endpoint requires the master_key (it performs an
// authenticated deep health check against configured upstreams) and returns
// 401 without it; /health/liveliness is the unauthenticated liveness probe
// and returns 200 as soon as the proxy process is up. We use the latter
// since Controller has no way to pass the master_key here and only needs to
// know the process is alive and serving.
func (c *Controller) Health() error {
	client := http.Client{Timeout: 2 * time.Second}
	resp, err := client.Get(fmt.Sprintf("http://localhost:%d/health/liveliness", c.Port))
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 300 {
		return fmt.Errorf("health status %d", resp.StatusCode)
	}
	return nil
}

// Stop terminates the proxy process and reaps it so it does not linger as a
// zombie. It is idempotent: a second call (or a call after the process already
// exited) returns nil.
func (c *Controller) Stop() error {
	c.mu.Lock()
	cmd := c.cmd
	c.cmd = nil // idempotent: subsequent Stop() is a no-op
	c.running = false
	c.mu.Unlock()
	if cmd == nil || cmd.Process == nil {
		return nil
	}
	if err := cmd.Process.Kill(); err != nil && !errors.Is(err, os.ErrProcessDone) {
		return err
	}
	_ = cmd.Wait() // reap the process; ignore the "signal: killed" exit error
	return nil
}
