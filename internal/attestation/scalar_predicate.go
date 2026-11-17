package attestation

import (
	"errors"
	"math"
)

// A deliberately versioned, bounded PURE subset of v1.x Game IR predicate
// coercions. It is not a full Go replay interpreter, game score or verdict.
// Unsupported values must stop an eventual replay, not be treated as false.
var errUnsupportedScalarPredicate = errors.New("unsupported scalar predicate in server replay capability")

const maxReplayScalarMagnitude int64 = 1_000_000_000

func replayScalarInteger(value any) (int64, bool) {
	var n int64
	switch v := value.(type) {
	case int:
		n = int64(v)
	case int64:
		n = v
	case float64:
		if math.IsNaN(v) || math.IsInf(v, 0) ||
			math.Trunc(v) != v ||
			v < -float64(maxReplayScalarMagnitude) ||
			v > float64(maxReplayScalarMagnitude) {
			return 0, false
		}
		n = int64(v)
	default:
		// JSON string "5", floats with fractions and arbitrary coercible
		// objects exist in the JS engine. This strict declared capability
		// does NOT support them; never interpret them as zero or a false rule.
		return 0, false
	}
	if n < -maxReplayScalarMagnitude || n > maxReplayScalarMagnitude {
		return 0, false
	}
	return n, true
}

// compareReplayScalarV1 reproduces the preinstalled JS
// runtime-rule-predicates.mjs conditionsMet only for the admitted integer
// and boolean scalar subset. JS Number(true) == 1 when RHS is numeric;
// but if RHS is boolean, JS uses strict equality/inequality instead.
// Unsupported types/operators return ERROR, not the Boolean predicate false.
func compareReplayScalarV1(lhs any, operator string, rhs any) (bool, error) {
	switch operator {
	case "eq", "neq", "gt", "gte", "lt", "lte":
	default:
		return false, errUnsupportedScalarPredicate
	}
	if rightBool, ok := rhs.(bool); ok {
		leftBool, lhsIsBool := lhs.(bool)
		if !lhsIsBool {
			if _, integer := replayScalarInteger(lhs); !integer {
				return false, errUnsupportedScalarPredicate
			}
		}
		// When RHS is bool, JS bypasses Number(...), even if both values
		// have numeric representations. Ordered bool comparisons are false.
		switch operator {
		case "eq":
			return lhsIsBool && leftBool == rightBool, nil
		case "neq":
			return !lhsIsBool || leftBool != rightBool, nil
		default:
			return false, nil
		}
	}
	right, valid := replayScalarInteger(rhs)
	if !valid {
		return false, errUnsupportedScalarPredicate
	}
	var left int64
	if leftBool, ok := lhs.(bool); ok {
		if leftBool {
			left = 1
		}
	} else {
		var leftValid bool
		left, leftValid = replayScalarInteger(lhs)
		if !leftValid {
			return false, errUnsupportedScalarPredicate
		}
	}
	switch operator {
	case "eq":
		return left == right, nil
	case "neq":
		return left != right, nil
	case "gt":
		return left > right, nil
	case "gte":
		return left >= right, nil
	case "lt":
		return left < right, nil
	case "lte":
		return left <= right, nil
	default:
		return false, errUnsupportedScalarPredicate
	}
}
