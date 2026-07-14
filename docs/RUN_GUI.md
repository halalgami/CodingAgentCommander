# Running the GUI (manual visual verification)

The automated suite proves the backend runtime path (launch -> tmux -> pty
bridge -> websocket) works end-to-end (see `e2e_test.go`, gated behind
`COMMANDER_E2E=1`). It cannot verify that the Wails window actually renders
and is interactive — that needs a human. Follow these steps:

1. **Install the config.**

   ```sh
   mkdir -p ~/.config/commander
   cp example.config.toml ~/.config/commander/config.toml
   ```

   Edit `~/.config/commander/config.toml` if needed (e.g. adjust
   `tmux_session`, model list, or pricing).

2. **Launch the app.**

   Either run it in dev mode (hot reload, opens a window automatically):

   ```sh
   make dev
   ```

   or build the production app bundle and open it:

   ```sh
   make build
   open build/bin/commander-gui.app
   ```

3. **Verify the picker.** The window should show a project-folder input and
   a model dropdown populated with the native (non-routed) models from your
   config (e.g. `claude-opus-4-8`, `claude-sonnet-5`).

4. **Type a project folder.** Enter an absolute path to a directory that
   exists on disk (e.g. your home directory or a scratch folder).

5. **Pick a model** from the dropdown.

6. **Click Launch.** Confirm:
   - A new entry appears in the sidebar list of sessions.
   - The right-hand pane renders a live `claude` TUI session (the real
     `claude` CLI running inside a tmux window, streamed over the
     websocket).
   - Typing in the pane is echoed and drives the running `claude` session
     (you should be able to move the cursor / interact with its UI; you do
     not need to send a prompt or consume tokens to confirm this — basic
     keystroke echo/interaction is enough).

7. **Click a sidebar session to reselect it.** Launch a second session (or
   use the one you already have), click back and forth between sidebar
   entries, and confirm the right-hand pane switches to show the selected
   session's live content each time.

8. **Clean up.** Quit the app, then remove any leftover tmux session it
   created:

   ```sh
   tmux kill-session -t commander 2>/dev/null; tmux ls 2>/dev/null || true
   ```

## Routed models (Zen / non-Anthropic) — B2b

Routed models run through a local LiteLLM proxy Commander starts on demand.

1. Add your routed models to `~/.config/commander/config.toml`, e.g. a Zen model:
   ```toml
   [[models]]
   id = "gpt-5.5"
   label = "Zen · GPT-5.5"
   provider = "opencode-go"
   upstream = "openai/gpt-5.5"            # LiteLLM model string
   api_base = "https://opencode.ai/zen/v1"
   key_env = "ZEN_KEY"                    # keychain slot name
   input_price = 1.25
   output_price = 10.0
   ```
   (Zen Claude/Qwen models use `upstream = "anthropic/<id>"` + `api_base = "https://opencode.ai/zen"`.)
2. `make dev` → open **Providers** → paste your Zen API key next to `ZEN_KEY` → **Save**.
3. The routed model in the dropdown loses its "(needs key)" tag. Pick it → **Launch**.
4. Commander starts LiteLLM (once), points the session at it, and streams the live routed session in the pane.

Notes: changing a key takes effect on the next `make dev` (LiteLLM reads keys at start). LiteLLM is stopped when you quit the app.

## Swap a session's model (B4)

Each session card has a **swap** dropdown. Pick a different model → Commander
kills the window and relaunches `claude --resume <id>` under the new model, in
the same folder. Same conversation, new brain (brief reload). Routed models are
pre-flighted (key + LiteLLM) before the old window is killed, so a failed swap
never loses the session. If the session has no saved conversation yet, swap
starts a fresh session on the new model.

## Settings, palette, remote control (2026-07-14 sprints)

- **Settings drawer** (sidebar footer → Settings): accent color, terminal font
  size / scrollback / width cap, UI scale, remote-control-on-launch toggle.
  Sidebar width: drag the divider. Font size also via ⌘= / ⌘- / ⌘0.
- **⌘K palette**: jump to sessions, relaunch recent folder+model pairs, swap
  the focused session's model, open any drawer, hand off to phone.
- **Remote Control** (native Anthropic sessions only): 📱 on a session card
  types `/remote-control <name>` into the session — scan the QR the terminal
  prints with the Claude mobile app. RC survives native→native swaps; swapping
  to a routed model drops it (toast tells you). Or toggle "Enable remote
  control on launch" in Settings.
