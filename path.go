package main

import (
	"fmt"
	"strings"
	"sync/atomic"
	"time"

	"github.com/aler9/rtsp-simple-server/conf"
	"github.com/aler9/rtsp-simple-server/externalcmd"
)

const (
	describeTimeout                    = 5 * time.Second
	sourceStopAfterDescribePeriod      = 10 * time.Second
	onDemandCmdStopAfterDescribePeriod = 10 * time.Second
)

// a source can be a client, a sourceRtsp or a sourceRtmp
type source interface {
	isSource()
}

type path struct {
	p                      *program
	name                   string
	conf                   *conf.PathConf
	source                 source
	sourceReady            bool
	sourceTrackCount       int
	sourceSdp              []byte
	lastDescribeReq        time.Time
	lastDescribeActivation time.Time
	onInitCmd              *externalcmd.ExternalCmd
	onDemandCmd            *externalcmd.ExternalCmd
}

func newPath(p *program, name string, conf *conf.PathConf) *path {
	pa := &path{
		p:    p,
		name: name,
		conf: conf,
	}

	if strings.HasPrefix(conf.Source, "rtsp://") {
		s := newSourceRtsp(p, pa)
		pa.source = s

	} else if strings.HasPrefix(conf.Source, "rtmp://") {
		s := newSourceRtmp(p, pa)
		pa.source = s
	}

	return pa
}

func (pa *path) log(format string, args ...interface{}) {
	pa.p.log("[path "+pa.name+"] "+format, args...)
}

func (pa *path) onInit() {
	if source, ok := pa.source.(*sourceRtsp); ok {
		go source.run(source.state)

	} else if source, ok := pa.source.(*sourceRtmp); ok {
		go source.run(source.state)
	}

	if pa.conf.RunOnInit != "" {
		pa.log("starting on init command")

		var err error
		pa.onInitCmd, err = externalcmd.New(pa.conf.RunOnInit, pa.name)
		if err != nil {
			pa.log("ERR: %s", err)
		}
	}
}

func (pa *path) onClose() {
	if source, ok := pa.source.(*sourceRtsp); ok {
		close(source.terminate)
		<-source.done

	} else if source, ok := pa.source.(*sourceRtmp); ok {
		close(source.terminate)
		<-source.done
	}

	if pa.onInitCmd != nil {
		pa.log("stopping on init command (closing)")
		pa.onInitCmd.Close()
	}

	if pa.onDemandCmd != nil {
		pa.log("stopping on demand command (closing)")
		pa.onDemandCmd.Close()
	}

	for c := range pa.p.clients {
		if c.path == pa {
			if c.state == clientStateWaitDescription {
				c.path = nil
				c.state = clientStateInitial
				c.describe <- describeRes{nil, fmt.Errorf("publisher of path '%s' has timed out", pa.name)}
			} else {
				c.close()
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
		if c.path == pa && c != pa.source {
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
	if source, ok := pa.source.(*sourceRtsp); ok {
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
	} else if source, ok := pa.source.(*sourceRtmp); ok {
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
		pa.onDemandCmd.Close()
		pa.onDemandCmd = nil
	}

	// remove regular expression paths
	if pa.conf.Regexp != nil &&
		pa.source == nil &&
		!pa.hasClients() {
		pa.onClose()
		delete(pa.p.paths, pa.name)
	}
}

func (pa *path) onSourceRemove() {
	pa.source = nil

	// close all clients that are reading or waiting for reading
	for c := range pa.p.clients {
		if c.path == pa &&
			c.state != clientStateWaitDescription &&
			c != pa.source {
			c.close()
		}
	}
}

func (pa *path) onSourceSetReady() {
	pa.sourceReady = true

	// reply to all clients that are waiting for a description
	for c := range pa.p.clients {
		if c.state == clientStateWaitDescription &&
			c.path == pa {
			c.path = nil
			c.state = clientStateInitial
			c.describe <- describeRes{pa.sourceSdp, nil}
		}
	}
}

func (pa *path) onSourceSetNotReady() {
	pa.sourceReady = false

	// close all clients that are reading or waiting for reading
	for c := range pa.p.clients {
		if c.path == pa &&
			c.state != clientStateWaitDescription &&
			c != pa.source {
			c.close()
		}
	}
}

func (pa *path) onDescribe(client *client) {
	pa.lastDescribeReq = time.Now()

	// publisher not found
	if pa.source == nil {
		// on demand command is available: put the client on hold
		if pa.conf.RunOnDemand != "" {
			if pa.onDemandCmd == nil { // start if needed
				pa.log("starting on demand command")
				pa.lastDescribeActivation = time.Now()

				var err error
				pa.onDemandCmd, err = externalcmd.New(pa.conf.RunOnDemand, pa.name)
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
	} else if !pa.sourceReady {
		// start rtsp source if needed
		if source, ok := pa.source.(*sourceRtsp); ok {
			if source.state == sourceRtspStateStopped {
				pa.log("starting on demand rtsp source")
				pa.lastDescribeActivation = time.Now()
				atomic.AddInt64(pa.p.countSourcesRtspRunning, +1)
				source.state = sourceRtspStateRunning
				source.setState <- source.state
			}

			// start rtmp source if needed
		} else if source, ok := pa.source.(*sourceRtmp); ok {
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
		client.describe <- describeRes{pa.sourceSdp, nil}
	}
}
