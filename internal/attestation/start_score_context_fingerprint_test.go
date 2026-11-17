package attestation

import (
	"encoding/json"
	"reflect"
	"testing"

	"doodlequest/internal/gameir"
)

func TestStartScoreContextFingerprintStableAcrossDecodeAndRepeat(t *testing.T) {
	doc := loadStartScoreFixture(t)
	before, err := json.Marshal(doc)
	if err != nil {
		t.Fatal(err)
	}
	const seed uint64 = 43
	first, err := fingerprintStartScoreContext(doc, seed, 180, nil)
	if err != nil {
		t.Fatalf("context fingerprint: %v", err)
	}
	if first.Profile != "go-scalar-start-context-v1" || first.Score != 1 ||
		first.Seed != seed || first.DurationTicks != 180 ||
		len(first.CanonicalIRSHA256) != 64 || len(first.OrderedWorkSHA256) != 64 ||
		len(first.ContextSHA256) != 64 {
		t.Fatalf("unexpected diagnostic fingerprint: %+v", first)
	}
	for i := 0; i < 20; i++ {
		redecoded, err := gameir.DecodeAndValidate(before)
		if err != nil {
			t.Fatal(err)
		}
		again, err := fingerprintStartScoreContext(redecoded, seed, 180, nil)
		if err != nil || !reflect.DeepEqual(first, again) {
			t.Fatalf("determinism lost on fresh typed decode %d: %+v %v", i, again, err)
		}
	}
	after, err := json.Marshal(doc)
	if err != nil || string(before) != string(after) {
		t.Fatalf("diagnostic mutated the caller's IR: %v", err)
	}
}

func TestStartScoreContextFingerprintSeparatesEqualScoreDifferentIdentity(t *testing.T) {
	doc := loadStartScoreFixture(t)
	base, err := fingerprintStartScoreContext(doc, 7, 60, nil)
	if err != nil {
		t.Fatal(err)
	}
	newSeed, err := fingerprintStartScoreContext(doc, 8, 60, nil)
	if err != nil {
		t.Fatal(err)
	}
	longer, err := fingerprintStartScoreContext(doc, 7, 120, nil)
	if err != nil {
		t.Fatal(err)
	}
	doc.Rules[0], doc.Rules[1] = doc.Rules[1], doc.Rules[0]
	reordered, err := fingerprintStartScoreContext(doc, 7, 60, nil)
	if err != nil {
		t.Fatal(err)
	}
	if base.Score != 1 || base.Score != newSeed.Score ||
		base.Score != longer.Score || base.Score != reordered.Score ||
		base.ContextSHA256 == newSeed.ContextSHA256 ||
		base.ContextSHA256 == longer.ContextSHA256 ||
		base.ContextSHA256 == reordered.ContextSHA256 ||
		base.OrderedWorkSHA256 == reordered.OrderedWorkSHA256 ||
		base.CanonicalIRSHA256 == reordered.CanonicalIRSHA256 {
		t.Fatalf("equal score hid differing context/ordered work: base=%+v seed=%+v duration=%+v order=%+v",
			base, newSeed, longer, reordered)
	}
}

func TestStartScoreContextFingerprintFailsClosedOnInvalidHorizonEdgesAndIR(t *testing.T) {
	doc := loadStartScoreFixture(t)
	for _, ticks := range []int{-1, 0, 108_001, 1_000_000} {
		got, err := fingerprintStartScoreContext(doc, 1, ticks, nil)
		if err == nil || !reflect.DeepEqual(got, startScoreContextFingerprint{}) {
			t.Fatalf("invalid virtual tick horizon %d admitted: %+v %v", ticks, got, err)
		}
	}
	if _, err := fingerprintStartScoreContext(doc, 1, 1, nil); err != nil {
		t.Fatalf("minimum virtual horizon is valid: %v", err)
	}
	if _, err := fingerprintStartScoreContext(doc, 1, 108_000, nil); err != nil {
		t.Fatalf("maximum virtual horizon is valid: %v", err)
	}
	for _, edges := range [][]string{{"pointer"}, {""}, {"keyup", "keydown"}} {
		got, err := fingerprintStartScoreContext(doc, 1, 60, edges)
		if err == nil || !reflect.DeepEqual(got, startScoreContextFingerprint{}) {
			t.Fatalf("input edge trace admitted: %+v %v", got, err)
		}
	}
	got, err := fingerprintStartScoreContext(nil, 1, 60, nil)
	if err == nil || !reflect.DeepEqual(got, startScoreContextFingerprint{}) {
		t.Fatalf("nil IR admitted: %+v %v", got, err)
	}
	doc.Rules[0].Actions[0].Value = "3"
	got, err = fingerprintStartScoreContext(doc, 1, 60, nil)
	if err == nil || !reflect.DeepEqual(got, startScoreContextFingerprint{}) {
		t.Fatalf("unsupported score coercion admitted: %+v %v", got, err)
	}
}
