<#
.SYNOPSIS
    Windows build helper for Commander — the Makefile equivalent.

.DESCRIPTION
    The Makefile targets the macOS toolchain and assumes `make`, which Windows
    does not ship. This script mirrors the build / dev / test / vet targets
    natively so a Windows checkout needs no extra tooling.

.PARAMETER Target
    build (default) — production build to build\bin\commander-gui.exe
    dev             — live-reload dev mode
    test            — Go suite (serial; see -p 1 note below)
    vet             — go vet
    all             — vet, then test, then build
    release         — portable exe for distribution, plus its SHA256 file

.EXAMPLE
    .\build.ps1
    .\build.ps1 test
#>
[CmdletBinding()]
param(
    [ValidateSet('build', 'dev', 'test', 'vet', 'all', 'release')]
    [string]$Target = 'build',

    [string]$Version = '0.11.9'
)

$ErrorActionPreference = 'Stop'
Set-Location -Path $PSScriptRoot

# Commander shells out to tmux (the psmux shim), claude and litellm. Those live
# in per-user tool dirs that are on the interactive PATH but easy to miss from a
# bare shell, so make them resolvable here too.
$extra = @(
    (Join-Path $env:LOCALAPPDATA 'Programs\Go\bin'),
    (Join-Path $env:USERPROFILE 'go\bin'),
    (Join-Path $env:USERPROFILE '.local\bin'),
    (Join-Path $env:LOCALAPPDATA 'Programs\PowerShell\7')
) | Where-Object { Test-Path $_ }
foreach ($p in $extra) {
    if (($env:Path -split ';') -notcontains $p) { $env:Path = "$p;$env:Path" }
}

# The psmux release the Windows exe embeds, pinned by content hash. Bundling it
# is what stops a release being defeated by a package manager that reports psmux
# as installed while its `tmux` shim is missing or unregistered — the exact
# failure this mechanism exists to prevent.
#
# To bump: change both values here together with PsmuxVersion in
# internal/deps/psmux_bundled.go. Ensure-PsmuxAsset fails the build on drift.
$psmuxVersion = '3.3.8'
$psmuxSha256 = '1ad127ba937194a890b933a73d9b023e297bd73dc742abd841bf159984c2effe'

# Built with the `bundled` tag so internal/deps embeds the psmux binary. Every
# target passes it, vet and test included: code behind a build tag is invisible
# to a toolchain that does not pass the tag, so omitting it from the gates would
# mean shipping a path nothing ever type-checked.
$buildTags = 'bundled'

function Ensure-PsmuxAsset {
    $assetDir = Join-Path $PSScriptRoot 'internal\deps\assets'
    $exe = Join-Path $assetDir 'tmux.exe'
    $lic = Join-Path $assetDir 'psmux-LICENSE'

    # Keep this pin and the version baked into the Go const from drifting apart:
    # the extracted binary would otherwise be installed into a directory named
    # for a version it is not.
    $goSrc = Join-Path $PSScriptRoot 'internal\deps\psmux_bundled.go'
    $m = Select-String -Path $goSrc -Pattern 'PsmuxVersion\s*=\s*"([^"]+)"' | Select-Object -First 1
    if (-not $m) { throw "could not find PsmuxVersion in $goSrc" }
    $declared = $m.Matches[0].Groups[1].Value
    if ($declared -ne $psmuxVersion) {
        throw "psmux version mismatch: build.ps1 pins $psmuxVersion, $goSrc declares $declared"
    }

    # The asset is gitignored and cached, so this downloads once per version per
    # working tree rather than on every build.
    if ((Test-Path $exe) -and (Test-Path $lic)) { return }

    New-Item -ItemType Directory -Force $assetDir | Out-Null
    $zipName = "psmux-v$psmuxVersion-windows-x64.zip"
    $url = "https://github.com/psmux/psmux/releases/download/v$psmuxVersion/$zipName"
    $tmpDir = Join-Path ([IO.Path]::GetTempPath()) "commander-psmux-$psmuxVersion"
    New-Item -ItemType Directory -Force $tmpDir | Out-Null
    $zip = Join-Path $tmpDir $zipName

    Write-Host "==> fetch $zipName" -ForegroundColor Cyan
    Invoke-WebRequest -Uri $url -OutFile $zip -UseBasicParsing

    $got = (Get-FileHash -Path $zip -Algorithm SHA256).Hash.ToLower()
    if ($got -ne $psmuxSha256) {
        Remove-Item $zip -Force
        throw "psmux checksum mismatch for ${zipName}: got $got, expected $psmuxSha256"
    }

    $unpack = Join-Path $tmpDir 'unpacked'
    if (Test-Path $unpack) { Remove-Item $unpack -Recurse -Force }
    Expand-Archive -Path $zip -DestinationPath $unpack -Force

    # tmux.exe is the shim Commander execs; psmux.exe and pmux.exe in the archive
    # are byte-identical copies under other names, so only one gets embedded.
    Copy-Item (Join-Path $unpack 'tmux.exe') $exe -Force
    Copy-Item (Join-Path $unpack 'LICENSE') $lic -Force
    Write-Host "    psmux $psmuxVersion staged for embedding" -ForegroundColor Green
}

function Resolve-Tool {
    param([string]$Name, [string[]]$Candidates, [string]$Hint)
    $cmd = Get-Command $Name -ErrorAction SilentlyContinue
    if ($cmd) { return $cmd.Source }
    foreach ($c in $Candidates) { if (Test-Path $c) { return $c } }
    throw "$Name not found on PATH. $Hint"
}

$go = Resolve-Tool -Name 'go' `
    -Candidates @((Join-Path $env:LOCALAPPDATA 'Programs\Go\bin\go.exe'), 'C:\Program Files\Go\bin\go.exe') `
    -Hint 'Install Go 1.25+ from https://go.dev/dl/'

$wails = Resolve-Tool -Name 'wails' `
    -Candidates @((Join-Path $env:USERPROFILE 'go\bin\wails.exe')) `
    -Hint 'Install with: go install github.com/wailsapp/wails/v2/cmd/wails@v2.13.0'

$commit = (& git rev-parse --short HEAD 2>$null)
if (-not $commit) { $commit = 'unknown' }
$buildDate = (Get-Date).ToUniversalTime().ToString('yyyy-MM-dd')
$ldflags = "-X main.appVersion=$Version -X main.appCommit=$commit -X main.appBuildDate=$buildDate"

# NOTE: no CGO_LDFLAGS here. The Makefile exports
# `-framework UniformTypeIdentifiers` for the Xcode 26 linker bug; that is an
# Apple framework flag and passing it to the MinGW linker fails the build.

function Invoke-Step {
    param([string]$Label, [scriptblock]$Body)
    Write-Host "==> $Label" -ForegroundColor Cyan
    & $Body
    if ($LASTEXITCODE -ne 0) { throw "$Label failed (exit $LASTEXITCODE)" }
}

# The psmux-backed integration tests (tmux, ptybridge, wsterm) share one psmux
# server and wait on fixed timeouts for pwsh to start; running those packages
# concurrently blows the budget and flakes. -p 1 keeps them serial.
$testArgs = @('test', './...', '-p', '1', '-tags', $buildTags)

# Every target compiles the embedding path, so every target needs the asset.
Ensure-PsmuxAsset

switch ($Target) {
    'build' { Invoke-Step 'wails build' { & $wails build -tags $buildTags -ldflags $ldflags } }
    'dev' { Invoke-Step 'wails dev' { & $wails dev -tags $buildTags } }
    'test' { Invoke-Step 'go test' { & $go @testArgs } }
    'vet' { Invoke-Step 'go vet' { & $go vet -tags $buildTags ./... } }
    'all' {
        Invoke-Step 'go vet' { & $go vet -tags $buildTags ./... }
        Invoke-Step 'go test' { & $go @testArgs }
        Invoke-Step 'wails build' { & $wails build -tags $buildTags -ldflags $ldflags }
    }
    'release' {
        # -webview2 embed bundles Microsoft's WebView2 bootstrapper (~150 KB).
        # Without it the default 'download' strategy sends a user whose machine
        # lacks the runtime off to a browser mid-launch; Windows 11 always has
        # it, Windows 10 often does not.
        #
        # Deliberately NOT using -upx: compressed binaries match packer
        # heuristics, and antivirus false positives are the main thing that
        # stops people running an unsigned download.
        # -tags bundled embeds the hash-pinned psmux staged by Ensure-PsmuxAsset,
        # so the exe can host sessions on a machine that has never installed it.
        Invoke-Step 'wails build (release)' {
            & $wails build -clean -webview2 embed -tags $buildTags -ldflags $ldflags
        }

        $exe = Join-Path $PSScriptRoot 'build\bin\commander-gui.exe'
        if (-not (Test-Path $exe)) { throw "release build produced no exe at $exe" }

        # Ship the hash next to the binary so a downloader can prove the file
        # arrived intact. This is integrity only, not authenticity — anyone who
        # could swap the exe could swap the .sha256 beside it. Provenance comes
        # from the GitHub attestation (see .github/workflows/release.yml).
        $hash = (Get-FileHash -Path $exe -Algorithm SHA256).Hash.ToLower()
        $sums = "$exe.sha256"
        "$hash  commander-gui.exe" | Set-Content -Path $sums -Encoding ascii
        Write-Host "SHA256  $hash" -ForegroundColor Green
        Write-Host "Wrote   $sums" -ForegroundColor Green
    }
}

if ($Target -in @('build', 'all', 'release')) {
    $exe = Join-Path $PSScriptRoot 'build\bin\commander-gui.exe'
    if (Test-Path $exe) {
        Write-Host "Built $exe" -ForegroundColor Green
    }
}
