package preview

import "testing"

func TestPickWindowPrefersMeaningfulEventAfterInteraction(t *testing.T) {
	got := PickWindow([]Event{
		{TimeMS: 900, Type: "first_input"},
		{TimeMS: 1200, Type: "collision"},
		{TimeMS: 3500, Type: "score_change"},
		{TimeMS: 4800, Type: "win"},
	}, 12000)
	if got != (Window{StartMS: 2800, EndMS: 8800}) {
		t.Fatalf("window = %+v", got)
	}
}

func TestPickWindowFallsBackToFirstInteraction(t *testing.T) {
	got := PickWindow([]Event{{TimeMS: 1400, Type: "first_input"}}, 12000)
	if got != (Window{StartMS: 1400, EndMS: 7400}) {
		t.Fatalf("window = %+v", got)
	}
}

func TestPickWindowIsClampedForShortRuns(t *testing.T) {
	got := PickWindow([]Event{{TimeMS: 100, Type: "first_input"}, {TimeMS: 500, Type: "score_change"}}, 900)
	if got != (Window{StartMS: 0, EndMS: 900}) {
		t.Fatalf("window = %+v", got)
	}
}

func TestPickWindowIgnoresEventsBeforeInteraction(t *testing.T) {
	got := PickWindow([]Event{
		{TimeMS: 100, Type: "score_change"},
		{TimeMS: 1000, Type: "first_input"},
		{TimeMS: 2000, Type: "collision"},
	}, 9000)
	if got != (Window{StartMS: 1000, EndMS: 7000}) {
		t.Fatalf("window = %+v", got)
	}
}
