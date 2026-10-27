package main

import (
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

func TestRuntimeProjectionDoesNotReadPreviewFiles(t *testing.T) {
	isolateDesktopUserDirs(t)
	dir := t.TempDir()
	preview := filepath.Join(dir, "unrelated-preview.json")
	a := &App{tabs: map[string]*WorkspaceTab{
		"one": {ID: "one", Scope: "project", WorkspaceRoot: dir, TopicID: "topic", SessionID: "first"},
		"two": {ID: "two", Scope: "project", WorkspaceRoot: dir, TopicID: "topic", SessionID: "second"},
	}}
	if err := os.WriteFile(preview, []byte(`{"preview":"before"}`), 0o600); err != nil {
		t.Fatal(err)
	}
	before := a.GetRuntimeStateSnapshot()
	if err := os.WriteFile(preview, []byte(`{"preview":"after"}`), 0o600); err != nil {
		t.Fatal(err)
	}
	for range 20 {
		if after := a.GetRuntimeStateSnapshot(); !reflect.DeepEqual(before, after) {
			t.Fatalf("unstable memory projection: %+v", after)
		}
	}
}
