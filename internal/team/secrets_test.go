package team

import (
	"context"
	"deepzora/internal/config"
	"deepzora/internal/teamroster"
	"errors"
	"testing"
	"time"
)

func TestTeamSecretCommandAuthorizationReloadsEveryCallAndNeverGrantsNewMembers(t *testing.T) {
	h := newHarness(t, "Chief", "Ada", "Ben")
	slots := config.TeamSecretSlots{Dir: t.TempDir()}
	h.svc.secretSlots = slots
	h.svc.secretsReady = func() bool { return true }
	slot, err := slots.Save(t.Context(), "private-secret-value")
	if err != nil {
		t.Fatal(err)
	}
	book, err := h.svc.store.MutateTeamSecrets(t.Context(), 0, "save-test-secret", "digest", func(book *teamroster.TeamSecretBook) error {
		book.Secrets = append(book.Secrets, teamroster.TeamSecret{Name: "DEPLOY", Slot: slot, Agents: []string{h.id("Ada")}})
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	ada := Seat{Service: h.svc, Ref: SeatRef{Agent: h.id("Ada")}}
	got, err := ada.CommandSecretEnvironment(t.Context(), "bash", "/bin/bash", `curl -H "$DEEPZORA_SECRET_DEPLOY"`)
	if err != nil || len(got) != 1 || got[0] != "DEEPZORA_SECRET_DEPLOY=private-secret-value" {
		t.Fatalf("explicit authorization: %v %v", got, err)
	}
	ben := Seat{Service: h.svc, Ref: SeatRef{Agent: h.id("Ben")}}
	if _, err := ben.CommandSecretEnvironment(t.Context(), "bash", "/bin/bash", `echo "$DEEPZORA_SECRET_DEPLOY"`); err == nil {
		t.Fatal("new member inherited secret")
	}
	_, err = h.svc.store.MutateTeamSecrets(t.Context(), book.Revision, "revoke-test-secret", "digest2", func(book *teamroster.TeamSecretBook) error {
		book.Retire(book.Secrets[0], time.Now())
		book.Secrets = nil
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := ada.CommandSecretEnvironment(t.Context(), "bash", "/bin/bash", `echo "$DEEPZORA_SECRET_DEPLOY"`); err == nil {
		t.Fatal("revocation used stale authorization")
	}
	h.svc.secretsReady = func() bool { return false }
	if _, err := ada.CommandSecretEnvironment(context.Background(), "bash", "/bin/bash", `echo "$DEEPZORA_SECRET_DEPLOY"`); !errors.Is(err, ErrSecretsUnavailable) {
		t.Fatalf("unmasked secret feature opened: %v", err)
	}
}
