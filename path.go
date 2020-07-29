package main

import (
	"time"

	"github.com/aler9/sdp/v3"
)

// a publisher is either a client or a source
type publisher interface {
	isPublisher()
}

type path struct {
	p                  *program
	id                 string
	publisher          publisher
	publisherReady     bool
	publisherSdpText   []byte
	publisherSdpParsed *sdp.SessionDescription
	lastRequested      time.Time
}

func newPath(p *program, id string, publisher publisher) *path {
	return &path{
		p:         p,
		id:        id,
		publisher: publisher,
	}
}

func (p *path) check() {
	hasClients := func() bool {
		for c := range p.p.clients {
			if c.path == p.id {
				return true
			}
		}
		return false
	}()
	source, publisherIsSource := p.publisher.(*source)

	// stop source if needed
	if !hasClients &&
		publisherIsSource &&
		source.state == sourceStateRunning &&
		time.Since(p.lastRequested) >= 10*time.Second {
		source.log("stopping due to inactivity")
		source.state = sourceStateStopped
		source.events <- sourceEventApplyState{source.state}
	}
}

func (p *path) describe() ([]byte, bool) {
	p.lastRequested = time.Now()

	// publisher was found but is not ready: wait
	if !p.publisherReady {
		// start source if needed
		if source, ok := p.publisher.(*source); ok && source.state == sourceStateStopped {
			source.log("starting on demand")
			source.state = sourceStateRunning
			source.events <- sourceEventApplyState{source.state}
		}

		return nil, true
	}

	// publisher was found and is ready
	return p.publisherSdpText, false
}

func (p *path) publisherSetReady() {
	p.publisherReady = true

	// reply to all clients that are waiting for a description
	for c := range p.p.clients {
		if c.state == clientStateWaitingDescription &&
			c.path == p.id {
			c.path = ""
			c.state = clientStateInitial
			c.describeRes <- p.publisherSdpText
		}
	}
}

func (p *path) publisherSetNotReady() {
	p.publisherReady = false

	// close all clients that are reading
	for c := range p.p.clients {
		if c.state != clientStateWaitingDescription &&
			c != p.publisher &&
			c.path == p.id {
			c.conn.NetConn().Close()
		}
	}
}

func (p *path) publisherReset() {
	// reply to all clients that were waiting for a description
	for oc := range p.p.clients {
		if oc.state == clientStateWaitingDescription &&
			oc.path == p.id {
			oc.path = ""
			oc.state = clientStateInitial
			oc.describeRes <- nil
		}
	}
}
