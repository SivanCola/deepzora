package main

import (
	"deepzora/internal/config"
	"deepzora/internal/teamroster"
	"encoding/json"
	"strings"
	"testing"
)

func TestTeamSecretsRemainUnavailableUntilMaskingIsReady(t *testing.T) {
	a, _ := newMemoryTeamApp(t)
	view, err := a.GetTeamSecrets()
	if err != nil || view.Available || len(view.Secrets) != 0 {
		t.Fatalf("opened feature before masking: %+v %v", view, err)
	}
	result, err := a.SaveTeamSecret(SaveTeamSecretRequest{Operation: "save", Name: "DEPLOY", Value: "must-never-be-written"})
	if err != nil || result.Reason != "unavailable" {
		t.Fatalf("accepted unmasked secret: %+v %v", result, err)
	}
}

func TestUserSecretRotationKeepsRetiredSlotsAndExplicitEmptyGrants(t *testing.T) {
	slots := config.TeamSecretSlots{Dir: t.TempDir()}
	roster := teamroster.Roster{}
	member := roster.AddAgent(teamroster.Agent{Name: "Ada"})
	book := teamroster.TeamSecretBook{}
	req := SaveTeamSecretRequest{RequestID: "save-secret-fixture", Operation: "save", Name: "DEPLOY", Purpose: "deploy", Value: "first-value", Agents: []string{member.ID}}
	if err := applyUserTeamSecret(t.Context(), &book, roster, slots, req); err != nil {
		t.Fatal(err)
	}
	old := book.Secrets[0].Slot
	req.Value = "replacement-value"
	req.Agents = []string{}
	if err := applyUserTeamSecret(t.Context(), &book, roster, slots, req); err != nil {
		t.Fatal(err)
	}
	if len(book.Retired) != 1 || book.Retired[0].Slot != old || len(book.Secrets[0].Agents) != 0 {
		t.Fatalf("lost revoke/retirement: %+v", book)
	}
	raw, err := json.Marshal(book)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(raw), "first-value") || strings.Contains(string(raw), "replacement-value") {
		t.Fatal("serialized secret value into metadata")
	}
	req.Operation = "forget_retired"
	req.ConfirmForget = false
	if err := applyUserTeamSecret(t.Context(), &book, roster, slots, req); err == nil {
		t.Fatal("forgot old values without new confirmation")
	}
	req.ConfirmForget = true
	if err := applyUserTeamSecret(t.Context(), &book, roster, slots, req); err != nil || len(book.Retired) != 0 {
		t.Fatalf("confirmed forget: %+v %v", book, err)
	}
}
