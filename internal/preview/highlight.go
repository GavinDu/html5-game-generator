// Package preview contains deterministic preview-selection helpers.
package preview

const DefaultWindowMS = 6000

// Event is the small, replay-derived event shape exposed by the browser
// runtime and playtest worker.
type Event struct {
	TimeMS int    `json:"t"`
	Type   string `json:"type"`
	Detail string `json:"detail,omitempty"`
}

// Window identifies the half-open time range to render from a run.
type Window struct {
	StartMS int `json:"start_ms"`
	EndMS   int `json:"end_ms"`
}

// PickWindow chooses the same preview window for the same event trace. The
// first real interaction anchors the fallback. When a meaningful event exists,
// the window is centered slightly before the strongest event, while preserving
// the run bounds and the interaction anchor.
func PickWindow(events []Event, runDurationMS int) Window {
	if runDurationMS < 0 {
		runDurationMS = 0
	}
	length := DefaultWindowMS
	if runDurationMS < length {
		length = runDurationMS
	}
	interaction := 0
	for _, event := range events {
		if event.Type == "first_input" {
			interaction = clamp(event.TimeMS, 0, runDurationMS)
			break
		}
	}
	start := interaction
	if candidate, ok := strongestEvent(events, interaction, runDurationMS); ok {
		start = candidate.TimeMS - length/3
		if start < interaction {
			start = interaction
		}
	}
	maxStart := runDurationMS - length
	if start > maxStart {
		start = maxStart
	}
	if start < 0 {
		start = 0
	}
	return Window{StartMS: start, EndMS: start + length}
}

func strongestEvent(events []Event, interaction, duration int) (Event, bool) {
	priorities := map[string]int{
		"win": 6, "lose": 6, "score_change": 5, "first_reward": 4,
		"collision": 3, "milestone": 2,
	}
	var best Event
	bestPriority := -1
	for _, event := range events {
		if event.TimeMS < interaction || event.TimeMS > duration {
			continue
		}
		priority, interesting := priorities[event.Type]
		if !interesting || priority < bestPriority {
			continue
		}
		// Equal-priority events choose the earliest timestamp, making the
		// result stable even when the trace has multiple wins or collisions.
		if priority == bestPriority && best.Type != "" && event.TimeMS >= best.TimeMS {
			continue
		}
		best, bestPriority = event, priority
	}
	return best, bestPriority >= 0
}

func clamp(value, low, high int) int {
	if value < low {
		return low
	}
	if value > high {
		return high
	}
	return value
}
