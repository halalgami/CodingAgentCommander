package raidpack

import (
	"archive/zip"
	"bytes"
	"encoding/json"
	"image"
	"image/png"
	"io"
	"io/fs"
	"os"
	"regexp"
	"strings"
)

var hashed = regexp.MustCompile(`^(.+)-[A-Za-z0-9_-]{8}\.webp$`)

const readme = `Commander — Raid heroine pack (starter kit)

Replace any image with your own, keep the file name, then in Commander:
Settings → Raid heroines → Import zip… (zip this folder's files, not the folder).

Rules
- Full body, facing right, transparent PNG looks best (JPEG/WebP also accepted).
- Any size up to 8000 px a side; Commander scales to 840 px tall.
- A heroine is fully yours once her <id>-stance image is in the pack: her
  missing poses then use your stance, never the built-in art.
- manifest.json: optional "name", and per heroine "name" (≤ 24) and "tint" (#rrggbb).
- heroine-prompts.csv has the prompts the built-in art was made from.

Heroines: opus Archmage, sonnet Spellblade, haiku Ninja, glm Gunslinger,
qwen Priestess, kimi Mecha Pilot.
Poses: stance (working), attack (running a tool), hurt (error),
victory (finished), await (needs you).
Optional: <id>-v1-<pose>, <id>-v2-<pose> (alternate looks),
<id>-legend-<pose> (level 20 outfit), gear-familiar-<id> (level 3 pet),
captain-<id> (Fleet Ops captain portrait, head and shoulders).

Every file name:
`

// WriteKit zips every heroine stem: the pack's own file when it has one,
// otherwise the bundled webp decoded to PNG. Undecodable bundled files are skipped.
func WriteKit(w io.Writer, p Pack, bundled fs.FS, prompts []byte) error {
	byStem := map[string]string{}
	if ents, err := fs.ReadDir(bundled, "."); err == nil {
		for _, e := range ents {
			if m := hashed.FindStringSubmatch(e.Name()); m != nil && ValidStem(m[1]) {
				byStem[m[1]] = e.Name()
			}
		}
	}
	zw := zip.NewWriter(w)
	put := func(name string, b []byte) error {
		f, err := zw.Create(name)
		if err != nil {
			return err
		}
		_, err = f.Write(b)
		return err
	}
	for _, stem := range AllStems() {
		var b []byte
		if path, ok := p.Files[stem]; ok {
			src, err := os.ReadFile(path)
			if err != nil {
				continue
			}
			if strings.HasSuffix(strings.ToLower(path), ".png") {
				b = src
			} else if im, err := normalize(src); err == nil {
				var out bytes.Buffer
				png.Encode(&out, im)
				b = out.Bytes()
			}
		} else if name, ok := byStem[stem]; ok {
			src, err := fs.ReadFile(bundled, name)
			if err != nil {
				continue
			}
			im, _, err := image.Decode(bytes.NewReader(src))
			if err != nil {
				continue
			}
			var out bytes.Buffer
			png.Encode(&out, im)
			b = out.Bytes()
		}
		if b == nil {
			continue
		}
		if err := put(stem+".png", b); err != nil {
			return err
		}
	}
	// No pack (built-in heroines): an empty manifest, never ./manifest.json.
	m := Manifest{}
	if p.Dir != "" {
		m, _ = ReadManifest(p.Dir)
	}
	if m.Name == "" {
		m.Name = "My heroines"
	}
	m.Schema = 1
	mb, _ := json.MarshalIndent(m, "", "  ")
	if err := put("manifest.json", mb); err != nil {
		return err
	}
	if err := put("heroine-prompts.csv", prompts); err != nil {
		return err
	}
	if err := put("README.txt", []byte(readme+strings.Join(AllStems(), "\n")+"\n")); err != nil {
		return err
	}
	return zw.Close()
}
