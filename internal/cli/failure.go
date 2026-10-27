package cli

import (
	"errors"
	"fmt"
	"os"

	"deepzora/internal/i18n"
	"deepzora/internal/provider"
)

// cliFailure reports err on stderr and yields the process exit code, so a
// command can end a failed step in one statement.
func cliFailure(err error) int {
	fmt.Fprintln(os.Stderr, i18n.M.ErrorPrefix, err)
	return 1
}

// setupFailureCode is the exit code for a failed controller assembly. Levels
// are model-specific, so a bad --effort is only detectable here; it is still a
// rejected flag and exits 2 like the other argument errors.
func setupFailureCode(err error, effortFlag bool) int {
	var unsupported *provider.UnsupportedReasoningEffort
	if effortFlag && errors.As(err, &unsupported) {
		return 2
	}
	return 1
}

// validateMaxSteps rejects negative --max-steps values, which assembly would
// otherwise clamp to 0 (automatic) without telling the caller.
func validateMaxSteps(n int) error {
	if n < 0 {
		return fmt.Errorf("--max-steps must be 0 (automatic) or a positive number, got %d", n)
	}
	return nil
}
