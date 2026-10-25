package socialgraph

import (
	"context"
	"errors"
	"strings"
	"testing"
)

func TestFollowValidationRunsBeforeDatabaseAccess(t *testing.T) {
	s := &PostgresStore{}
	if err := s.Follow(context.Background(), "same-user", "same-user"); !errors.Is(err, ErrSelfFollow) {
		t.Fatalf("self-follow error = %v, want ErrSelfFollow", err)
	}
	if err := s.Follow(context.Background(), "", "creator"); err == nil {
		t.Fatal("empty viewer ID should be rejected")
	}
	if err := s.Unfollow(context.Background(), "viewer", ""); err == nil {
		t.Fatal("empty creator ID should be rejected")
	}
	if _, err := s.ListFollowing(context.Background(), ""); err == nil {
		t.Fatal("empty viewer ID should be rejected")
	}
	if _, err := s.ListFollowers(context.Background(), ""); err == nil {
		t.Fatal("empty creator ID should be rejected")
	}
}

func TestFollowImportRejectsInvalidEdgeBeforeDatabaseAccess(t *testing.T) {
	s := &PostgresStore{}
	_, err := s.ImportFollows(context.Background(), []Edge{{ViewerID: "same", CreatorID: "same"}})
	if err == nil {
		t.Fatal("self-follow edge should be rejected")
	}
}

func TestCommentValidationRunsBeforeDatabaseAccess(t *testing.T) {
	s := &PostgresStore{}
	if _, _, err := s.CreateComment(context.Background(), Comment{
		ID: "comment-1", GameID: "game-1", UserID: "viewer-1", Body: "hello",
	}, strings.Repeat("k", 129)); err == nil {
		t.Fatal("oversized idempotency key should be rejected before database access")
	}
	if _, err := s.ListComments(context.Background(), ""); err == nil {
		t.Fatal("empty game ID should be rejected before database access")
	}
	if found, owned, err := s.DeleteComment(context.Background(), "game-1", "", "viewer-1"); err == nil || found || owned {
		t.Fatalf("invalid delete request = found %v owned %v err %v", found, owned, err)
	}
}
