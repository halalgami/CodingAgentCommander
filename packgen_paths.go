package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// Pack naming and the staging-to-live promotion.
//
// A generated pack's folder name is derived from a name the user types, so it
// is untrusted input that becomes a filesystem path. It is whitelisted rather
// than blacklisted: everything outside [a-z0-9-] is replaced, which makes
// "../../etc" and "C:\Windows" and a NUL byte all the same uninteresting case
// instead of three separate defences to get right.

const (
	// packGenStagingSuffix and packGenOldSuffix are appended to a slug to name
	// the two transient folders. Because a slug can never contain a dot (see
	// packGenSlug), neither suffixed name can ever collide with another pack's
	// folder — the test asserts this, because it is the property that lets
	// promotion rename freely inside the packs directory.
	packGenStagingSuffix = ".staging"
	packGenOldSuffix     = ".old"

	// packGenMaxSlug leaves headroom under Windows' default 260-character path
	// limit once the packs directory, a ".staging" suffix and a file name are
	// all added.
	packGenMaxSlug = 48
)

// packGenReservedNames are Windows device names. They are reserved with OR
// without an extension, and CreateFile on them opens the device rather than a
// file — so a pack called "con" would fail in a way that looks like corruption.
// Go's os package does not shield us from this.
var packGenReservedNames = map[string]bool{
	"con": true, "prn": true, "aux": true, "nul": true,
	"com0": true, "com1": true, "com2": true, "com3": true, "com4": true,
	"com5": true, "com6": true, "com7": true, "com8": true, "com9": true,
	"lpt0": true, "lpt1": true, "lpt2": true, "lpt3": true, "lpt4": true,
	"lpt5": true, "lpt6": true, "lpt7": true, "lpt8": true, "lpt9": true,
}

// packGenSlug turns a display name into a folder name.
//
// The result is always safe as a single path segment on every platform we
// ship: lowercase ASCII, no separators, no dots, no spaces, no device names,
// bounded length. It is deliberately lossy — a name that is entirely non-ASCII
// is rejected with an explanation rather than silently becoming "pack-1",
// because a user who types a name in their own script deserves to be told we
// cannot use it, not to find a folder they do not recognise.
func packGenSlug(display string) (string, error) {
	var b strings.Builder
	for _, r := range strings.ToLower(strings.TrimSpace(display)) {
		switch {
		case r >= 'a' && r <= 'z', r >= '0' && r <= '9':
			b.WriteRune(r)
		default:
			// Everything else — separators, dots, spaces, punctuation, control
			// characters, and any non-ASCII rune — becomes a hyphen, which the
			// collapse below then folds away.
			b.WriteByte('-')
		}
	}
	slug := strings.Trim(b.String(), "-")
	for strings.Contains(slug, "--") {
		slug = strings.ReplaceAll(slug, "--", "-")
	}
	if slug == "" {
		return "", fmt.Errorf("that name has no letters or digits we can use in a folder name; please use A-Z or 0-9")
	}
	if len(slug) > packGenMaxSlug {
		slug = strings.Trim(slug[:packGenMaxSlug], "-")
	}
	if packGenReservedNames[slug] {
		// Suffixing keeps the user's word visible; rejecting would be baffling
		// on macOS, where "con" is a perfectly ordinary folder name.
		slug += "-pack"
	}
	return slug, nil
}

// packGenPaths is the set of folders one run touches.
type packGenPaths struct {
	Slug    string
	Live    string // packs/<slug>          — only written by promotion
	Staging string // packs/<slug>.staging  — everything a run produces
	Old     string // packs/<slug>.old      — exists only during promotion
}

func packGenPathsFor(display string) (packGenPaths, error) {
	slug, err := packGenSlug(display)
	if err != nil {
		return packGenPaths{}, err
	}
	root := companionPacksDir()
	return packGenPaths{
		Slug:    slug,
		Live:    filepath.Join(root, slug),
		Staging: filepath.Join(root, slug+packGenStagingSuffix),
		Old:     filepath.Join(root, slug+packGenOldSuffix),
	}, nil
}

// promoteRename is a seam. The restore branch below only runs when a rename
// fails after a backup was taken, and there is no portable way to make the
// filesystem refuse that specific rename from a test — every trick that blocks
// it (read-only parent, occupied destination) also blocks the restore, so the
// test would pass while proving nothing. A swappable rename lets the test drive
// exactly the failure it is about.
var promoteRename = os.Rename

func packGenIsDir(p string) bool {
	fi, err := os.Stat(p)
	return err == nil && fi.IsDir()
}

// promotePack moves a finished staging folder into place.
//
// os.Rename is atomic on POSIX but fails over an existing directory on Windows,
// and this app ships on Windows — hence the three-step dance rather than one
// rename. The ordering is chosen so that the live pack is never absent for
// longer than a single rename, and so that a crash at any point leaves either
// the old pack or the new one, never a half-merged mixture of both.
//
// The final RemoveAll is unconditional on purpose: it also collects a stale
// .old left behind by an earlier crash, which the guarded branch above would
// otherwise skip forever once .live went missing.
func promotePack(paths packGenPaths) error {
	// RECOVER FIRST. A crash between the two renames below leaves Live gone and
	// .old holding the user's only copy — and from there the library hides it
	// (ListCompanionPacks skips .old), regeneration refuses it, and the next
	// promotion's unconditional cleanup DELETES it. Putting it back before
	// anything else turns the crash window from "loses the pack" into "loses
	// the promotion".
	if !packGenIsDir(paths.Live) && packGenIsDir(paths.Old) {
		if err := promoteRename(paths.Old, paths.Live); err != nil {
			return fmt.Errorf("recovering the pack left behind by an interrupted save: %w", err)
		}
	}
	if !packGenIsDir(paths.Staging) {
		return fmt.Errorf("nothing staged to save at %s", paths.Staging)
	}
	if packGenIsDir(paths.Live) {
		if err := os.RemoveAll(paths.Old); err != nil {
			return fmt.Errorf("clearing the previous backup: %w", err)
		}
		if err := promoteRename(paths.Live, paths.Old); err != nil {
			return fmt.Errorf("setting the existing pack aside: %w", err)
		}
	}
	if err := promoteRename(paths.Staging, paths.Live); err != nil {
		// Put the old pack back rather than leaving the user with no pack at
		// all. Best effort: if this also fails the original error is the one
		// worth reporting, and the art is still on disk under .old.
		if packGenIsDir(paths.Old) {
			_ = promoteRename(paths.Old, paths.Live)
		}
		return fmt.Errorf("moving the new pack into place: %w", err)
	}
	// The new pack is live from here on. A failure to delete the backup is
	// untidy, not a failure of the save, so it is not returned.
	_ = os.RemoveAll(paths.Old)
	return nil
}

// packGenPathsAt builds the promote/staging trio for a pack that already exists
// at dir, rather than deriving them from a display name.
//
// Regeneration must address the pack by PATH: the folder is slug(name) only for
// packs this app generated. An imported pack keeps the manifest's name while
// taking its folder from the archive's filename, so name-addressing resolved to
// a different pack and rewrote that one instead.
//
// Containment is enforced here rather than trusted from the caller — this value
// is handed to promotePack, which renames and deletes.
func packGenPathsAt(dir string) (packGenPaths, error) {
	if strings.TrimSpace(dir) == "" {
		return packGenPaths{}, fmt.Errorf("no pack folder given")
	}
	root := companionPacksDir()
	realRoot, err := filepath.EvalSymlinks(root)
	if err != nil {
		return packGenPaths{}, fmt.Errorf("the packs folder could not be resolved: %w", err)
	}
	real, err := filepath.EvalSymlinks(dir)
	if err != nil {
		return packGenPaths{}, fmt.Errorf("that pack no longer exists: %w", err)
	}
	if filepath.Dir(real) != realRoot {
		return packGenPaths{}, fmt.Errorf("%s is not a pack in %s", dir, root)
	}
	slug := filepath.Base(real)
	return packGenPaths{
		Slug:    slug,
		Live:    filepath.Join(realRoot, slug),
		Staging: filepath.Join(realRoot, slug+packGenStagingSuffix),
		Old:     filepath.Join(realRoot, slug+packGenOldSuffix),
	}, nil
}
