package main

import (
	"reflect"
	"testing"

	"deepzora/internal/agentpreset"
	"deepzora/internal/boot"
)

func TestTeamAgentCopiesSelectedProjectPresetIntoIndependentDirectory(t *testing.T) {
	for _, targetDirectory := range []string{"default", "other-project"} {
		for _, legacyID := range []bool{false, true} {
			t.Run(targetDirectory+map[bool]string{false: "/snapshot", true: "/snapshot-with-id"}[legacyID], func(t *testing.T) {
				isolateDesktopUserDirs(t)
				a := NewApp()
				a.ctx = t.Context()
				installNoopRuntimeEvents(a)
				t.Cleanup(func() {
					if err := a.stopTeamHost(); err != nil {
						t.Error(err)
					}
				})
				id, sourceRoot, targetRoot := "review", t.TempDir(), ""
				if targetDirectory == "other-project" {
					targetRoot = t.TempDir()
					if _, err := agentpreset.Write(boot.AgentPresetDirs(targetRoot).Project, id, "WRONG_TARGET_PROJECT"); err != nil {
						t.Fatal(err)
					}
				}
				if _, err := agentpreset.Write(boot.AgentPresetDirs("").User, id, "WRONG_USER_PRESET"); err != nil {
					t.Fatal(err)
				}
				if _, err := agentpreset.Write(boot.AgentPresetDirs(sourceRoot).Project, id, "---\nname: Source project\ncomplete: true\ninclude-runtime-context: false\ntools: []\n---\nSELECTED_PROJECT_PERSONA"); err != nil {
					t.Fatal(err)
				}
				selected, err := agentpreset.Resolve(boot.AgentPresetDirs(sourceRoot), id)
				if err != nil {
					t.Fatal(err)
				}
				req := SaveTeamAgentRequest{RequestID: "copy-project-snapshot", Settings: TeamAgentSettings{Name: "Ada", Permission: "read-only", Folder: targetRoot, Preset: teamPresetView(&selected)}}
				if legacyID {
					req.PresetID = &id
				}
				result, err := a.SaveTeamAgent(req)
				if err != nil || result.Outcome != "applied" {
					t.Fatalf("copy selected project preset: %+v %v", result, err)
				}
				copied := result.View.Roster.Agents[0].Preset
				if !reflect.DeepEqual(copied, req.Settings.Preset) {
					t.Fatalf("changed selected configuration: got %+v, want %+v", copied, req.Settings.Preset)
				}
				if _, err := agentpreset.Write(boot.AgentPresetDirs(sourceRoot).Project, id, "SOURCE_CHANGED_LATER"); err != nil {
					t.Fatal(err)
				}
				view, err := a.GetTeam()
				if err != nil || !reflect.DeepEqual(view.Roster.Agents[0].Preset, req.Settings.Preset) {
					t.Fatalf("member did not retain independent copy: %+v %v", view, err)
				}
			})
		}
	}
}

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
