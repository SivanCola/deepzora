package main

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"strings"
	"time"

	"deepzora/internal/agent"
	"deepzora/internal/gitcmd"
	"deepzora/internal/jobs"
	"deepzora/internal/session"
)

type BackgroundJobInspection struct {
	ID     string `json:"id"`
	Kind   string `json:"kind"`
	Label  string `json:"label"`
	Status string `json:"status"`
	Output string `json:"output"`
}

func (a *App) InspectBackgroundJobForTab(tabID, jobID string) (BackgroundJobInspection, error) {
	if strings.TrimSpace(tabID) == "" || strings.TrimSpace(jobID) == "" {
		return BackgroundJobInspection{}, fmt.Errorf("tab and job are required")
	}
	reader, ok := a.ctrlForRuntimeTabID(tabID).(interface {
		InspectBackgroundJob(string) (jobs.Result, bool)
	})
	if !ok {
		return BackgroundJobInspection{}, fmt.Errorf("background job inspection is unavailable")
	}
	result, found := reader.InspectBackgroundJob(jobID)
	if !found {
		return BackgroundJobInspection{}, fmt.Errorf("background job is unavailable in this session")
	}
	return BackgroundJobInspection{ID: result.ID, Kind: result.Kind, Label: result.Label, Status: string(result.Status), Output: result.Output}, nil
}

type SubagentRunView struct {
	Ref       string `json:"ref"`
	Name      string `json:"name"`
	Kind      string `json:"kind"`
	Status    string `json:"status"`
	Outcome   string `json:"outcome,omitempty"`
	CreatedAt int64  `json:"createdAt"`
	UpdatedAt int64  `json:"updatedAt"`
}

// The parent query selects the host and storage root; callers never supply a
// filesystem path. Child refs are authorized against that exact parent.
func (a *App) subagentQueryForTab(tabID, childRef string) (*session.Query, session.SessionRef, error) {
	if strings.TrimSpace(tabID) == "" {
		return nil, session.SessionRef{}, fmt.Errorf("parent tab is required")
	}
	parent, parentRef, err := a.canonicalSessionQuery(tabID)
	if err != nil {
		return nil, session.SessionRef{}, err
	}
	children, err := parent.Subagents()
	if err != nil {
		return nil, session.SessionRef{}, err
	}
	artifacts, err := agent.ListSubagentsByParent(children.StorageRoot(), parentRef.SessionID)
	if err != nil {
		return nil, session.SessionRef{}, err
	}
	for _, artifact := range artifacts {
		if artifact.Ref == childRef {
			return children, session.SessionRef{HostID: parentRef.HostID, SessionID: childRef}, nil
		}
	}
	return nil, session.SessionRef{}, fmt.Errorf("subagent does not belong to this session")
}

func (a *App) ListSubagentRunsForTab(tabID string) ([]SubagentRunView, error) {
	if strings.TrimSpace(tabID) == "" {
		return nil, fmt.Errorf("parent tab is required")
	}
	parent, ref, err := a.canonicalSessionQuery(tabID)
	if err != nil {
		return nil, err
	}
	children, err := parent.Subagents()
	if err != nil {
		return nil, err
	}
	artifacts, err := agent.ListSubagentsByParent(children.StorageRoot(), ref.SessionID)
	if err != nil {
		return nil, err
	}
	out := make([]SubagentRunView, 0, len(artifacts))
	for _, item := range artifacts {
		m := item.Meta
		out = append(out, SubagentRunView{Ref: m.Ref, Name: m.Name, Kind: m.Kind, Status: string(m.Status), Outcome: m.Outcome, CreatedAt: m.CreatedAt.UnixMilli(), UpdatedAt: m.UpdatedAt.UnixMilli()})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].CreatedAt > out[j].CreatedAt })
	return out, nil
}

func (a *App) ReadSubagentHistoryForTab(tabID, childRef string, request session.HistoryWindowRequest) (session.HistoryWindowPage, error) {
	query, ref, err := a.subagentQueryForTab(tabID, childRef)
	if err != nil {
		return session.HistoryWindowPage{}, err
	}
	request.Limit = 32
	return query.ReadHistoryWindow(context.Background(), ref, request)
}

type SubagentMessageText struct {
	Text       string `json:"text"`
	NextOffset int64  `json:"nextOffset"`
}

// Read one UTF-8 aligned chunk without making the UI retain a whole message.
func (a *App) ReadSubagentMessageForTab(tabID, childRef, messageID string, version int, offset int64) (SubagentMessageText, error) {
	query, ref, err := a.subagentQueryForTab(tabID, childRef)
	if err != nil {
		return SubagentMessageText{}, err
	}
	page, err := query.ReadMessageField(context.Background(), ref, messageID, version, "content", offset, session.MessageFieldMaxChunk)
	if err != nil {
		return SubagentMessageText{}, err
	}
	if page.Status != "ready" {
		return SubagentMessageText{}, fmt.Errorf("subagent message is not ready")
	}
	data := page.Data
	if len(data) == 0 {
		return SubagentMessageText{}, nil
	}
	if offset == 0 && data[0] == '"' {
		data = data[1:]
	}
	if page.NextOffset == 0 && len(data) > 0 && data[len(data)-1] == '"' {
		data = data[:len(data)-1]
	}
	var text string
	quoted := append([]byte{'"'}, data...)
	quoted = append(quoted, '"')
	if err := json.Unmarshal(quoted, &text); err != nil {
		return SubagentMessageText{}, err
	}
	return SubagentMessageText{Text: text, NextOffset: page.NextOffset}, nil
}

func (a *App) CancelSubagentForTab(tabID, childRef string) (bool, error) {
	a.mu.RLock()
	tab := a.tabByIDLocked(tabID)
	readOnly := tab == nil || tab.ReadOnly
	a.mu.RUnlock()
	if readOnly {
		return false, fmt.Errorf("this session is read-only")
	}
	query, ref, err := a.subagentQueryForTab(tabID, childRef)
	if err != nil {
		return false, err
	}
	// Cancel through the child catalog's existing runtime. Query intentionally
	// owns no execution controls; obtain the same host service from the parent.
	service, err := a.subagentServiceForTab(tabID)
	if err != nil {
		return false, err
	}
	if service.Query().StorageRoot() != query.StorageRoot() {
		return false, fmt.Errorf("session host changed")
	}
	runtime, ok := service.Runtime(ref)
	if !ok {
		return false, nil
	}
	return runtime.Cancel(), nil
}

func (a *App) subagentServiceForTab(tabID string) (*session.Service, error) {
	ctrl := a.ctrlForRuntimeTabID(tabID)
	identity, ok := ctrl.(interface{ SessionService() *session.Service })
	if !ok || identity.SessionService() == nil {
		return nil, fmt.Errorf("subagent runtime is unavailable")
	}
	return identity.SessionService().Subagents()
}

// Explicit user Git actions stay pinned to the tab's expected workspace.
// There is no force/reset command in this surface.
func (a *App) RunWorkspaceGitActionForTab(tabID, workspaceRoot, action, message string) (string, error) {
	repo, err := a.gitWorkspaceRepoForTab(tabID, workspaceRoot)
	if err != nil {
		return "", err
	}
	a.mu.RLock()
	tab := a.tabByIDLocked(tabID)
	readOnly := tab == nil || tab.ReadOnly
	a.mu.RUnlock()
	if readOnly {
		return "", fmt.Errorf("this session is read-only")
	}
	if ctrl := a.ctrlForRuntimeTabID(tabID); ctrl != nil && ctrl.Running() {
		return "", fmt.Errorf("wait for the agent to become idle before running Git actions")
	}
	args := []string{"-C", repo.Dir}
	switch action {
	case "stage":
		args = append(args, "add", "--all", "--", ".")
	case "commit":
		message = strings.TrimSpace(message)
		if message == "" {
			return "", fmt.Errorf("commit message is required")
		}
		if err := workspaceCommitScope(repo); err != nil {
			return "", err
		}
		args = append(args, "commit", "-m", message)
	case "fetch":
		args = append(args, "fetch")
	case "pull":
		args = append(args, "pull", "--ff-only")
	case "push":
		args = append(args, "push")
	default:
		return "", fmt.Errorf("unsupported Git action")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	output := &statusCommandTail{}
	command := workspaceGitCommand(ctx, repo, args...)
	command.Stdout = output
	command.Stderr = output
	err = command.Run()
	if err != nil {
		return string(output.data), fmt.Errorf("git %s: %w: %s", action, err, output.data)
	}
	return string(output.data), nil
}

// A normal commit consumes the whole index, even from a workspace subdirectory.
// Reject hidden staged paths rather than using a pathspec commit, which would
// also pick up unstaged working-tree edits to the selected files.
func workspaceCommitScope(repo gitcmd.Repo) error {
	prefix, err := workspaceGitOutputWithTimeout(3*time.Second, repo, "rev-parse", "--show-prefix")
	if err != nil {
		return fmt.Errorf("check commit workspace: %w", err)
	}
	workspacePrefix := strings.TrimSuffix(string(prefix), "\n")
	if workspacePrefix == "" {
		return nil
	}
	paths, err := workspaceGitOutputWithTimeout(3*time.Second, repo.Top(), "diff", "--cached", "--name-only", "--no-renames", "-z", "--")
	if err != nil {
		return fmt.Errorf("check staged paths: %w", err)
	}
	for path := range strings.SplitSeq(string(paths), "\x00") {
		if path != "" && !strings.HasPrefix(path, workspacePrefix) {
			return fmt.Errorf("staged changes outside this workspace; review the repository index before committing")
		}
	}
	return nil
}

type statusCommandTail struct{ data []byte }

func (b *statusCommandTail) Write(p []byte) (int, error) {
	n := len(p)
	const limit = 64 * 1024
	if len(p) >= limit {
		b.data = append(b.data[:0], p[len(p)-limit:]...)
	} else {
		if len(b.data)+len(p) > limit {
			b.data = b.data[len(b.data)+len(p)-limit:]
		}
		b.data = append(b.data, p...)
	}
	return n, nil
}
