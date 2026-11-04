package teamroster

import (
	"errors"
	"strings"
	"testing"
)

func TestDesktopMutationRetryCannotChangeOrRepeatConfiguration(t *testing.T) {
	s := Store{Dir: t.TempDir()}
	digest := strings.Repeat("a", 64)
	apply := func(r *Roster) (string, error) { a := r.AddAgent(Agent{Name: "Chief", Main: true}); return a.ID, nil }
	first, id, err := s.Mutate(t.Context(), 0, "request-one", digest, apply)
	if err != nil {
		t.Fatal(err)
	}
	replay, replayedID, err := s.Mutate(t.Context(), 0, "request-one", digest, func(*Roster) (string, error) { t.Fatal("repeated configuration write"); return "", nil })
	if err != nil || id != replayedID || replay.Revision != first.Revision {
		t.Fatalf("retry: %s %+v %v", replayedID, replay, err)
	}
	_, _, err = s.Mutate(t.Context(), 0, "request-one", strings.Repeat("b", 64), apply)
	if !errors.Is(err, ErrRequestReused) {
		t.Fatalf("changed request: %v", err)
	}
	_, _, err = s.Mutate(t.Context(), 0, "request-two", digest, apply)
	if !errors.Is(err, ErrConflict) {
		t.Fatalf("stale revision: %v", err)
	}
	current, err := s.Load()
	if err != nil || len(current.Agents) != 1 || current.Revision != 1 {
		t.Fatalf("retry changed storage: %+v %v", current, err)
	}
}
