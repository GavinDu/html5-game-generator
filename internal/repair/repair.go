// Package repair is the deterministic tier of the auto-repair loop in the
// fused detect-and-fix flow (Phase 3). It fixes the classes of breakage
// that have a provably safe mechanical fix, records every change as a
// diff, and reports the rest as unfixable — it never guesses at intent
// and never loosens a check to make a failure disappear.
//
// The loop that consumes this package is bounded: at most MaxAttempts
// repair rounds, and it stops early when a round changes nothing or the
// same failures survive a round unchanged. The bounded-LLM tier sits
// behind the LLMRepairer interface; with no repairer configured it is
// reported as skipped, never simulated.
package repair

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"image"
	_ "image/gif"
	_ "image/jpeg"
	"image/png"
	"math"
	"sort"
	"strings"

	"doodlequest/internal/gameir"
)

// MaxAttempts bounds the repair loop (validator.DefaultMaxAttempts).
const MaxAttempts = 3

// Change is one recorded edit: which rule fired, where, from what to what.
type Change struct {
	Rule   string `json:"rule"`
	Path   string `json:"path"`
	Before string `json:"before,omitempty"`
	After  string `json:"after,omitempty"`
}

// Result is the outcome of one deterministic repair pass.
type Result struct {
	Doc         *gameir.Document
	Changes     []Change
	Unfixable   []string
	FixedAssets map[string][]byte // asset id -> re-encoded bytes (rule reencode-image)
}

// ErrLLMUnavailable marks the bounded-LLM tier as not configured; callers
// surface it as a skipped tier, never as a pass.
var ErrLLMUnavailable = errors.New("repair: no LLM repairer configured")

// LLMRepairer is the bounded-LLM tier's seam. Implementations must return
// a full replacement document plus the diff they claim; the loop
// re-validates whatever they return.
type LLMRepairer interface {
	Repair(doc *gameir.Document, failing []string) (*gameir.Document, []Change, error)
}

func clone(doc *gameir.Document) *gameir.Document {
	raw, err := json.Marshal(doc)
	if err != nil {
		return nil
	}
	var out gameir.Document
	if err := json.Unmarshal(raw, &out); err != nil {
		return nil
	}
	return &out
}

func clamp(v, lo, hi float64) float64 {
	if math.IsNaN(v) || math.IsInf(v, 0) {
		return lo
	}
	if v < lo {
		return lo
	}
	if v > hi {
		return hi
	}
	return v
}

func ftoa(v float64) string { return fmt.Sprintf("%g", v) }

// RepairDeterministic applies every deterministic rule to a deep copy of
// doc. assets (id -> bytes) may be nil; the image rule then records its
// skip in Unfixable rather than pretending to have checked.
func RepairDeterministic(doc *gameir.Document, assets map[string][]byte) Result {
	out := clone(doc)
	res := Result{Doc: out, FixedAssets: map[string][]byte{}}
	if out == nil {
		res.Unfixable = append(res.Unfixable, "document could not be cloned")
		return res
	}
	assetIDs := map[string]bool{}
	for _, a := range out.Assets {
		assetIDs[a.ID] = true
	}
	res.clampPhysics(out)
	res.dropDanglingAssets(out, assetIDs)
	res.clampPositions(out)
	res.addWinRule(out)
	res.repairImages(out, assets)
	return res
}

// --- rule: clamp-physics -------------------------------------------------
// Out-of-range physics numbers are generation noise, not design intent:
// Rapier either rejects them or behaves chaotically. Clamp to the ranges
// the runtime contract documents.
func (r *Result) clampPhysics(doc *gameir.Document) {
	clampPtr := func(path string, p **float64, lo, hi float64) {
		if *p == nil {
			return
		}
		if c := clamp(**p, lo, hi); c != **p {
			r.Changes = append(r.Changes, Change{Rule: "clamp-physics", Path: path, Before: ftoa(**p), After: ftoa(c)})
			**p = c
		}
	}
	for si := range doc.Scenes {
		sc := &doc.Scenes[si]
		for i, g := range sc.Gravity {
			if c := clamp(g, -5000, 5000); c != g {
				r.Changes = append(r.Changes, Change{Rule: "clamp-physics", Path: fmt.Sprintf("scenes[%s].gravity[%d]", sc.ID, i), Before: ftoa(g), After: ftoa(c)})
				sc.Gravity[i] = c
			}
		}
		if sc.PixelsPerMeter != nil {
			clampPtr(fmt.Sprintf("scenes[%s].pixelsPerMeter", sc.ID), &sc.PixelsPerMeter, 10, 1000)
		}
		for ei := range sc.Entities {
			r.clampEntityPhysics(sc.ID, &sc.Entities[ei], clampPtr)
		}
		for ti := range sc.Templates {
			r.clampEntityPhysics(sc.ID, &sc.Templates[ti], clampPtr)
		}
	}
}

func (r *Result) clampEntityPhysics(sceneID string, e *gameir.Entity, clampPtr func(string, **float64, float64, float64)) {
	if e.Physics == nil {
		return
	}
	base := fmt.Sprintf("scenes[%s].entities[%s].physics", sceneID, e.ID)
	clampPtr(base+".density", &e.Physics.Density, 0.05, 50)
	clampPtr(base+".friction", &e.Physics.Friction, 0, 2)
	clampPtr(base+".restitution", &e.Physics.Restitution, 0, 1)
	clampPtr(base+".gravityScale", &e.Physics.GravityScale, -5, 5)
	clampPtr(base+".linearDamping", &e.Physics.LinearDamping, 0, 10)
	clampPtr(base+".angularDamping", &e.Physics.AngularDamping, 0, 10)
}

// --- rule: drop-dangling-asset -------------------------------------------
// An asset that was in the manifest at publish time can be dropped later
// (regeneration, storage cleanup). The IR then fails validation and the
// runtime cannot draw the reference. Clearing the reference restores the
// runtime's documented fallback (colored shape / background color);
// gameplay-bearing references (set_asset targets) are NOT guessed at.
func (r *Result) dropDanglingAssets(doc *gameir.Document, assetIDs map[string]bool) {
	drop := func(path string, p *string) {
		if *p != "" && !assetIDs[*p] {
			r.Changes = append(r.Changes, Change{Rule: "drop-dangling-asset", Path: path, Before: *p, After: ""})
			*p = ""
		}
	}
	for si := range doc.Scenes {
		sc := &doc.Scenes[si]
		drop(fmt.Sprintf("scenes[%s].backgroundAssetId", sc.ID), &sc.BackgroundAssetID)
		for li := range sc.Layers {
			drop(fmt.Sprintf("scenes[%s].layers[%s].assetId", sc.ID, sc.Layers[li].ID), &sc.Layers[li].AssetID)
		}
		for ei := range sc.Emitters {
			drop(fmt.Sprintf("scenes[%s].emitters[%d].textureAssetId", sc.ID, ei), &sc.Emitters[ei].TextureAssetID)
		}
		visit := func(e *gameir.Entity) {
			drop(fmt.Sprintf("scenes[%s].entities[%s].assetId", sc.ID, e.ID), &e.AssetID)
			if e.Render != nil && e.Render.Particles != nil {
				drop(fmt.Sprintf("scenes[%s].entities[%s].particles.textureAssetId", sc.ID, e.ID), &e.Render.Particles.TextureAssetID)
			}
			if e.Render != nil {
				for fi := range e.Render.Filters {
					if e.Render.Filters[fi].Type == "displacement" {
						drop(fmt.Sprintf("scenes[%s].entities[%s].filters[%d].assetId", sc.ID, e.ID, fi), &e.Render.Filters[fi].AssetID)
					}
				}
			}
		}
		for ei := range sc.Entities {
			visit(&sc.Entities[ei])
		}
		for ti := range sc.Templates {
			visit(&sc.Templates[ti])
		}
	}
	for ri := range doc.Rules {
		rule := &doc.Rules[ri]
		kept := rule.Actions[:0]
		for _, a := range rule.Actions {
			if a.AssetID != "" && !assetIDs[a.AssetID] {
				switch a.Type {
				case "play_sound", "emit_particles":
					r.Changes = append(r.Changes, Change{Rule: "drop-dangling-asset",
						Path: fmt.Sprintf("rules[%s].actions[%s]", rule.ID, a.Type), Before: a.AssetID, After: "(action dropped)"})
					continue
				default:
					r.Unfixable = append(r.Unfixable, fmt.Sprintf(
						"rule %q action %q references missing asset %q: gameplay-bearing, needs a human or regeneration", rule.ID, a.Type, a.AssetID))
				}
			}
			kept = append(kept, a)
		}
		rule.Actions = kept
	}
}

// --- rule: clamp-position --------------------------------------------------
// Entities generated far outside the world can never be reached or seen.
// Pull them back inside the world bounds with a small margin.
func (r *Result) clampPositions(doc *gameir.Document) {
	for si := range doc.Scenes {
		sc := &doc.Scenes[si]
		w, h := 390.0, 844.0
		if len(sc.WorldSize) == 2 {
			w, h = sc.WorldSize[0], sc.WorldSize[1]
		}
		const margin = 100
		for ei := range sc.Entities {
			e := &sc.Entities[ei]
			if len(e.Position) != 2 {
				continue
			}
			x, y := e.Position[0], e.Position[1]
			nx, ny := clamp(x, -margin, w+margin), clamp(y, -margin, h+margin)
			if nx != x || ny != y {
				r.Changes = append(r.Changes, Change{Rule: "clamp-position",
					Path:   fmt.Sprintf("scenes[%s].entities[%s].position", sc.ID, e.ID),
					Before: fmt.Sprintf("[%g %g]", x, y), After: fmt.Sprintf("[%g %g]", nx, ny)})
				e.Position = []float64{nx, ny}
			}
		}
	}
}

// --- rule: add-win-rule ----------------------------------------------------
// A collect-a-thon with pickups and score rules but no end_game(true)
// rule can never be won — the session just stops being fun. The missing
// threshold is mechanical: the total collectible score.
func (r *Result) addWinRule(doc *gameir.Document) {
	for _, rule := range doc.Rules {
		for _, a := range rule.Actions {
			if a.Type == "end_game" {
				if v, ok := a.Value.(bool); ok && v {
					return // a win rule already exists
				}
			}
		}
	}
	pickups := map[string]bool{}
	playerID := "player"
	for _, sc := range doc.Scenes {
		for _, e := range sc.Entities {
			if e.Kind == "pickup" {
				pickups[e.ID] = true
			}
			if e.Kind == "player" {
				playerID = e.ID
			}
		}
	}
	if len(pickups) == 0 {
		return
	}
	total := 0.0
	scored := map[string]bool{}
	for _, rule := range doc.Rules {
		if rule.When.Event != "collected" || !pickups[rule.When.Target] {
			continue
		}
		for _, a := range rule.Actions {
			if a.Type == "add_score" {
				if v, ok := a.Value.(float64); ok {
					total += v
				} else {
					total += 1
				}
				scored[rule.When.Target] = true
			}
		}
	}
	for id := range pickups {
		if !scored[id] {
			total += 1
		}
	}
	if total <= 0 {
		return
	}
	win := gameir.Rule{
		ID:   "auto-repair-win",
		When: gameir.Condition{Event: "score_reached", Source: playerID, Value: &total},
		Actions: []gameir.Action{
			{Type: "show_text", Value: "You did it!"},
			{Type: "end_game", Value: true},
		},
	}
	doc.Rules = append(doc.Rules, win)
	r.Changes = append(r.Changes, Change{Rule: "add-win-rule", Path: "rules[auto-repair-win]",
		Before: "(no win rule)", After: fmt.Sprintf("score_reached %g -> end_game(true)", total)})
}

// --- rule: reencode-image ----------------------------------------------------
// A decodable image in a format the player cannot use everywhere (or with
// corrupt metadata) is re-encoded to PNG. An undecodable image is NOT
// repairable — it needs regeneration, and this rule says so instead of
// substituting art (child artwork is preserved, never replaced).
func (r *Result) repairImages(doc *gameir.Document, assets map[string][]byte) {
	if assets == nil {
		r.Unfixable = append(r.Unfixable, "asset bytes not available to the repairer; image checks skipped")
		return
	}
	for _, a := range doc.Assets {
		if a.Kind != "image" && a.Kind != "sprite" && a.Kind != "background" {
			continue
		}
		raw, ok := assets[a.ID]
		if !ok || len(raw) == 0 {
			continue
		}
		img, format, err := image.Decode(bytes.NewReader(raw))
		if err != nil {
			r.Unfixable = append(r.Unfixable, fmt.Sprintf("asset %q: image bytes undecodable (%v); needs regeneration", a.ID, err))
			continue
		}
		if format == "png" {
			continue
		}
		var buf bytes.Buffer
		if err := png.Encode(&buf, img); err != nil {
			r.Unfixable = append(r.Unfixable, fmt.Sprintf("asset %q: re-encode to PNG failed: %v", a.ID, err))
			continue
		}
		r.FixedAssets[a.ID] = buf.Bytes()
		r.Changes = append(r.Changes, Change{Rule: "reencode-image", Path: "assets[" + a.ID + "]",
			Before: format, After: "png"})
	}
}

// GateFunc reports the failing check ids for a document (empty = clean).
type GateFunc func(doc *gameir.Document) []string

// LoopResult is the repair loop's outcome.
type LoopResult struct {
	Result    Result
	Attempts  int
	Remaining []string // failing ids after the loop stopped
}

// RunLoop applies deterministic repair rounds against the gate. The
// first repair round ALWAYS runs: several rules fix breakage the schema
// gate cannot measure (an entity placed outside the world validates
// fine and is unplayable), so gating the repair itself on a schema
// failure would skip exactly the fixes that matter. After each round the
// gate re-runs; the loop stops when the gate is clean, a round changes
// nothing, the same failures survive a round unchanged, or MaxAttempts
// rounds have run — the guardrails from the fused design. Attempts
// counts the rounds that produced at least one change.
func RunLoop(doc *gameir.Document, assets map[string][]byte, gate GateFunc) LoopResult {
	var all Result
	all.Doc = doc
	all.FixedAssets = map[string][]byte{}
	merge := func(round Result) {
		all.Changes = append(all.Changes, round.Changes...)
		all.Unfixable = append(all.Unfixable, round.Unfixable...)
		for id, b := range round.FixedAssets {
			all.FixedAssets[id] = b
		}
		all.Doc = round.Doc
	}
	current := doc
	seen := map[string]bool{}
	attempts := 0
	for roundNo := 1; roundNo <= MaxAttempts; roundNo++ {
		round := RepairDeterministic(current, assets)
		merge(round)
		current = round.Doc
		if len(round.Changes) > 0 {
			attempts++
		}
		failing := gate(current)
		if len(failing) == 0 {
			return LoopResult{Result: all, Attempts: attempts, Remaining: nil}
		}
		if len(round.Changes) == 0 {
			return LoopResult{Result: all, Attempts: attempts, Remaining: failing}
		}
		key := strings.Join(sortedCopy(failing), ",")
		if seen[key] {
			return LoopResult{Result: all, Attempts: attempts, Remaining: failing}
		}
		seen[key] = true
	}
	return LoopResult{Result: all, Attempts: attempts, Remaining: gate(current)}
}

func sortedCopy(in []string) []string {
	out := append([]string{}, in...)
	sort.Strings(out)
	return out
}
