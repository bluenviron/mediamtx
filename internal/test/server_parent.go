package test

import (
	"context"

	"github.com/bluenviron/mediamtx/internal/logger"
)

// ServerParent is a dummy server parent.
type ServerParent struct {
	OnFatalErrorImpl func(context.Context, error)
}

// Log implements logger.Writer.
func (*ServerParent) Log(logger.Level, string, ...any) {
}

// OnFatalError implements serverParent.
func (p *ServerParent) OnFatalError(ctx context.Context, err error) {
	if p.OnFatalErrorImpl != nil {
		p.OnFatalErrorImpl(ctx, err)
	}
}
