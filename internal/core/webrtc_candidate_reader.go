package core

import (
	"context"

	"github.com/pion/webrtc/v3"

	"github.com/aler9/mediamtx/internal/websocket"
)

type webRTCCandidateReader struct {
	ws *websocket.ServerConn

	ctx       context.Context
	ctxCancel func()

	stopGathering   chan struct{}
	readError       chan error
	remoteCandidate chan *webrtc.ICECandidateInit
}

func newWebRTCCandidateReader(ws *websocket.ServerConn) *webRTCCandidateReader {
	ctx, ctxCancel := context.WithCancel(context.Background())

	r := &webRTCCandidateReader{
		ws:              ws,
		ctx:             ctx,
		ctxCancel:       ctxCancel,
		stopGathering:   make(chan struct{}),
		readError:       make(chan error),
		remoteCandidate: make(chan *webrtc.ICECandidateInit),
	}

	go r.run()

	return r
}

func (r *webRTCCandidateReader) close() {
	r.ctxCancel()
	// do not wait for ReadJSON() to return
	// it is terminated by ws.Close() later
}

func (r *webRTCCandidateReader) run() {
	for {
		candidate, err := r.readCandidate()
		if err != nil {
			select {
			case r.readError <- err:
			case <-r.ctx.Done():
			}
			return
		}

		select {
		case r.remoteCandidate <- candidate:
		case <-r.stopGathering:
		case <-r.ctx.Done():
			return
		}
	}
}

func (r *webRTCCandidateReader) readCandidate() (*webrtc.ICECandidateInit, error) {
	var candidate webrtc.ICECandidateInit
	err := r.ws.ReadJSON(&candidate)
	if err != nil {
		return nil, err
	}

	return &candidate, err
}
