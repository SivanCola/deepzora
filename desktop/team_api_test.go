package main

import (
	"testing"

	"deepzora/internal/agentpreset"
	"deepzora/internal/boot"
)

func TestDesktopTeamConfigurationCASRetryAndPresetCopy(t *testing.T) {
	isolateDesktopUserDirs(t)
	a := NewApp()
	a.ctx = t.Context()
	installNoopRuntimeEvents(a)
	t.Cleanup(func() {
		if err := a.stopTeamHost(); err != nil {
			t.Error(err)
		}
	})
	presetID := "review"
	if _, err := agentpreset.Write(boot.AgentPresetDirs("").User, presetID, "---\ncomplete: true\ninclude-runtime-context: false\ntools: []\n---\nCOPY_ME"); err != nil {
		t.Fatal(err)
	}
	req := SaveTeamAgentRequest{ExpectedRevision: 0, RequestID: "create-review", PresetID: &presetID, Settings: TeamAgentSettings{Name: "Ada", Permission: "read-only"}}
	first, err := a.SaveTeamAgent(req)
	if err != nil || first.Outcome != "applied" {
		t.Fatalf("create: %+v %v", first, err)
	}
	retry, err := a.SaveTeamAgent(req)
	if err != nil || retry.TargetID != first.TargetID || retry.View.Roster.Revision != first.View.Roster.Revision {
		t.Fatalf("retry: %+v %v", retry, err)
	}
	req.Settings.Name = "Changed"
	changed, err := a.SaveTeamAgent(req)
	if err != nil || changed.Reason != "request_reused" {
		t.Fatalf("changed retry: %+v %v", changed, err)
	}
	req.RequestID = "stale-request"
	stale, err := a.SaveTeamAgent(req)
	if err != nil || stale.Reason != "conflict" {
		t.Fatalf("stale edit: %+v %v", stale, err)
	}
	if _, err := agentpreset.Write(boot.AgentPresetDirs("").User, presetID, "CHANGED"); err != nil {
		t.Fatal(err)
	}
	view, err := a.GetTeam()
	if err != nil {
		t.Fatal(err)
	}
	member := view.Roster.Agents[0]
	if member.Preset == nil || member.Preset.Body != "COPY_ME" || !member.Preset.Complete || member.Preset.IncludeRuntimeContext || member.Preset.Tools == nil || len(member.Preset.Tools) != 0 {
		t.Fatalf("incomplete preset copy: %+v", member.Preset)
	}
}
