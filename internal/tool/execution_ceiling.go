package tool

import "context"

type executionCeilingKey struct{}

// WithExecutionCeiling carries an attenuating host restriction through child
// and proxy calls. Model arguments cannot manufacture a context capability.
func WithExecutionCeiling(ctx context.Context, mode string) context.Context {
	return context.WithValue(ctx, executionCeilingKey{}, mode)
}

func ExecutionCeiling(ctx context.Context) string {
	mode, _ := ctx.Value(executionCeilingKey{}).(string)
	return mode
}

// ExecutionPresetAllowed is checked before any per-call escalation or child
// shell launch, including runtimes without an interactive write-access gate.
func ExecutionPresetAllowed(ctx context.Context, requested string) bool {
	cap := ExecutionCeiling(ctx)
	if cap == "" || requested == "" {
		return true
	}
	ranks := map[string]int{"read-only": 0, "workspace-write": 1, "danger-full-access": 2}
	limit, validCap := ranks[cap]
	want, validWant := ranks[requested]
	return validCap && validWant && want <= limit
}
