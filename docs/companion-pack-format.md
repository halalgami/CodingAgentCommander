# Companion pack format

A **pack** is a folder containing `manifest.json` and image files, used by the
sidebar companion ("panel" kind, Settings → Companion). Packs live at:

```
<UserConfigDir>/Commander/packs/<name>/
```

This is user data — never the repo. Pick a pack folder from Settings via
**Choose folder…**.

Source of truth: `docs/superpowers/specs/2026-08-28-companion-image-room-design.md` §5.

## 1. Frozen slot vocabulary

The slot list is the pack format's public API — what every image is named for,
what a generator produces against, and the one artifact that cannot change once
art exists.

**Foreground**

| Slot | Meaning | Prompt hint |
|---|---|---|
| `idle` | Nothing running, user present | relaxed, waiting, looking toward the screen |
| `working` | Selected session producing output | focused, leaning in, screen light on the face |
| `done` | Selected session just finished | pleased, looking up at you |
| `awaiting` | Selected session finished, waiting on the user | expectant, slight lean, eye contact |
| `error` | Selected session errored | concerned, brow drawn, light gone cold |
| `bored` | No input for > 5 min | slouched, looking away, half-lit |

**Background** (generic, reused for every non-selected session)

`bg_idle`, `bg_running`, `bg_finished`, `bg_error`.

Only `idle` is required. The `SLOTS` list and the `status → bg_*` map ship as
exported data from `pack.js`, not as prose — do not rename or paraphrase any of
these (e.g. `bg_running` is not `bg_active`).

## 2. Manifest

```jsonc
{
  "schema": 1,
  "name": "Monitor Glow",
  "author": "algam",
  "license": "personal-use",
  "canvas": { "w": 832, "h": 1216, "scale": 2, "anchor": "bottom-center" },
  "alpha": true,
  "slots": {
    "idle": [
      { "id": "idle-a", "file": "idle_a.png", "focusX": 0.5, "focusY": 0.28, "headScale": 0.22,
        "anim": { "strip": "idle_a_strip.png", "frames": 12, "fps": 8 } },
      { "id": "idle-night", "file": "night.png", "when": "lateNight" },
      { "id": "idle-secret", "file": "secret.png", "rarity": 0.02 }
    ],
    "working": [ { "id": "work-a", "file": "work.png", "mood": "relaxed" } ],
    "bg_running": [ { "id": "bgr", "file": "bg_run.png" } ]
  }
}
```

**Pack-level.** `canvas` is required. `w`/`h` are file pixels and `scale` is the
authoring DPI factor. `alpha: true` declares RGBA art; black-matted packs are
accepted with a warning because they cannot composite over the room's floor.
Images must be sRGB and are normalised on load.

**Variant fields**, all optional except `file`:

- `id` — stable identity, used for the serving URL, the cache key and
  sticky-pick. Defaults to a content hash, which only Go can compute, which is
  why Go owns normalisation.
- `mood` — `neutral | relaxed | happy | sad | surprised`, matching
  `reactions.js` exactly. `relaxed` is included and is the most common value.
- `rarity` — the probability that this variant appears **during any given
  minute its slot is showing**, in `(0, 0.5]`. Converted internally to a
  per-tick probability so the authored number stays meaningful if the resolve
  rate ever changes. `0` is rejected as a warning rather than silently
  producing dead art.
- `when` — a named condition (see §4). May be combined with `rarity`, in which
  case the condition **gates** and the rarity still rolls.
- `weight` — relative preference within a slot, default 1, for non-egg
  variants.
- `focusX`, `focusY`, `headScale` — registration. A single Y pins the eye line
  and nothing else; face X and head scale are what make two generated variants
  read as one person under a transition.
- `anim` — see §3.
- `meta` — `{ tool, model, prompt, seed, identityMethod, generatedAt }`. Free
  now, impossible to backfill, and required to regenerate one variant or hold
  a character across a set.

### Framing guidance

The sidebar slot is roughly **300 × 560** at default width. A **full-body**
image renders its face very small there — visibly so. Pack art should be an
**upper-body portrait**, head-and-torso, with the face in the upper third.
`canvas.anchor` should be `"top-center"` for that framing so `object-fit: cover`
crops the bottom rather than the head. This is guidance for authors and for
generator prompt templates, not a validation rule: a full-body pack still loads
and still works, it just reads smaller.

## 3. Animated idle loops

A variant may loop. The mechanism is a **sprite strip plus CSS `steps()`**, not
GIF:

- GIF is 256 colours with dithering, which bands badly on dark gradients, and
  carries no real alpha.
- An `<img>`-hosted animation cannot be frame-paused, which breaks the
  freeze-during-output rule.
- A sprite strip animates via `background-position` with
  `animation-play-state: paused` for suppression — compositor-driven,
  alpha-safe, deterministic, and testable.

`anim: { strip, frames, fps }` with `frames ≤ 24` and `fps ≤ 12`. The still
`file` remains required and is what shows when animation is suppressed or
unsupported. Animated WebP is accepted as an alternative with a warning that it
cannot be frame-paused and will be hidden rather than frozen during output.

## 4. Conditions

Named predicates, unit-tested with an injected clock. No expression language.

| Name | Class | Fires |
|---|---|---|
| `lateNight` | ambient | local hour in [2, 5) |
| `weekend` | ambient | Saturday or Sunday |
| `marathon` | ambient | selected session active > 60 min |
| `longIdle` | ambient | no terminal input > 30 min |
| `freshInstall` | ambient | first launch ever |
| `firstRunOfDay` | event | first launch of the calendar day |
| `century` | event | `finishSeq` crosses a multiple of 100 |
| `streak` | event | 3 finishes within 10 min |

- **Ambient** conditions do not flash. A gated variant simply joins the base
  pool while its condition holds, weighted normally, no TTL, no cooldown.
- **Event** conditions fire once per rising edge, show for `EGG_TTL_MS`, and
  require the condition to go false before firing again.

**Slot compatibility is validated.** `longIdle` is unreachable in `idle`
because `bored` outranks it; `marathon` only reaches `working`; `century` and
`streak` only reach `done`/`awaiting`. `pack.js` warns on an incompatible
pairing.

## 5. Validation and warnings

Go returns `{pack, warnings[]}` and never fails the app. Warnings cover:
missing file, bad JSON, unknown slot, unknown condition, incompatible
slot×condition, `rarity: 0`, out-of-range rarity, two ambient conditions that
can be simultaneously true in one slot, missing `canvas`, black-matted art, and
declared-vs-intrinsic size mismatch (intrinsic wins for layout). Settings
renders the merged warning list whenever it opens.

**Schema rule.** Unknown top-level keys, unknown slot names and unknown variant
keys are ignored with a warning, so newer packs can add fields an older
Commander still loads. Only a *major* schema bump refuses to load, with a
user-visible message; `schema: 1` and any minor increment load. Absent or
non-integer `schema` is treated as `1` with a warning.

## 6. No pack configured

If no pack is selected, the sidebar shows a short "Set up your companion"
affordance on the plain panel backdrop — no procedural or placeholder figure.
