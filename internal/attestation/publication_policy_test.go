package attestation

import (
	"strings"
	"testing"
)

func eligiblePublicationFixture() (publicationSubject, publishedGameSnapshot, requestedPublishedGame) {
	subject := publicationSubject{
		PlayerID: "owner-1", Authenticated: true,
		CredentialEpochSHA256: strings.Repeat("a", 64),
		CurrentCredentialEpochSHA256: strings.Repeat("a", 64),
	}
	published := publishedGameSnapshot{
		GameID: "published-game", OwnerID: "owner-1",
		RevisionID: "release_2026_11", IRSHA256: strings.Repeat("b", 64),
		CapabilitySHA256: strings.Repeat("c", 64),
		Published: true, Immutable: true, Removed: false,
		ModerationStatus: "approved", CapabilitiesApproved: true,
		Visibility: "public",
	}
	requested := requestedPublishedGame{
		GameID: published.GameID, RevisionID: published.RevisionID,
		IRSHA256: published.IRSHA256, CapabilitySHA256: published.CapabilitySHA256,
	}
	return subject, published, requested
}

func TestPrivatePublicationEligibilityAcceptsOnlyPinnedServerFacts(t *testing.T) {
	subject, published, requested := eligiblePublicationFixture()
	allowed := evaluatePublishedAttemptEligibility(subject, published, requested)
	if !allowed.Allowed || allowed.Reason != eligibilityAllowed {
		t.Fatalf("eligible authenticated public revision refused: %+v", allowed)
	}
	published.Visibility = "owner_private"
	allowed = evaluatePublishedAttemptEligibility(subject, published, requested)
	if !allowed.Allowed || allowed.Reason != eligibilityAllowed {
		t.Fatalf("eligible owner-private revision refused: %+v", allowed)
	}
	// Public games may be played anonymously through an independent route;
	// they may not mint an authenticated replay attempt from no subject.
	published.Visibility = "public"
	subject.Authenticated = false
	got := evaluatePublishedAttemptEligibility(subject, published, requested)
	if got.Allowed || got.Reason != eligibilityUnauthenticated {
		t.Fatalf("anonymous trusted replay attempt admitted: %+v", got)
	}
}

func TestPrivatePublicationEligibilityRejectsMutatedFullImmutableIRAndCapabilities(t *testing.T) {
	subject, published, requested := eligiblePublicationFixture()
	for name, mutate := range map[string]func(*requestedPublishedGame){
		"foreign_game": func(r *requestedPublishedGame) { r.GameID = "foreign" },
		"stale_revision": func(r *requestedPublishedGame) { r.RevisionID = "old" },
		"full_ir": func(r *requestedPublishedGame) { r.IRSHA256 = strings.Repeat("b", 63) + "d" },
		"full_capability": func(r *requestedPublishedGame) { r.CapabilitySHA256 = strings.Repeat("c", 63) + "d" },
	} {
		t.Run(name, func(t *testing.T) {
			fake := requested
			mutate(&fake)
			got := evaluatePublishedAttemptEligibility(subject, published, fake)
			if got.Allowed || got.Reason != eligibilityRevisionMismatch {
				t.Fatalf("same-prefix/foreign/stale package admitted: %+v", got)
			}
		})
	}
}

func TestPrivatePublicationEligibilityFailClosedForRemovedModeratedOrUnsupported(t *testing.T) {
	subject, published, requested := eligiblePublicationFixture()
	for name, mutate := range map[string]func(*publishedGameSnapshot){
		"draft": func(p *publishedGameSnapshot) { p.Published = false },
		"mutable": func(p *publishedGameSnapshot) { p.Immutable = false },
		"deleted": func(p *publishedGameSnapshot) { p.Removed = true },
		"under_review": func(p *publishedGameSnapshot) { p.ModerationStatus = "pending" },
		"blocked": func(p *publishedGameSnapshot) { p.ModerationStatus = "blocked" },
		"unknown_moderation": func(p *publishedGameSnapshot) { p.ModerationStatus = "unexpected" },
		"unsupported_capability": func(p *publishedGameSnapshot) { p.CapabilitiesApproved = false },
	} {
		t.Run(name, func(t *testing.T) {
			bad := published
			mutate(&bad)
			got := evaluatePublishedAttemptEligibility(subject, bad, requested)
			if got.Allowed || got.Reason != eligibilityNotPublished {
				t.Fatalf("ineligible publication admitted: %+v", got)
			}
		})
	}
}

func TestPrivatePublicationEligibilityIsExactOwnerAndCurrentEpochOnly(t *testing.T) {
	subject, published, requested := eligiblePublicationFixture()
	published.Visibility = "owner_private"
	subject.PlayerID = "other-account"
	got := evaluatePublishedAttemptEligibility(subject, published, requested)
	if got.Allowed || got.Reason != eligibilityPermissionDenied {
		t.Fatalf("foreign owner accepted: %+v", got)
	}
	subject.PlayerID = "owner-1"
	for _, visibility := range []string{"friends", "collaborators", "group", "internal", "", "PUBLIC"} {
		published.Visibility = visibility
		got = evaluatePublishedAttemptEligibility(subject, published, requested)
		if got.Allowed || got.Reason != eligibilityPermissionDenied {
			t.Fatalf("unknown scope %q accepted: %+v", visibility, got)
		}
	}
	published.Visibility = "public"
	subject.CurrentCredentialEpochSHA256 = strings.Repeat("d", 64)
	got = evaluatePublishedAttemptEligibility(subject, published, requested)
	if got.Allowed || got.Reason != eligibilityStaleCredential {
		t.Fatalf("rotated/revoked credential accepted: %+v", got)
	}
	// A later credential rotation is a transactional-fence responsibility:
	// this pure decision is only true for the snapshotted epoch.
}

func TestPrivatePublicationEligibilityRejectsMalformedServerFacts(t *testing.T) {
	subject, published, requested := eligiblePublicationFixture()
	tests := map[string]func(*publicationSubject, *publishedGameSnapshot, *requestedPublishedGame){
		"empty_player": func(s *publicationSubject, _ *publishedGameSnapshot, _ *requestedPublishedGame) { s.PlayerID = "" },
		"unicode_player": func(s *publicationSubject, _ *publishedGameSnapshot, _ *requestedPublishedGame) { s.PlayerID = "👾" },
		"epoch_missing": func(s *publicationSubject, _ *publishedGameSnapshot, _ *requestedPublishedGame) { s.CredentialEpochSHA256 = "" },
		"epoch_upper": func(s *publicationSubject, _ *publishedGameSnapshot, _ *requestedPublishedGame) { s.CredentialEpochSHA256 = strings.Repeat("A", 64) },
		"current_epoch_missing": func(s *publicationSubject, _ *publishedGameSnapshot, _ *requestedPublishedGame) { s.CurrentCredentialEpochSHA256 = "" },
		"game_query": func(_ *publicationSubject, p *publishedGameSnapshot, _ *requestedPublishedGame) { p.GameID = "game?admin=true" },
		"game_slash": func(_ *publicationSubject, p *publishedGameSnapshot, _ *requestedPublishedGame) { p.GameID = "../other" },
		"unicode_game": func(_ *publicationSubject, p *publishedGameSnapshot, _ *requestedPublishedGame) { p.GameID = "中文" },
		"empty_owner": func(_ *publicationSubject, p *publishedGameSnapshot, _ *requestedPublishedGame) { p.OwnerID = "" },
		"empty_revision": func(_ *publicationSubject, p *publishedGameSnapshot, _ *requestedPublishedGame) { p.RevisionID = "" },
		"invalid_revision": func(_ *publicationSubject, p *publishedGameSnapshot, _ *requestedPublishedGame) { p.RevisionID = "2026/11" },
		"invalid_ir_digest": func(_ *publicationSubject, p *publishedGameSnapshot, _ *requestedPublishedGame) { p.IRSHA256 = strings.Repeat("b", 12) },
		"invalid_capability_digest": func(_ *publicationSubject, p *publishedGameSnapshot, _ *requestedPublishedGame) { p.CapabilitySHA256 = strings.Repeat("z", 64) },
		"missing_requested_game": func(_ *publicationSubject, _ *publishedGameSnapshot, q *requestedPublishedGame) { q.GameID = "" },
		"missing_requested_revision": func(_ *publicationSubject, _ *publishedGameSnapshot, q *requestedPublishedGame) { q.RevisionID = "" },
		"missing_requested_ir": func(_ *publicationSubject, _ *publishedGameSnapshot, q *requestedPublishedGame) { q.IRSHA256 = "" },
		"missing_requested_capability": func(_ *publicationSubject, _ *publishedGameSnapshot, q *requestedPublishedGame) { q.CapabilitySHA256 = "" },
		"oversized_game": func(_ *publicationSubject, p *publishedGameSnapshot, _ *requestedPublishedGame) { p.GameID = strings.Repeat("g", 129) },
	}
	for name, mutate := range tests {
		t.Run(name, func(t *testing.T) {
			s, p, q := subject, published, requested
			mutate(&s, &p, &q)
			got := evaluatePublishedAttemptEligibility(s, p, q)
			if got.Allowed || got.Reason != eligibilityInvalidFacts {
				t.Fatalf("malformed server facts accepted: %+v", got)
			}
		})
	}
	if validPublishedGameID("a/b") || validPublishedGameID("middle space") ||
		validPublishedGameID("x.") || validPublishedGameID("é") {
		t.Fatal("published Game IR selector accepted non-ASCII or punctuation")
	}
	if !validPublishedGameID(strings.Repeat("a", 128)) ||
		validPublishedGameID(strings.Repeat("a", 129)) {
		t.Fatal("published Game IR selector length boundary mismatch")
	}
}
