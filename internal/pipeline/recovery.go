package pipeline

import (
	"time"

	"doodlequest/internal/model"
	"doodlequest/internal/store"
)

const interruptedJobMessage = "Generation interrupted by backend restart; start a new generation."

// RecoverInterruptedJobs is called exactly once after loading durable state,
// before the HTTP server accepts requests or starts a new generation worker.
//
// Go goroutines cannot survive a process restart. Leaving a persisted job in
// running/queued state traps the creator in a spinner forever. Fail these jobs
// explicitly without touching completed or awaiting-confirmation work.
// In particular, awaiting_confirm retains the questionnaire and can still be
// confirmed after the restart.
func RecoverInterruptedJobs(st store.Store) int {
	now := time.Now()
	recovered := 0
	for _, job := range st.ListJobs() {
		if job.Status != model.JobRunning && job.Status != model.JobQueued {
			continue
		}
		job.Status = model.JobFailed
		job.Error = interruptedJobMessage
		job.UpdatedAt = now
		for i := range job.Stages {
			if job.Stages[i].Status == model.StageRunning {
				job.Stages[i].Status = model.StageFailed
				job.Stages[i].Error = interruptedJobMessage
				job.Stages[i].EndedAt = &now
			}
		}
		st.UpdateJob(job)
		// Never demote a published ready version: a shutdown between the
		// PUBLISH commit and JobDone marker can leave a valid release visible.
		// Only unpublished drafts/generating games are marked failed.
		if game, exists := st.GetGame(job.GameID); exists &&
			(game.Status == model.GameDraft || game.Status == model.GameGenerating) {
			game.Status = model.GameFailed
			game.UpdatedAt = now
			st.UpdateGame(game)
		}
		recovered++
	}
	return recovered
}
