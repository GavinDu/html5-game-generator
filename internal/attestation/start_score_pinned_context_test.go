package attestation

import (
	"crypto/sha256"
	"encoding/hex"
	"os"
	"reflect"
	"strings"
	"testing"
)

func pinnedStartScoreFixture(t *testing.T) ([]byte, string) {
	t.Helper()
	raw, err := os.ReadFile("testdata/start_score_ir_v1.json")
	if err != nil {
		t.Fatal(err)
	}
	sum := sha256.Sum256(raw)
	return raw, hex.EncodeToString(sum[:])
}

func TestPinnedStartScoreDiagnosticBindsActualRawBytesSeedAndHorizon(t *testing.T) {
	raw, expectedSHA := pinnedStartScoreFixture(t)
	const seed uint64 = 43
	got, err := fingerprintPinnedStartScoreContext(raw, expectedSHA, seed, 180, nil)
	if err != nil {
		t.Fatalf("legitimate raw published-IR diagnostic source rejected: %v", err)
	}
	if got.Profile != "go-scalar-pinned-start-score-v1" ||
		got.RawIRSHA256 != expectedSHA || got.Score != 1 ||
		got.Seed != seed || got.DurationTicks != 180 ||
		len(got.TypedIRSHA256) != 64 || len(got.OrderedWorkSHA256) != 64 ||
		len(got.TypedContextSHA256) != 64 || len(got.PinnedContextSHA256) != 64 {
		t.Fatalf("bad versioned private pinned context: %+v", got)
	}
	for i := 0; i < 64; i++ {
		again, err := fingerprintPinnedStartScoreContext(raw, expectedSHA, seed, 180, nil)
		if err != nil || !reflect.DeepEqual(again, got) {
			t.Fatalf("raw context non-deterministic on repetition %d: %+v %v", i, again, err)
		}
	}
	seedChanged, err := fingerprintPinnedStartScoreContext(raw, expectedSHA, seed+1, 180, nil)
	if err != nil {
		t.Fatal(err)
	}
	horizonChanged, err := fingerprintPinnedStartScoreContext(raw, expectedSHA, seed, 181, nil)
	if err != nil {
		t.Fatal(err)
	}
	if got.Score != seedChanged.Score || got.Score != horizonChanged.Score ||
		got.PinnedContextSHA256 == seedChanged.PinnedContextSHA256 ||
		got.PinnedContextSHA256 == horizonChanged.PinnedContextSHA256 {
		t.Fatalf("equal score hid different seed or horizon: base=%+v seed=%+v ticks=%+v",
			got, seedChanged, horizonChanged)
	}
	// The supplied []byte is immutable from the helper's perspective.
	if sum := sha256.Sum256(raw); hex.EncodeToString(sum[:]) != expectedSHA {
		t.Fatal("pinned reducer mutated raw input")
	}
}

func TestPinnedStartScoreDiagnosticRejectsDigestForgeryAndByteDistinctIR(t *testing.T) {
	raw, expectedSHA := pinnedStartScoreFixture(t)
	for name, sha := range map[string]string{
		"empty": "",
		"short_legacy_hash": expectedSHA[:12],
		"uppercase": strings.ToUpper(expectedSHA),
		"same_12_prefix_changed_suffix": expectedSHA[:12] + strings.Repeat("0", 52),
		"all_zero_digest": strings.Repeat("0", 64),
		"nonhex": strings.Repeat("x", 64),
	} {
		t.Run(name, func(t *testing.T) {
			got, err := fingerprintPinnedStartScoreContext(raw, sha, 43, 180, nil)
			if err == nil || !reflect.DeepEqual(got, pinnedStartScoreContext{}) {
				t.Fatalf("forged/untrusted expected digest accepted: %+v %v", got, err)
			}
		})
	}
	// Same typed Game IR semantics, different original wire bytes: equality
	// of Go's typed canonical digest MUST NOT authenticate the raw artifact.
	rawWithWhitespace := append(append([]byte{}, raw...), ' ', '\n')
	_, err := fingerprintPinnedStartScoreContext(rawWithWhitespace, expectedSHA, 43, 180, nil)
	if err == nil {
		t.Fatal("changed published raw bytes admitted under the old full hash")
	}
	digestOfNewBytes := sha256.Sum256(rawWithWhitespace)
	newSHA := hex.EncodeToString(digestOfNewBytes[:])
	rebound, err := fingerprintPinnedStartScoreContext(rawWithWhitespace, newSHA, 43, 180, nil)
	if err != nil {
		t.Fatal(err)
	}
	original, err := fingerprintPinnedStartScoreContext(raw, expectedSHA, 43, 180, nil)
	if err != nil {
		t.Fatal(err)
	}
	if rebound.Score != original.Score ||
		rebound.TypedIRSHA256 != original.TypedIRSHA256 ||
		rebound.TypedContextSHA256 != original.TypedContextSHA256 ||
		rebound.RawIRSHA256 == original.RawIRSHA256 ||
		rebound.PinnedContextSHA256 == original.PinnedContextSHA256 {
		t.Fatalf("raw-byte identity not separated from typed diagnostic: before=%+v after=%+v",
			original, rebound)
	}
}

func TestPinnedStartScoreDiagnosticFailsClosedOnUnsupportedOrUnboundedInputs(t *testing.T) {
	raw, expectedSHA := pinnedStartScoreFixture(t)
	if out, err := fingerprintPinnedStartScoreContext(nil, expectedSHA, 1, 60, nil); err == nil ||
		!reflect.DeepEqual(out, pinnedStartScoreContext{}) {
		t.Fatalf("empty document admitted: %+v %v", out, err)
	}
	tooLarge := make([]byte, maxPinnedStartScoreIRBytes+1)
	if out, err := fingerprintPinnedStartScoreContext(tooLarge, expectedSHA, 1, 60, nil); err == nil ||
		!reflect.DeepEqual(out, pinnedStartScoreContext{}) {
		t.Fatalf("oversize document admitted: %+v %v", out, err)
	}
	for _, ticks := range []int{-1, 0, 108001} {
		if out, err := fingerprintPinnedStartScoreContext(raw, expectedSHA, 1, ticks, nil); err == nil ||
			!reflect.DeepEqual(out, pinnedStartScoreContext{}) {
			t.Fatalf("invalid tick budget %d admitted: %+v %v", ticks, out, err)
		}
	}
	for _, edge := range []string{"pointer_down", "", "keyup"} {
		if out, err := fingerprintPinnedStartScoreContext(raw, expectedSHA, 1, 60, []string{edge}); err == nil ||
			!reflect.DeepEqual(out, pinnedStartScoreContext{}) {
			t.Fatalf("untrusted input edge admitted: %+v %v", out, err)
		}
	}
	for name, input := range map[string][]byte{
		"truncated": raw[:len(raw)/2],
		"invalid": []byte("{broken"),
		"unsupported_js_score_coercion": []byte(strings.Replace(string(raw), "\"value\": 3", "\"value\": 2.5", 1)),
		"unknown_action": []byte(strings.Replace(string(raw), "\"add_score\"", "\"custom_exec\"", 1)),
	} {
		t.Run(name, func(t *testing.T) {
			sum := sha256.Sum256(input)
			out, err := fingerprintPinnedStartScoreContext(input, hex.EncodeToString(sum[:]), 43, 180, nil)
			if err == nil || !reflect.DeepEqual(out, pinnedStartScoreContext{}) {
				t.Fatalf("unsupported/invalid raw IR admitted: %+v %v", out, err)
			}
		})
	}
}
