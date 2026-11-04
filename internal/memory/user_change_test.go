package memory

import (
	"errors"
	"testing"
)

func TestUserMemoryTransactionRetryBindsExactContentAndRevision(t *testing.T) {
	s := Store{Dir: t.TempDir()}
	change := BatchChange{Memory: Memory{Name: "launch", Description: "launch date", Body: "Friday"}}
	first, err := s.ApplyUserChange(t.Context(), "create", "first", change)
	if err != nil {
		t.Fatal(err)
	}
	retry, err := s.ApplyUserChange(t.Context(), "create", "first", change)
	if err != nil || retry.ID != first.ID || len(s.ListAll()) != 1 {
		t.Fatalf("retry duplicated: %+v %v", retry, err)
	}
	if _, err := s.ApplyUserChange(t.Context(), "create", "changed", change); !errors.Is(err, ErrRequestReused) {
		t.Fatalf("reused key: %v", err)
	}
	stale := BatchChange{Memory: first, ExpectedRevision: first.Revision + 1, Remove: true}
	if _, err := s.ApplyUserChange(t.Context(), "archive", "stale", stale); !errors.Is(err, ErrRevisionConflict) {
		t.Fatalf("stale archive: %v", err)
	}
	stale.ExpectedRevision = first.Revision
	if _, err := s.ApplyUserChange(t.Context(), "archive", "correct", stale); err != nil {
		t.Fatal(err)
	}
	if _, err := s.ApplyUserChange(t.Context(), "archive", "correct", stale); err != nil {
		t.Fatal(err)
	}
	if len(s.ListAll()) != 0 {
		t.Fatal("archive did not remove active entry")
	}
}
