package attestation

import (
	"crypto/sha256"
	"encoding/hex"
	"reflect"
	"strings"
	"testing"
)

func TestPinnedStartScoreWireRejectsAmbiguousMembersEvenWithMatchingDigest(t *testing.T) {
	raw, _ := pinnedStartScoreFixture(t)
	good := string(raw)
	cases := []struct {
		name string
		old  string
		next string
	}{
		{"duplicate_root", `"irVersion": "1.0"`, `"irVersion": "1.0", "irVersion": "1.0"`},
		{"duplicate_escaped", `"value": 3`, `"value": 3, "v\u0061lue": 9`},
		{"case_alias_root", `"irVersion"`, `"IRVersion"`},
		{"case_shadow_root", `"irVersion": "1.0"`, `"irVersion": "1.0", "IRVersion": "1.0"`},
		{"case_alias_game", `"orientation"`, `"Orientation"`},
		{"case_alias_scene", `"entities"`, `"Entities"`},
		{"case_alias_rule", `"when"`, `"When"`},
		{"case_alias_event", `"event"`, `"Event"`},
		{"case_alias_action", `"type"`, `"Type"`},
		{"case_shadow_value", `"value": 3`, `"value": 3, "Value": 9`},
		{"unknown_object", `"value": 3`, `"value": {"value": 3}`},
		{"depth", `"value": 3`, `"value": [[[[[[[[[3]]]]]]]]]`},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if !strings.Contains(good, tc.old) {
				t.Fatal("fixture drift")
			}
			bad := []byte(strings.Replace(good, tc.old, tc.next, 1))
			if err := validatePinnedStartScoreWire(bad); err == nil {
				t.Fatal("wire scanner admitted ambiguous member or structure")
			}
			sum := sha256.Sum256(bad)
			got, err := fingerprintPinnedStartScoreContext(bad, hex.EncodeToString(sum[:]), 43, 180, nil)
			if err == nil || !reflect.DeepEqual(got, pinnedStartScoreContext{}) {
				t.Fatalf("ambiguous wire admitted despite correct digest: %+v %v", got, err)
			}
		})
	}
}

func TestPinnedStartScoreWirePreservesCanonicalEscapesAndRawIdentity(t *testing.T) {
	raw, sha := pinnedStartScoreFixture(t)
	original, err := fingerprintPinnedStartScoreContext(raw, sha, 43, 180, nil)
	if err != nil {
		t.Fatal(err)
	}
	escaped := []byte(strings.Replace(string(raw), `"value": 3`, `"v\u0061lue": 3`, 1))
	sum := sha256.Sum256(escaped)
	got, err := fingerprintPinnedStartScoreContext(escaped, hex.EncodeToString(sum[:]), 43, 180, nil)
	if err != nil || got.Score != original.Score || got.TypedContextSHA256 != original.TypedContextSHA256 ||
		got.PinnedContextSHA256 == original.PinnedContextSHA256 {
		t.Fatalf("canonical key escape lost raw/typed distinction: %+v %v", got, err)
	}
	for _, bad := range [][]byte{
		append(append([]byte(nil), raw...), 0xff),
		append(append([]byte(nil), raw...), []byte("{}")...),
		raw[:len(raw)/2],
	} {
		sum := sha256.Sum256(bad)
		got, err := fingerprintPinnedStartScoreContext(bad, hex.EncodeToString(sum[:]), 43, 180, nil)
		if err == nil || !reflect.DeepEqual(got, pinnedStartScoreContext{}) {
			t.Fatalf("malformed wire admitted: %+v %v", got, err)
		}
	}
}
