package service

import (
	"deepzora/internal/config"
	"deepzora/internal/provider"
	"deepzora/internal/sandbox"
	"deepzora/internal/secrets"
	"deepzora/internal/team"
	"deepzora/internal/tool/builtin"
	"encoding/json"
	"strings"
	"testing"
)

func TestDesktopTeamSecretSaveCommandRotationAndOutboundProjection(t *testing.T) {
	previousGate := teamSecretMaskingReady
	teamSecretMaskingReady = func() bool { return true }
	defer func() { teamSecretMaskingReady = previousGate }()
	a, id := newMemoryTeamApp(t)
	view, err := a.GetTeamSecrets()
	if err != nil {
		t.Fatal(err)
	}
	value := "boundary-live-value-91827364"
	request := SaveTeamSecretRequest{ExpectedRevision: view.Revision, ExpectedRosterRevision: view.RosterRevision, RequestID: "boundary-save-secret", Operation: "save", Name: "BOUNDARY", Value: value, Agents: []string{id}}
	saved, err := a.SaveTeamSecret(request)
	if err != nil || saved.Outcome != "applied" {
		t.Fatalf("save: %+v %v", saved, err)
	}
	raw, _ := json.Marshal(saved.View)
	if strings.Contains(string(raw), value) || strings.Contains(string(raw), "ts-") {
		t.Fatalf("metadata exposed a value or credential slot: %s", raw)
	}
	h, err := a.localTeamHost()
	if err != nil {
		t.Fatal(err)
	}
	seat := team.Seat{Service: h.BindingService(), Ref: team.SeatRef{Agent: id}}
	sh := sandbox.ResolveShell("", "", nil)
	command := `printf '%s' "$DEEPZORA_SECRET_BOUNDARY"`
	if sh.Kind == sandbox.ShellPowerShell {
		command = `[Console]::Out.Write($env:DEEPZORA_SECRET_BOUNDARY)`
	}
	runner := builtin.ConfineBash(sandbox.Spec{Mode: "off", Shell: sh}, builtin.SessionDataGuard{})
	runner, ok := builtin.BindTeamSecretEnvironment(runner, seat.CommandSecretEnvironment)
	if !ok {
		t.Fatal("shell injection owner was not bound")
	}
	args, _ := json.Marshal(map[string]string{"command": command})
	output, err := runner.Execute(sandbox.WithPermissionPreset(t.Context(), "danger-full-access"), args)
	if err != nil {
		t.Fatalf("native secret command: %v", err)
	}
	// The runner is the execution boundary. Agent/events/provider/history own
	// presentation; the model never receives the actual command environment.
	if !strings.Contains(output, value) {
		t.Fatalf("authorized native command did not receive value: %q", output)
	}
	projected := provider.MaskOutbound(provider.Request{Messages: []provider.Message{{Role: provider.RoleTool, Content: output}}}, provider.SignedProjection{})
	if strings.Contains(projected.Messages[0].Content, value) || !strings.Contains(projected.Messages[0].Content, "[secret:BOUNDARY]") {
		t.Fatalf("provider boundary leaked command output: %+v", projected)
	}
	request.ExpectedRevision = saved.View.Revision
	request.RequestID = "boundary-delete-secret"
	request.Operation = "delete"
	request.Value = ""
	deleted, err := a.SaveTeamSecret(request)
	if err != nil || deleted.Outcome != "applied" {
		t.Fatalf("delete: %+v %v", deleted, err)
	}
	if _, err := seat.CommandSecretEnvironment(t.Context(), sh.Kind.String(), sh.Path, command); err == nil {
		t.Fatal("deleted secret authorized a new command")
	}
	if got := secrets.Masker().String(value); got != "[secret:BOUNDARY]" {
		t.Fatalf("retired value no longer masked: %s", got)
	}
	book, err := desktopTeamStore().TeamSecrets()
	if err != nil {
		t.Fatal(err)
	}
	if len(book.Retired) != 1 {
		t.Fatal("retired value was not persisted")
	}
	if _, err := config.DefaultTeamSecretSlots().Read(book.Retired[0].Slot); err != nil {
		t.Fatal("restart cannot restore retired mask")
	}
	request.ExpectedRevision = deleted.View.Revision
	request.RequestID = "boundary-forget-without-confirm"
	request.Operation = "forget_retired"
	request.Name = ""
	rejected, err := a.SaveTeamSecret(request)
	if err != nil || rejected.Outcome != "rejected" || secrets.Masker().String(value) == value {
		t.Fatalf("retired masks forgotten without confirmation: %+v %v", rejected, err)
	}
	request.RequestID = "boundary-forget-confirmed"
	request.ConfirmForget = true
	forgotten, err := a.SaveTeamSecret(request)
	if err != nil || forgotten.Outcome != "applied" || forgotten.View.RetiredCount != 0 || secrets.Masker().String(value) != value {
		t.Fatalf("confirmed retirement did not forget its value: %+v %v", forgotten, err)
	}
	if _, err := config.DefaultTeamSecretSlots().Read(book.Retired[0].Slot); err == nil {
		t.Fatal("confirmed forgetting retained its plaintext credential slot")
	}
}
