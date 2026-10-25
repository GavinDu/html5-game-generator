package socialgraph

import (
	"context"
	"errors"
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
