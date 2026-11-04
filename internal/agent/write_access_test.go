package agent

import (
	"context"
	"encoding/json"
	"path/filepath"
	"strings"
	"testing"

	"deepzora/internal/sandbox"
	"deepzora/internal/tool"
	"deepzora/internal/tool/builtin"
)

func TestBindChildWriteRootsSnapshotsParentGrants(t *testing.T) {
	work := t.TempDir()
	extra := t.TempDir()
	parent := sandbox.NewWritableRootSet([]string{work})
	parent.GrantSession([]string{extra})
	reg := tool.NewRegistry()
	for _, tl := range (builtin.Workspace{Dir: work, WriteRoots: []string{work}, WriteRootSet: parent}).Tools("write_file") {
		if tl.Name() == "write_file" {
			reg.Add(tl)
		}
	}
	_, child := BindChildWriteRoots(reg, parent, WritePathSet{})
	later := t.TempDir()
	parent.GrantSession([]string{later})
	if child.Covers(later) {
		t.Fatal("child must not inherit later parent grants")
	}
	if !child.Covers(extra) {
		t.Fatal("child should inherit the snapshot that existed at spawn")
	}

}

func TestApplyWriteAccessSubagentUsesStructuredHint(t *testing.T) {
	work := t.TempDir()
	outside := t.TempDir()
	set := sandbox.NewWritableRootSet([]string{work})
	a := &Agent{svc: agentServices{
		writeRoots:            set,
		writeAccessExpandable: false,
		workspaceRoot:         work,
		homeDir:               work,
		stateRoot:             t.TempDir(),
	}}
	var write tool.Tool
	for _, tl := range (builtin.Workspace{Dir: work, WriteRoots: []string{work}, WriteRootSet: set}).Tools("write_file") {
		if tl.Name() == "write_file" {
			write = tl
		}
	}
	if write == nil {
		t.Fatal("write_file missing")
	}
	args, err := json.Marshal(map[string]string{"path": filepath.Join(outside, "x.go"), "content": "x"})
	if err != nil {
		t.Fatal(err)
	}
	out, early := a.applyWriteAccess(context.Background(), &toolCallPlan{
		execTool: write,
		permName: "write_file",
		permArgs: args,
	})
	if !early || !out.blocked {
		t.Fatalf("expected blocked write access, got %+v early=%v", out, early)
	}
	if !strings.Contains(out.output, "parent agent") {
		t.Fatalf("sub-agent hint missing: %s", out.output)
	}
}
