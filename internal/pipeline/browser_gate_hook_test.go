package pipeline

import (
	"context"
	"errors"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"doodlequest/internal/gameir"
	"doodlequest/internal/model"
	"doodlequest/internal/store"
)

func startBrowserGateJob(t *testing.T, runner *Runner, st store.Store, id string) string {
	t.Helper()
	game := &model.Game{
		ID: id, Title: "Prepublish game", Status: model.GameGenerating,
		Visibility: model.VisibilityPrivate, Intent: "A small tap-to-collect game",
	}
	st.CreateGame(game)
	job := runner.CreateAndStart(game, nil, game.Intent, nil)
	waitPipelineStatus(t, st, job.ID, model.JobAwaitingConfirm)
	if _, err := runner.Confirm(game.ID, nil); err != nil {
		t.Fatalf("confirm: %v", err)
	}
	return job.ID
}

func TestRequiredBrowserGateRejectsMissingAdapterBeforePublish(t *testing.T) {
	st := store.NewMemoryStore()
	runner := NewRunner(st)
	runner.ConfigureBrowserGate(nil, true)
	jobID := startBrowserGateJob(t, runner, st, "browser-gate-not-installed")
	failed := waitPipelineStatus(t, st, jobID, model.JobFailed)
	if !strings.Contains(failed.Error, "browser playtest gate required") {
		t.Fatalf("missing required adapter was ignored: %q", failed.Error)
	}
	checkPublishNeverRan(t, st, failed)
}

func TestBrowserGateRejectsInconclusiveGameAndCannotPublish(t *testing.T) {
	st := store.NewMemoryStore()
	runner := NewRunner(st)
	var calls atomic.Int32
	runner.ConfigureBrowserGate(func(_ context.Context, gameID string, ir *gameir.Document) error {
		calls.Add(1)
		if gameID != "browser-gate-rejected" || ir == nil || ir.Game.ID != gameID {
			return errors.New("wrong canonical IR delivered")
		}
		return errors.New("webkit report omitted actual input provenance")
	}, true)
	jobID := startBrowserGateJob(t, runner, st, "browser-gate-rejected")
	failed := waitPipelineStatus(t, st, jobID, model.JobFailed)
	if calls.Load() != 1 || !strings.Contains(failed.Error, "webkit report omitted actual input provenance") {
		t.Fatalf("failed browser evidence did not gate publication: calls=%d error=%q", calls.Load(), failed.Error)
	}
	checkPublishNeverRan(t, st, failed)
}

func TestBrowserGateMustFinishSuccessfullyBeforePublish(t *testing.T) {
	st := store.NewMemoryStore()
	runner := NewRunner(st)
	entered := make(chan struct{}, 1)
	release := make(chan struct{})
	defer func() {
		select {
		case <-release:
		default:
			close(release)
		}
	}()
	runner.ConfigureBrowserGate(func(_ context.Context, gameID string, ir *gameir.Document) error {
		if gameID != "browser-gate-accepted" || ir == nil || ir.Game.ID != gameID {
			return errors.New("unexpected staged Game IR")
		}
		entered <- struct{}{}
		<-release
		return nil
	}, true)
	jobID := startBrowserGateJob(t, runner, st, "browser-gate-accepted")
	select {
	case <-entered:
	case <-time.After(5 * time.Second):
		t.Fatal("prepublish browser gate was never called")
	}
	g, _ := st.GetGame("browser-gate-accepted")
	if g.Status == model.GameReady || g.Manifest != nil || g.GameIR != nil {
		t.Fatalf("game became publicly ready while browser validation was blocked: %+v", g)
	}
	close(release)
	waitPipelineStatus(t, st, jobID, model.JobDone)
	g, _ = st.GetGame("browser-gate-accepted")
	if g.Status != model.GameReady || g.Manifest == nil || g.GameIR == nil {
		t.Fatalf("successful required gate did not allow normal publish: %+v", g)
	}
}

func TestBrowserGateDeadlineFailsClosedEvenWhenAdapterIgnoresContext(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
	defer cancel()
	release := make(chan struct{})
	defer close(release)
	started := time.Now()
	err := runBrowserGate(ctx, func(_ context.Context, _ string, _ *gameir.Document) error {
		<-release // simulate a broken browser worker that ignores cancellation
		return nil
	}, "slow-game", &gameir.Document{})
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("an uncooperative browser worker must time out, got %v", err)
	}
	if time.Since(started) > 2*time.Second {
		t.Fatal("blocked browser worker prevented bounded validation return")
	}
}

func checkPublishNeverRan(t *testing.T, st store.Store, failed *model.Job) {
	t.Helper()
	for _, stage := range failed.Stages {
		if stage.Name == model.StageValidate && stage.Status != model.StageFailed {
			t.Fatalf("browser validation must fail VALIDATE, not a later stage: %+v", stage)
		}
		if stage.Name == model.StagePublish && stage.Status != model.StagePending {
			t.Fatalf("PUBLISH executed despite failed browser gate: %+v", stage)
		}
	}
	game, ok := st.GetGame(failed.GameID)
	if !ok || game.Status != model.GameFailed || game.Manifest != nil || game.GameIR != nil {
		t.Fatalf("failed browser gate leaked ready game: %+v", game)
	}
}
