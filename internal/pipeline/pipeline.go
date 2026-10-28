// Package pipeline runs the async generation job: spec -> (user confirm) ->
// art -> codegen -> validate -> publish. Stage implementations are stubs
// with the real interfaces; each records latency/tokens/cost.
package pipeline

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"strings"
	"strconv"
	"time"

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
	store store.Store
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

func capTitle(s string, n int) string {
	s = strings.TrimSpace(s)
	if len(s) > n {
		s = s[:n]
	}
	return s
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
			job.DesignDoc.Title = "Bobo's Rainbow: Heart Rescue"
			job.DesignDoc.Plot = "Help Bobo restore the rainbow by collecting three magic hearts so the kitten can cross safely."
			job.DesignDoc.Mechanics = "Drag Bobo to collect three hearts. Each heart restores part of the rainbow; collecting all three wins."
			job.DesignDoc.Controls = []string{"drag", "tap"}
			job.DesignDoc.WinCondition = "Collect three magic hearts"
			job.DesignDoc.LoseCondition = "No lose condition; play at your own pace."
			job.DesignDoc.MusicMood = "calm"
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
			needed := map[string]bool{"bear": true, "kitten": true, "heart": true}
			for i := 0; i < 6; i++ {
				needed[fmt.Sprintf("band%d", i)] = true
			}
			for _, sprite := range parent.Manifest.Assets.Sprites {
				if !needed[sprite.ID] {
					continue
				}
				data, contentType, exists := r.store.GetArtifact(parent.ID, sprite.Src)
				if !exists {
					return fmt.Errorf("remix parent asset %q (%s) is missing", sprite.ID, sprite.Src)
				}
				r.store.PutArtifact(job.GameID, sprite.Src, data, contentType)
			}
			if music := parent.Manifest.Assets.Music; music.Src != "" {
				if data, contentType, exists := r.store.GetArtifact(parent.ID, music.Src); exists {
					r.store.PutArtifact(job.GameID, music.Src, data, contentType)
				}
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
		r.store.PutArtifact(job.GameID, "sprites/star.png", media.StarPNG(), "image/png")
		if game, ok := r.store.GetGame(job.GameID); !ok || game.ParentGameID == "" {
			r.store.PutArtifact(job.GameID, "audio/theme.wav", media.MusicWAV(doc.MusicMood), "audio/wav")
		}
		return nil
	})

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
			canonicalIR, err = buildBoboRemixIR(doc, child, parent)
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

	// VALIDATE: semantic Game IR validation is authoritative; the legacy
	// static check only keeps the temporary HTML5 oracle safe during migration.
	r.runStage(job, model.StageValidate, func() error {
		if err := gameir.Validate(canonicalIR); err != nil {
			return fmt.Errorf("invalid generated Game IR: %w", err)
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

	// PUBLISH: assemble the versioned bundle manifest.
	r.runStage(job, model.StagePublish, func() error {
		game, ok := r.store.GetGame(job.GameID)
		if !ok {
			return fmt.Errorf("game %s not found", job.GameID)
		}
		m := &model.BundleManifest{
			GameID:   game.ID,
			Title:    doc.Title,
			Version:  2,
			Shim:     "gameir/v1.2",
			Entry:    "game-ir.json",
			Controls: doc.Controls,
		}
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
		m.Assets.Sprites = append(m.Assets.Sprites, model.SpriteAsset{
			ID: "star", Src: "sprites/star.png", Source: "procedural", Label: "star",
		})
		m.Assets.Music = model.MusicAsset{Src: "audio/theme.wav", Mood: doc.MusicMood, Source: "library"}
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

	if job.Status == model.JobFailed {
		r.finishFailed(job, job.Error)
		return
	}
	job.Status = model.JobDone
	job.UpdatedAt = time.Now()
	r.store.UpdateJob(job)
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

func targetScoreFromDesignDoc(doc *model.DesignDoc) int {
	// The confirmed design document is authoritative. Extract the first
	// positive integer from its win condition (for example, "Catch 10 stars").
	// Keep targets bounded so malformed creator text cannot generate huge
	// entities or a game that is practically impossible to finish.
	if doc != nil {
		for _, field := range []string{doc.WinCondition, doc.Plot, doc.Mechanics} {
			for _, token := range strings.Fields(field) {
				digits := strings.Builder{}
				for _, r := range token {
					if r >= '0' && r <= '9' {
						digits.WriteRune(r)
					}
				}
				if digits.Len() == 0 {
					continue
				}
				if n, err := strconv.Atoi(digits.String()); err == nil && n >= 1 && n <= 100 {
					return n
				}
			}
		}
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
			Size: []float64{64, 64}, Components: components,
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
		Behavior: &gameir.Behavior{Type: "fall", Speed: &speed},
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
	if objectiveLabel == "" {
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
		IRVersion: gameir.Version12,
		Capabilities: []string{"shell.v1", "audio"},
		Game: gameir.Game{
			ID: gameID, Title: doc.Title, Description: doc.Plot,
			EntryScene: "main", Orientation: "portrait",
			SessionTargetSeconds: 90, Audience: "family",
		},
		Assets: assets,
		Scenes: []gameir.Scene{{ID: "main", Entities: entities, Templates: []gameir.Entity{starTemplate}}},
		InputBindings: []gameir.InputBinding{{Action: "move", Input: "drag"}, {Action: "move", Input: "tap"}},
		Rules: rules,
		Experience: &gameir.Experience{
			Archetype: "catcher",
			Controls: gameir.ExpControls{Scheme: "drag_x", Hint: controlsHint},
			Objective: gameir.ExpObjective{Kind: "collect", Target: target, Label: objectiveLabel},
			HUD: gameir.ExpHUD{Left: "objective_progress", Right: "score"},
			Start: gameir.ExpStart{Mode: "tap_to_start"},
			Failure: gameir.ExpFailure{Mode: failureMode},
			Pacing: gameir.ExpPacing{TargetSessionSec: 90, FirstRewardWithinSec: 3, ExpectedCatchRate: 0.7, Ramp: []gameir.ExpRampStep{{At: 0, SpawnIntervalMs: 700, SpeedMul: 1}, {At: 0.65, SpawnIntervalMs: 550, SpeedMul: 1.15}}},
			Result: gameir.ExpResult{Win: gameir.ExpResultSide{Title: "You did it!"}},
			Leaderboard: gameir.ExpLeaderboard{Metric: "score", Order: "desc"},
			Feedback: gameir.ExpFeedback{Juice: "standard"},
		},
		Metadata: gameir.Metadata{Tags: []string{"generated", "collecting"}},
	}
}

func floatPtr(v float64) *float64 { return &v }

// buildBoboRemixIR creates a playable variant from the original legacy Bobo's
// Rainbow inventory entry. It reuses parent assets and expresses new behavior
// as Game IR data rather than downloaded executable code.
func buildBoboRemixIR(doc *model.DesignDoc, child, parent *model.Game) (*gameir.Document, error) {
	if parent.ID != "sample-rainbow-bear" || parent.Manifest == nil {
		return nil, fmt.Errorf("no Game IR remix profile for parent %q", parent.ID)
	}
	sprites := make(map[string]model.SpriteAsset, len(parent.Manifest.Assets.Sprites))
	for _, sprite := range parent.Manifest.Assets.Sprites {
		sprites[sprite.ID] = sprite
	}
	ids := []string{"bear", "kitten", "heart", "band0", "band1", "band2", "band3", "band4", "band5"}
	assets := make([]gameir.Asset, 0, 10)
	for _, id := range ids {
		sprite, ok := sprites[id]
		if !ok {
			return nil, fmt.Errorf("Bobo remix parent is missing sprite %q", id)
		}
		source := "generated"
		if sprite.Source == "kid-drawing" {
			source = "user_drawing"
		}
		assets = append(assets, gameir.Asset{ID: id, Kind: "image", URI: sprite.Src, MIMEType: "image/png", Source: source, AltText: sprite.Label})
	}
	music := parent.Manifest.Assets.Music
	if music.Src != "" {
		source := "generated"
		if music.Source == "library" {
			source = "licensed_library"
		}
		assets = append(assets, gameir.Asset{ID: "theme", Kind: "audio", URI: music.Src, MIMEType: "audio/wav", Source: source, AltText: "Original calm theme"})
	}
	entities := []gameir.Entity{
		{ID: "player", Kind: "player", AssetID: "bear", Position: []float64{195, 650}, Size: []float64{100, 120}, Components: []string{"movement", "collider", "score"}},
		{ID: "kitten", Kind: "decoration", AssetID: "kitten", Position: []float64{320, 650}, Size: []float64{70, 84}, Components: []string{}},
	}
	for i := 0; i < 6; i++ {
		entities = append(entities, gameir.Entity{ID: fmt.Sprintf("rainbow-band-%d", i), Kind: "decoration", AssetID: fmt.Sprintf("band%d", i), Position: []float64{195, float64(350 + i*16)}, Size: []float64{220, 14}, Components: []string{}})
	}
	for i, pos := range [][]float64{{90, 250}, {195, 210}, {300, 250}} {
		entities = append(entities, gameir.Entity{ID: fmt.Sprintf("heart-%d", i+1), Kind: "pickup", AssetID: "heart", Position: pos, Size: []float64{42, 42}, Components: []string{"collider", "collectible"}})
	}
	rules := make([]gameir.Rule, 0, 4)
	for i := 1; i <= 3; i++ {
		target := fmt.Sprintf("heart-%d", i)
		rules = append(rules, gameir.Rule{ID: "collect-" + target, When: gameir.Condition{Event: "collected", Source: "player", Target: target}, Actions: []gameir.Action{{Type: "add_score", Target: "player", Value: 1}, {Type: "destroy_entity", Target: target}}})
	}
	threshold := 3.0
	rules = append(rules, gameir.Rule{ID: "restore-rainbow", When: gameir.Condition{Event: "score_reached", Source: "player", Value: &threshold}, Actions: []gameir.Action{{Type: "show_text", Value: "The rainbow shines again! The kitten can cross."}, {Type: "end_game", Value: true}}})
	return &gameir.Document{
		IRVersion:     gameir.Version,
		Game:          gameir.Game{ID: child.ID, Title: doc.Title, Description: doc.Plot, EntryScene: "rainbow-meadow", Orientation: "portrait", SessionTargetSeconds: 60, Audience: "family"},
		Assets:        assets,
		Scenes:        []gameir.Scene{{ID: "rainbow-meadow", Entities: entities}},
		InputBindings: []gameir.InputBinding{{Action: "move", Input: "drag"}, {Action: "move", Input: "tap"}},
		Rules:         rules,
		Metadata:      gameir.Metadata{ParentGameID: parent.ID, ParentVersion: parent.Manifest.Version, Tags: []string{"remix", "bobo", "collecting", "family"}},
	}, nil
}
