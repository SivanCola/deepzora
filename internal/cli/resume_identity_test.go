package cli

import (
	"context"
	"errors"
	"path/filepath"
	"reflect"
	"testing"

	"deepzora/internal/config"
	"deepzora/internal/session"
)

func TestResumeTargetModelUsesSavedSelectionAndExplicitOverride(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "sessions")
	service := testCLISessionService(t, dir)
	createCanonicalTestSession(t, session.RootForSessionDir(dir), "model-source", "question")
	target := cliResumeTarget{ref: session.SessionRef{HostID: service.HostID(), SessionID: "model-source"}, service: service}
	if err := service.SetModel(t.Context(), target.ref, "saved/model", ""); err != nil {
		t.Fatal(err)
	}
	cfg := &config.Config{Providers: []config.ProviderEntry{{Name: "saved", Kind: "openai", BaseURL: "https://saved.invalid/v1", Model: "model"}}}
	model := ""
	if err := applyResumeTargetModel(&model, target, cfg); err != nil || model != "saved/model" {
		t.Fatalf("saved model = %q, %v", model, err)
	}
	model = "explicit/model"
	if err := applyResumeTargetModel(&model, target, cfg); err != nil || model != "explicit/model" {
		t.Fatalf("explicit model = %q, %v", model, err)
	}
}

func TestCanonicalResumeRefusalPreservesSourceBinding(t *testing.T) {
	m, ctrl, source, _ := newCanonicalSessionTUI(t)
	before, _ := ctrl.SessionRef()
	dir := filepath.Join(t.TempDir(), "sessions")
	owner, err := session.NewService("local", session.NewFilesystemPersistence(session.RootForSessionDir(dir)))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = owner.Shutdown(context.Background()) })
	held, err := owner.Create(t.Context(), session.CreateOptions{SessionID: "held-foreign"})
	if err != nil {
		t.Fatal(err)
	}
	foreign := testCLISessionService(t, dir)
	err = m.commitResumeTarget(cliResumeTarget{ref: held.Ref(), service: foreign})
	if !errors.Is(err, session.ErrWriterOwned) {
		t.Fatalf("resume error = %v, want writer owned", err)
	}
	after, _ := ctrl.SessionRef()
	if after != before || ctrl.SessionService() != source {
		t.Fatal("failed resume replaced the outgoing binding")
	}
	if err := ctrl.Snapshot(); err != nil {
		t.Fatalf("outgoing writer was lost: %v", err)
	}
}

func TestResumeAcrossCatalogsKeepsCanonicalIdentity(t *testing.T) {
	m, ctrl, original, _ := newCanonicalSessionTUI(t)
	originalRef, _ := ctrl.SessionRef()
	dir := filepath.Join(t.TempDir(), "sessions")
	foreign := testCLISessionService(t, dir)
	createCanonicalTestSession(t, session.RootForSessionDir(dir), "foreign", "other project question")
	entry := resumeEntry{target: cliResumeTarget{ref: session.SessionRef{HostID: foreign.HostID(), SessionID: "foreign"}, service: foreign}}
	if err := m.commitResumeTarget(entry.target); err != nil {
		t.Fatal(err)
	}
	if !resumeEntryIsActive(ctrl, entry) || ctrl.SessionService() != foreign {
		t.Fatal("resume did not adopt the foreign catalog binding")
	}
	if err := m.commitResumeTarget(cliResumeTarget{ref: originalRef, service: original}); err != nil {
		t.Fatalf("return to original catalog: %v", err)
	}
	if ctrl.SessionService() != original {
		t.Fatal("return did not restore the original catalog")
	}
}

func TestCanonicalCopyPreservesHistoryAndSource(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "sessions")
	service := testCLISessionService(t, dir)
	createCanonicalTestSession(t, session.RootForSessionDir(dir), "source", "copy this conversation")
	source := cliResumeTarget{ref: session.SessionRef{HostID: service.HostID(), SessionID: "source"}, service: service}
	before, err := service.Query().Snapshot(context.Background(), source.ref)
	if err != nil {
		t.Fatal(err)
	}
	copy, err := copyResumeTarget("", source, nil)
	if err != nil {
		t.Fatal(err)
	}
	if copy.ref == source.ref || copy.service != service {
		t.Fatal("copy did not create a distinct identity")
	}
	got, err := service.Query().History(context.Background(), copy.ref)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(got, before.Projection.Messages) {
		t.Fatalf("copy lost history: %+v", got)
	}
	after, err := service.Query().Snapshot(context.Background(), source.ref)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(before, after) {
		t.Fatal("copy changed the source")
	}
	if rc := copySessionExitCode(true, source); rc != 0 {
		t.Fatalf("copy refused current identity: %d", rc)
	}
}
