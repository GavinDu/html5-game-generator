package pipeline

import (
	"path/filepath"
	"strings"
	"testing"

	"doodlequest/internal/model"
	"doodlequest/internal/store"
)

func TestRecoverInterruptedJobsAfterSnapshotRestore(t *testing.T) {
	path := filepath.Join(t.TempDir(), "creator-state.json")
	before := store.NewMemoryStore()
	if err := before.EnablePersistence(path); err != nil {
		t.Fatal(err)
	}
	fixtures := []struct {
		id         string
		jobStatus  model.JobStatus
		gameStatus model.GameStatus
		stage      model.StageStatus
	}{
		{"queued-creator", model.JobQueued, model.GameDraft, model.StagePending},
		{"running-creator", model.JobRunning, model.GameGenerating, model.StageRunning},
		{"questionnaire-creator", model.JobAwaitingConfirm, model.GameGenerating, model.StageDone},
		{"finished-creator", model.JobDone, model.GameReady, model.StageDone},
		// An interrupted job must never remove an already-published version
		// if shutdown happened after PUBLISH but before the JobDone marker.
		{"published-creator", model.JobRunning, model.GameReady, model.StageDone},
	}
	for _, item := range fixtures {
		before.CreateGame(&model.Game{ID: item.id, Status: item.gameStatus, Title: item.id})
		before.CreateJob(&model.Job{ID: "job-" + item.id, GameID: item.id, Status: item.jobStatus,
			Stages: []model.StageRecord{{Name: model.StageSpec, Status: item.stage}},
		})
	}

	after := store.NewMemoryStore()
	if err := after.EnablePersistence(path); err != nil {
		t.Fatalf("restore durable creator snapshot: %v", err)
	}
	if count := RecoverInterruptedJobs(after); count != 3 {
		t.Fatalf("recovered %d interrupted jobs, expected 3", count)
	}
	for _, item := range fixtures {
		game, exists := after.GetGame(item.id)
		if !exists {
			t.Fatalf("missing creator game %s", item.id)
		}
		job, exists := after.GetJob("job-" + item.id)
		if !exists {
			t.Fatalf("missing generation job %s", item.id)
		}
		wasInterrupted := item.jobStatus == model.JobRunning || item.jobStatus == model.JobQueued
		if wasInterrupted {
			if job.Status != model.JobFailed || !strings.Contains(job.Error, "interrupted") {
				t.Errorf("%s: restart must leave actionable failed job: %+v", item.id, job)
			}
			if item.stage == model.StageRunning {
				if job.Stages[0].Status != model.StageFailed || job.Stages[0].EndedAt == nil {
					t.Errorf("%s: active stage remains stuck: %+v", item.id, job.Stages[0])
				}
			} else if job.Stages[0].Status != item.stage {
				t.Errorf("%s: should not fabricate a started stage", item.id)
			}
		} else if job.Status != item.jobStatus {
			t.Errorf("%s: restart changed resumable/completed job: %s", item.id, job.Status)
		}
		wantGameStatus := item.gameStatus
		if wasInterrupted && item.gameStatus != model.GameReady {
			wantGameStatus = model.GameFailed
		}
		if game.Status != wantGameStatus {
			t.Errorf("%s: game status %s, want %s", item.id, game.Status, wantGameStatus)
		}
	}
	if count := RecoverInterruptedJobs(after); count != 0 {
		t.Fatalf("recovery must be idempotent, second call returned %d", count)
	}

	// Verify recovery writes are durable, not only mutations in memory.
	reloaded := store.NewMemoryStore()
	if err := reloaded.EnablePersistence(path); err != nil {
		t.Fatal(err)
	}
	if queued, _ := reloaded.GetJob("job-queued-creator"); queued.Status != model.JobFailed {
		t.Fatalf("interrupted queued job did not survive next restart: %+v", queued)
	}
	if confirmation, _ := reloaded.GetJob("job-questionnaire-creator"); confirmation.Status != model.JobAwaitingConfirm {
		t.Fatalf("the approved design must remain resumable: %+v", confirmation)
	}
}
