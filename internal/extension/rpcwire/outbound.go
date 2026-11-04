package rpcwire

import (
	"bytes"
	"context"
	"encoding/json"
)

func (c *Conn) prepareWrite(m outbound, ctx context.Context) (writeJob, error) {
	var buf bytes.Buffer
	enc := json.NewEncoder(&buf)
	enc.SetEscapeHTML(false)
	if err := enc.Encode(m); err != nil {
		return writeJob{}, err
	}
	if c.opts.ProjectOutbound != nil {
		projected := c.opts.ProjectOutbound(buf.Bytes())
		buf.Reset()
		buf.Write(projected)
	}
	if limit := c.opts.MaxOutboundBytes; limit > 0 && buf.Len() > limit {
		return writeJob{}, &FrameTooLargeError{Direction: "outbound", Size: buf.Len(), Limit: limit}
	}
	if ctx == nil {
		ctx = context.Background()
	}
	return writeJob{frame: buf.Bytes(), ctx: ctx, res: make(chan error, 1)}, nil
}
