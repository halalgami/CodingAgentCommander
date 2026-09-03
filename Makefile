# ClaudeCodeCommander build helpers.
#
# Host detection: Windows has no uname, but sets OS=Windows_NT.
ifeq ($(OS),Windows_NT)
  HOST_OS := windows
else
  HOST_OS := $(shell uname -s)
endif

ifeq ($(HOST_OS),Darwin)
# Wails v2.13 on the Xcode 26 SDK fails to link without this flag (the linker
# can't find the UTType symbol from UniformTypeIdentifiers). Persisted here so
# every build/dev invocation carries it — do not remove until Wails ships a fix.
# Scoped to macOS: UniformTypeIdentifiers is an Apple framework, and handing
# -framework to the MinGW or GNU linker fails the build outright.
export CGO_LDFLAGS := -framework UniformTypeIdentifiers
endif

ifeq ($(HOST_OS),windows)
  WAILS ?= $(USERPROFILE)\go\bin\wails.exe
  BUILD_DATE ?= $(shell powershell -NoProfile -Command "(Get-Date).ToUniversalTime().ToString('yyyy-MM-dd')")
  # The psmux-backed integration tests (tmux, ptybridge, wsterm) drive one
  # shared psmux server and wait on fixed timeouts for pwsh to start. Running
  # those packages concurrently blows the timeout budget and flakes, so keep
  # package execution serial on Windows.
  TESTFLAGS ?= -p 1
else
  WAILS ?= $(HOME)/go/bin/wails
  BUILD_DATE ?= $(shell date -u +%Y-%m-%d)
  TESTFLAGS ?=
endif

VERSION ?= 0.12.1
COMMIT ?= $(shell git rev-parse --short HEAD)
LDFLAGS := -X main.appVersion=$(VERSION) -X main.appCommit=$(COMMIT) -X main.appBuildDate=$(BUILD_DATE)

# dist/release/install package a macOS .app bundle (create-dmg, ditto,
# /Applications) and cannot work anywhere else. Refuse early, and only when one
# of them is actually the requested goal — a bare top-level $(error) would abort
# every target, including `build`, on a non-Darwin host. A recipe-level shell
# guard is no good either: make drives recipes through cmd.exe on Windows, where
# `[ ... ]` is not a command.
ifneq ($(HOST_OS),Darwin)
ifneq ($(filter dist release install,$(MAKECMDGOALS)),)
$(error '$(MAKECMDGOALS)' packages a macOS .app and only runs on macOS. On Windows build with .\build.ps1 — the exe lands in build\bin\commander-gui.exe)
endif
endif

.PHONY: build dev test vet check dist release install

build:
	$(WAILS) build -ldflags "$(LDFLAGS)"

dev:
	$(WAILS) dev

test:
	go test ./... $(TESTFLAGS)

vet:
	go vet ./...

# Everything, including the browser specs. CI deliberately does NOT run
# Playwright: the runner is Windows and would have to install browsers on every
# job, for specs that are a developer feedback loop rather than a release gate.
# So this is the pre-merge check to run locally.
#
# It matters more than a normal smoke run: two of those specs are the SOLE
# verification of correctness invariants rather than UI behaviour — that
# toggling optional sidebar content causes no pty resize or xterm change, and
# that the terminal's activity registration does not leak across session
# switches. Neither has any other coverage.
check: vet test
	cd frontend && npm test
	cd frontend && npx playwright test

# Tier-(b) distribution (docs/BUNDLING_MACOS.md): ad-hoc build -> DMG.
# Recipients right-click -> Open once (unsigned) or `xattr -cr` the app.
dist: build
	rm -f build/bin/Commander.dmg
	create-dmg --volname "Commander" --app-drop-link 375 150 \
		build/bin/Commander.dmg build/bin/commander-gui.app

release: dist
	gh release create v$(VERSION) build/bin/Commander.dmg \
		--repo halalgami/CodingAgentCommander \
		--title "Commander v$(VERSION)" --generate-notes

# Build and install into /Applications as Commander.app. ditto preserves
# bundle structure/xattrs (plain cp can mangle .app bundles).
install: build
	rm -rf /Applications/Commander.app
	ditto build/bin/commander-gui.app /Applications/Commander.app
	@echo "Installed /Applications/Commander.app"
