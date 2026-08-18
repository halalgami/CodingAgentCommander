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

.EXAMPLE
    .\build.ps1
    .\build.ps1 test
#>
[CmdletBinding()]
param(
    [ValidateSet('build', 'dev', 'test', 'vet', 'all')]
    [string]$Target = 'build',

    [string]$Version = '0.11.3'
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
$testArgs = @('test', './...', '-p', '1')

switch ($Target) {
    'build' { Invoke-Step 'wails build' { & $wails build -ldflags $ldflags } }
    'dev' { Invoke-Step 'wails dev' { & $wails dev } }
    'test' { Invoke-Step 'go test' { & $go @testArgs } }
    'vet' { Invoke-Step 'go vet' { & $go vet ./... } }
    'all' {
        Invoke-Step 'go vet' { & $go vet ./... }
        Invoke-Step 'go test' { & $go @testArgs }
        Invoke-Step 'wails build' { & $wails build -ldflags $ldflags }
    }
}

if ($Target -in @('build', 'all')) {
    $exe = Join-Path $PSScriptRoot 'build\bin\commander-gui.exe'
    if (Test-Path $exe) {
        Write-Host "Built $exe" -ForegroundColor Green
    }
}
