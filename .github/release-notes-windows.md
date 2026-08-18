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

## SmartScreen will warn you

The exe is **not** code-signed, so Windows shows *"Windows protected your PC"*
on first run. That warning is expected and is not evidence of a problem — it is
what Windows shows for any binary without a paid Authenticode certificate.
Verify the two checks above, then **More info → Run anyway**.

## Prerequisites

The exe itself is self-contained, but Commander drives other tools:

- [psmux](https://github.com/psmux/psmux) ≥ 3.3 — hosts the sessions
- **PowerShell 7+** (`pwsh`) — not the bundled Windows PowerShell 5.1
- the [`claude` CLI](https://code.claude.com/docs)
- WebView2 — preinstalled on Windows 11; the installer is bundled for Windows 10

See the README for full setup.
