# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

Commander is a Wails v2 desktop app (Go backend, Svelte 5 frontend) that runs many Claude Code
sessions as tmux windows, each on a different model. macOS is the primary platform; Windows runs
natively via psmux (a native tmux for Windows) with no WSL.

## Build, test, run

macOS uses `make`; Windows has no `make`, so `build.ps1` mirrors the same targets.

| | macOS | Windows |
| --- | --- | --- |
| build | `make build` → `build/bin/commander-gui.app` | `.\build.ps1` → `build\bin\commander-gui.exe` |
| live reload | `make dev` | `.\build.ps1 dev` |
| test | `make test` | `.\build.ps1 test` |
| vet | `make vet` | `.\build.ps1 vet` |
| vet + test + build | — | `.\build.ps1 all` |

**Build before vet or test on a clean checkout.** `main.go` carries
`//go:embed all:frontend/dist` and `frontend/dist` is gitignored, so nothing in the module compiles
until a Wails build has run the frontend. Both workflows in `.github/workflows/` order the steps that
way for this reason; so should you.

Single test: `go test ./internal/tmux -run TestWindowTargetedCommandsHitTheRightWindow -v`

`make test` passes `-p 1` on Windows: the psmux-backed tests in `internal/tmux`, `internal/ptybridge`
and `internal/wsterm` drive one shared psmux server and wait on fixed timeouts for `pwsh`, so
concurrent packages flake. `CGO_LDFLAGS=-framework UniformTypeIdentifiers` is required on macOS for
Wails v2.13 to link under the Xcode 26 SDK — the Makefile sets it; don't drop it.

`make dist`/`release`/`install` package a macOS `.app` and refuse to run elsewhere.

## Architecture

**Two provider tiers, and the difference is the whole design.** `internal/launch` owns the contract:

- **Native Anthropic** — sets `ANTHROPIC_MODEL` only, and deliberately *omits* `ANTHROPIC_BASE_URL`
  so the subscription's OAuth (and Remote Control, which refuses a proxied base URL) keeps working.
- **Routed** (OpenCode Zen/Go, Bedrock) — sets `ANTHROPIC_BASE_URL` + `ANTHROPIC_AUTH_TOKEN` +
  `ANTHROPIC_MODEL` pointing at a local LiteLLM proxy that `internal/router` starts lazily and
  configures from the catalog. Keys are referenced as `os.environ/<KEY_ENV>` in the generated yaml,
  never inlined.

Omitting a variable is not the same as clearing one: `launch.EnvKeys()` is the full set Commander
manages, and every launch clears all of them before applying the ones it needs. A native launch that
merely omits `ANTHROPIC_BASE_URL` will inherit a routed launch's proxy and silently answer as the
wrong model.

**Sessions are tmux windows**, so they outlive the app. `internal/tmux` shells out to the `tmux`
binary (the psmux shim on Windows) — there is no library. `App.reconcile` adopts windows surviving a
previous run, reading per-window facts from `internal/winstate`, and prunes records whose windows are
gone.

**The terminal pane** is `internal/ptybridge` (a `tmux attach` inside a pty, or ConPTY on Windows)
streamed by `internal/wsterm` over a websocket to xterm.js. The server is loopback-only and gated by
a random per-run token; `/notify` on the same server receives Claude Code's Stop hook, which
`internal/hookmgr` installs into `~/.claude/settings.json` at startup and removes at shutdown.

**Frontend boundary:** exported methods on `App` are the entire binding surface; `frontend/wailsjs`
is generated, never hand-edited. Backend→frontend messages go through `wruntime.EventsEmit`:
`App.svelte` handles the app-wide ones (`session:finished`, `menu:about`, `models:updated`,
`app:error`), while `stores.svelte.js` subscribes to the scoped install streams
(`pwsh-install:*`, `litellm-install:*`) for as long as their modal is open.

**Where state lives**

| Path | What |
| --- | --- |
| `~/.config/commander/config.toml` | model catalog + providers (`COMMANDER_CONFIG` overrides) |
| `~/.config/commander/projects.json` | recent-project history |
| `~/.local/state/commander/windows.json` | per-window model + Remote Control flag |
| `~/.local/state/commander/litellm.yaml`, `strip_thinking.py` | generated proxy config and pre-call hook |
| OS keychain (`internal/secrets`) | provider API keys — never on disk |

## psmux is not tmux

The Windows shim accepts tmux's command surface and returns **rc=0 while doing something else**.
Three releases (v0.11.7–v0.11.9) fixed bugs that were all this. Measured on psmux 3.3.7:

| Command with `-t <window-id>` | psmux behaviour |
| --- | --- |
| `send-keys`, `capture-pane` | correct |
| `select-window` | drops the `@`, uses the rest as a window **index** |
| `kill-window` | resolves the id in **another session** that has it |
| `rename-window` | silently does nothing |
| `set-option -w` | applies to **every** window |
| `display-message -p -t` | ignores `-t`, reports the **active** window |
| `list-windows -a` | omits non-active windows of other sessions |
| `-e` on `new-window` | ignored entirely |
| `-e` on `new-session` | lands in the session env, inherited by every later window |

Consequences that are now load-bearing rules:

- **Target `session:index`, never a bare window id** — `tmux.WindowTarget` resolves it immediately
  before use and never caches, because indexes move when windows die. An unresolvable id is an
  error, not a fall-through to the raw id.
- **Per-window state goes in `internal/winstate`, never a tmux option.** That is the whole reason
  the package exists.
- **`Launch` stages env in the session environment and clears it again** around window creation,
  because `-e` alone reaches nothing on psmux.
- Window ids are **not unique across sessions**. When probing by hand, use a throwaway session — a
  bare `@id` can hit a live window in another session.

Assume any other tmux surface is suspect until probed, and check what actually happened rather than
the exit code.

## Conventions worth knowing

- **Persist, then commit.** Catalog mutations build a `next` copy, `config.Save` it, and only then
  assign to `a.cfg`, so a failed write never leaves memory and disk disagreeing.
- **The native model catalog is add-only.** `internal/anthropic` holds the built-in list; startup
  merges anything missing and stamps `anthropic_catalog_rev`, so a model the user renamed, repriced
  or deleted survives, and a deletion only reverses after a `CatalogRev` bump. Live `GET /v1/models`
  discovery is best-effort and reads `ANTHROPIC_API_KEY` from the environment only — Anthropic is not
  a config provider, so there is no keychain slot for it.
- New `Config` scalars must be declared **before** the `Models`/`Providers` slices: the encoder emits
  fields in declaration order, and a bare TOML key written after an array of tables belongs to that
  table, not the document.
- **Don't use `log`.** This is a GUI binary with no console; failures the user should know about go
  through `App.reportError`, which emits `app:error` and renders as a toast.
- Wrap any child process that can run on Windows in `proc.Hide`. Commander is a GUI app with no
  console, so an unwrapped console child flashes a window — and `internal/tmux` polls constantly.
  `Hide` is a no-op elsewhere, which is why the unix- and darwin-only files call `exec.Command`
  directly.

## CI and releases

`ci.yml` runs build/vet/test on `windows-latest` only, on purpose: every Windows-specific path
(psmux hosting, the pty bridge, the bundled tmux) is the part that actually breaks.

**Green CI is not evidence the session paths work.** The psmux-backed integration tests skip on the
runner — no psmux on `PATH`, no `pwsh` where they probe — so a passing run proves it compiles, vets,
and the non-session tests pass. Validate session-host changes by driving psmux directly first, then
encode the confirmed sequence in Go, and say in the PR that CI skips those tests.

`release.yml` fires only on a `v*` tag and publishes the exe with a GitHub-signed provenance
attestation — the point of building in CI rather than on a laptop. Cutting a release: merge to main,
bump the version in `build.ps1`, `Makefile` and `wails.json`, then tag. Never tag before the merge.
