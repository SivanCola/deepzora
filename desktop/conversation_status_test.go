package service

import (
	"context"
	"deepzora/internal/agent"
	"deepzora/internal/gitcmd"
	"deepzora/internal/provider"
	"deepzora/internal/session"
	"deepzora/internal/subagentpersist"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestStatusGitActionsAreScopedAndCommitOnlyStagedChanges(t *testing.T) {
	root, other := gitScopeRepo(t), gitScopeRepo(t)
	a := &App{tabs: map[string]*WorkspaceTab{"a": {ID: "a", WorkspaceRoot: root}, "b": {ID: "b", WorkspaceRoot: other}}}
	for _, args := range [][]string{{"config", "user.name", "Test"}, {"config", "user.email", "test@example.invalid"}} {
		if out, err := gitcmd.Command(t.Context(), root, args...).CombinedOutput(); err != nil {
			t.Fatalf("git config: %v %s", err, out)
		}
	}
	if err := os.WriteFile(filepath.Join(root, "file.txt"), []byte("one\n"), 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := a.RunWorkspaceGitActionForTab("a", other, "stage", ""); err == nil {
		t.Fatal("accepted a replacement workspace")
	}
	if _, err := a.RunWorkspaceGitActionForTab("a", root, "reset", ""); err == nil {
		t.Fatal("accepted an unlisted action")
	}
	if _, err := a.RunWorkspaceGitActionForTab("a", root, "stage", ""); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "unstaged.txt"), []byte("two\n"), 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := a.RunWorkspaceGitActionForTab("a", root, "commit", "test status panel"); err != nil {
		t.Fatal(err)
	}
	output, err := gitcmd.Command(t.Context(), root, "show", "--format=", "--name-only", "HEAD").Output()
	if err != nil || strings.TrimSpace(string(output)) != "file.txt" {
		t.Fatalf("commit staged scope: %s %v", output, err)
	}
	a.tabs["a"].ReadOnly = true
	if _, err := a.RunWorkspaceGitActionForTab("a", root, "stage", ""); err == nil {
		t.Fatal("read-only session mutated Git")
	}
}

func TestStatusSubdirectoryCommitRejectsHiddenStagedPaths(t *testing.T) {
	for _, change := range []string{"add", "delete", "rename-in", "rename-out"} {
		t.Run(change, func(t *testing.T) {
			root := gitScopeRepo(t)
			git := func(args ...string) string {
				t.Helper()
				out, err := gitcmd.Command(t.Context(), root, args...).CombinedOutput()
				if err != nil {
					t.Fatalf("git %v: %v %s", args, err, out)
				}
				return strings.TrimSpace(string(out))
			}
			git("config", "user.name", "Test")
			git("config", "user.email", "test@example.invalid")
			workspace := filepath.Join(root, "pkg")
			if err := os.Mkdir(workspace, 0700); err != nil {
				t.Fatal(err)
			}
			write := func(path, text string) {
				t.Helper()
				if err := os.WriteFile(filepath.Join(root, path), []byte(text), 0600); err != nil {
					t.Fatal(err)
				}
			}
			write("outside.txt", "existing outside\n")
			write("pkg/moved.txt", "existing inside\n")
			git("add", ".")
			git("commit", "-m", "seed")
			switch change {
			case "add":
				// A prefix sibling must not count as inside pkg/.
				write("pkg-other.txt", "hidden\n")
				git("add", "pkg-other.txt")
			case "delete":
				git("rm", "outside.txt")
			case "rename-in":
				git("mv", "outside.txt", "pkg/renamed.txt")
			case "rename-out":
				git("mv", "pkg/moved.txt", "renamed.txt")
			}
			write("pkg/inside.txt", "staged version\n")
			git("add", "pkg/inside.txt")
			write("pkg/inside.txt", "unstaged version\n")
			beforeHead, beforeIndex := git("rev-parse", "HEAD"), git("write-tree")
			a := &App{tabs: map[string]*WorkspaceTab{"a": {ID: "a", WorkspaceRoot: workspace}}}
			if _, err := a.RunWorkspaceGitActionForTab("a", workspace, "commit", "panel commit"); err == nil || !strings.Contains(err.Error(), "outside this workspace") {
				t.Fatalf("hidden staged %s accepted: %v", change, err)
			}
			if git("rev-parse", "HEAD") != beforeHead || git("write-tree") != beforeIndex {
				t.Fatal("rejected commit mutated HEAD or index")
			}
			// Once the user unstages the wider index, commit only the staged
			// inside version, leaving the newer working-tree content intact.
			git("reset", "--mixed", "HEAD")
			write("pkg/inside.txt", "staged version\n")
			git("add", "pkg/inside.txt")
			write("pkg/inside.txt", "unstaged version\n")
			if _, err := a.RunWorkspaceGitActionForTab("a", workspace, "commit", "panel commit"); err != nil {
				t.Fatal(err)
			}
			if git("show", "HEAD:pkg/inside.txt") != "staged version" || git("show", "--format=", "--name-only", "HEAD") != "pkg/inside.txt" {
				t.Fatal("commit included hidden paths or unstaged file content")
			}
			data, err := os.ReadFile(filepath.Join(workspace, "inside.txt"))
			if err != nil || string(data) != "unstaged version\n" {
				t.Fatalf("commit changed working-tree content: %q %v", data, err)
			}
		})
	}
}

type statusRuntimeController struct {
	*forkTargetsStubController
	service *session.Service
	ref     session.SessionRef
}

func (c *statusRuntimeController) SessionService() *session.Service       { return c.service }
func (c *statusRuntimeController) SessionRef() (session.SessionRef, bool) { return c.ref, true }

func TestStatusReadOnlyTabCannotCancelOwnedLiveChild(t *testing.T) {
	a, tab := currentBoundHistoryFixture(t, nil)
	query, ref, err := a.canonicalSessionQuery(tab.ID)
	if err != nil {
		t.Fatal(err)
	}
	service := a.desktopSessionService(query.StorageRoot())
	tab.Ctrl = &statusRuntimeController{forkTargetsStubController: &forkTargetsStubController{tabScopedActionController: newTabScopedActionController()}, service: service, ref: ref}
	children, err := service.Subagents()
	if err != nil {
		t.Fatal(err)
	}
	meta := agent.SubagentMeta{Ref: "sa_readonly", ParentSession: ref.SessionID, Status: agent.SubagentRunning, CreatedAt: time.Now(), UpdatedAt: time.Now()}
	handle, err := subagentpersist.New(children).Create(t.Context(), meta, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer handle.Close(context.Background())
	data, _ := json.Marshal(meta)
	if err := os.WriteFile(filepath.Join(children.Query().StorageRoot(), meta.Ref+".meta.json"), data, 0600); err != nil {
		t.Fatal(err)
	}
	ctx, release := (&agent.SubagentRun{Persistence: handle}).CancellableContext(t.Context())
	defer release()
	tab.ReadOnly = true
	if accepted, err := a.CancelSubagentForTab(tab.ID, meta.Ref); accepted || err == nil || !strings.Contains(err.Error(), "read-only") {
		t.Fatalf("read-only cancellation = %v, %v", accepted, err)
	}
	if ctx.Err() != nil {
		t.Fatal("read-only backend call cancelled the child")
	}
	if _, err := a.ListSubagentRunsForTab(tab.ID); err != nil {
		t.Fatalf("read-only inspection should remain available: %v", err)
	}
	tab.ReadOnly = false
	if accepted, err := a.CancelSubagentForTab(tab.ID, meta.Ref); !accepted || err != nil || ctx.Err() != context.Canceled {
		t.Fatalf("writable owned cancellation = %v, %v, ctx=%v", accepted, err, ctx.Err())
	}
}

func TestStatusChildHistoryRejectsForeignParentAndUsesBoundedPages(t *testing.T) {
	a, tab := currentBoundHistoryFixture(t, []provider.Message{{Role: provider.RoleUser, Content: "parent"}})
	query, ref, err := a.canonicalSessionQuery(tab.ID)
	if err != nil {
		t.Fatal(err)
	}
	children, err := query.Subagents()
	if err != nil {
		t.Fatal(err)
	}
	// Write current canonical child sessions and their real ownership metadata.
	persistence := session.NewFilesystemPersistence(children.StorageRoot())
	service, err := session.NewService(ref.HostID, persistence)
	if err != nil {
		t.Fatal(err)
	}
	defer service.Shutdown(context.Background())
	longText := strings.Repeat("中文\n\"\\", 70_000)
	for _, fixture := range []struct{ id, parent string }{{"sa_own", ref.SessionID}, {"sa_foreign", "another-parent"}} {
		runtime, err := service.Create(t.Context(), session.CreateOptions{SessionID: fixture.id})
		if err != nil {
			t.Fatal(err)
		}
		appendSessionTestMessage(t, runtime, "child-message", provider.Message{ID: "child-message", Role: provider.RoleAssistant, Content: "child answer"})
		if fixture.id == "sa_own" {
			appendSessionTestMessage(t, runtime, "long-message", provider.Message{ID: "long-message", Role: provider.RoleAssistant, Content: longText})
		}
		if err := service.Close(t.Context(), runtime.Ref()); err != nil {
			t.Fatal(err)
		}
		meta, _ := json.Marshal(agent.SubagentMeta{Ref: fixture.id, ParentSession: fixture.parent, Name: "review", Status: agent.SubagentCompleted, CreatedAt: time.Now(), UpdatedAt: time.Now()})
		if err := os.WriteFile(filepath.Join(children.StorageRoot(), fixture.id+".meta.json"), meta, 0600); err != nil {
			t.Fatal(err)
		}
	}
	list, err := a.ListSubagentRunsForTab(tab.ID)
	if err != nil || len(list) != 1 || list[0].Ref != "sa_own" {
		t.Fatalf("owned list: %+v %v", list, err)
	}
	if _, err := a.ReadSubagentHistoryForTab(tab.ID, "sa_foreign", session.HistoryWindowRequest{Anchor: "newest"}); err == nil {
		t.Fatal("foreign child history exposed")
	}
	if _, err := a.CancelSubagentForTab(tab.ID, "sa_foreign"); err == nil {
		t.Fatal("foreign child cancellation accepted")
	}
	deadline := time.Now().Add(10 * time.Second)
	for {
		page, err := a.ReadSubagentHistoryForTab(tab.ID, "sa_own", session.HistoryWindowRequest{Anchor: "newest", Limit: 1000})
		if err != nil {
			t.Fatal(err)
		}
		if page.Status == "ready" {
			if len(page.Messages) != 2 || !strings.Contains(string(page.Messages[0].Inline), "child answer") {
				t.Fatalf("child page: %+v", page)
			}
			var full strings.Builder
			offset := int64(0)
			for {
				chunk, err := a.ReadSubagentMessageForTab(tab.ID, "sa_own", page.Messages[1].MessageID, page.Messages[1].Version, offset)
				if err != nil {
					t.Fatal(err)
				}
				if len(chunk.Text) > session.MessageFieldMaxChunk {
					t.Fatal("child text chunk exceeds its read budget")
				}
				full.WriteString(chunk.Text)
				if chunk.NextOffset == 0 {
					break
				}
				if chunk.NextOffset <= offset {
					t.Fatal("nonadvancing child body cursor")
				}
				offset = chunk.NextOffset
			}
			if full.String() != longText {
				t.Fatal("child message chunks changed escaped or UTF-8 content")
			}
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("child history remained %s", page.Status)
		}
		time.Sleep(10 * time.Millisecond)
	}
}

func TestStatusGitNetworkActionsUseNormalPushAndFastForwardPull(t *testing.T) {
	root := gitScopeRepo(t)
	remote := filepath.Join(t.TempDir(), "origin.git")
	git := func(dir string, args ...string) {
		t.Helper()
		if out, err := gitcmd.Command(t.Context(), dir, args...).CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v %s", args, err, out)
		}
	}
	git(root, "init", "--bare", remote)
	git(root, "remote", "add", "origin", remote)
	git(root, "push", "--set-upstream", "origin", "main")
	a := &App{tabs: map[string]*WorkspaceTab{"a": {ID: "a", WorkspaceRoot: root}}}
	for _, dir := range []string{root} {
		git(dir, "config", "user.name", "Test")
		git(dir, "config", "user.email", "test@example.invalid")
	}
	if err := os.WriteFile(filepath.Join(root, "local.txt"), []byte("local\n"), 0600); err != nil {
		t.Fatal(err)
	}
	for _, action := range []string{"stage", "commit", "push"} {
		if _, err := a.RunWorkspaceGitActionForTab("a", root, action, "local change"); err != nil {
			t.Fatal(err)
		}
	}
	peer := filepath.Join(t.TempDir(), "peer")
	git(root, "clone", "--branch", "main", remote, peer)
	git(peer, "config", "user.name", "Test")
	git(peer, "config", "user.email", "test@example.invalid")
	if err := os.WriteFile(filepath.Join(peer, "remote.txt"), []byte("remote\n"), 0600); err != nil {
		t.Fatal(err)
	}
	git(peer, "add", ".")
	git(peer, "commit", "-m", "remote change")
	git(peer, "push")
	for _, action := range []string{"fetch", "pull"} {
		if _, err := a.RunWorkspaceGitActionForTab("a", root, action, ""); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := os.Stat(filepath.Join(root, "remote.txt")); err != nil {
		t.Fatal("fast-forward pull did not apply the remote commit")
	}
	// Divergence must fail instead of silently creating a merge or rewriting refs.
	if err := os.WriteFile(filepath.Join(root, "ours.txt"), []byte("ours\n"), 0600); err != nil {
		t.Fatal(err)
	}
	git(root, "add", ".")
	git(root, "commit", "-m", "ours")
	if err := os.WriteFile(filepath.Join(peer, "theirs.txt"), []byte("theirs\n"), 0600); err != nil {
		t.Fatal(err)
	}
	git(peer, "add", ".")
	git(peer, "commit", "-m", "theirs")
	git(peer, "push")
	if _, err := a.RunWorkspaceGitActionForTab("a", root, "pull", ""); err == nil {
		t.Fatal("divergent pull was not rejected")
	}
	if _, err := a.RunWorkspaceGitActionForTab("a", root, "push", ""); err == nil {
		t.Fatal("divergent push unexpectedly rewrote the remote")
	}
}
