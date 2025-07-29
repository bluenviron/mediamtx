package test

import (
	"github.com/bluenviron/mediamtx/internal/defs"
	"github.com/bluenviron/mediamtx/internal/logger"
	"github.com/bluenviron/mediamtx/internal/stream"
	"github.com/bluenviron/mediamtx/internal/unit"
)

// StaticSourceParent is a dummy static source parent.
type StaticSourceParent struct {
	stream *stream.Stream
	reader stream.Reader
	Unit   chan unit.Unit
}

// Log implements logger.Writer.
func (*StaticSourceParent) Log(logger.Level, string, ...interface{}) {}

// Initialize initializes StaticSourceParent.
func (p *StaticSourceParent) Initialize() {
	p.Unit = make(chan unit.Unit)
}

// Close closes StaticSourceParent.
func (p *StaticSourceParent) Close() {
	p.stream.RemoveReader(p.reader)
}

// SetReady implements parent.
func (p *StaticSourceParent) SetReady(req defs.PathSourceStaticSetReadyReq) defs.PathSourceStaticSetReadyRes {
	p.stream = &stream.Stream{
		WriteQueueSize:     512,
		RTPMaxPayloadSize:  1450,
		Desc:               req.Desc,
		GenerateRTPPackets: req.GenerateRTPPackets,
		Parent:             p,
	}
	err := p.stream.Initialize()
	if err != nil {
		panic(err)
	}

	p.reader = NilLogger

	p.stream.AddReader(p.reader, req.Desc.Medias[0], req.Desc.Medias[0].Formats[0], func(u unit.Unit) error {
		p.Unit <- u
		close(p.Unit)
		return nil
	})

	p.stream.StartReader(p.reader)

	return defs.PathSourceStaticSetReadyRes{Stream: p.stream}
}

// SetNotReady implements parent.
func (StaticSourceParent) SetNotReady(_ defs.PathSourceStaticSetNotReadyReq) {}
