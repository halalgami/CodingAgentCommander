# ClaudeCodeCommander build helpers.
#
# Wails v2.13 on the Xcode 26 SDK fails to link without this flag (the linker
# can't find the UTType symbol from UniformTypeIdentifiers). Persisted here so
# every build/dev invocation carries it — do not remove until Wails ships a fix.
export CGO_LDFLAGS := -framework UniformTypeIdentifiers

WAILS ?= $(HOME)/go/bin/wails

VERSION ?= 0.9.2

.PHONY: build dev test vet dist release install

build:
	$(WAILS) build

dev:
	$(WAILS) dev

test:
	go test ./...

vet:
	go vet ./...

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
