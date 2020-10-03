package main

import (
	"fmt"
	"strings"
	"sync/atomic"
	"time"
)

const (
	describeTimeout                    = 5 * time.Second
	sourceStopAfterDescribePeriod      = 10 * time.Second
	onDemandCmdStopAfterDescribePeriod = 10 * time.Second
)

// a publisher can be a client, a sourceRtsp or a sourceRtmp
type publisher interface {
	isPublisher()
}

type path struct {
	p                      *program
	name                   string
	conf                   *pathConf
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

	if strings.HasPrefix(conf.Source, "rtsp://") {
		s := newSourceRtsp(p, pa)
		pa.publisher = s

	} else if strings.HasPrefix(conf.Source, "rtmp://") {
		s := newSourceRtmp(p, pa)
		pa.publisher = s
	}

	return pa
}

func (pa *path) log(format string, args ...interface{}) {
	pa.p.log("[path "+pa.name+"] "+format, args...)
}

func (pa *path) onInit() {
	if source, ok := pa.publisher.(*sourceRtsp); ok {
		go source.run(source.state)

	} else if source, ok := pa.publisher.(*sourceRtmp); ok {
		go source.run(source.state)
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
	if source, ok := pa.publisher.(*sourceRtsp); ok {
		close(source.terminate)
		<-source.done

	} else if source, ok := pa.publisher.(*sourceRtmp); ok {
		close(source.terminate)
		<-source.done
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

	// stop on demand rtsp source if needed
	if source, ok := pa.publisher.(*sourceRtsp); ok {
		if pa.conf.SourceOnDemand &&
			source.state == sourceRtspStateRunning &&
			!pa.hasClients() &&
			time.Since(pa.lastDescribeReq) >= sourceStopAfterDescribePeriod {
			pa.log("stopping on demand rtsp source (not requested anymore)")
			atomic.AddInt64(pa.p.countSourcesRtspRunning, -1)
			source.state = sourceRtspStateStopped
			source.setState <- source.state
		}

		// stop on demand rtmp source if needed
	} else if source, ok := pa.publisher.(*sourceRtmp); ok {
		if pa.conf.SourceOnDemand &&
			source.state == sourceRtmpStateRunning &&
			!pa.hasClients() &&
			time.Since(pa.lastDescribeReq) >= sourceStopAfterDescribePeriod {
			pa.log("stopping on demand rtmp source (not requested anymore)")
			atomic.AddInt64(pa.p.countSourcesRtmpRunning, -1)
			source.state = sourceRtmpStateStopped
			source.setState <- source.state
		}
	}

	// stop on demand command if needed
	if pa.onDemandCmd != nil &&
		!pa.hasClientReaders() &&
		time.Since(pa.lastDescribeReq) >= onDemandCmdStopAfterDescribePeriod {
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
		// start rtsp source if needed
		if source, ok := pa.publisher.(*sourceRtsp); ok {
			if source.state == sourceRtspStateStopped {
				pa.log("starting on demand rtsp source")
				pa.lastDescribeActivation = time.Now()
				atomic.AddInt64(pa.p.countSourcesRtspRunning, +1)
				source.state = sourceRtspStateRunning
				source.setState <- source.state
			}

			// start rtmp source if needed
		} else if source, ok := pa.publisher.(*sourceRtmp); ok {
			if source.state == sourceRtmpStateStopped {
				pa.log("starting on demand rtmp source")
				pa.lastDescribeActivation = time.Now()
				atomic.AddInt64(pa.p.countSourcesRtmpRunning, +1)
				source.state = sourceRtmpStateRunning
				source.setState <- source.state
			}
		}

		client.path = pa
		client.state = clientStateWaitDescription

		// publisher was found and is ready
	} else {
		client.describe <- describeRes{pa.publisherSdp, nil}
	}
}
