package teamroster

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"testing"
)

func TestDeepzoraPermissionVocabulary(t *testing.T) {
	for _, mode := range []string{"", "read-only", "workspace-write", "danger-full-access"} {
		r := team(t)
		r.Agents[0].Permission = mode
		if err := r.Validate(); err != nil {
			t.Fatalf("permission %q: %v", mode, err)
		}
	}
	for _, mode := range []string{"ask", "auto", "yolo", "readOnly", "root"} {
		r := team(t)
		r.Agents[0].Permission = mode
		if err := r.Validate(); !errors.Is(err, ErrInvalid) {
			t.Fatalf("foreign permission %q accepted: %v", mode, err)
		}
		s := Store{Dir: t.TempDir()}
		x := ask(ByUser, newAgentID(), "")
		x.Ceiling = mode
		if _, _, err := s.OpenExchange(t.Context(), x, "payload"); !errors.Is(err, ErrInvalid) {
			t.Fatalf("foreign ceiling %q accepted: %v", mode, err)
		}
	}
}

// Exercise the OS lock, not only the process-local queue. This also runs on
// Windows, whose shell isolation contract differs from its file-lock support.
func TestWriterLeaseAcrossProcesses(t *testing.T) {
	if dir := os.Getenv("DEEPZORA_TEAM_LEASE_TEST_DIR"); dir != "" {
		_, err := (Store{Dir: dir}).AcquireLease()
		if !errors.Is(err, ErrReadOnly) {
			t.Fatalf("child acquired held lease: %v", err)
		}
		return
	}
	s := Store{Dir: t.TempDir()}
	release, err := s.AcquireLease()
	if err != nil {
		t.Fatal(err)
	}
	defer release()
	cmd := exec.CommandContext(t.Context(), os.Args[0], "-test.run=^TestWriterLeaseAcrossProcesses$")
	cmd.Env = append(os.Environ(), "DEEPZORA_TEAM_LEASE_TEST_DIR="+s.Dir)
	if output, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("lease child: %v\n%s", err, output)
	}
}

func TestRejectedRevisionLeavesRosterUnchanged(t *testing.T) {
	s := Store{Dir: t.TempDir()}
	first, err := s.Update(context.Background(), addChief)
	if err != nil {
		t.Fatal(err)
	}
	called := false
	_, err = s.UpdateAt(t.Context(), first.Revision-1, func(*Roster) error {
		called = true
		return nil
	})
	if !errors.Is(err, ErrConflict) || called {
		t.Fatalf("stale operation ran: called=%v error=%v", called, err)
	}
	next, err := s.Load()
	if err != nil || next.Revision != first.Revision || next.Agents[0].ID != first.Agents[0].ID {
		t.Fatalf("stale operation changed state: %+v, %v", next, err)
	}
}

func TestExchangeRetryCannotChangeRoutingOrAuthority(t *testing.T) {
	for _, field := range []string{"to", "parent", "group", "replyTo", "ceiling"} {
		t.Run(field, func(t *testing.T) {
			s := Store{Dir: t.TempDir()}
			x := ask(newAgentID(), newAgentID(), "same-request")
			if _, _, err := s.OpenExchange(t.Context(), x, "same text"); err != nil {
				t.Fatal(err)
			}
			switch field {
			case "to":
				x.To = newAgentID()
			case "parent":
				x.Parent = "x-0000000000000000"
			case "group":
				x.Group = newGroupID()
			case "replyTo":
				x.ReplyTo = ReplyToUser
			case "ceiling":
				x.Ceiling = "danger-full-access"
			}
			if _, _, err := s.OpenExchange(t.Context(), x, "same text"); !errors.Is(err, ErrRequestReused) {
				t.Fatalf("retry changed %s: %v", field, err)
			}
		})
	}
}
