package attestation

import (
	"errors"
	"math"
	"testing"
)

func TestReplayScalarV1NumericComparatorMatchesPreinstalledJSSubset(t *testing.T) {
	// Mirror the admitted numeric/boolean cases from the actual
	// tools/playtest/runtime-rule-predicates.test.mjs v1 predicate matrix.
	tests := []struct {
		name string
		lhs  any
		op   string
		rhs  any
		want bool
	}{
		{"score_eq", float64(5), "eq", float64(5), true},
		{"score_wrong", 5, "eq", 6, false},
		{"score_neq_false", 5, "neq", 5, false},
		{"score_neq_true", 5, "neq", 4, true},
		{"gt", 5, "gt", 4, true},
		{"gt_equal", 5, "gt", 5, false},
		{"gte_equal", 5, "gte", 5, true},
		{"gte_greater", 5, "gte", 6, false},
		{"lt", 5, "lt", 6, true},
		{"lt_equal", 5, "lt", 5, false},
		{"lte_equal", 5, "lte", 5, true},
		{"lte_small", 5, "lte", 4, false},
		{"true_eq_true", true, "eq", true, true},
		{"true_eq_false", true, "eq", false, false},
		{"true_neq_false", true, "neq", false, true},
		{"true_numeric_one", true, "eq", 1, true},
		{"true_numeric_two", true, "eq", 2, false},
		{"false_numeric_zero", false, "eq", 0, true},
		{"bool_numeric_gte", true, "gte", 1, true},
		{"number_rhs_boolean", 5, "eq", true, false},
		{"number_rhs_boolean_neq", 5, "neq", true, true},
		{"number_rhs_boolean_order", 5, "gt", false, false},
		{"bool_rhs_boolean_order", true, "gt", false, false},
		{"zero", 0, "eq", 0, true},
		{"negative", int64(-1), "lt", 0, true},
		{"negative_gte", int64(-1), "gte", int64(-1), true},
		{"lower_bound", -maxReplayScalarMagnitude, "eq", float64(-maxReplayScalarMagnitude), true},
		{"upper_bound", maxReplayScalarMagnitude, "lte", maxReplayScalarMagnitude, true},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got, err := compareReplayScalarV1(tc.lhs, tc.op, tc.rhs)
			if err != nil || got != tc.want {
				t.Fatalf("JS subset comparator mismatch: lhs=%v op=%s rhs=%v got=%v err=%v expected=%v",
					tc.lhs, tc.op, tc.rhs, got, err, tc.want)
			}
		})
	}
}

func TestReplayScalarV1UnsupportedSemanticsNeverBecomeFalseOrVerified(t *testing.T) {
	// Strings (including numeric strings coerced by legacy JS), fractional
	// and nonfinite numbers, external objects and oversized counters all
	// require a separately versioned replay capability. Do not silently
	// return false and allow a fake independent replay score to advance.
	tests := []struct {
		name string
		lhs any
		op  string
		rhs any
	}{
		{"numeric_string_lhs", "5", "eq", 5},
		{"numeric_string_rhs", 5, "eq", "5"},
		{"non_numeric_string", "ready", "eq", "ready"},
		{"nil_lhs", nil, "eq", 0},
		{"nil_rhs", 0, "eq", nil},
		{"float_fraction_lhs", float64(1.5), "gt", 1},
		{"float_fraction_rhs", 1, "gte", float64(0.5)},
		{"NaN", math.NaN(), "eq", 2},
		{"infinity", 3, "eq", math.Inf(1)},
		{"huge_positive", maxReplayScalarMagnitude + 1, "gt", 0},
		{"huge_negative", -maxReplayScalarMagnitude - 1, "lt", 0},
		{"slice", []int{1}, "eq", 1},
		{"unknown_op", 5, "includes", 5},
		{"empty_op", 5, "", 5},
		{"unsupported_bool", "true", "eq", true},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			matched, err := compareReplayScalarV1(tc.lhs, tc.op, tc.rhs)
			if !errors.Is(err, errUnsupportedScalarPredicate) || matched {
				t.Fatalf("unsupported scalar must fail closed, not return an ordinary false/score: %v %s %v => %v, %v",
					tc.lhs, tc.op, tc.rhs, matched, err)
			}
		})
	}
}
