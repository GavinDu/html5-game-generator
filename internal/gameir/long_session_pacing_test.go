package gameir

import (
	"strings"
	"testing"
)

func TestLongFormExperiencePacingAllowsTheAuthoredTwentyMinuteCeiling(t *testing.T) {
	base := v12Doc(t)
	old := `"targetSessionSec": 60`
	if !strings.Contains(base, old) {
		t.Fatal("1.2 experience fixture no longer has an explicit session target")
	}
	valid := strings.Replace(base, old, `"targetSessionSec": 1200`, 1)
	doc, err := DecodeAndValidate([]byte(valid))
	if err != nil {
		t.Fatalf("a 20-minute, bounded authored session must validate: %v", err)
	}
	if doc.Experience.Pacing.TargetSessionSec != 1200 {
		t.Fatalf("decoded target = %d; want 1200", doc.Experience.Pacing.TargetSessionSec)
	}

	tooLong := strings.Replace(base, old, `"targetSessionSec": 1201`, 1)
	if _, err := DecodeAndValidate([]byte(tooLong)); err == nil ||
		!strings.Contains(err.Error(), "between 10 and 1200") {
		t.Fatalf("unbounded long sessions must still fail validation: %v", err)
	}
}
