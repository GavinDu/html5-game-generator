package socialgraph

import (
	"context"
	"fmt"
)

// AllIDs is the full, unbounded set of a viewer's social relations, for
// internal filter reads only (feed visibility, block checks, preferences).
// It is deliberately NOT a paged API: truncating any of these sets could
// leak blocked or not-interested content into a feed, so internal callers
// must keep the whole set. Public list endpoints use the paged List*Page
// methods instead, and must never call ListAllIDs.
type AllIDs struct {
	Following     []string
	Followers     []string
	Blocked       []string
	Blockers      []string
	SavedGames    []string
	NotInterested []string
	MutedTopics   []string
}

// AllIDStore is implemented by stores that can return the full internal
// set in one call. It is an optional interface, like the pager interfaces:
// callers type-assert for it and fall back to the individual List*
// methods when a store does not implement it.
type AllIDStore interface {
	ListAllIDs(ctx context.Context, viewerID string) (AllIDs, error)
}

// ListAllIDs returns the viewer's full internal set by delegating to the
// existing List* queries. No new SQL is needed: every set already has a
// keyset/indexed query. The first failing set aborts the whole call, so a
// caller never acts on a partially loaded filter set.
func (s *PostgresStore) ListAllIDs(ctx context.Context, viewerID string) (AllIDs, error) {
	var all AllIDs
	var err error
	if all.Following, err = s.ListFollowing(ctx, viewerID); err != nil {
		return AllIDs{}, fmt.Errorf("list following: %w", err)
	}
	if all.Followers, err = s.ListFollowers(ctx, viewerID); err != nil {
		return AllIDs{}, fmt.Errorf("list followers: %w", err)
	}
	if all.Blocked, err = s.ListBlocked(ctx, viewerID); err != nil {
		return AllIDs{}, fmt.Errorf("list blocked: %w", err)
	}
	if all.Blockers, err = s.ListBlockers(ctx, viewerID); err != nil {
		return AllIDs{}, fmt.Errorf("list blockers: %w", err)
	}
	if all.SavedGames, err = s.ListSavedGames(ctx, viewerID); err != nil {
		return AllIDs{}, fmt.Errorf("list saved games: %w", err)
	}
	if all.NotInterested, err = s.ListNotInterested(ctx, viewerID); err != nil {
		return AllIDs{}, fmt.Errorf("list not interested: %w", err)
	}
	if all.MutedTopics, err = s.ListMutedTopics(ctx, viewerID); err != nil {
		return AllIDs{}, fmt.Errorf("list muted topics: %w", err)
	}
	return all, nil
}
