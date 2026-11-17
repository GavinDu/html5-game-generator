package attestation

// Private, pure publication eligibility policy for a future authenticated
// replay-attempt issuer. Inputs MUST be resolved independently by trusted
// server auth middleware and the authoritative published Game IR store.
// Constructing these structs is NOT authentication and never certifies scores.

type publicationSubject struct {
	PlayerID                     string
	Authenticated                bool
	CredentialEpochSHA256        string
	CurrentCredentialEpochSHA256 string
}

type publishedGameSnapshot struct {
	GameID               string
	OwnerID              string
	RevisionID           string
	IRSHA256             string
	CapabilitySHA256     string
	Published            bool
	Immutable            bool
	Removed              bool
	ModerationStatus     string
	CapabilitiesApproved bool
	Visibility           string
}

type requestedPublishedGame struct {
	GameID           string
	RevisionID       string
	IRSHA256         string
	CapabilitySHA256 string
}

// EligibilityReason deliberately carries no score, run ID or verified state.
// These codes are for private diagnostics, NOT detailed public authorization
// errors that could reveal owner-private publication or moderation metadata.
type eligibilityReason string

const (
	eligibilityAllowed          eligibilityReason = "eligible"
	eligibilityInvalidFacts     eligibilityReason = "invalid_server_facts"
	eligibilityUnauthenticated  eligibilityReason = "unauthenticated"
	eligibilityStaleCredential  eligibilityReason = "stale_credential_epoch"
	eligibilityNotPublished     eligibilityReason = "publication_unavailable"
	eligibilityRevisionMismatch eligibilityReason = "immutable_revision_mismatch"
	eligibilityPermissionDenied eligibilityReason = "permission_denied"
)

type publicationEligibility struct {
	Allowed bool
	Reason  eligibilityReason
}

func deniedPublication(reason eligibilityReason) publicationEligibility {
	return publicationEligibility{Reason: reason}
}

// Published immutable Game IR IDs are intentionally stricter than the generic
// printable ASCII internal verdict IDs: these IDs can eventually be used as
// path/package selectors, so slash/query/URL punctuation must not be admitted.
func validPublishedGameID(value string) bool {
	if len(value) == 0 || len(value) > maxIdentifierBytes {
		return false
	}
	for _, ch := range value {
		if !((ch >= 'a' && ch <= 'z') ||
			(ch >= 'A' && ch <= 'Z') ||
			(ch >= '0' && ch <= '9') ||
			ch == '-' || ch == '_') {
			return false
		}
	}
	return true
}

// evaluatePublishedAttemptEligibility is a deterministic subdecision ONLY.
// It cannot resolve session/auth provenance or atomically prevent a package
// removal/credential rotation between this check and the future SQL verdict.
// Parent #460 must build server-resolved facts, #461/#462 grant+fence them and
// #382 must independently execute the full game before any positive score.
func evaluatePublishedAttemptEligibility(
	subject publicationSubject,
	published publishedGameSnapshot,
	requested requestedPublishedGame,
) publicationEligibility {
	// Reject malformed server-supplied facts BEFORE evaluating access. Never
	// accept a truncated digest or silently normalize ambiguous identifiers.
	if !validIdentifier(subject.PlayerID) ||
		!validSHA256(subject.CredentialEpochSHA256) ||
		!validSHA256(subject.CurrentCredentialEpochSHA256) ||
		!validPublishedGameID(published.GameID) ||
		!validIdentifier(published.OwnerID) ||
		!validPublishedGameID(published.RevisionID) ||
		!validSHA256(published.IRSHA256) ||
		!validSHA256(published.CapabilitySHA256) ||
		!validPublishedGameID(requested.GameID) ||
		!validPublishedGameID(requested.RevisionID) ||
		!validSHA256(requested.IRSHA256) ||
		!validSHA256(requested.CapabilitySHA256) {
		return deniedPublication(eligibilityInvalidFacts)
	}
	if !subject.Authenticated {
		// Anonymous public play is still possible, but no trusted replay
		// attempt is issued to a caller without an authenticated identity.
		return deniedPublication(eligibilityUnauthenticated)
	}
	if subject.CredentialEpochSHA256 != subject.CurrentCredentialEpochSHA256 {
		return deniedPublication(eligibilityStaleCredential)
	}
	if !published.Published || !published.Immutable ||
		published.Removed || published.ModerationStatus != "approved" ||
		!published.CapabilitiesApproved {
		return deniedPublication(eligibilityNotPublished)
	}
	if published.GameID != requested.GameID ||
		published.RevisionID != requested.RevisionID ||
		published.IRSHA256 != requested.IRSHA256 ||
		published.CapabilitySHA256 != requested.CapabilitySHA256 {
		// Full 64-hex digests: identical abbreviated hashes are not enough.
		return deniedPublication(eligibilityRevisionMismatch)
	}
	switch published.Visibility {
	case "public":
		return publicationEligibility{Allowed: true, Reason: eligibilityAllowed}
	case "owner_private":
		if subject.PlayerID != published.OwnerID {
			return deniedPublication(eligibilityPermissionDenied)
		}
		return publicationEligibility{Allowed: true, Reason: eligibilityAllowed}
	default:
		// Group/friend/collaborator permissions need independently versioned
		// server entitlements. Never guess or fall back to public.
		return deniedPublication(eligibilityPermissionDenied)
	}
}
