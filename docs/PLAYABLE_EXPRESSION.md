# Playable Expression / Game Brief v1

The Playable Expression is provenance and interpretation metadata attached to a game record. It is **not** a second runtime format: executable behavior remains canonical Game IR, and delivery remains the versioned Game Package.

## Contract boundaries

- `schemaVersion` is independent from Game IR and package versions.
- `source` stores an opaque server-owned reference and/or SHA-256 digest, never a signed delivery URL.
- `creator` binds consent, owner, and source visibility. The create API requires the expression owner to equal the authenticated creator.
- `mode` is `make_playable`, `what_if`, or `remix`.
- `anchors` identify characters, objects, places, words, events, and feelings; `preserve` states whether the generation should retain each anchor.
- `motifs` and `optionalDetails` are variation guidance, not preservation guarantees.
- `evidence` points to text spans, image regions, or audio/video timestamps. Public projections must not expose private source text, source references, or media.
- `interpretation.preserved`, `invented`, and `uncertain` explicitly separate evidence-grounded content from creative additions and ambiguity. Claims can cite evidence IDs and carry confidence from 0 through 1.
- `generation` records the deterministic seed, Game IR/package versions, and model/prompt versions.
- `lineage` binds a remix to the exact parent game/version and, when available, parent expression.

## Draft and ready states

Drafts may be incomplete so creators can save an expression before interpretation is resolved. A ready expression must include creator intent, at least one anchor, a chosen interpretation, at least one categorized interpretation claim, and Game IR/package versions. Remix mode always requires a parent game ID.

The Go validator enforces schema version, source type/provenance, consent, visibility and mode allowlists, unique anchor/evidence IDs, supported evidence/anchor kinds, valid confidence ranges, evidence references, and ready/remix requirements. The JSON Schema at `schemas/playable-expression-v1.schema.json` defines the wire shape and size limits; cross-field references are enforced by the Go validator.

## Privacy and publication

The initial API accepts the contract on `POST /v1/games`, validates it, verifies owner equality, and persists it on the game record. It does not make source media public and does not change game visibility. A future public card projection must explicitly whitelist safe interpretation summaries and must omit source references, quotes, private evidence, and original media unless creator permission and access policy allow them.

## Migration policy

- Missing expression on legacy games means “not supplied”; do not synthesize source facts.
- Unknown schema versions fail closed for mutation/interpretation. Readers may display the game without expression metadata.
- Additive optional fields may be introduced within v1. Breaking field meaning or required semantics requires a new schema version and explicit migration.
- Game IR and package compatibility remain independently versioned.
