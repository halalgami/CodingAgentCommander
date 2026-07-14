# Bundling Commander for macOS

Researched 2026-07-14 (Wails v2.13, Xcode 26 era). Decision ladder by audience.

## Tier (a) — personal use, own Mac (current state)

Nothing needed. `wails build -platform darwin/arm64` output runs unsigned —
no quarantine xattr on locally built apps. Symlink into /Applications if
wanted. Update flow = `git pull && wails build`.

## Tier (b) — share with a few people (private repo)

Ad-hoc build + DMG + GitHub release; recipients bypass Gatekeeper once.

```bash
wails build -platform darwin/arm64 -clean
brew install create-dmg
create-dmg --volname "Commander" --app-drop-link 375 150 \
  build/bin/Commander.dmg build/bin/commander-gui.app
gh release create vX.Y.Z build/bin/Commander.dmg --title "vX.Y.Z"
```

Recipient (needs repo read access):
```bash
gh auth login
gh release download vX.Y.Z --repo halalgami/ClaudeCodeCommander -p "*.dmg"
# open DMG → drag to Applications → right-click → Open (once)
# or: xattr -cr /Applications/commander-gui.app
```

Homebrew cask + private repo = token plumbing Homebrew 5.1.14 partially
broke; `gh release download` replaces it for free. Skip the $99/yr
Developer ID until right-click-Open friction actually hurts.

Prereqs to document for recipients: tmux 3.2+, claude CLI, litellm on PATH.

## Tier (c) — public release

1. Apple Developer Program ($99/yr) → Developer ID Application cert.
2. `build/darwin/entitlements.plist` (doesn't exist yet — create):
   `com.apple.security.cs.allow-jit`,
   `com.apple.security.cs.allow-unsigned-executable-memory`,
   `com.apple.security.cs.disable-library-validation`.
   Do NOT enable app-sandbox — it breaks exec of PATH-discovered
   tmux/claude/litellm.
3. Sign (no `--deep`; hardened runtime mandatory for notarization):
   `codesign -f -s "Developer ID Application: NAME (TEAMID)" --options runtime --entitlements build/darwin/entitlements.plist --timestamp build/bin/commander-gui.app`
4. Notarize (`altool` is dead since 2023; Wails docs' `gon` is stale):
   `ditto -c -k --keepParent app zip` (plain zip can break signatures) →
   `xcrun notarytool submit --keychain-profile ... --wait` →
   `xcrun stapler staple` the .app → re-archive stapled app.
5. `create-dmg --codesign ... --notarize ...` supports the whole flow.
6. CI: GitHub Actions macOS ARM64 runner, secrets for cert + notary
   profile, `-platform darwin/universal` (Intel users now matter),
   `gh release create`.

## Repo gaps found by the research

- ~~`wails.json` has no `info` block~~ — fixed 2026-07-14 (productName
  Commander, version 0.9.0).
- `build/darwin/entitlements.plist` missing (needed only for tier c).
- ~~PATH probing gaps~~ — fixed 2026-07-14: `launch.AugmentPATH()` runs at
  startup (homebrew, pip-user globs, /etc/paths.d). Never write
  /etc/paths.d or launchctl setenv from the app.

## Auto-update

Wails has no built-in updater (wails#1178 open). For this project: skip.
Git-pull-rebuild (tier a) / `gh release download` (tier b). Cheapest
future middle ground: startup check of GitHub releases API + "new version"
toast linking to the release — no Sparkle/selfupdate machinery.

## Gotchas

- `CGO_LDFLAGS="-framework UniformTypeIdentifiers"` is compile-time only
  (Xcode 26 linker) — irrelevant to signing/notarizing.
- zip-transferred apps get Gatekeeper *translocation* (random /private
  path) until moved out — prefer DMG.
- `wails build` never signs anything; no `--sign` flag exists.
- Icon: `build/appicon.png` → auto .icns; stale icon = macOS icon cache
  (`killall Dock`).
