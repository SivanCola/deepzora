package cli

import (
	"errors"
	"fmt"
	"testing"

	"deepzora/internal/provider"
)

func TestSetupFailureCodeTreatsRejectedEffortFlagAsUsageError(t *testing.T) {
	rejected := fmt.Errorf("execution model: %w", &provider.UnsupportedReasoningEffort{Model: "m", Effort: "bogus", Supported: []string{"low"}})
	if got := setupFailureCode(rejected, true); got != 2 {
		t.Fatalf("rejected --effort exit code = %d, want 2", got)
	}
	// The same rejection from a configured default effort is a config problem,
	// not a bad flag on this command line.
	if got := setupFailureCode(rejected, false); got != 1 {
		t.Fatalf("configured effort exit code = %d, want 1", got)
	}
	if got := setupFailureCode(errors.New("network down"), true); got != 1 {
		t.Fatalf("other setup failure exit code = %d, want 1", got)
	}
}

func TestValidateMaxStepsRejectsNegative(t *testing.T) {
	for _, n := range []int{0, 1, 40} {
		if err := validateMaxSteps(n); err != nil {
			t.Fatalf("validateMaxSteps(%d) = %v", n, err)
		}
	}
	if err := validateMaxSteps(-1); err == nil {
		t.Fatal("validateMaxSteps(-1) accepted a negative value")
	}
}
