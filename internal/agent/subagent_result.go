package agent

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"slices"
	"strings"
	"unicode/utf8"

	"deepzora/internal/provider"
	"deepzora/internal/tool"
)

const (
	// Leave room for the generic tool-result guard to add metadata without
	// clipping a task manifest. The aggregate itself owns fair per-task preview
	// allocation so every completed child keeps its status and retrieval ref.
	subagentAggregateBudgetBytes = maxToolOutputBytes - 512
	subagentResultDefaultBytes   = 12 * 1024
	subagentResultMaxBytes       = 24 * 1024
)

// SubagentResultTool pages through the retained answer of a persisted
// sub-agent, including partial and retryable failures. Parallel/fleet
// aggregates use this stable reader instead of forcing every child answer
// through one fixed-size tool result.
type SubagentResultTool struct {
	store         *SubagentStore
	workspaceRoot string
}

func NewSubagentResultTool(task *SubagentRunner) *SubagentResultTool {
	if task == nil {
		return &SubagentResultTool{}
	}
	return &SubagentResultTool{store: task.transcripts, workspaceRoot: task.workspaceRoot}
}

func (*SubagentResultTool) Name() string { return tool.HostReadSubagentResult }

func (*SubagentResultTool) Description() string {
	return "Read a completed or partial sub-agent's retained answer by the Subagent reference returned from spawn_agents. Failed runs may expose their last useful output and an explicit retryability status. Results are scoped to the current conversation lineage and paged by UTF-8 byte offset so large answers remain lossless without overflowing one tool result."
}

func (*SubagentResultTool) Schema() json.RawMessage {
	return json.RawMessage(`{"type":"object","properties":{"ref":{"type":"string","description":"The sa_... value from a Subagent reference line."},"offset_bytes":{"type":"integer","description":"UTF-8 byte offset to start reading from. Omit for the beginning; use next_offset_bytes from the previous page.","minimum":0},"limit_bytes":{"type":"integer","description":"Maximum UTF-8 bytes to return. Defaults to 12288 and is capped at 24576.","minimum":1,"maximum":24576}},"required":["ref"]}`)
}

func (*SubagentResultTool) ReadOnly() bool { return true }

func (*SubagentResultTool) PlanModeSafe() bool { return true }

func (t *SubagentResultTool) Execute(ctx context.Context, args json.RawMessage) (string, error) {
	var p struct {
		Ref         string `json:"ref"`
		OffsetBytes int    `json:"offset_bytes"`
		LimitBytes  int    `json:"limit_bytes"`
	}
	dec := json.NewDecoder(bytes.NewReader(args))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&p); err != nil {
		return "", fmt.Errorf("invalid args: %w", err)
	}
	p.Ref = strings.TrimSpace(p.Ref)
	if p.Ref == "" {
		return "", fmt.Errorf("ref is required")
	}
	if p.OffsetBytes < 0 {
		return "", fmt.Errorf("offset_bytes must be non-negative")
	}
	if p.LimitBytes == 0 {
		p.LimitBytes = subagentResultDefaultBytes
	}
	if p.LimitBytes < 1 || p.LimitBytes > subagentResultMaxBytes {
		return "", fmt.Errorf("limit_bytes must be between 1 and %d", subagentResultMaxBytes)
	}
	if t == nil || t.store == nil {
		return "", fmt.Errorf("subagent result storage is not available in this session")
	}
	parentSession := ParentSession(ctx)
	if parentSession == "" {
		return "", fmt.Errorf("subagent result retrieval requires a persisted parent session")
	}

	answer, status, err := t.store.ReadFinalAnswer(p.Ref, parentSession, t.workspaceRoot)
	if err != nil {
		return "", err
	}
	if p.OffsetBytes > len(answer) {
		return "", fmt.Errorf("offset_bytes %d exceeds result size %d", p.OffsetBytes, len(answer))
	}
	if p.OffsetBytes < len(answer) && !utf8.RuneStart(answer[p.OffsetBytes]) {
		return "", fmt.Errorf("offset_bytes %d is not at a UTF-8 character boundary; use next_offset_bytes from the previous page", p.OffsetBytes)
	}
	end := min(p.OffsetBytes+p.LimitBytes, len(answer))
	for end > p.OffsetBytes && end < len(answer) && !utf8.RuneStart(answer[end]) {
		end--
	}

	var b strings.Builder
	fmt.Fprintf(&b, "Subagent result %s (status=%s, bytes %d-%d of %d):\n", p.Ref, status, p.OffsetBytes, end, len(answer))
	b.WriteString(answer[p.OffsetBytes:end])
	if end < len(answer) {
		fmt.Fprintf(&b, "\n\nMore remains. Call read_subagent_result with ref=%q and offset_bytes=%d.", p.Ref, end)
	} else {
		b.WriteString("\n\nEnd of subagent result.")
	}
	return b.String(), nil
}

// ReadFinalAnswer returns a completed child answer only when the caller owns
// the parent conversation (or a verified descendant) and the workspace still
// matches. The per-ref lock prevents a read racing the terminal transcript save.
func (s *SubagentStore) ReadFinalAnswer(ref, parentSession, workspaceRoot string) (string, SubagentStatus, error) {
	if s == nil {
		return "", "", fmt.Errorf("subagent result storage is not available")
	}
	ref = strings.TrimSpace(ref)
	parentSession = strings.TrimSpace(parentSession)
	if parentSession == "" {
		return "", "", fmt.Errorf("subagent result parent session is required")
	}
	release, err := s.lock(ref)
	if err != nil {
		return "", "", err
	}
	defer release()

	meta, err := s.LoadMeta(ref)
	if err != nil {
		return "", "", err
	}
	owner := strings.TrimSpace(meta.ParentSession)
	if owner != parentSession {
		ok, lineageErr := s.isAncestorSession(owner, parentSession)
		if lineageErr != nil {
			return "", meta.Status, fmt.Errorf("subagent reference %q ownership could not be verified: %w", ref, lineageErr)
		}
		if !ok {
			return "", meta.Status, fmt.Errorf("subagent reference %q does not belong to the current conversation lineage", ref)
		}
	}
	if want := strings.TrimSpace(workspaceRoot); want != "" && strings.TrimSpace(meta.WorkspaceRoot) != want {
		return "", meta.Status, fmt.Errorf("subagent reference %q belongs to a different workspace", ref)
	}
	if meta.Status == SubagentRunning {
		return "", meta.Status, fmt.Errorf("subagent reference %q is still in progress", ref)
	}
	if meta.Status == SubagentInterrupted && meta.Outcome != string(SubagentOutcomeCancelled) {
		return "", meta.Status, fmt.Errorf("subagent reference %q was interrupted; only completed or retained partial results can be read", ref)
	}

	msgs, err := s.persistence.Read(context.Background(), ref)
	if err != nil {
		return "", meta.Status, fmt.Errorf("load subagent transcript %q: %w", ref, err)
	}
	for _, v := range slices.Backward(msgs) {
		if v.Role == provider.RoleAssistant && strings.TrimSpace(v.Content) != "" {
			status := meta.Status
			if meta.Outcome != "" {
				status = SubagentStatus(meta.Outcome)
			}
			return v.Content, status, nil
		}
	}
	return "", meta.Status, fmt.Errorf("subagent reference %q has no final assistant answer", ref)
}

func utf8Prefix(s string, limit int) string {
	if limit >= len(s) {
		return s
	}
	if limit <= 0 {
		return ""
	}
	for limit > 0 && !utf8.RuneStart(s[limit]) {
		limit--
	}
	return s[:limit]
}

func boundedInline(s string, limit int) string {
	s = strings.Join(strings.Fields(s), " ")
	if len(s) <= limit {
		return s
	}
	if limit <= len("…") {
		return utf8Prefix(s, limit)
	}
	return utf8Prefix(s, limit-len("…")) + "…"
}

func splitSubagentRunResult(output string) (answer, ref string) {
	ref = extractSubagentRef(output)
	if ref == "" {
		return strings.TrimSpace(output), ""
	}
	const marker = "\n\nFinal answer:\n"
	if _, after, ok := strings.Cut(output, marker); ok {
		return strings.TrimSpace(after), ref
	}
	return strings.TrimSpace(output), ref
}

func extractSubagentRef(output string) string {
	const prefix = "Subagent reference: "
	if !strings.HasPrefix(output, prefix) {
		return ""
	}
	line := output
	if end := strings.IndexByte(line, '\n'); end >= 0 {
		line = line[:end]
	}
	ref := strings.TrimSpace(strings.TrimPrefix(line, prefix))
	if validSubagentRef(ref) {
		return ref
	}
	return ""
}
