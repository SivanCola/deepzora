package agent

import "context"

// CancellableContext connects the existing child runtime's Cancel capability
// to this invocation alone. Parent and sibling contexts remain untouched.
func (r *SubagentRun) CancellableContext(parent context.Context) (context.Context, func()) {
	ctx, cancel := context.WithCancel(parent)
	if r != nil {
		if target, ok := r.Persistence.(interface{ SetCancellation(context.CancelFunc) }); ok {
			target.SetCancellation(cancel)
			return ctx, func() { target.SetCancellation(nil); cancel() }
		}
	}
	return ctx, cancel
}
