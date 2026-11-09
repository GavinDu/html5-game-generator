// Package pipeline runs the async generation job: spec -> (user confirm) ->
// art -> codegen -> validate -> publish. Stage implementations are stubs
// with the real interfaces; each records latency/tokens/cost.
package pipeline

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
	"sync"
	"unicode/utf8"
	"time"

	"doodlequest/internal/expression"
	"doodlequest/internal/gameir"
	"doodlequest/internal/media"
	"doodlequest/internal/model"
	"doodlequest/internal/shim"
	"doodlequest/internal/store"
)

// stageOrder is the full pipeline. Spec runs first and pauses for user
// confirmation; the rest run after Confirm.
var stageOrder = []model.StageName{
	model.StageSpec,
	model.StageArt,
	model.StageCodegen,
	model.StageValidate,
	model.StagePublish,
}

// Stages used to sleep on a fixed schedule (simulateMs) so the demo
// *looked* asynchronous; the sleeps are gone (architecture review
// 2026-10-23). The pipeline is genuinely async — jobs run on a
// goroutine and clients poll — it just no longer pretends the stub
// stages take time they do not.

type Runner struct {
	store     store.Store
	confirmMu sync.Mutex // serializes confirmation check + running transition
}

func NewRunner(st store.Store) *Runner { return &Runner{store: st} }

func newID() string {
	var b [9]byte
	_, _ = rand.Read(b[:])
	return hex.EncodeToString(b[:])
}

func NewID() string { return newID() }

func freshStages() []model.StageRecord {
	out := make([]model.StageRecord, len(stageOrder))
	for i, n := range stageOrder {
		out[i] = model.StageRecord{Name: n, Status: model.StagePending, Provider: "stub"}
	}
	return out
}

func stageIdx(j *model.Job, n model.StageName) int {
	for i := range j.Stages {
		if j.Stages[i].Name == n {
			return i
		}
	}
	return -1
}

// StubVisionAnalyze pretends to read the kid's drawing: keywords become
// elements, and an intent is guessed from them. The real vision model
// replaces this function; the contract (elements + intent) stays.
func StubVisionAnalyze(images []model.ImageInput, keywords []string) ([]model.Element, string) {
	kinds := []string{"character", "prop", "prop", "background"}
	labels := keywords
	if len(labels) == 0 {
		labels = []string{"dragon", "castle"}
	}
	elements := make([]model.Element, 0, len(labels))
	for i, l := range labels {
		l = strings.ToLower(strings.TrimSpace(l))
		if l == "" {
			continue
		}
		kind := kinds[i%len(kinds)]
		elements = append(elements, model.Element{
			ID:          fmt.Sprintf("el_%d", i+1),
			Label:       l,
			Kind:        kind,
			Description: fmt.Sprintf("a kid's drawing of a %s (vision stub)", l),
		})
	}
	intent := fmt.Sprintf("A tap-to-collect game starring %s", joinLabels(elements))
	return elements, intent
}

func joinLabels(els []model.Element) string {
	ls := make([]string, len(els))
	for i, e := range els {
		ls[i] = e.Label
	}
	return strings.Join(ls, ", ")
}

// capTitle preserves the existing byte budget without cutting a UTF-8
// character in half. Remix parents can have bilingual or emoji titles; raw
// s[:n] produced invalid UTF-8 when the boundary landed inside a code point.
func capTitle(s string, n int) string {
	if n <= 0 {
		return ""
	}
	s = strings.ToValidUTF8(strings.TrimSpace(s), "")
	if len(s) <= n {
		return s
	}
	cut := n
	for cut > 0 && !utf8.RuneStart(s[cut]) {
		cut--
	}
	return strings.TrimSpace(s[:cut])
}

// buildDesignDoc is the stub SPEC stage: elements + intent -> design doc.
// The real SPEC LLM replaces this; output schema stays.
func buildDesignDoc(elements []model.Element, intent string, keywords []string) *model.DesignDoc {
	star := "dragon"
	if len(elements) > 0 {
		star = elements[0].Label
	}
	titleStar := strings.Title(star)
	mood := "adventurous"
	if len(keywords) > 0 {
		for _, k := range keywords {
			switch strings.ToLower(k) {
			case "spooky", "scary", "ghost":
				mood = "spooky"
			case "calm", "sleepy", "bedtime":
				mood = "calm"
			}
		}
	}
	return &model.DesignDoc{
		Title:         fmt.Sprintf("%s's Big Day", titleStr(titleStar)),
		Plot:          fmt.Sprintf("%s wakes up to find the sky raining stars. %s must catch 10 falling stars before bedtime!", titleStar, titleStar),
		Mechanics:     "Tap to move your character left/right. Catch falling stars. First to 10 wins.",
		Controls:      []string{"tap"},
		WinCondition:  "Catch 10 stars",
		LoseCondition: "None — endless until you win (kid-friendly)",
		Elements:      elements,
		MusicMood:     mood,
	}
}

func titleStr(s string) string {
	if s == "" {
		return s
	}
	return strings.ToUpper(s[:1]) + s[1:]
}

// CreateAndStart creates the job and runs the SPEC stage, then pauses for
// user confirmation of the design doc.
func (r *Runner) CreateAndStart(game *model.Game, elements []model.Element, intent string, keywords []string) *model.Job {
	now := time.Now()
	job := &model.Job{
		ID:        newID(),
		GameID:    game.ID,
		Status:    model.JobQueued,
		Stages:    freshStages(),
		CreatedAt: now,
		UpdatedAt: now,
	}
	r.store.CreateJob(job)
	go r.runSpec(job.ID, elements, intent, keywords)
	return job
}

// applyRemixDesignDoc derives the confirmation summary from the canonical
// parent package for every remix profile. The IR remains authoritative for
// mechanics, controls, objective, and failure behavior; a child must never
// silently replace a parent's loop with a Bobo-only questionnaire template.
func applyRemixDesignDoc(doc *model.DesignDoc, parent *model.Game) {
	if doc == nil || parent == nil {
		return
	}
	doc.Title = capTitle("Remix: "+parent.Title, 80)
	if parent.GameIR == nil {
		return
	}
	if description := strings.TrimSpace(parent.GameIR.Game.Description); description != "" {
		doc.Plot = description
	}
	if exp := parent.GameIR.Experience; exp != nil {
		doc.Mechanics = fmt.Sprintf("Preserve the parent's %s gameplay loop and adapt only its presentation and story.", exp.Archetype)
		doc.Controls = []string{exp.Controls.Scheme}
		doc.WinCondition = exp.Objective.Label
		switch exp.Failure.Mode {
		case "gentle", "none":
			doc.LoseCondition = "No failure penalty; preserve the parent's gentle play rules."
		case "health":
			if exp.Failure.Health > 0 {
				doc.LoseCondition = fmt.Sprintf("The run ends when the player exhausts %d health.", exp.Failure.Health)
			}
		case "sudden-death":
			doc.LoseCondition = "A single failed collision ends the run."
		}
	} else if parent.Manifest != nil {
		// The canonical input bindings remain authoritative even when the
		// parent predates the additive experience block.
		doc.Controls = append([]string(nil), parent.Manifest.Controls...)
	}
}

func (r *Runner) runSpec(jobID string, elements []model.Element, intent string, keywords []string) {
	job, ok := r.store.GetJob(jobID)
	if !ok {
		return
	}
	job.Status = model.JobRunning
	r.runStage(job, model.StageSpec, func() error {
		job.DesignDoc = buildDesignDoc(elements, intent, keywords)
		if game, ok := r.store.GetGame(job.GameID); ok && game.ParentGameID != "" {
			parent, found := r.store.GetGame(game.ParentGameID)
			if !found || parent.Manifest == nil {
				return fmt.Errorf("remix parent %q is unavailable", game.ParentGameID)
			}
			applyRemixDesignDoc(job.DesignDoc, parent)
		}
		return nil
	})
	if job.Status == model.JobFailed {
		r.finishFailed(job, "spec stage failed")
		return
	}
	job.Status = model.JobAwaitingConfirm
	job.UpdatedAt = time.Now()
	r.store.UpdateJob(job)

	if game, ok := r.store.GetGame(job.GameID); ok {
		game.Status = model.GameGenerating
		game.UpdatedAt = time.Now()
		r.store.UpdateGame(game)
	}
}

// Confirm resumes the pipeline after the user approves (or edits) the
// design doc. Returns an error if the job isn't awaiting confirmation.
func (r *Runner) Confirm(gameID string, doc *model.DesignDoc) (*model.Job, error) {
	// Multiple network retries can hit Confirm before the async ART goroutine
	// has reached its first stage update. Check and transition under one
	// lock so only one request starts the pipeline and wins the design edits.
	// The store returns value clones; the persisted transition MUST happen
	// before scheduling the worker, not afterward.
	r.confirmMu.Lock()
	defer r.confirmMu.Unlock()

	job, ok := r.store.GetJobByGame(gameID)
	if !ok {
		return nil, fmt.Errorf("no job for game %s", gameID)
	}
	if job.Status != model.JobAwaitingConfirm {
		return nil, fmt.Errorf("job %s is %s, not awaiting confirmation", job.ID, job.Status)
	}
	if doc != nil && doc.Title != "" {
		job.DesignDoc = doc
	}
	job.Status = model.JobRunning
	job.UpdatedAt = time.Now()
	r.store.UpdateJob(job)
	go r.runRemaining(job.ID)
	return job, nil
}

func (r *Runner) runRemaining(jobID string) {
	job, ok := r.store.GetJob(jobID)
	if !ok {
		return
	}
	doc := job.DesignDoc

	// ART: polish sprites + music.
	r.runStage(job, model.StageArt, func() error {
		if child, ok := r.store.GetGame(job.GameID); ok && child.ParentGameID != "" {
			parent, found := r.store.GetGame(child.ParentGameID)
			if !found || parent.Manifest == nil {
				return fmt.Errorf("remix parent %q is unavailable", child.ParentGameID)
			}
			if parent.GameIR != nil {
				// Canonical IR is authoritative for every remix profile, including
				// Bobo. Copy every declared asset so the child package cannot
				// retain references to files that were not copied.
				for _, asset := range parent.GameIR.Assets {
					if asset.URI == "" {
						continue
					}
					data, contentType, exists := r.store.GetArtifact(parent.ID, asset.URI)
					if !exists {
						return fmt.Errorf("remix parent asset %q (%s) is missing", asset.ID, asset.URI)
					}
					r.store.PutArtifact(job.GameID, asset.URI, data, contentType)
				}
			} else {
				return fmt.Errorf("remix parent %q has no canonical Game IR to preserve", parent.ID)
			}
		}
		for i, el := range doc.Elements {
			if i >= 4 {
				break
			}
			spriteID := fmt.Sprintf("el_%d", i+1)
			r.store.PutArtifact(job.GameID, "sprites/"+spriteID+".png",
				media.SpritePNG(el.Label), "image/png")
		}
		// A creator can confirm a questionnaire with no extracted elements.
		// CODEGEN supplies a fallback player asset reference in that case;
		// ART must emit its real PNG bytes before the package is published.
		if len(doc.Elements) == 0 {
			if game, ok := r.store.GetGame(job.GameID); ok && game.ParentGameID == "" {
				r.store.PutArtifact(job.GameID, "sprites/el_1.png",
					media.SpritePNG("friendly player"), "image/png")
			}
		}
		r.store.PutArtifact(job.GameID, "sprites/star.png", media.StarPNG(), "image/png")
		if game, ok := r.store.GetGame(job.GameID); !ok || game.ParentGameID == "" {
			r.store.PutArtifact(job.GameID, "audio/theme.wav", media.MusicWAV(doc.MusicMood), "audio/wav")
		}
		return nil
	})
	if r.stopOnStageFailure(job) {
		return
	}

	// CODEGEN: produce the canonical declarative Game IR. The GameShim output
	// is retained only as a temporary HTML5 test oracle during migration.
	var gameJS string
	var canonicalIR *gameir.Document
	r.runStage(job, model.StageCodegen, func() error {
		child, _ := r.store.GetGame(job.GameID)
		if child != nil && child.ParentGameID != "" {
			parent, found := r.store.GetGame(child.ParentGameID)
			if !found {
				return fmt.Errorf("remix parent %q is unavailable", child.ParentGameID)
			}
			var err error
			canonicalIR, err = buildRemixIR(doc, child, parent)
			if err != nil {
				return err
			}
		} else {
			canonicalIR = buildGameIR(doc, job.GameID)
		}
		data, err := json.MarshalIndent(canonicalIR, "", "  ")
		if err != nil {
			return fmt.Errorf("encode Game IR: %w", err)
		}
		r.store.PutArtifact(job.GameID, "game-ir.json", data, "application/json")
		gameJS = shim.BuildGameJS(doc, "el_1")
		r.store.PutArtifact(job.GameID, "game.js", []byte(gameJS), "application/javascript")
		return nil
	})
	if r.stopOnStageFailure(job) {
		return
	}

	// VALIDATE: semantic Game IR validation is authoritative; the legacy
	// static check only keeps the temporary HTML5 oracle safe during migration.
	r.runStage(job, model.StageValidate, func() error {
		if err := gameir.Validate(canonicalIR); err != nil {
			return fmt.Errorf("invalid generated Game IR: %w", err)
		}
		// Static IR validation does not verify persisted artifact bytes.
		// Fail the job before PUBLISH if a referenced hero, sound or other
		// asset is absent/corrupt; a syntactically valid missing-hero game
		// must never enter a playable feed.
		if err := validateGeneratedAssets(r.store, job.GameID, canonicalIR); err != nil {
			return fmt.Errorf("generated asset integrity: %w", err)
		}
		if game, ok := r.store.GetGame(job.GameID); ok && game.PlayableExpression != nil {
			if err := expression.ValidateAgainstGameIR(game.PlayableExpression, canonicalIR); err != nil {
				return fmt.Errorf("playable expression does not match generated Game IR: %w", err)
			}
		}
		if err := staticChecks(gameJS); err != nil {
			return fmt.Errorf("temporary HTML5 reference failed static checks: %w", err)
		}
		// No quality number is recorded here: the validate stage used
		// to stamp a hard-coded 0.85 "stub vision-judge score", a
		// fabricated measurement (architecture review 2026-10-23).
		// Quality comes from the playtest scorecard or stays absent.
		return nil
	})
	if r.stopOnStageFailure(job) {
		return
	}

	// PUBLISH: assemble the versioned bundle manifest.
	r.runStage(job, model.StagePublish, func() error {
		game, ok := r.store.GetGame(job.GameID)
		if !ok {
			return fmt.Errorf("game %s not found", job.GameID)
		}
		m := &model.BundleManifest{
			GameID:   game.ID,
			Title:    doc.Title,
			Version:  1,
			Shim:     "gameir/v1.2",
			Entry:    "game-ir.json",
			Controls: doc.Controls,
		}
		preserveParentAssets := false
		if game.ParentGameID != "" {
			if parent, found := r.store.GetGame(game.ParentGameID); found &&
				parent.Manifest != nil && parent.GameIR != nil {
				m.Assets = parent.Manifest.Assets
				m.Controls = append([]string(nil), parent.Manifest.Controls...)
				preserveParentAssets = true
			}
		}
		if !preserveParentAssets {
			for i, el := range doc.Elements {
				if i >= 4 {
					break
				}
				spriteID := fmt.Sprintf("el_%d", i+1)
				m.Assets.Sprites = append(m.Assets.Sprites, model.SpriteAsset{
					ID:     spriteID,
					Src:    "sprites/" + spriteID + ".png",
					Source: "kid-drawing",
					Label:  el.Label,
				})
			}
			// Mirror CODEGEN's fallback asset in the bundle manifest so the
			// player's image URI resolves even when the confirmed source
			// questionnaire contained no identified elements.
			if len(doc.Elements) == 0 {
				m.Assets.Sprites = append(m.Assets.Sprites, model.SpriteAsset{
					ID: "player", Src: "sprites/el_1.png", Source: "generated", Label: "friendly player",
				})
			}
			m.Assets.Sprites = append(m.Assets.Sprites, model.SpriteAsset{
				ID: "star", Src: "sprites/star.png", Source: "procedural", Label: "star",
			})
			m.Assets.Music = model.MusicAsset{Src: "audio/theme.wav", Mood: doc.MusicMood, Source: "library"}
		}
		m.Visibility = game.Visibility
		m.SharedGroupIDs = game.GroupIDs
		game.Manifest = m
		game.GameIR = canonicalIR
		game.Title = doc.Title
		game.Status = model.GameReady
		game.UpdatedAt = time.Now()
		r.store.UpdateGame(game)
		return nil
	})

	if r.stopOnStageFailure(job) {
		return
	}
	job.Status = model.JobDone
	job.UpdatedAt = time.Now()
	r.store.UpdateJob(job)
}

// stopOnStageFailure keeps all later stages strictly unreachable after a
// failure. In particular PUBLISH must never temporarily expose a ready game
// (even privately) after missing assets, invalid IR, or failed validation.
// Leaving later stages queued also makes the first actionable failure clear
// to the creator instead of overwriting it with downstream symptoms.
func (r *Runner) stopOnStageFailure(job *model.Job) bool {
	if job.Status != model.JobFailed {
		return false
	}
	r.finishFailed(job, job.Error)
	return true
}

func (r *Runner) finishFailed(job *model.Job, errMsg string) {
	job.Status = model.JobFailed
	job.Error = errMsg
	job.UpdatedAt = time.Now()
	r.store.UpdateJob(job)
	if game, ok := r.store.GetGame(job.GameID); ok {
		game.Status = model.GameFailed
		game.UpdatedAt = time.Now()
		r.store.UpdateGame(game)
	}
}

func (r *Runner) runStage(job *model.Job, name model.StageName, fn func() error) {
	i := stageIdx(job, name)
	if i < 0 {
		return
	}
	st := &job.Stages[i]
	now := time.Now()
	st.Status = model.StageRunning
	st.StartedAt = &now
	st.Provider = "stub"
	job.UpdatedAt = now
	r.store.UpdateJob(job)

	start := time.Now()
	err := fn()

	end := time.Now()
	st.EndedAt = &end
	st.LatencyMs = end.Sub(start).Milliseconds()
	st.TokensIn, st.TokensOut, st.CostUSD = 0, 0, 0 // stub: instrumented, zero cost
	if err != nil {
		st.Status = model.StageFailed
		st.Error = err.Error()
		job.Status = model.JobFailed
		job.Error = fmt.Sprintf("stage %s: %s", name, err)
	} else {
		st.Status = model.StageDone
	}
	job.UpdatedAt = end
	r.store.UpdateJob(job)
}

// validateGeneratedAssets checks persisted bytes before a game can be
// published. Canonical IR may validate even when a referenced file was
// never written (or was replaced with different bytes after its hash was
// declared); static schema checks alone cannot catch this.
func validateGeneratedAssets(st store.Store, gameID string, ir *gameir.Document) error {
	if ir == nil {
		return fmt.Errorf("missing canonical Game IR")
	}
	for _, asset := range ir.Assets {
		if strings.TrimSpace(asset.URI) == "" {
			return fmt.Errorf("asset %q has no URI", asset.ID)
		}
		data, mimeType, ok := st.GetArtifact(gameID, asset.URI)
		if !ok || len(data) == 0 {
			return fmt.Errorf("asset %q (%s) has no stored bytes", asset.ID, asset.URI)
		}
		if asset.MIMEType != "" && mimeType != asset.MIMEType {
			return fmt.Errorf("asset %q (%s) has MIME %q, expected %q", asset.ID, asset.URI, mimeType, asset.MIMEType)
		}
		if asset.SHA256 != "" {
			sum := sha256.Sum256(data)
			if !strings.EqualFold(asset.SHA256, hex.EncodeToString(sum[:])) {
				return fmt.Errorf("asset %q (%s) SHA-256 mismatch", asset.ID, asset.URI)
			}
		}
	}
	return nil
}

// staticChecks enforces the shim hard rules that don't need a browser:
// no network, no eval, Shim.end reachable.
func staticChecks(gameJS string) error {
	if gameJS == "" {
		return fmt.Errorf("empty game.js")
	}
	for _, banned := range []string{"fetch(", "XMLHttpRequest", "eval(", "import("} {
		if strings.Contains(gameJS, banned) {
			return fmt.Errorf("banned construct %q in game.js", banned)
		}
	}
	if !strings.Contains(gameJS, "Shim.end(") {
		return fmt.Errorf("game.js never calls Shim.end")
	}
	return nil
}

func designTargetFromWinCondition(doc *model.DesignDoc) (int, bool) {
	if doc == nil {
		return 0, false
	}
	for _, token := range strings.Fields(doc.WinCondition) {
		digits := strings.Builder{}
		for _, r := range token {
			if r >= '0' && r <= '9' {
				digits.WriteRune(r)
			}
		}
		if digits.Len() == 0 {
			continue
		}
		n, err := strconv.Atoi(digits.String())
		if err != nil || n < 1 || n > 100 {
			return 0, false
		}
		return n, true
	}
	return 0, false
}

func targetScoreFromDesignDoc(doc *model.DesignDoc) int {
	// The confirmed win condition is authoritative; unrelated numbers in
	// plot/mechanics text must never silently change the objective.
	if n, ok := designTargetFromWinCondition(doc); ok {
		return n
	}
	return 10
}

func buildGameIR(doc *model.DesignDoc, gameID string) *gameir.Document {
	if doc == nil {
		doc = &model.DesignDoc{Title: "A Little Adventure", Plot: "Catch falling stars.", WinCondition: "Catch 10 stars"}
	}
	target := targetScoreFromDesignDoc(doc)
	targetScore := float64(target)
	speed := 180.0
	assets := make([]gameir.Asset, 0, len(doc.Elements)+2)
	entities := make([]gameir.Entity, 0, len(doc.Elements)+2)
	for i, element := range doc.Elements {
		if i >= 4 {
			break
		}
		id := fmt.Sprintf("el_%d", i+1)
		assets = append(assets, gameir.Asset{
			ID: id, Kind: "image", URI: "sprites/" + id + ".png",
			MIMEType: "image/png", Source: "generated", AltText: element.Label,
		})
		kind := "decoration"
		components := []string{}
		if i == 0 {
			kind = "player"
			components = []string{"movement", "collider", "score"}
		}
		entities = append(entities, gameir.Entity{
			ID: id, Kind: kind, AssetID: id,
			Position: []float64{float64(100 + i*80), 690},
			Size:     []float64{64, 64}, Components: components,
			Properties: map[string]any{"sourceLabel": element.Label, "sourceKind": element.Kind},
		})
	}
	// A generated design should always have a controllable player, even if
	// a caller confirms a design document with no extracted elements.
	playerID := "el_1"
	if len(doc.Elements) == 0 {
		playerID = "player"
		assets = append(assets, gameir.Asset{ID: playerID, Kind: "image", URI: "sprites/el_1.png", MIMEType: "image/png", Source: "generated", AltText: "player"})
		entities = append(entities, gameir.Entity{ID: playerID, Kind: "player", AssetID: playerID, Position: []float64{195, 700}, Size: []float64{64, 64}, Components: []string{"movement", "collider", "score"}})
	}
	assets = append(assets,
		gameir.Asset{ID: "star", Kind: "image", URI: "sprites/star.png", MIMEType: "image/png", Source: "generated", AltText: "collectible star"},
		gameir.Asset{ID: "theme", Kind: "audio", URI: "audio/theme.wav", MIMEType: "audio/wav", Source: "generated", AltText: "background music"},
	)
	// Two initial pickups make the first interaction immediate. A template
	// plus timer continuously supplies more stars until the design target is
	// reached; collecting one never makes the objective permanently unreachable.
	for i, pos := range [][]float64{{120, 250}, {270, 180}} {
		entities = append(entities, gameir.Entity{
			ID: fmt.Sprintf("star-%d", i+1), Kind: "pickup", AssetID: "star",
			Position: pos, Size: []float64{30, 30}, Components: []string{"collider", "collectible"},
			Behavior: &gameir.Behavior{Type: "fall", Speed: &speed},
		})
	}
	starTemplate := gameir.Entity{
		ID: "star-pickup-template", Kind: "pickup", AssetID: "star",
		Position: []float64{195, 120}, Size: []float64{30, 30},
		Components: []string{"collider", "collectible"},
		Behavior:   &gameir.Behavior{Type: "fall", Speed: &speed},
	}
	collectActions := []gameir.Action{
		{Type: "add_score", Value: 1},
		{Type: "destroy_entity"},
	}
	rules := []gameir.Rule{
		{ID: "start-theme", When: gameir.Condition{Event: "game_start"}, Actions: []gameir.Action{{Type: "play_sound", AssetID: "theme"}}},
		{ID: "collect-star-1", When: gameir.Condition{Event: "collected", Source: playerID, Target: "star-1"}, Actions: collectActions},
		{ID: "collect-star-2", When: gameir.Condition{Event: "collected", Source: playerID, Target: "star-2"}, Actions: collectActions},
		{ID: "collect-spawned-star", When: gameir.Condition{Event: "collected", Source: playerID, Target: "star-pickup-template"}, Actions: collectActions},
		{ID: "spawn-falling-stars", When: gameir.Condition{Event: "timer_elapsed", Value: floatPtr(0.7)}, Actions: []gameir.Action{{Type: "spawn_entity", Target: "star-pickup-template"}}},
		{ID: "finish-game", When: gameir.Condition{Event: "score_reached", Source: playerID, Value: &targetScore}, Actions: []gameir.Action{{Type: "show_text", Value: "You did it!"}, {Type: "end_game", Value: true}}},
	}
	controlsHint := strings.TrimSpace(doc.Mechanics)
	if len(controlsHint) > 80 {
		controlsHint = controlsHint[:80]
	}
	objectiveLabel := strings.TrimSpace(doc.WinCondition)
	if _, ok := designTargetFromWinCondition(doc); !ok || objectiveLabel == "" {
		objectiveLabel = fmt.Sprintf("Catch %d stars", target)
	}
	if len(objectiveLabel) > 60 {
		objectiveLabel = objectiveLabel[:60]
	}
	failureMode := "gentle"
	if strings.Contains(strings.ToLower(doc.LoseCondition), "none") || strings.Contains(strings.ToLower(doc.LoseCondition), "endless") {
		failureMode = "none"
	}
	return &gameir.Document{
		IRVersion:    gameir.Version12,
		Capabilities: []string{"shell.v1", "audio"},
		Game: gameir.Game{
			ID: gameID, Title: doc.Title, Description: doc.Plot,
			EntryScene: "main", Orientation: "portrait",
			SessionTargetSeconds: 90, Audience: "family",
		},
		Assets:        assets,
		Scenes:        []gameir.Scene{{ID: "main", Entities: entities, Templates: []gameir.Entity{starTemplate}}},
		InputBindings: []gameir.InputBinding{{Action: "move", Input: "drag"}, {Action: "move", Input: "tap"}},
		Rules:         rules,
		Experience: &gameir.Experience{
			Archetype:   "catcher",
			Controls:    gameir.ExpControls{Scheme: "drag_x", Hint: controlsHint},
			Objective:   gameir.ExpObjective{Kind: "collect", Target: target, Label: objectiveLabel},
			HUD:         gameir.ExpHUD{Left: "objective_progress", Right: "score"},
			Start:       gameir.ExpStart{Mode: "tap_to_start"},
			Failure:     gameir.ExpFailure{Mode: failureMode},
			Pacing:      gameir.ExpPacing{TargetSessionSec: 90, FirstRewardWithinSec: 3, ExpectedCatchRate: 0.7, Ramp: []gameir.ExpRampStep{{At: 0, SpawnIntervalMs: 700, SpeedMul: 1}, {At: 0.65, SpawnIntervalMs: 550, SpeedMul: 1.15}}},
			Result:      gameir.ExpResult{Win: gameir.ExpResultSide{Title: "You did it!"}},
			Leaderboard: gameir.ExpLeaderboard{Metric: "score", Order: "desc"},
			Feedback:    gameir.ExpFeedback{Juice: "standard"},
		},
		Metadata: gameir.Metadata{Tags: []string{"generated", "collecting"}},
	}
}

func floatPtr(v float64) *float64 { return &v }

// buildGenericRemixIR applies a metadata delta to a canonical parent IR.
// Gameplay scenes, entities, assets, bindings, experience, and rules are
// copied intact so a remix does not silently replace its parent's game loop.
// buildRemixIR always copies the parent's canonical gameplay contract.
// A legacy manifest without Game IR is unsupported rather than silently
// compiling a profile-specific substitute that changes the parent's loop.
func buildRemixIR(doc *model.DesignDoc, child, parent *model.Game) (*gameir.Document, error) {
	return buildGenericRemixIR(doc, child, parent)
}

func buildGenericRemixIR(doc *model.DesignDoc, child, parent *model.Game) (*gameir.Document, error) {
	if parent == nil || parent.GameIR == nil || parent.Manifest == nil {
		return nil, fmt.Errorf("remix parent must have a published canonical Game IR")
	}
	encoded, err := json.Marshal(parent.GameIR)
	if err != nil {
		return nil, fmt.Errorf("encode remix parent Game IR: %w", err)
	}
	var remix gameir.Document
	if err := json.Unmarshal(encoded, &remix); err != nil {
		return nil, fmt.Errorf("copy remix parent Game IR: %w", err)
	}
	remix.Game.ID = child.ID
	if doc != nil && strings.TrimSpace(doc.Title) != "" {
		remix.Game.Title = strings.TrimSpace(doc.Title)
	}
	if doc != nil && strings.TrimSpace(doc.Plot) != "" {
		remix.Game.Description = strings.TrimSpace(doc.Plot)
	}
	remix.Metadata.ParentGameID = parent.ID
	remix.Metadata.ParentVersion = parent.Manifest.Version
	foundRemixTag := false
	for _, tag := range remix.Metadata.Tags {
		if tag == "remix" {
			foundRemixTag = true
			break
		}
	}
	if !foundRemixTag {
		remix.Metadata.Tags = append(remix.Metadata.Tags, "remix")
	}
	return &remix, nil
}
