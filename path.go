package main

import (
	"fmt"
	"os"
	"os/exec"
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
	confp              *confPath
	permanent          bool
	publisher          publisher
	publisherReady     bool
	publisherSdpText   []byte
	publisherSdpParsed *sdp.SessionDescription
	lastRequested      time.Time
	lastActivation     time.Time
	onDemandCmd        *exec.Cmd
}

func newPath(p *program, id string, confp *confPath, permanent bool) {
	pa := &path{
		p:         p,
		id:        id,
		confp:     confp,
		permanent: permanent,
	}

	p.paths[id] = pa
}

func (pa *path) check() {
	hasClientsWaitingDescribe := func() bool {
		for c := range pa.p.clients {
			if c.state == clientStateWaitingDescription && c.pathId == pa.id {
				return true
			}
		}
		return false
	}()

	// reply to DESCRIBE requests if they are in timeout
	if hasClientsWaitingDescribe &&
		time.Since(pa.lastActivation) >= 5*time.Second {
		for c := range pa.p.clients {
			if c.state == clientStateWaitingDescription &&
				c.pathId == pa.id {
				c.pathId = ""
				c.state = clientStateInitial
				c.describeRes <- describeRes{nil, fmt.Errorf("publisher of path '%s' has timed out", pa.id)}
			}
		}

		// perform actions below in next run
		return
	}

	if source, ok := pa.publisher.(*source); ok {
		if source.state == sourceStateRunning {
			hasClients := func() bool {
				for c := range pa.p.clients {
					if c.pathId == pa.id {
						return true
					}
				}
				return false
			}()

			// stop source if needed
			if !hasClients &&
				time.Since(pa.lastRequested) >= 10*time.Second {
				source.log("stopping since we're not requested anymore")
				source.state = sourceStateStopped
				source.events <- sourceEventApplyState{source.state}
			}
		}

	} else {
		if pa.onDemandCmd != nil {
			hasClientReaders := func() bool {
				for c := range pa.p.clients {
					if c.pathId == pa.id && c != pa.publisher {
						return true
					}
				}
				return false
			}()

			// stop on demand command if needed
			if !hasClientReaders &&
				time.Since(pa.lastRequested) >= 10*time.Second {
				pa.p.log("stopping on demand command since it is not requested anymore")

				pa.onDemandCmd.Process.Signal(os.Interrupt)
				pa.onDemandCmd.Wait()
				pa.onDemandCmd = nil
			}
		}
	}
}

func (pa *path) describe(client *client) {
	pa.lastRequested = time.Now()

	// publisher not found
	if pa.publisher == nil {
		if pa.confp.RunOnDemand != "" {
			if pa.onDemandCmd == nil {
				pa.p.log("starting on demand command")

				pa.lastActivation = time.Now()
				pa.onDemandCmd = exec.Command("/bin/sh", "-c", pa.confp.RunOnDemand)
				pa.onDemandCmd.Stdout = os.Stdout
				pa.onDemandCmd.Stderr = os.Stderr
				err := pa.onDemandCmd.Start()
				if err != nil {
					pa.p.log("ERR: %s", err)
				}
			}

			client.pathId = pa.id
			client.state = clientStateWaitingDescription
			return

		} else {
			client.describeRes <- describeRes{nil, fmt.Errorf("no one is publishing on path '%s'", pa.id)}
			return
		}
	}

	// publisher was found but is not ready: put the client on hold
	if !pa.publisherReady {
		// start source if needed
		if source, ok := pa.publisher.(*source); ok && source.state == sourceStateStopped {
			source.log("starting on demand")
			pa.lastActivation = time.Now()
			source.state = sourceStateRunning
			source.events <- sourceEventApplyState{source.state}
		}

		client.pathId = pa.id
		client.state = clientStateWaitingDescription
		return
	}

	// publisher was found and is ready
	client.describeRes <- describeRes{pa.publisherSdpText, nil}
}

func (pa *path) publisherRemove() {
	for c := range pa.p.clients {
		if c.state == clientStateWaitingDescription &&
			c.pathId == pa.id {
			c.pathId = ""
			c.state = clientStateInitial
			c.describeRes <- describeRes{nil, fmt.Errorf("publisher of path '%s' is not available anymore", pa.id)}
		}
	}

	pa.publisher = nil
}

func (pa *path) publisherSetReady() {
	pa.publisherReady = true

	// reply to all clients that are waiting for a description
	for c := range pa.p.clients {
		if c.state == clientStateWaitingDescription &&
			c.pathId == pa.id {
			c.pathId = ""
			c.state = clientStateInitial
			c.describeRes <- describeRes{pa.publisherSdpText, nil}
		}
	}
}

func (pa *path) publisherSetNotReady() {
	pa.publisherReady = false

	// close all clients that are reading
	for c := range pa.p.clients {
		if c.state != clientStateWaitingDescription &&
			c != pa.publisher &&
			c.pathId == pa.id {
			c.conn.NetConn().Close()
		}
	}
}
