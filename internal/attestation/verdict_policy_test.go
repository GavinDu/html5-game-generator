package attestation

import (
	"errors"
	"strings"
	"testing"
)

func policyIdentity() Identity {
	return Identity{
		RunID: "attempt-1",
		GameID: "public-game",
		PlayerID: "authorized-player",
		IRSHA256: strings.Repeat("a", 64),
		TraceSHA256: strings.Repeat("b", 64),
		CapabilitySHA256: strings.Repeat("c", 64),
		Seed: 42,
		ProofVersion: ProofVersion,
	}
}

func TestVerdictPolicyPendingVerifiedIsExactIdempotentAndImmutable(t *testing.T) {
	id := policyIdentity()
	pending, err := NewPending(id)
	if err != nil {
		t.Fatal(err)
	}
	if pending.Status != Pending || pending.Score != 0 || pending.Won ||
		pending.StateSHA256 != "" || pending.RejectionReason != "" {
		t.Fatalf("pending has trusted outcome before replay: %+v", pending)
	}
	decision := Decision{
		Identity: id, Status: Verified, Score: 17, Won: true,
		StateSHA256: strings.Repeat("d", 64),
	}
	out, err := ApplyServerDecision(pending, decision)
	if err != nil || out.Status != Verified || out.Score != 17 || !out.Won ||
		out.StateSHA256 != decision.StateSHA256 {
		t.Fatalf("verified result not applied: %+v %v", out, err)
	}
	if pending.Status != Pending || pending.Score != 0 {
		t.Fatalf("pure reducer mutated previous pending state: %+v", pending)
	}
	for i := 0; i < 100; i++ {
		again, err := ApplyServerDecision(out, decision)
		if err != nil || again != out {
			t.Fatalf("idempotent retry drifted: %+v %v", again, err)
		}
	}
}

func TestVerdictPolicyRejectsAllConflictingFinalsAndIdentityEdits(t *testing.T) {
	id := policyIdentity()
	start, err := NewPending(id)
	if err != nil {
		t.Fatal(err)
	}
	decision := Decision{Identity: id, Status: Verified, Score: 3,
		StateSHA256: strings.Repeat("e", 64)}
	final, err := ApplyServerDecision(start, decision)
	if err != nil {
		t.Fatal(err)
	}
	contradictions := []Decision{
		{Identity: id, Status: Rejected, RejectionReason: "invalid_trace"},
		{Identity: id, Status: Verified, Score: 4, StateSHA256: decision.StateSHA256},
		{Identity: id, Status: Verified, Score: 3, Won: true, StateSHA256: decision.StateSHA256},
		{Identity: id, Status: Verified, Score: 3, StateSHA256: strings.Repeat("f", 64)},
	}
	for _, next := range contradictions {
		out, err := ApplyServerDecision(final, next)
		if !errors.Is(err, ErrConflictingVerdict) || out != (Verdict{}) {
			t.Fatalf("changed final result accepted: %+v, %v", out, err)
		}
	}
	changes := []func(*Identity){
		func(x *Identity) { x.Seed++ },
		func(x *Identity) { x.RunID = "attempt-2" },
		func(x *Identity) { x.GameID = "different-game" },
		func(x *Identity) { x.PlayerID = "other-user" },
		func(x *Identity) { x.IRSHA256 = strings.Repeat("f", 64) },
		func(x *Identity) { x.TraceSHA256 = strings.Repeat("f", 64) },
		func(x *Identity) { x.CapabilitySHA256 = strings.Repeat("f", 64) },
		func(x *Identity) { x.ProofVersion = "old-client-v1" },
	}
	for _, mutate := range changes {
		next := decision
		mutate(&next.Identity)
		out, err := ApplyServerDecision(start, next)
		if !errors.Is(err, ErrInvalidIdentity) || out != (Verdict{}) {
			t.Fatalf("foreign replay result crossed identity: %+v %v", out, err)
		}
	}
}

func TestVerdictPolicyRejectsInvalidStatusAndForgedUntrustedOutcomes(t *testing.T) {
	id := policyIdentity()
	pending, err := NewPending(id)
	if err != nil {
		t.Fatal(err)
	}
	invalid := []Decision{
		{Identity: id, Status: Pending},
		{Identity: id, Status: "verified_by_browser"},
		{Identity: id, Status: Verified, StateSHA256: ""},
		{Identity: id, Status: Verified, StateSHA256: strings.Repeat("A", 64)},
		{Identity: id, Status: Verified, StateSHA256: strings.Repeat("1", 64), Score: maxAbsScore + 1},
		{Identity: id, Status: Verified, StateSHA256: strings.Repeat("1", 64), Score: -maxAbsScore - 1},
		{Identity: id, Status: Verified, StateSHA256: strings.Repeat("1", 64), RejectionReason: "replay_internal_error"},
		{Identity: id, Status: Rejected, RejectionReason: ""},
		{Identity: id, Status: Rejected, RejectionReason: "client_verified"},
		{Identity: id, Status: Rejected, RejectionReason: "invalid_trace", Score: 1},
		{Identity: id, Status: Rejected, RejectionReason: "invalid_trace", Won: true},
		{Identity: id, Status: Rejected, RejectionReason: "invalid_trace", StateSHA256: strings.Repeat("1", 64)},
	}
	for _, next := range invalid {
		out, err := ApplyServerDecision(pending, next)
		if !errors.Is(err, ErrInvalidDecision) || out != (Verdict{}) {
			t.Fatalf("invalid decision accepted: %+v %v", out, err)
		}
	}
	pending.Score = 1
	out, err := ApplyServerDecision(pending, Decision{Identity: id,
		Status: Rejected, RejectionReason: "invalid_trace"})
	if !errors.Is(err, ErrInvalidDecision) || out != (Verdict{}) {
		t.Fatalf("spoofed pending outcome accepted: %+v %v", out, err)
	}
}

func TestVerdictPolicyRejectedReasonIsStableAndDoesNotBecomeVerified(t *testing.T) {
	id := policyIdentity()
	pending, err := NewPending(id)
	if err != nil {
		t.Fatal(err)
	}
	rejected := Decision{Identity: id, Status: Rejected, RejectionReason: "unsupported_ir"}
	first, err := ApplyServerDecision(pending, rejected)
	if err != nil || first.Status != Rejected || first.RejectionReason != "unsupported_ir" {
		t.Fatalf("refusal lost provenance: %+v %v", first, err)
	}
	again, err := ApplyServerDecision(first, rejected)
	if err != nil || again != first {
		t.Fatalf("rejected idempotence failed: %+v %v", again, err)
	}
	next := Decision{Identity: id, Status: Verified,
		StateSHA256: strings.Repeat("d", 64)}
	if got, err := ApplyServerDecision(first, next); !errors.Is(err, ErrConflictingVerdict) ||
		got != (Verdict{}) {
		t.Fatalf("rejected record upgraded without re-issuance: %+v %v", got, err)
	}
}

func TestVerdictPolicyRequiresCanonicalServerAttemptIdentity(t *testing.T) {
	id := policyIdentity()
	invalid := []func(*Identity){
		func(x *Identity) { x.RunID = "" },
		func(x *Identity) { x.RunID = "bad\nrun" },
		func(x *Identity) { x.PlayerID = " player" },
		func(x *Identity) { x.GameID = strings.Repeat("x", 129) },
		func(x *Identity) { x.IRSHA256 = strings.Repeat("a", 12) },
		func(x *Identity) { x.TraceSHA256 = strings.Repeat("A", 64) },
		func(x *Identity) { x.CapabilitySHA256 = strings.Repeat("g", 64) },
		func(x *Identity) { x.ProofVersion = "client-v1" },
	}
	for _, mutate := range invalid {
		bad := id
		mutate(&bad)
		if got, err := NewPending(bad); !errors.Is(err, ErrInvalidIdentity) ||
			got != (Verdict{}) {
			t.Fatalf("bad immutable attempt identity admitted: %+v %v", got, err)
		}
	}
}
