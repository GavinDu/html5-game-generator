# Shipped gameplay poster fallbacks

The initial four inventory games include gameplay screenshots from the successful CI run [42228113902](https://github.com/GavinDu/html5-game-generator/actions/runs/42228113902). These are rendered game frames, not source photographs, invented game art, or evidence of an observed win. Bobo and Escape did not finish inside that Playability Kit run's 40-second window; their PLAYABLE verdict must not be described as an observed completion.

Each `preview-fixture.json` binds the poster to its manifest version, the exact SHA-256 of `game-ir.json`, and the PNG bytes. A changed IR or version suppresses the old poster while keeping the game playable. Corrupt poster bytes fail inventory validation. Source screenshot and CI run provenance are retained in the fixture metadata.

The loader stores only a poster under the existing version-scoped preview delivery path. It uses the existing `failed` poster-only fallback status, leaves clip URL and duration absent, and does not imply a generated video. Preview delivery retains all existing authorization, moderation, quarantine, and cache checks. This repository-authored fixture path does not change user-upload scanning or source-sharing consent.

Restarting an inventory game preserves an existing creator preview only when its published version and canonical IR still match. It does not replace a valid uploaded clip with a fixture poster. The generic generation/playtest/preview publication pipeline remains tracked in #98 and #100; physical-device preview performance remains Hephaestus #55.

Verification includes poster content/hash/version tests and the real feed/preview delivery contract. Review the native and browser feed screenshots after deployment to confirm these actual gameplay frames replace gradient placeholders.

Bobo’s poster is refreshed after its catcher-control repair, using the local Chromium gameplay screenshot and observed-completion report recorded in its fixture metadata. The original three other screenshots retain their CI provenance.

Bobo's 2026-11-08 end-game jingle ordering change alters the canonical IR. The previously reviewed mid-play Bobo poster is deliberately **not** rebound to a new IR hash without a fresh rendered screenshot review. Until a new version-bound screenshot/clip is approved, the feed may show the explicitly labeled **Preview unavailable — Tap Play now to explore** fallback; the game remains playable. This does not relax the independent poster-hash validation test or publish a fabricated frame.

## Replacing invalidated posters (2026-11-08)

A screenshot from an earlier version MUST NOT be re-labeled as current gameplay after
an IR edit. The Chromium game-quality job now runs
`tools/playtest/prepare-preview-fixtures.mjs` against four *real, passed*
game-specific screenshot sequences: Penguin's aurora at six hearts, Ribbon
after an independently repaired sky hole, Escape with a real falling lane
hazard, and Bobo after a real short tap. The job uploads the
`gameplay-preview-candidates` GitHub Actions artifact with:

- A readable `proof.png` for visual review; it is not a source photograph.
- A `preview-poster.png.b64` containing the exact PNG bytes from that CI run.
- A `preview-fixture.json` binding the PNG SHA-256, canonical
  `game-ir.json` SHA-256, version, source screenshot and workflow run.
- An `index.json` explicitly marked `AWAITING_SCREENSHOT_REVIEW`.

After reviewing each screenshot against the source questionnaire, artwork and
gameplay, **GPT must create a normal PR using the GitHub connector** to copy the
approved candidate into its inventory folder, and GitHub Actions must verify
hash/version, game rules and screenshot-backed playability before merging.
Never auto-commit generated images from Actions. A pending or failed preview
does not imply a published clip; genuine video capture and replay verification
remain tracked in #100. If a screenshot does not honestly depict the current
IR, reject it and keep the fallback rather than inventing media.
