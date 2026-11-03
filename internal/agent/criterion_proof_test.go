package agent

import (
	"deepzora/internal/completion"
	"deepzora/internal/evidence"
	"deepzora/internal/plancontract"
	"testing"
)

func TestCriterionClaimDoesNotCertifyFailedExecution(t *testing.T) {
	ledger := evidence.NewLedger()
	ledger.Record(evidence.Receipt{ToolName: "bash", Command: "go test ./...", Success: false})
	ledger.Record(evidence.Receipt{ToolName: "todo_write", Success: true, Todos: []evidence.TodoItem{{Content: "tests passed", Status: "completed"}}})
	report := completion.BuildFacts(ledger, "", nil)
	if report.Verdict != completion.VerdictUnknown || len(report.Verifications) != 1 || report.Verifications[0].Passed {
		t.Fatalf("claim altered facts: %+v", report)
	}
}

func criterionPlan() plancontract.Plan {
	return plancontract.Plan{Objective: "fix the retry race", Steps: []plancontract.Step{{Title: "fix it"}}}.Normalize()
}
