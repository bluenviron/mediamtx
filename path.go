package main

import (
	"fmt"
	"sync/atomic"
	"time"
)

const (
	describeTimeout                  = 5 * time.Second
	proxyStopAfterDescribeSecs       = 10 * time.Second
	onDemandCmdStopAfterDescribeSecs = 10 * time.Second
)

// a publisher is either a client or a proxy
type publisher interface {
	isPublisher()
}

type path struct {
	p                      *program
	name                   string
	conf                   *pathConf
	proxy                  *proxy
	publisher              publisher
	publisherReady         bool
	publisherTrackCount    int
	publisherSdp           []byte
	lastDescribeReq        time.Time
	lastDescribeActivation time.Time
	onInitCmd              *externalCmd
	onDemandCmd            *externalCmd
}

func newPath(p *program, name string, conf *pathConf) *path {
	pa := &path{
		p:    p,
		name: name,
		conf: conf,
	}

	if conf.Source != "record" {
		s := newProxy(p, pa, conf)
		pa.proxy = s
		pa.publisher = s
	}

	return pa
}

func (pa *path) log(format string, args ...interface{}) {
	pa.p.log("[path "+pa.name+"] "+format, args...)
}

func (pa *path) onInit() {
	if pa.proxy != nil {
		go pa.proxy.run(pa.proxy.state)
	}

	if pa.conf.RunOnInit != "" {
		pa.log("starting on init command")

		var err error
		pa.onInitCmd, err = startExternalCommand(pa.conf.RunOnInit, pa.name)
		if err != nil {
			pa.log("ERR: %s", err)
		}
	}
}

func (pa *path) onClose(wait bool) {
	if pa.proxy != nil {
		close(pa.proxy.terminate)
		<-pa.proxy.done
	}

	if pa.onInitCmd != nil {
		pa.log("stopping on init command (closing)")
		pa.onInitCmd.close()
	}

	if pa.onDemandCmd != nil {
		pa.log("stopping on demand command (closing)")
		pa.onDemandCmd.close()
	}

	for c := range pa.p.clients {
		if c.path == pa {
			if c.state == clientStateWaitDescription {
				c.path = nil
				c.state = clientStateInitial
				c.describe <- describeRes{nil, fmt.Errorf("publisher of path '%s' has timed out", pa.name)}
			} else {
				c.close()

				if wait {
					<-c.done
				}
			}
		}
	}
}

func (pa *path) hasClients() bool {
	for c := range pa.p.clients {
		if c.path == pa {
			return true
		}
	}
	return false
}

func (pa *path) hasClientsWaitingDescribe() bool {
	for c := range pa.p.clients {
		if c.state == clientStateWaitDescription && c.path == pa {
			return true
		}
	}
	return false
}

func (pa *path) hasClientReaders() bool {
	for c := range pa.p.clients {
		if c.path == pa && c != pa.publisher {
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
			if c.state == clientStateWaitDescription &&
				c.path == pa {
				c.path = nil
				c.state = clientStateInitial
				c.describe <- describeRes{nil, fmt.Errorf("publisher of path '%s' has timed out", pa.name)}
			}
		}
	}

	// stop on demand proxy if needed
	if pa.proxy != nil &&
		pa.conf.SourceOnDemand &&
		pa.proxy.state == proxyStateRunning &&
		!pa.hasClients() &&
		time.Since(pa.lastDescribeReq) >= proxyStopAfterDescribeSecs {
		pa.log("stopping on demand proxy (not requested anymore)")
		atomic.AddInt64(&pa.p.countProxiesRunning, -1)
		pa.proxy.state = proxyStateStopped
		pa.proxy.setState <- pa.proxy.state
	}

	// stop on demand command if needed
	if pa.onDemandCmd != nil &&
		!pa.hasClientReaders() &&
		time.Since(pa.lastDescribeReq) >= onDemandCmdStopAfterDescribeSecs {
		pa.log("stopping on demand command (not requested anymore)")
		pa.onDemandCmd.close()
		pa.onDemandCmd = nil
	}

	// remove regular expression paths
	if pa.conf.regexp != nil &&
		pa.publisher == nil &&
		!pa.hasClients() {
		pa.onClose(false)
		delete(pa.p.paths, pa.name)
	}
}

func (pa *path) onPublisherRemove() {
	pa.publisher = nil

	// close all clients that are reading or waiting for reading
	for c := range pa.p.clients {
		if c.path == pa &&
			c.state != clientStateWaitDescription &&
			c != pa.publisher {
			c.close()
		}
	}
}

func (pa *path) onPublisherSetReady() {
	pa.publisherReady = true

	// reply to all clients that are waiting for a description
	for c := range pa.p.clients {
		if c.state == clientStateWaitDescription &&
			c.path == pa {
			c.path = nil
			c.state = clientStateInitial
			c.describe <- describeRes{pa.publisherSdp, nil}
		}
	}
}

func (pa *path) onPublisherSetNotReady() {
	pa.publisherReady = false

	// close all clients that are reading or waiting for reading
	for c := range pa.p.clients {
		if c.path == pa &&
			c.state != clientStateWaitDescription &&
			c != pa.publisher {
			c.close()
		}
	}
}

func (pa *path) onDescribe(client *client) {
	pa.lastDescribeReq = time.Now()

	// publisher not found
	if pa.publisher == nil {
		// on demand command is available: put the client on hold
		if pa.conf.RunOnDemand != "" {
			if pa.onDemandCmd == nil { // start if needed
				pa.log("starting on demand command")
				pa.lastDescribeActivation = time.Now()

				var err error
				pa.onDemandCmd, err = startExternalCommand(pa.conf.RunOnDemand, pa.name)
				if err != nil {
					pa.log("ERR: %s", err)
				}
			}

			client.path = pa
			client.state = clientStateWaitDescription

			// no on-demand: reply with 404
		} else {
			client.describe <- describeRes{nil, fmt.Errorf("no one is publishing on path '%s'", pa.name)}
		}

		// publisher was found but is not ready: put the client on hold
	} else if !pa.publisherReady {
		if pa.proxy != nil && pa.proxy.state == proxyStateStopped { // start if needed
			pa.log("starting on demand proxy")
			pa.lastDescribeActivation = time.Now()
			atomic.AddInt64(&pa.p.countProxiesRunning, +1)
			pa.proxy.state = proxyStateRunning
			pa.proxy.setState <- pa.proxy.state
		}

		client.path = pa
		client.state = clientStateWaitDescription

		// publisher was found and is ready
	} else {
		client.describe <- describeRes{pa.publisherSdp, nil}
	}
}
