package agent

import (
	"context"
	"fmt"
	"strings"

	"deepzora/internal/event"
	"deepzora/internal/evidence"
	"deepzora/internal/tool"
)

// hostReceiptsMaxItems bounds each rendered line so a long child run cannot
// crowd the parent's context out with paths and commands.
const hostReceiptsMaxItems = 8

const hostReceiptsHeader = "Host receipts (recorded by the host as the sub-agent ran, not claimed by it):"

// splitHostReceipts separates a child's own prose from the host attestation
// appended to it. Aggregates truncate prose to fit a budget; the attestation is
// bounded already and must never be the part that gets cut.
func splitHostReceipts(answer string) (prose, receipts string) {
	idx := strings.LastIndex(answer, hostReceiptsHeader)
	if idx < 0 {
		return answer, ""
	}
	return strings.TrimRight(answer[:idx], "\n"), strings.TrimSpace(answer[idx:])
}

// decorateExecutionReceipt records what the host itself observed about one tool
// call. The output length and the process outcome never come from model
// arguments, which is what makes the resulting attestation trustworthy.
func decorateExecutionReceipt(rec *evidence.Receipt, result string, ex *tool.ShellExecution) {
	if rec == nil {
		return
	}
	rec.ObserveOutput(result)
	if ex == nil {
		return
	}
	if ex.ExitCode != nil {
		code := *ex.ExitCode
		rec.ExitCode = &code
	}
	rec.Verification = ex.Verification
	rec.Interrupted = ex.State == tool.ShellStateCancelled
}

// composeSubagentAnswer assembles everything the parent is shown for one child
// run: the child's own prose followed by bounded host observations.
func composeSubagentAnswer(ctx context.Context, answer string, sub *Agent, delegationText string) string {
	summary := sub.EvidenceSummary()
	recordDelegationAudit(ctx, summary, delegationText)
	return appendHostReceipts(answer, summary)
}

// recordDelegationAudit emits one structured receipt per child run. It reports
// what the host observed, so an orchestration
// benchmark can separate real gains from extra tokens spent. delegationText is
// the parent-authored task before host framing, which is what makes the
// evidence-origin split a host record rather than a claim.
func recordDelegationAudit(ctx context.Context, summary evidence.ChildEvidenceSummary, delegationText string) {
	audit := evidence.DelegationAudit{
		Depth:         SubagentDepth(ctx),
		ToolCalls:     len(summary.Receipts),
		MutationPaths: summary.MutationPaths(),
	}
	audit.Mutations = len(audit.MutationPaths)
	audit.ClassifyEvidenceOrigin(delegationText, summary.EvidencePaths())
	_, sink, _, _ := CallContext(ctx)
	event.RecordDelegationAudit(sink, audit)
}

func appendHostReceipts(answer string, summary evidence.ChildEvidenceSummary) string {
	block := formatHostReceipts(summary)
	if block == "" {
		return answer
	}
	if strings.TrimSpace(answer) == "" {
		return block
	}
	return answer + "\n\n" + block
}

// formatHostReceipts renders only what the host is willing to attest to: files
// the child really changed, and commands whose outcome the host observed.
// Ordinary reads and greps are excluded on purpose — they are not claims a
// parent has to adjudicate, and every rendered line costs parent context.
func formatHostReceipts(summary evidence.ChildEvidenceSummary) string {
	changed := receiptDisplayPaths(summary.MutationPaths())
	commands := hostReceiptCommands(summary)
	if len(changed) == 0 && len(commands) == 0 {
		return ""
	}
	var b strings.Builder
	b.WriteString(hostReceiptsHeader)
	if len(changed) > 0 {
		b.WriteString("\n  changed: ")
		b.WriteString(joinBoundedReceipts(changed))
	}
	if len(commands) > 0 {
		b.WriteString("\n  commands: ")
		b.WriteString(joinBoundedReceipts(commands))
	}
	return b.String()
}

func receiptDisplayPaths(paths []string) []string {
	out := make([]string, len(paths))
	for i, value := range paths {
		out[i] = strings.ReplaceAll(value, `\`, "/")
	}
	return out
}

// hostReceiptCommands keeps only shell receipts carrying an outcome worth
// attesting: a host-classified verification, or a command that did not succeed.
func hostReceiptCommands(summary evidence.ChildEvidenceSummary) []string {
	var out []string
	seen := map[string]bool{}
	for _, r := range summary.Receipts {
		cmd := strings.TrimSpace(r.Command)
		outcome := hostReceiptOutcome(r)
		if cmd == "" || outcome == "" || seen[cmd] {
			continue
		}
		seen[cmd] = true
		out = append(out, cmd+outcome)
	}
	return out
}

func hostReceiptOutcome(r evidence.Receipt) string {
	var parts []string
	switch r.Verification {
	case evidence.VerificationPassed:
		parts = append(parts, "verification passed")
	case evidence.VerificationFailed:
		parts = append(parts, "verification failed")
	}
	switch {
	case r.ExitCode != nil && (*r.ExitCode != 0 || len(parts) > 0):
		parts = append(parts, fmt.Sprintf("exit %d", *r.ExitCode))
	case r.ExitCode == nil && !r.Success:
		parts = append(parts, "did not complete")
	}
	if len(parts) == 0 {
		return ""
	}
	return " (" + strings.Join(parts, ", ") + ")"
}

func joinBoundedReceipts(items []string) string {
	if len(items) <= hostReceiptsMaxItems {
		return strings.Join(items, ", ")
	}
	return strings.Join(items[:hostReceiptsMaxItems], ", ") +
		fmt.Sprintf(" (+%d more)", len(items)-hostReceiptsMaxItems)
}
