Portable Windows build — no installer, no admin rights. Download
`commander-gui.exe` and run it.

## Verify what you downloaded

Both checks are quick and worth doing:

```powershell
# 1. Integrity — must match the published commander-gui.exe.sha256
(Get-FileHash .\commander-gui.exe -Algorithm SHA256).Hash.ToLower()

# 2. Provenance — proves GitHub built this exe from this repo's source
gh attestation verify .\commander-gui.exe --repo halalgami/CodingAgentCommander
```

The second check is the meaningful one. The checksum only proves the file
arrived intact; anyone able to swap the exe could swap the checksum beside it.
The attestation is signed by GitHub against the workflow run that built the
binary, so it ties this file to a specific commit of the public source.

It needs the [`gh` CLI](https://cli.github.com) **signed in** (`gh auth login`):
the CLI will not query the attestations API anonymously. The attestation is
itself public, though, so without an account you can download the bundle and
verify offline instead — see "Verify what you downloaded" in the README.

## SmartScreen will warn you

The exe is **not** code-signed, so Windows shows *"Windows protected your PC"*
on first run. That warning is expected and is not evidence of a problem — it is
what Windows shows for any binary without a paid Authenticode certificate.
Verify the two checks above, then **More info → Run anyway**.

## Prerequisites

Only one now — the [`claude` CLI](https://code.claude.com/docs)
(`npm install -g @anthropic-ai/claude-code`). It stays external because it is a
self-updating npm package that owns its own login; freezing a copy in here would
break its updater.

Everything else the exe handles itself:

- [psmux](https://github.com/psmux/psmux) 3.3.8 — **bundled**, extracted to
  `%AppData%\Commander\bin` on first launch. It hosts the sessions. A psmux
  already on your `PATH` still takes precedence.
- **PowerShell 7+** (`pwsh`) — **fetched on demand** if missing (~106 MB,
  checksum-pinned), because psmux's Claude Code integration needs it and the
  Windows PowerShell 5.1 that ships with Windows does not qualify.
- WebView2 — the bootstrapper is embedded; preinstalled on Windows 11 anyway.

A missing dependency is now named in a startup panel with the command that fixes
it, rather than surfacing as `exec: "tmux": executable file not found in %PATH%`
on your first launch. Nothing is written outside your user profile.

See the README for full setup.
