// Package attestation owns a pure, server-internal verdict transition policy.
//
// A verdict produced from client-owned HTTP fields is NEVER an attestation.
// The future independent Go replay executor and immutable attempt issuer must
// supply the inputs; persistence and ranking are separate release gates.
package attestation

import (
	"errors"
	"strings"
)

const (
	ProofVersion             = "server-replay-verdict-v1"
	maxIdentifierBytes       = 128
	maxAbsScore        int64 = 1000000000
)

type Status string

const (
	Pending  Status = "pending"
	Verified Status = "verified"
	Rejected Status = "rejected"
)

var (
	ErrInvalidIdentity    = errors.New("invalid server-owned replay identity")
	ErrInvalidDecision    = errors.New("invalid independent replay decision")
	ErrConflictingVerdict = errors.New("conflicting final replay verdict")
)

// Identity must be minted from a server-authorized game, account, session and
// published immutable package. Browser inputs may contain similar fields,
// but they are never suitable for this trust boundary.
type Identity struct {
	RunID            string
	GameID           string
	PlayerID         string
	IRSHA256         string
	TraceSHA256      string
	CapabilitySHA256 string
	Seed             uint32
	ProofVersion     string
}

type Verdict struct {
	Identity        Identity
	Status          Status
	Score           int64
	Won             bool
	StateSHA256     string
	RejectionReason string
}

// Decision is acceptable only from the *future* server-side replay worker.
// This type alone does not authenticate a caller, verify a trace or sign data.
type Decision struct {
	Identity        Identity
	Status          Status
	Score           int64
	Won             bool
	StateSHA256     string
	RejectionReason string
}

func validIdentifier(id string) bool {
	if len(id) == 0 || len(id) > maxIdentifierBytes || strings.TrimSpace(id) != id {
		return false
	}
	for _, ch := range id {
		if ch < 0x21 || ch > 0x7e {
			return false
		}
	}
	return true
}

func validSHA256(sha string) bool {
	if len(sha) != 64 {
		return false
	}
	for _, ch := range sha {
		if (ch < '0' || ch > '9') && (ch < 'a' || ch > 'f') {
			return false
		}
	}
	return true
}

func validateIdentity(id Identity) bool {
	return validIdentifier(id.RunID) &&
		validIdentifier(id.GameID) &&
		validIdentifier(id.PlayerID) &&
		validSHA256(id.IRSHA256) &&
		validSHA256(id.TraceSHA256) &&
		validSHA256(id.CapabilitySHA256) &&
		id.ProofVersion == ProofVersion
}

func NewPending(id Identity) (Verdict, error) {
	if !validateIdentity(id) {
		return Verdict{}, ErrInvalidIdentity
	}
	return Verdict{Identity: id, Status: Pending}, nil
}

func validRejection(reason string) bool {
	switch reason {
	case "unsupported_ir", "invalid_trace", "state_mismatch",
		"revoked_attempt", "expired_attempt", "replay_internal_error":
		return true
	default:
		return false
	}
}

// ApplyServerDecision never mutates its input. The identical final decision
// may be applied repeatedly, but no other final -> final/pending transition
// is permitted. Durable atomic compare-and-swap is the responsibility of
// the future storage ledger, not this pure policy.
func ApplyServerDecision(previous Verdict, next Decision) (Verdict, error) {
	if !validateIdentity(previous.Identity) || !validateIdentity(next.Identity) ||
		previous.Identity != next.Identity {
		return Verdict{}, ErrInvalidIdentity
	}
	if previous.Status != Pending && previous.Status != Verified && previous.Status != Rejected {
		return Verdict{}, ErrInvalidDecision
	}
	if previous.Status == Pending &&
		(previous.Score != 0 || previous.Won || previous.StateSHA256 != "" ||
			previous.RejectionReason != "") {
		return Verdict{}, ErrInvalidDecision
	}
	out := Verdict{Identity: next.Identity, Status: next.Status}
	switch next.Status {
	case Verified:
		if !validSHA256(next.StateSHA256) ||
			next.Score < -maxAbsScore || next.Score > maxAbsScore ||
			next.RejectionReason != "" {
			return Verdict{}, ErrInvalidDecision
		}
		out.Score, out.Won, out.StateSHA256 = next.Score, next.Won, next.StateSHA256
	case Rejected:
		if !validRejection(next.RejectionReason) || next.Score != 0 ||
			next.Won || next.StateSHA256 != "" {
			return Verdict{}, ErrInvalidDecision
		}
		out.RejectionReason = next.RejectionReason
	default:
		return Verdict{}, ErrInvalidDecision
	}
	if previous.Status == Pending {
		return out, nil
	}
	if previous == out {
		return previous, nil
	}
	return Verdict{}, ErrConflictingVerdict
}
