package pipeline

import (
	"fmt"
	"sync"
	"testing"
	"time"

	"doodlequest/internal/model"
	"doodlequest/internal/store"
)

// Two fast taps on Confirm (or a client HTTP retry) previously found the
// same awaiting_confirm status and started separate ART -> PUBLISH goroutines.
// Only one request may win, including its confirmed questionnaire edits.
func TestConfirmConcurrentRetriesLaunchExactlyOneGeneration(t *testing.T) {
	st := store.NewMemoryStore()
	game := &model.Game{
		ID:       "concurrent-confirm-child",
		Title:    "Untitled",
		OwnerID:  "original-creator",
		Status:   model.GameGenerating,
		Elements: []model.Element{{ID: "el_1", Label: "bear", Kind: "character"}},
		Intent:   "Catch stars with the bear",
	}
	st.CreateGame(game)
	runner := NewRunner(st)
	created := runner.CreateAndStart(game, game.Elements, game.Intent, nil)
	awaitConfirmJobStatus(t, st, created.ID, model.JobAwaitingConfirm)
	pending, ok := st.GetJob(created.ID)
	if !ok || pending.DesignDoc == nil {
		t.Fatal("source questionnaire missing")
	}

	type outcome struct {
		title string
		err   error
	}
	const retries = 32
	outcomes := make(chan outcome, retries)
	var wg sync.WaitGroup
	wg.Add(retries)
	for i := 0; i < retries; i++ {
		i := i
		go func() {
			defer wg.Done()
			confirmed := *pending.DesignDoc
			confirmed.Title = fmt.Sprintf("Winner of confirm %02d", i)
			_, err := runner.Confirm(game.ID, &confirmed)
			outcomes <- outcome{title: confirmed.Title, err: err}
		}()
	}
	wg.Wait()
	close(outcomes)
	winners := []string{}
	for result := range outcomes {
		if result.err == nil {
			winners = append(winners, result.title)
		}
	}
	if len(winners) != 1 {
		t.Fatalf("concurrent confirmations accepted %d, expected exactly one: %#v", len(winners), winners)
	}
	final := awaitConfirmJobStatus(t, st, created.ID, model.JobDone)
	if final.Status != model.JobDone {
		t.Fatalf("job reached %s", final.Status)
	}
	published, ok := st.GetGame(game.ID)
	if !ok || published.Status != model.GameReady || published.Manifest == nil {
		t.Fatalf("winning confirmation did not publish exactly one ready game: %+v", published)
	}
	if published.Title != winners[0] {
		t.Fatalf("published title %q does not match sole accepted edit %q", published.Title, winners[0])
	}
	// A late retry must remain rejected after the job has completed.
	if _, err := runner.Confirm(game.ID, nil); err == nil {
		t.Fatal("completed job was confirmed again")
	}
}

func awaitConfirmJobStatus(t *testing.T, st store.Store, id string, wanted model.JobStatus) *model.Job {
	t.Helper()
	deadline := time.Now().Add(8 * time.Second)
	for time.Now().Before(deadline) {
		job, ok := st.GetJob(id)
		if ok && job.Status == wanted {
			return job
		}
		if ok && job.Status == model.JobFailed {
			t.Fatalf("job failed before %s: %s", wanted, job.Error)
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("job %s never reached %s", id, wanted)
	return nil
}
