// Package pipeline runs the async generation job: spec -> (user confirm) ->
// art -> codegen -> validate -> publish. Stage implementations are stubs
// with the real interfaces; each records latency/tokens/cost.
package pipeline

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"strings"
	"time"

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

// simulateMs keeps the demo honest about async behavior without slow tests.
var simulateMs = map[model.StageName]int64{
	model.StageSpec:     300,
	model.StageArt:      500,
	model.StageCodegen:  400,
	model.StageValidate: 250,
	model.StagePublish:  150,
}

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
		for i, el := range doc.Elements {
			if i >= 4 {
				break
			}
			spriteID := fmt.Sprintf("el_%d", i+1)
			r.store.PutArtifact(job.GameID, "sprites/"+spriteID+".png",
				media.SpritePNG(el.Label), "image/png")
		}
		r.store.PutArtifact(job.GameID, "sprites/star.png", media.StarPNG(), "image/png")
		r.store.PutArtifact(job.GameID, "audio/theme.wav",
			media.MusicWAV(doc.MusicMood), "audio/wav")
		return nil
	})

	// CODEGEN: design doc -> game.js against GameShim v1.
	var gameJS string
	r.runStage(job, model.StageCodegen, func() error {
		playerSprite := "el_1"
		gameJS = shim.BuildGameJS(doc, playerSprite)
		r.store.PutArtifact(job.GameID, "game.js", []byte(gameJS), "application/javascript")
		return nil
	})

	// VALIDATE: static safety + contract checks (stub for headless playtest).
	r.runStage(job, model.StageValidate, func() error {
		if err := staticChecks(gameJS); err != nil {
			return err
		}
		q := 0.85 // stub vision-judge score
		job.Stages[stageIdx(job, model.StageValidate)].Quality = &q
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
			Version:  1,
			Shim:     "gameshim/v1",
			Entry:    "game.js",
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
	if ms, ok := simulateMs[name]; ok && ms > 0 {
		time.Sleep(time.Duration(ms) * time.Millisecond)
	}
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
