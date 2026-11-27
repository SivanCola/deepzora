package provider

import "time"

// DefaultStreamIdleTimeout caps how long a started SSE stream may go silent
// before the client treats it as a dropped connection. A half-open TCP
// connection (e.g. a proxy switched mid-stream) sends no RST, so scanning would
// block forever; this turns that hang into a recoverable error. Generous on
// purpose — live streams emit tokens/keepalives far more often.
const DefaultStreamIdleTimeout = 300 * time.Second

// DefaultStreamFirstOutputTimeout caps the wait for the first model output on a
// connection that is still delivering bytes. Long on purpose: a loaded provider
// holds a queued request open with keep-alives, and a hidden-reasoning model
// sends only scaffolding until it finishes thinking; replaying either would
// just restart the wait.
const DefaultStreamFirstOutputTimeout = 10 * time.Minute

// DefaultStreamProgressTimeout caps the gap between model outputs once
// generation has started. Keep-alives without output past this point mean the
// generation died behind a live connection, which the idle timeout never sees.
const DefaultStreamProgressTimeout = 120 * time.Second
