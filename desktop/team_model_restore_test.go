package main

import (
	"testing"

	"deepzora/internal/control"
	"deepzora/internal/team"
)

func TestTeamEmptySeatRestoresSavedModelBeforeFirstMessage(t *testing.T) {
	a, owner, _, id, _ := teamAdmissionFixture(t)
	if _, err := a.SaveProviderWithKey(ProviderView{Name: "team-alternate", Kind: "openai", BaseURL: "http://127.0.0.1:1", Models: []string{"other"}, NoProxy: true}, "disposable-fixture"); err != nil {
		t.Fatal(err)
	}
	meta, err := a.RevealBackgroundRuntime(owner.ID)
	if err != nil {
		t.Fatal(err)
	}
	if err := a.SetModelForTab(meta.ID, "team-alternate/other"); err != nil {
		t.Fatal(err)
	}
	ctrl := a.controllerForTab(a.tabByID(meta.ID)).(*control.Controller)
	ref, ok := ctrl.SessionRef()
	if !ok {
		t.Fatal("no session")
	}
	if err := a.CloseTab(meta.ID); err != nil {
		t.Fatal(err)
	}
	runtime := &desktopTeamRuntime{app: a, ref: team.SeatRef{Agent: id}, sessionID: ref.SessionID}
	if err := runtime.Close(t.Context()); err != nil {
		t.Fatal(err)
	}
	if err := a.desktopSessionService("").Close(t.Context(), ref); err != nil {
		t.Fatal(err)
	}
	h, err := a.localTeamHost()
	if err != nil {
		t.Fatal(err)
	}
	pane, err := a.openTeamRuntime(t.Context(), team.SeatRef{Agent: id}, h.BindingService())
	if err != nil {
		t.Fatal(err)
	}
	restored := a.controllerForTab(pane.(*desktopTeamRuntime).owner()).(*control.Controller)
	if got := restored.ModelRef(); got != "team-alternate/other" {
		t.Fatalf("saved empty-seat model = %q, want team-alternate/other", got)
	}
}
