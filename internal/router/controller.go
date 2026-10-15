package router

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/halalgami/CodingAgentCommander/internal/proc"
)

// Controller runs a local LiteLLM proxy process.
type Controller struct {
	Port       int      // 0 => choose a free port on Start
	ConfigPath string   // path to a generated config.yaml
	Env        []string // extra env (e.g. provider keys) for the litellm process
	// LogPath receives litellm's stdout and stderr, truncated on each Start.
	// Empty means the null device. Either way litellm gets a VALID stdout: a
	// Windows GUI app has no std handles to pass on, and uvicorn's logging
	// setup calls sys.stdout.isatty(), so with stdout None litellm dies at
	// boot ("Unable to configure formatter 'default'") before binding its port.
	LogPath string
	mu      sync.Mutex
	cmd     *exec.Cmd
	running bool
	done    chan struct{} // closed once the process has exited
	waitErr error         // cmd.Wait's result; read only after done is closed
}

// HealthTimeout bounds one Health probe. A probe can take this long even when
// nothing is listening: Windows retries a refused loopback connect for ~2s.
const HealthTimeout = 2 * time.Second

// NewController returns a controller bound to port (0 = auto).
func NewController(port int) *Controller { return &Controller{Port: port} }

// isExecFile and platformLitellmCandidates are defined per-OS (litellm_unix.go,
// litellm_windows.go): executability and pip install layout differ between
// Unix (an exec permission bit; bin/ dirs) and Windows (PATHEXT at exec time;
// Scripts\*.exe).

// LitellmBin locates the litellm executable. GUI apps (and terminals without
// pip-user bins on PATH) can't find pip --user installs, so beyond PATH we probe
// the common install locations. Override with COMMANDER_LITELLM.
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
	for _, c := range platformLitellmCandidates(home) {
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
// No-op on an empty path.
//
// Every failure is swallowed: this runs on the startup and shutdown paths, and
// a machine that will not let us enumerate or kill processes is not a reason to
// refuse to boot. The mechanism is per-OS — see reap_unix.go / reap_windows.go.
func ReapStale(configPath string) {
	if configPath == "" {
		return
	}
	reapStale(configPath)
}

// Running reports whether the proxy process has been started, not stopped, and
// has not exited on its own. A proxy that crashed mid-session reports false, so
// callers restart it instead of handing sessions a dead port.
func (c *Controller) Running() bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	if !c.running {
		return false
	}
	select {
	case <-c.done:
		return false
	default:
		return true
	}
}

func freePort() (int, error) {
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return 0, err
	}
	defer l.Close()
	return l.Addr().(*net.TCPAddr).Port, nil
}

// litellmArgs builds the proxy's argv. Split out from Start so the bind address
// is assertable without spawning anything.
//
// --host is the load-bearing one. litellm's own default is 0.0.0.0
// (proxy_cli.py: `@click.option("--host", default="0.0.0.0")`), so omitting it
// publishes the proxy to every interface — and with it the Bedrock, Zen and
// Ollama credentials it holds, behind one bearer token, on whatever café or
// office Wi-Fi the laptop is on. /health/liveliness answers unauthenticated, so
// a port sweep finds it.
//
// The care taken in freePort() to pick the number on 127.0.0.1 was decorative
// without this, and Health()'s localhost probe meant the wider binding never
// showed up in normal use.
func litellmArgs(configPath string, port int) []string {
	return []string{
		"--config", configPath,
		"--host", "127.0.0.1",
		"--port", fmt.Sprintf("%d", port),
	}
}

// Start launches `litellm --config <ConfigPath> --host 127.0.0.1 --port <Port>`.
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
	logPath := c.LogPath
	if logPath == "" {
		logPath = os.DevNull
	}
	logFile, err := os.Create(logPath)
	if err != nil {
		return fmt.Errorf("open litellm log: %w", err)
	}
	// proc.Hide suppresses a console window on Windows (no-op elsewhere).
	cmd := proc.Hide(exec.Command(bin, litellmArgs(c.ConfigPath, c.Port)...))
	cmd.Env = pythonEnv(c.Env...)
	// Run from the config dir so the strip_thinking callback module (written
	// alongside the yaml) is importable by litellm.
	cmd.Dir = filepath.Dir(c.ConfigPath)
	cmd.Stdout = logFile
	cmd.Stderr = logFile
	if err := cmd.Start(); err != nil {
		_ = logFile.Close()
		return fmt.Errorf("start litellm: %w", err)
	}
	done := make(chan struct{})
	c.mu.Lock()
	c.cmd = cmd
	c.running = true
	c.done = done
	c.mu.Unlock()
	// The only Wait: it reaps the process, and closing done is how Running,
	// WaitHealthy and Stop learn it has exited.
	go func() {
		err := cmd.Wait()
		_ = logFile.Close()
		c.mu.Lock()
		c.waitErr = err
		c.mu.Unlock()
		close(done)
	}()
	return nil
}

// WaitHealthy polls Health until it passes, the process exits, ctx ends, or
// budget elapses, whichever comes first. The budget is wall-clock, not a probe
// count: a probe against a port nobody listens on can itself take HealthTimeout.
// An exit returns at once with the log tail, rather than polling a dead port
// until the budget runs out.
func (c *Controller) WaitHealthy(ctx context.Context, budget time.Duration) error {
	c.mu.Lock()
	done := c.done
	c.mu.Unlock()
	if done == nil {
		return errors.New("litellm is not started")
	}
	deadline := time.Now().Add(budget)
	for {
		select {
		case <-done:
			c.mu.Lock()
			werr := c.waitErr
			c.mu.Unlock()
			return fmt.Errorf("litellm exited during startup (%v)%s", werr, c.logTail())
		case <-ctx.Done():
			return ctx.Err()
		default:
		}
		if c.Health() == nil {
			return nil
		}
		if time.Now().After(deadline) {
			return fmt.Errorf("litellm did not answer within %s%s", budget, c.logTail())
		}
		select {
		case <-done: // handled at the top of the loop
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(200 * time.Millisecond):
		}
	}
}

// logTail returns the end of the log, formatted for appending to an error, or
// "" when there is no log to show.
func (c *Controller) logTail() string {
	if c.LogPath == "" {
		return ""
	}
	f, err := os.Open(c.LogPath)
	if err != nil {
		return ""
	}
	defer f.Close()
	const maxTail = 1500
	if fi, err := f.Stat(); err == nil && fi.Size() > maxTail {
		_, _ = f.Seek(-maxTail, io.SeekEnd)
	}
	b, _ := io.ReadAll(f)
	tail := strings.TrimSpace(string(b))
	if tail == "" {
		return fmt.Sprintf("; log: %s is empty", c.LogPath)
	}
	return fmt.Sprintf("; last output (%s):\n%s", c.LogPath, tail)
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
	client := http.Client{Timeout: HealthTimeout}
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
	done := c.done
	c.cmd = nil // idempotent: subsequent Stop() is a no-op
	c.running = false
	c.mu.Unlock()
	if cmd == nil || cmd.Process == nil {
		return nil
	}
	select {
	case <-done:
		return nil // already exited and reaped; nothing to kill
	default:
	}
	if err := cmd.Process.Kill(); err != nil && !errors.Is(err, os.ErrProcessDone) {
		// Kill races an exit the reaper is about to report: once Wait has
		// released the handle, Windows answers EINVAL, not ErrProcessDone.
		select {
		case <-done:
			return nil
		case <-time.After(time.Second):
			return err
		}
	}
	<-done // the Start goroutine reaps it; wait so no zombie outlives Stop
	return nil
}
