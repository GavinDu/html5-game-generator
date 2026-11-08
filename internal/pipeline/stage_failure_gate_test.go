package pipeline

import (
	"strings"
	"testing"
	"time"

	"doodlequest/internal/gameir"
	"doodlequest/internal/model"
	"doodlequest/internal/store"
)

// The generation state machine must not continue to codegen/validate/publish
// when ART fails. A broken remix used to run all later stages and briefly
// mark GameReady with an incomplete package before finishFailed reset it.
// Such a transient ready state can leak into a concurrent public feed read.
func TestArtFailureCannotReachPublishOrReplaceOriginalError(t *testing.T) {
	st := store.NewMemoryStore()
	parent := &model.Game{
		ID:       "parent-with-missing-asset",
		Title:    "Parent",
		Status:   model.GameReady,
		Manifest: &model.BundleManifest{Version: 1},
		GameIR: &gameir.Document{
			Game: gameir.Game{ID: "parent-with-missing-asset"},
			Assets: []gameir.Asset{{
				ID:   "source-drawing",
				URI:  "sprites/missing.png",
				Kind: "image",
			}},
		},
		Elements: []model.Element{{ID: "el_1", Label: "heart", Kind: "character"}},
		Intent:   "Collect the glowing hearts",
	}
	st.CreateGame(parent)
	child := &model.Game{
		ID:           "broken-remix-child",
		ParentGameID: parent.ID,
		Elements:     append([]model.Element(nil), parent.Elements...),
		Intent:       parent.Intent,
		Status:       model.GameGenerating,
	}
	st.CreateGame(child)
	runner := NewRunner(st)
	job := runner.CreateAndStart(child, child.Elements, child.Intent, nil)
	waitPipelineStatus(t, st, job.ID, model.JobAwaitingConfirm)
	if _, err := runner.Confirm(child.ID, nil); err != nil {
		t.Fatalf("confirm: %v", err)
	}
	failed := waitPipelineStatus(t, st, job.ID, model.JobFailed)
	if !strings.Contains(failed.Error, "stage art:") ||
		!strings.Contains(failed.Error, "missing") {
		t.Fatalf("first actionable ART failure overwritten: %q", failed.Error)
	}
	statusByStage := map[model.StageName]model.StageStatus{}
	for _, stage := range failed.Stages {
		statusByStage[stage.Name] = stage.Status
	}
	if statusByStage[model.StageSpec] != model.StageDone ||
		statusByStage[model.StageArt] != model.StageFailed {
		t.Fatalf("unexpected first-stage outcomes: %#v", statusByStage)
	}
	for _, stage := range []model.StageName{
		model.StageCodegen, model.StageValidate, model.StagePublish,
	} {
		if statusByStage[stage] != model.StagePending {
			t.Fatalf("later stage %s should remain pending, got %s", stage, statusByStage[stage])
		}
	}
	storedChild, ok := st.GetGame(child.ID)
	if !ok || storedChild.Status != model.GameFailed ||
		storedChild.Manifest != nil || storedChild.GameIR != nil {
		t.Fatalf("broken remix published or retained a manifest: %+v", storedChild)
	}
	if _, _, found := st.GetArtifact(child.ID, "game-ir.json"); found {
		t.Fatal("CODEGEN ran despite ART failure")
	}
}

func waitPipelineStatus(t *testing.T, st store.Store, jobID string, want model.JobStatus) *model.Job {
	t.Helper()
	deadline := time.Now().Add(6 * time.Second)
	for time.Now().Before(deadline) {
		job, found := st.GetJob(jobID)
		if found && job.Status == want {
			return job
		}
		if found && job.Status == model.JobFailed && want != model.JobFailed {
			t.Fatalf("job failed before %s: %s", want, job.Error)
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("job %s did not reach %s", jobID, want)
	return nil
}
