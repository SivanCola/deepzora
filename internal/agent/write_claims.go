package agent

import "context"

const DefaultMaxSubagentConcurrency = 6
const DefaultMaxParallelWriters = 1
const MaxSubagentConcurrencyLimit = 8

// WritePathSet is internal reservation metadata. Every writable run is exclusive;
// no path declarations or path overlap policy are accepted.
type WritePathSet struct {
	WholeWorkspace bool
	WorkspaceRoot  string
}
type parentWriteClaimIDKey struct{}
type subagentClaimIDKey struct{}

func NormalizeConcurrencyLimits(total, _ int) (int, int) {
	if total <= 0 {
		total = DefaultMaxSubagentConcurrency
	}
	if total > MaxSubagentConcurrencyLimit {
		total = MaxSubagentConcurrencyLimit
	}
	return total, 1
}

func WithParentWriteClaimID(ctx context.Context, id int64) context.Context {
	if id == 0 {
		return ctx
	}
	return context.WithValue(ctx, parentWriteClaimIDKey{}, id)
}

// ParentWriteClaimID returns the parent write claim the calling tool holds.
func ParentWriteClaimID(ctx context.Context) int64 {
	id, _ := ctx.Value(parentWriteClaimIDKey{}).(int64)
	return id
}

func WithSubagentClaimID(ctx context.Context, id int64) context.Context {
	if id == 0 {
		return ctx
	}
	return context.WithValue(ctx, subagentClaimIDKey{}, id)
}

// SubagentClaimID returns the live claim id of the running child, if any.
func SubagentClaimID(ctx context.Context) int64 {
	id, _ := ctx.Value(subagentClaimIDKey{}).(int64)
	return id
}
