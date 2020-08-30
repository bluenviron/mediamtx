package main

import (
	"fmt"
	"os"
	"os/exec"
	"time"

	"github.com/aler9/sdp/v3"
)

const (
	describeTimeout                  = 5 * time.Second
	sourceStopAfterDescribeSecs      = 10 * time.Second
	onDemandCmdStopAfterDescribeSecs = 10 * time.Second
)

// a publisher is either a client or a source
type publisher interface {
	isPublisher()
}

type path struct {
	p                      *program
	name                   string
	confp                  *confPath
	permanent              bool
	source                 *source
	publisher              publisher
	publisherReady         bool
	publisherSdpText       []byte
	publisherSdpParsed     *sdp.SessionDescription
	lastDescribeReq        time.Time
	lastDescribeActivation time.Time
	onInitCmd              *exec.Cmd
	onDemandCmd            *exec.Cmd
}

func newPath(p *program, name string, confp *confPath, permanent bool) *path {
	pa := &path{
		p:         p,
		name:      name,
		confp:     confp,
		permanent: permanent,
	}

	if confp.Source != "record" {
		s := newSource(p, name, confp)
		pa.source = s
		pa.publisher = s
	}

	return pa
}

func (pa *path) onInit() {
	if pa.source != nil {
		go pa.source.run()
	}

	if pa.confp.RunOnInit != "" {
		pa.p.log("starting on init command")

		pa.onInitCmd = exec.Command("/bin/sh", "-c", pa.confp.RunOnInit)
		pa.onInitCmd.Env = append(os.Environ(),
			"RTSP_SERVER_PATH="+pa.name,
		)
		pa.onInitCmd.Stdout = os.Stdout
		pa.onInitCmd.Stderr = os.Stderr
		err := pa.onInitCmd.Start()
		if err != nil {
			pa.p.log("ERR: %s", err)
		}
	}
}

func (pa *path) onClose() {
	if pa.source != nil {
		pa.source.events <- sourceEventTerminate{}
		<-pa.source.done
	}

	if pa.onInitCmd != nil {
		pa.p.log("stopping on init command (exited)")
		pa.onInitCmd.Process.Signal(os.Interrupt)
		pa.onInitCmd.Wait()
	}

	if pa.onDemandCmd != nil {
		pa.p.log("stopping on demand command (exited)")
		pa.onDemandCmd.Process.Signal(os.Interrupt)
		pa.onDemandCmd.Wait()
	}
}

func (pa *path) hasClients() bool {
	for c := range pa.p.clients {
		if c.pathName == pa.name {
			return true
		}
	}
	return false
}

func (pa *path) hasClientsWaitingDescribe() bool {
	for c := range pa.p.clients {
		if c.state == clientStateWaitingDescription && c.pathName == pa.name {
			return true
		}
	}
	return false
}

func (pa *path) hasClientReaders() bool {
	for c := range pa.p.clients {
		if c.pathName == pa.name && c != pa.publisher {
			return true
		}
	}
	return false
}

func (pa *path) onCheck() {
	// reply to DESCRIBE requests if they are in timeout
	if pa.hasClientsWaitingDescribe() &&
		time.Since(pa.lastDescribeActivation) >= describeTimeout {
		for c := range pa.p.clients {
			if c.state == clientStateWaitingDescription &&
				c.pathName == pa.name {
				c.pathName = ""
				c.state = clientStateInitial
				c.describeRes <- describeRes{nil, fmt.Errorf("publisher of path '%s' has timed out", pa.name)}
			}
		}
	}

	// stop on demand source if needed
	if pa.source != nil &&
		pa.confp.SourceOnDemand &&
		pa.source.state == sourceStateRunning &&
		!pa.hasClients() &&
		time.Since(pa.lastDescribeReq) >= sourceStopAfterDescribeSecs {
		pa.source.log("stopping since we're not requested anymore")
		pa.source.state = sourceStateStopped
		pa.source.events <- sourceEventApplyState{pa.source.state}
	}

	// stop on demand command if needed
	if pa.onDemandCmd != nil &&
		!pa.hasClientReaders() &&
		time.Since(pa.lastDescribeReq) >= onDemandCmdStopAfterDescribeSecs {
		pa.p.log("stopping on demand command (not requested anymore)")
		pa.onDemandCmd.Process.Signal(os.Interrupt)
		pa.onDemandCmd.Wait()
		pa.onDemandCmd = nil
	}

	// remove non-permanent paths
	if !pa.permanent &&
		pa.publisher == nil &&
		!pa.hasClients() {
		pa.onClose()
		delete(pa.p.paths, pa.name)
	}
}

func (pa *path) onPublisherNew(client *client, sdpText []byte, sdpParsed *sdp.SessionDescription) {
	pa.publisher = client
	pa.publisherSdpText = sdpText
	pa.publisherSdpParsed = sdpParsed

	client.pathName = pa.name
	client.state = clientStateAnnounce
}

func (pa *path) onPublisherRemove() {
	pa.publisher = nil
}

func (pa *path) onPublisherSetReady() {
	pa.publisherReady = true

	// reply to all clients that are waiting for a description
	for c := range pa.p.clients {
		if c.state == clientStateWaitingDescription &&
			c.pathName == pa.name {
			c.pathName = ""
			c.state = clientStateInitial
			c.describeRes <- describeRes{pa.publisherSdpText, nil}
		}
	}
}

func (pa *path) onPublisherSetNotReady() {
	pa.publisherReady = false

	// close all clients that are reading
	for c := range pa.p.clients {
		if c.state != clientStateWaitingDescription &&
			c != pa.publisher &&
			c.pathName == pa.name {
			c.conn.NetConn().Close()
		}
	}
}

func (pa *path) onDescribe(client *client) {
	pa.lastDescribeReq = time.Now()

	// publisher not found
	if pa.publisher == nil {
		// on demand command is available: put the client on hold
		if pa.confp.RunOnDemand != "" {
			if pa.onDemandCmd == nil { // start if needed
				pa.p.log("starting on demand command")

				pa.lastDescribeActivation = time.Now()
				pa.onDemandCmd = exec.Command("/bin/sh", "-c", pa.confp.RunOnDemand)
				pa.onDemandCmd.Env = append(os.Environ(),
					"RTSP_SERVER_PATH="+pa.name,
				)
				pa.onDemandCmd.Stdout = os.Stdout
				pa.onDemandCmd.Stderr = os.Stderr
				err := pa.onDemandCmd.Start()
				if err != nil {
					pa.p.log("ERR: %s", err)
				}
			}

			client.pathName = pa.name
			client.state = clientStateWaitingDescription

			// no on-demand: reply with 404
		} else {
			client.describeRes <- describeRes{nil, fmt.Errorf("no one is publishing on path '%s'", pa.name)}
		}

		// publisher was found but is not ready: put the client on hold
	} else if !pa.publisherReady {
		if pa.source != nil && pa.source.state == sourceStateStopped { // start if needed
			pa.source.log("starting on demand")
			pa.lastDescribeActivation = time.Now()
			pa.source.state = sourceStateRunning
			pa.source.events <- sourceEventApplyState{pa.source.state}
		}

		client.pathName = pa.name
		client.state = clientStateWaitingDescription

		// publisher was found and is ready
	} else {
		client.describeRes <- describeRes{pa.publisherSdpText, nil}
	}
}
