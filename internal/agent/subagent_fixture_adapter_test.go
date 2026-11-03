package agent

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"testing"
)

// Test adapters keep single-child fixtures independent of the public batch envelope.
// Production registers only Spawner and uses the same RunProfileSpec owner.
type ReadOnlySubagentRunner struct {
	task *SubagentRunner
}

func NewReadOnlySubagentRunner(task *SubagentRunner) *ReadOnlySubagentRunner {
	return &ReadOnlySubagentRunner{task: task}
}

func (r *ReadOnlySubagentRunner) Execute(ctx context.Context, args json.RawMessage) (string, error) {
	if r == nil || r.task == nil {
		return "", fmt.Errorf("read_only_task is not configured")
	}
	var p struct {
		Prompt      string   `json:"prompt"`
		Description string   `json:"description"`
		Tools       []string `json:"tools"`
		MaxSteps    int      `json:"max_steps"`
		Model       string   `json:"model"`
		Effort      string   `json:"effort"`
	}
	if err := json.Unmarshal(args, &p); err != nil {
		return "", fmt.Errorf("invalid args: %w", err)
	}
	// Every entry point compiles to a spec and runs through RunProfileSpec, so a
	// boundary added there cannot be missed by one caller. read_only_task keeps
	// its own promise of no durable side effects through Ephemeral.
	spec, err := r.task.buildTaskSpec(ctx, p.Prompt, p.Description, "", nil, p.Tools, p.MaxSteps, p.Model, p.Effort, "", false, true)
	if err != nil {
		return "", err
	}
	spec.Worker.SystemPrompt = DefaultReadOnlyTaskSystemPrompt
	spec.Context.Ephemeral = true
	return r.task.RunProfileSpec(ctx, spec)
}

func (t *SubagentRunner) Execute(ctx context.Context, args json.RawMessage) (string, error) {
	var p struct {
		Prompt          string   `json:"prompt"`
		Description     string   `json:"description"`
		Profile         string   `json:"profile"`
		WritePaths      []string `json:"write_paths"`
		Tools           []string `json:"tools"`
		MaxSteps        int      `json:"max_steps"`
		RunInBackground bool     `json:"run_in_background"`
		Model           string   `json:"model"`
		Effort          string   `json:"effort"`
		ContinueFrom    string   `json:"continue_from"`
	}
	if err := json.Unmarshal(args, &p); err != nil {
		return "", fmt.Errorf("invalid args: %w", err)
	}
	if strings.TrimSpace(p.Prompt) == "" {
		return "", fmt.Errorf("prompt is required")
	}

	spec, err := t.buildTaskSpec(ctx, p.Prompt, p.Description, p.Profile, p.WritePaths, p.Tools, p.MaxSteps, p.Model, p.Effort, p.ContinueFrom, p.RunInBackground, false)
	if err != nil {
		return "", err
	}
	return t.RunProfileSpec(ctx, spec)
}

func mustSubagentStore(t *testing.T) *SubagentStore {
	t.Helper()
	return newTestSubagentStore(t, t.TempDir())
}
