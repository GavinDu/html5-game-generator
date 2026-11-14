# Bounded GIF frame metadata admission — Epic #202

Parent: [Game runtime Epic #202](https://github.com/GavinDu/Hephaestus/issues/202), [animation resource admission #409](https://github.com/GavinDu/html5-game-generator/issues/409), independently testable scanner [#410](https://github.com/GavinDu/html5-game-generator/issues/410), existing decoded raster header [#405](https://github.com/GavinDu/html5-game-generator/issues/405).

## Source invariant

Go `image.DecodeConfig` reads the GIF **canvas**, not the number or aggregate decoded area of image frames. An 8 MiB compressed transfer ceiling and a 16.8M-pixel logical screen limit do not bound an animation with hundreds of repeated or full-size frames.

This change adds a **metadata-only** scanner when the recognized image format is GIF; it never calls `gif.DecodeAll` nor allocates a decoded frame. The scanner bounds/validates the 87a/89a signature, logical canvas, global/local color tables, every image descriptor, image-frame offsets, LZW minimum code byte, extension/image length-prefixed subblocks, terminator and trailing garbage. It rejects frame boxes beyond the canvas and aggregates decoded frame-area estimates across the animation.

Safety caps: maximum **128 image frames** and **64,000,000 aggregate frame pixels** for each GIF in addition to the already enforced 8,192px max dimension, 16,777,216 per-canvas pixels and compressed transfer bounds. The header parser runs before **both** package advertisement and direct asset response, with existing 422 `private, no-store` rejection on failure.

Tests include valid 1-/2-/128-frame encoded GIFs, an encoded 129-frame GIF, a synthetic under-128-byte container declaring 4 × 4096² image frames that exceeds aggregate pixel budget, truncated/invalid subblocks, trailing bytes and package/direct asset authorization behavior. New formats and MIME-less legacy contracts remain governed by the separately versioned #407 work.

## Senior engine-architect challenge / exclusion

This is a **container metadata limit**, not proof that LZW payloads decode successfully, that the browser releases every animation frame, or that a compositor will render one frame per tick. It deliberately leaves GIF disposal-mode intermediate buffers, texture caches, WebP/SVG animation, GPU render target/mipmap expansion, audio and physics budget, and physical iPhone/iPad thermal/perf acceptance for parent #409/#316/Epic #202. Server-owned attestation/authoritative score is unrelated. A future decoder-aware validator may inspect actual LZW payload under bounded memory if required by threat modeling.

Merge gate: all six exact-head Go/race/static, PostgreSQL, Chromium, WebKit, cross-browser determinism and Playability Kit checks must succeed. No "animated GIF safe" claim from metadata or CI alone. Rollback is isolated to GIF metadata scanner, its raster-header invocation and tests.
