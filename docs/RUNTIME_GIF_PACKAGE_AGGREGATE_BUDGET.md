# Aggregate GIF decoded-frame budget across a game package

Parents: [Epic #202](https://github.com/GavinDu/Hephaestus/issues/202), [resource preflight #316](https://github.com/GavinDu/html5-game-generator/issues/316), [animation memory #409](https://github.com/GavinDu/html5-game-generator/issues/409), independently testable [#414](https://github.com/GavinDu/html5-game-generator/issues/414).

## Problem and bounded implementation

Before this slice, the per-GIF header scanner correctly enforced <=128 frames and <=64,000,000 aggregate frame pixels **inside one GIF**, but `handleGetPackage` added only the GIF's logical canvas `width × height` to its **whole package** resource ledger. Multiple individually acceptable animations could therefore evade package-wide decoded memory admission with a few kilobytes of compressed content.

The scanner now returns its already-validated total frame rectangle pixels as an integer. The independent package preflight uses `rasterPackageDecodedCost`: static PNG/JPEG consume exactly header dimensions, GIF consumes the sum of **all its frame rectangles**. It checks `<=64,000,000` pixels across all admitted images with overflow-safe unsigned arithmetic. Each GIF still runs header, format, axis, per-frame and per-animation checks, and the entire package still obeys its compressed transfer-byte budget. The shared 422 `private, no-store` response occurs **before any package manifest is returned**. Direct asset URLs retain individual per-file constraints; the package endpoint alone has the whole asset set to compute combined memory.

The scanner is metadata-only, with no `gif.DecodeAll`, LZW bitmap allocation or new native capability. Tests include two synthetic 34×1024²-frame GIFs that each use a tiny compressed container and individually fit the per-animation 64M frame limit but together exceed the package 64M limit; the first alone remains accepted, the combined package fails 422/no-store, and both direct endpoints retain their independent per-asset admission. Existing static PNG and canonical absent-MIME GIF byte-sniff semantics remain.

## Senior game-engine architect review gate

This is a conservative **declared-frame rectangle count**, not an empirical GPU-memory ceiling. It does not account for actual decoder intermediate frame retention, disposal blending, mipmaps, WebGL render targets, WebP/SVG, audio buffers, physics entities, texture fragmentation, or physical iPhone/iPad p95/thermal. A malformed bitstream may still pass metadata checks but fail actual browser decode; full LZW validation is a separate goal. Broader #409/#316 and Epic #202 remain OPEN. Browser WebKit/Chromium/Go-race/PostgreSQL/Playability Kit CI does not equal real-device evidence or independent human reviewer approval.

Close only #414 after all six exact-head generator CI jobs complete SUCCESS. Rollback is isolated to the decoded cost helper, package ledger, GIF scanner return values, and the new tests/docs.
