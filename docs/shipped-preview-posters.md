# Shipped gameplay poster fallbacks

The initial four inventory games include gameplay screenshots from the successful CI run [42228113902](https://github.com/GavinDu/html5-game-generator/actions/runs/42228113902). These are rendered game frames, not source photographs, invented game art, or evidence of an observed win. Bobo and Escape did not finish inside that Playability Kit run's 40-second window; their PLAYABLE verdict must not be described as an observed completion.

Each `preview-fixture.json` binds the poster to its manifest version, the exact SHA-256 of `game-ir.json`, and the PNG bytes. A changed IR or version suppresses the old poster while keeping the game playable. Corrupt poster bytes fail inventory validation. Source screenshot and CI run provenance are retained in the fixture metadata.

The loader stores only a poster under the existing version-scoped preview delivery path. It uses the existing `failed` poster-only fallback status, leaves clip URL and duration absent, and does not imply a generated video. Preview delivery retains all existing authorization, moderation, quarantine, and cache checks. This repository-authored fixture path does not change user-upload scanning or source-sharing consent.

Restarting an inventory game preserves an existing creator preview only when its published version and canonical IR still match. It does not replace a valid uploaded clip with a fixture poster. The generic generation/playtest/preview publication pipeline remains tracked in #98 and #100; physical-device preview performance remains Hephaestus #55.

Verification includes poster content/hash/version tests and the real feed/preview delivery contract. Review the native and browser feed screenshots after deployment to confirm these actual gameplay frames replace gradient placeholders.
