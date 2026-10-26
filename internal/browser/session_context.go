package browser

import "context"

type sessionKey struct{}

// WithSession scopes ctx to one session ID, which executors use to keep each
// session's tabs invisible to the others.
func WithSession(ctx context.Context, id string) context.Context {
	if id == "" {
		return ctx
	}
	return context.WithValue(ctx, sessionKey{}, id)
}

// SessionFromContext returns the session ID set by WithSession, or "".
func SessionFromContext(ctx context.Context) string {
	if ctx == nil {
		return ""
	}
	id, _ := ctx.Value(sessionKey{}).(string)
	return id
}
