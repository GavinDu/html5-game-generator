# Partial raster decoded-resource admission for the HTML5 Game Player

Parent: [Hephaestus Epic #202](https://github.com/GavinDu/Hephaestus/issues/202), [capability/resource preflight #316](https://github.com/GavinDu/html5-game-generator/issues/316), [decoded asset budgets #383](https://github.com/GavinDu/html5-game-generator/issues/383), [scoped #405](https://github.com/GavinDu/html5-game-generator/issues/405).

## Implemented source boundary

The backend formerly capped only compressed transfer bytes (8 MiB per asset, 40 MiB per advertised package). That does **not** cap decoded texture memory: a tiny, valid PNG header can declare multi-gigapixel dimensions. Supported declared **PNG, JPEG and GIF** raster images are now inspected with Go `image.DecodeConfig` (header-only, no full decoded bitmap allocation), immediately before package advertisement and individual image delivery. An encoded format that disagrees with the declared MIME, a corrupt/truncated header or dimensions outside the bounds returns HTTP 422 with `Cache-Control: private, no-store`. Existing per-asset bytes, SHA-256, moderation/auth controls and immutable package ETags still apply.

Hard safety ceilings chosen before device calibration:
- Maximum raster width or height: **8,192 pixels**.
- Maximum decoded width×height per supported image: **16,777,216 pixels**, roughly 64 MiB for one 4-byte RGBA surface before GPU overhead.
- Maximum sum of admitted PNG/JPEG/GIF decoded pixels in one game package: **64,000,000 pixels**, roughly 244 MiB in RGBA before GPU textures, mipmaps or multiple frames.

A direct asset URL enforces the same per-image guard. Go tests exercise valid PNG/JPEG/GIF headers, header-dimension spoofing with recomputed PNG CRC, exact/over budget predicates, MIME mismatch, truncated bytes, package and direct URL 422/no-store.

## Security and platform exclusions

This is explicitly **partial**: *only* declared PNG/JPEG/GIF are admitted. The follow-up [#407](https://github.com/GavinDu/html5-game-generator/issues/407) now rejects unknown, empty or unsupported image MIME before package and asset delivery instead of permitting those formats to bypass the decoded budget. WebP/SVG and other formats require a separately reviewed capability/decoder addition (and migration for any historical package without supported MIME). Animated GIF multi-frame memory, mipmaps, framebuffers, GPU driver overhead, audio decode buffers and dynamically spawned physics entities are **not covered** by the header-only helper. A future format/asset-migration stage must handle them before #383 can close. The header parser does not prove that the complete compressed image decodes correctly. A malicious PNG with valid header and corrupted IDAT may be rejected later by the player; it must never be considered independently attested or playable solely from DecodeConfig. These limits are conservative hard ceilings, not measured memory targets for iPhone or iPad.

The iOS WKWebView should surface typed unsupported/invalid-package recovery rather than a silent blank canvas (#280), and physical iPhone/iPad memory/thermal measurements remain #279. App Store UGC and server-owned replay verification are also separate Epic gates. The rollback is limited to this explicit PNG/JPEG/GIF admission helper, endpoint checks and test-fixture updates. Six exact-head GitHub Actions jobs are mandatory before merging; no inherited or skipped checks.
