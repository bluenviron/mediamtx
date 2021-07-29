package core

import (
	"context"
	"fmt"
	"os"
	"reflect"
	"sync/atomic"

	"gopkg.in/alecthomas/kingpin.v2"

	"github.com/aler9/rtsp-simple-server/internal/conf"
	"github.com/aler9/rtsp-simple-server/internal/confwatcher"
	"github.com/aler9/rtsp-simple-server/internal/logger"
	"github.com/aler9/rtsp-simple-server/internal/rlimit"
)

var version = "v0.0.0"

// Core is an instance of rtsp-simple-server.
type Core struct {
	ctx             context.Context
	ctxCancel       func()
	confPath        string
	conf            *conf.Conf
	confFound       bool
	stats           *stats
	logger          *logger.Logger
	metrics         *metrics
	pprof           *pprof
	pathMan         *pathManager
	rtspServerPlain *rtspServer
	rtspServerTLS   *rtspServer
	rtmpServer      *rtmpServer
	hlsServer       *hlsServer
	confWatcher     *confwatcher.ConfWatcher

	// in
	pathSourceReady chan *path

	// out
	done chan struct{}
}

// New allocates a core.
func New(args []string) (*Core, bool) {
	k := kingpin.New("rtsp-simple-server",
		"rtsp-simple-server "+version+"\n\nRTSP server.")

	argVersion := k.Flag("version", "print version").Bool()
	argConfPath := k.Arg("confpath", "path to a config file. The default is rtsp-simple-server.yml.").Default("rtsp-simple-server.yml").String()

	kingpin.MustParse(k.Parse(args))

	if *argVersion {
		fmt.Println(version)
		os.Exit(0)
	}

	// on Linux, try to raise the number of file descriptors that can be opened
	// to allow the maximum possible number of clients
	// do not check for errors
	rlimit.Raise()

	ctx, ctxCancel := context.WithCancel(context.Background())

	p := &Core{
		ctx:             ctx,
		ctxCancel:       ctxCancel,
		confPath:        *argConfPath,
		pathSourceReady: make(chan *path),
		done:            make(chan struct{}),
	}

	var err error
	p.conf, p.confFound, err = conf.Load(p.confPath)
	if err != nil {
		fmt.Printf("ERR: %s\n", err)
		return nil, false
	}

	err = p.createResources(true)
	if err != nil {
		p.Log(logger.Info, "ERR: %s", err)
		p.closeResources(nil)
		return nil, false
	}

	if p.confFound {
		p.confWatcher, err = confwatcher.New(p.confPath)
		if err != nil {
			p.Log(logger.Info, "ERR: %s", err)
			p.closeResources(nil)
			return nil, false
		}
	}

	go p.run()

	return p, true
}

func (p *Core) close() {
	p.ctxCancel()
	<-p.done
}

// Wait waits for the Core to exit.
func (p *Core) Wait() {
	<-p.done
}

// Log is the main logging function.
func (p *Core) Log(level logger.Level, format string, args ...interface{}) {
	countPublishers := atomic.LoadInt64(p.stats.CountPublishers)
	countReaders := atomic.LoadInt64(p.stats.CountReaders)

	p.logger.Log(level, "[%d/%d] "+format, append([]interface{}{
		countPublishers, countReaders,
	}, args...)...)
}

func (p *Core) run() {
	defer close(p.done)

	confChanged := func() chan struct{} {
		if p.confWatcher != nil {
			return p.confWatcher.Watch()
		}
		return make(chan struct{})
	}()

outer:
	for {
		select {
		case <-confChanged:
			err := p.reloadConf()
			if err != nil {
				p.Log(logger.Info, "ERR: %s", err)
				break outer
			}

		case pa := <-p.pathSourceReady:
			if p.hlsServer != nil {
				p.hlsServer.OnPathSourceReady(pa)
			}

		case <-p.ctx.Done():
			break outer
		}
	}

	p.ctxCancel()

	p.closeResources(nil)

	if p.confWatcher != nil {
		p.confWatcher.Close()
	}
}

func (p *Core) createResources(initial bool) error {
	var err error

	if p.stats == nil {
		p.stats = newStats()
	}

	if p.logger == nil {
		p.logger, err = logger.New(
			p.conf.LogLevelParsed,
			p.conf.LogDestinationsParsed,
			p.conf.LogFile)
		if err != nil {
			return err
		}
	}

	if initial {
		p.Log(logger.Info, "rtsp-simple-server %s", version)
		if !p.confFound {
			p.Log(logger.Warn, "configuration file not found, using the default one")
		}
	}

	if p.conf.Metrics {
		if p.metrics == nil {
			p.metrics, err = newMetrics(
				p.conf.MetricsAddress,
				p.stats,
				p)
			if err != nil {
				return err
			}
		}
	}

	if p.conf.PPROF {
		if p.pprof == nil {
			p.pprof, err = newPPROF(
				p.conf.PPROFAddress,
				p)
			if err != nil {
				return err
			}
		}
	}

	if p.pathMan == nil {
		p.pathMan = newPathManager(
			p.ctx,
			p.conf.RTSPAddress,
			p.conf.ReadTimeout,
			p.conf.WriteTimeout,
			p.conf.ReadBufferCount,
			p.conf.ReadBufferSize,
			p.conf.AuthMethodsParsed,
			p.conf.Paths,
			p.stats,
			p)
	}

	if !p.conf.RTSPDisable &&
		(p.conf.EncryptionParsed == conf.EncryptionNo ||
			p.conf.EncryptionParsed == conf.EncryptionOptional) {
		if p.rtspServerPlain == nil {
			_, useUDP := p.conf.ProtocolsParsed[conf.ProtocolUDP]
			_, useMulticast := p.conf.ProtocolsParsed[conf.ProtocolMulticast]
			p.rtspServerPlain, err = newRTSPServer(
				p.ctx,
				p.conf.RTSPAddress,
				p.conf.ReadTimeout,
				p.conf.WriteTimeout,
				p.conf.ReadBufferCount,
				p.conf.ReadBufferSize,
				useUDP,
				useMulticast,
				p.conf.RTPAddress,
				p.conf.RTCPAddress,
				p.conf.MulticastIPRange,
				p.conf.MulticastRTPPort,
				p.conf.MulticastRTCPPort,
				false,
				"",
				"",
				p.conf.RTSPAddress,
				p.conf.ProtocolsParsed,
				p.conf.RunOnConnect,
				p.conf.RunOnConnectRestart,
				p.stats,
				p.pathMan,
				p)
			if err != nil {
				return err
			}
		}
	}

	if !p.conf.RTSPDisable &&
		(p.conf.EncryptionParsed == conf.EncryptionStrict ||
			p.conf.EncryptionParsed == conf.EncryptionOptional) {
		if p.rtspServerTLS == nil {
			p.rtspServerTLS, err = newRTSPServer(
				p.ctx,
				p.conf.RTSPSAddress,
				p.conf.ReadTimeout,
				p.conf.WriteTimeout,
				p.conf.ReadBufferCount,
				p.conf.ReadBufferSize,
				false,
				false,
				"",
				"",
				"",
				0,
				0,
				true,
				p.conf.ServerCert,
				p.conf.ServerKey,
				p.conf.RTSPAddress,
				p.conf.ProtocolsParsed,
				p.conf.RunOnConnect,
				p.conf.RunOnConnectRestart,
				p.stats,
				p.pathMan,
				p)
			if err != nil {
				return err
			}
		}
	}

	if !p.conf.RTMPDisable {
		if p.rtmpServer == nil {
			p.rtmpServer, err = newRTMPServer(
				p.ctx,
				p.conf.RTMPAddress,
				p.conf.ReadTimeout,
				p.conf.WriteTimeout,
				p.conf.ReadBufferCount,
				p.conf.RTSPAddress,
				p.conf.RunOnConnect,
				p.conf.RunOnConnectRestart,
				p.stats,
				p.pathMan,
				p)
			if err != nil {
				return err
			}
		}
	}

	if !p.conf.HLSDisable {
		if p.hlsServer == nil {
			p.hlsServer, err = newHLSServer(
				p.ctx,
				p.conf.HLSAddress,
				p.conf.HLSAlwaysRemux,
				p.conf.HLSSegmentCount,
				p.conf.HLSSegmentDuration,
				p.conf.HLSAllowOrigin,
				p.conf.ReadBufferCount,
				p.stats,
				p.pathMan,
				p)
			if err != nil {
				return err
			}
		}
	}

	return nil
}

func (p *Core) closeResources(newConf *conf.Conf) {
	closeStats := false
	if newConf == nil {
		closeStats = true
	}

	closeLogger := false
	if newConf == nil ||
		!reflect.DeepEqual(newConf.LogDestinationsParsed, p.conf.LogDestinationsParsed) ||
		newConf.LogFile != p.conf.LogFile {
		closeLogger = true
	}

	closeMetrics := false
	if newConf == nil ||
		newConf.Metrics != p.conf.Metrics ||
		newConf.MetricsAddress != p.conf.MetricsAddress ||
		closeStats {
		closeMetrics = true
	}

	closePPROF := false
	if newConf == nil ||
		newConf.PPROF != p.conf.PPROF ||
		newConf.PPROFAddress != p.conf.PPROFAddress ||
		closeStats {
		closePPROF = true
	}

	closePathMan := false
	if newConf == nil ||
		newConf.RTSPAddress != p.conf.RTSPAddress ||
		newConf.ReadTimeout != p.conf.ReadTimeout ||
		newConf.WriteTimeout != p.conf.WriteTimeout ||
		newConf.ReadBufferCount != p.conf.ReadBufferCount ||
		newConf.ReadBufferSize != p.conf.ReadBufferSize ||
		!reflect.DeepEqual(newConf.AuthMethodsParsed, p.conf.AuthMethodsParsed) ||
		closeStats {
		closePathMan = true
	} else if !reflect.DeepEqual(newConf.Paths, p.conf.Paths) {
		p.pathMan.OnConfReload(newConf.Paths)
	}

	closeServerPlain := false
	if newConf == nil ||
		newConf.RTSPDisable != p.conf.RTSPDisable ||
		newConf.EncryptionParsed != p.conf.EncryptionParsed ||
		newConf.RTSPAddress != p.conf.RTSPAddress ||
		newConf.ReadTimeout != p.conf.ReadTimeout ||
		newConf.WriteTimeout != p.conf.WriteTimeout ||
		newConf.ReadBufferCount != p.conf.ReadBufferCount ||
		!reflect.DeepEqual(newConf.ProtocolsParsed, p.conf.ProtocolsParsed) ||
		newConf.RTPAddress != p.conf.RTPAddress ||
		newConf.RTCPAddress != p.conf.RTCPAddress ||
		newConf.MulticastIPRange != p.conf.MulticastIPRange ||
		newConf.MulticastRTPPort != p.conf.MulticastRTPPort ||
		newConf.MulticastRTCPPort != p.conf.MulticastRTCPPort ||
		newConf.RTSPAddress != p.conf.RTSPAddress ||
		!reflect.DeepEqual(newConf.ProtocolsParsed, p.conf.ProtocolsParsed) ||
		newConf.RunOnConnect != p.conf.RunOnConnect ||
		newConf.RunOnConnectRestart != p.conf.RunOnConnectRestart ||
		closeStats ||
		closePathMan {
		closeServerPlain = true
	}

	closeServerTLS := false
	if newConf == nil ||
		newConf.RTSPDisable != p.conf.RTSPDisable ||
		newConf.EncryptionParsed != p.conf.EncryptionParsed ||
		newConf.RTSPSAddress != p.conf.RTSPSAddress ||
		newConf.ReadTimeout != p.conf.ReadTimeout ||
		newConf.WriteTimeout != p.conf.WriteTimeout ||
		newConf.ReadBufferCount != p.conf.ReadBufferCount ||
		newConf.ServerCert != p.conf.ServerCert ||
		newConf.ServerKey != p.conf.ServerKey ||
		newConf.RTSPAddress != p.conf.RTSPAddress ||
		!reflect.DeepEqual(newConf.ProtocolsParsed, p.conf.ProtocolsParsed) ||
		newConf.RunOnConnect != p.conf.RunOnConnect ||
		newConf.RunOnConnectRestart != p.conf.RunOnConnectRestart ||
		closeStats ||
		closePathMan {
		closeServerTLS = true
	}

	closeServerRTMP := false
	if newConf == nil ||
		newConf.RTMPDisable != p.conf.RTMPDisable ||
		newConf.RTMPAddress != p.conf.RTMPAddress ||
		newConf.ReadTimeout != p.conf.ReadTimeout ||
		newConf.WriteTimeout != p.conf.WriteTimeout ||
		newConf.ReadBufferCount != p.conf.ReadBufferCount ||
		newConf.RTSPAddress != p.conf.RTSPAddress ||
		newConf.RunOnConnect != p.conf.RunOnConnect ||
		newConf.RunOnConnectRestart != p.conf.RunOnConnectRestart ||
		closeStats ||
		closePathMan {
		closeServerRTMP = true
	}

	closeServerHLS := false
	if newConf == nil ||
		newConf.HLSDisable != p.conf.HLSDisable ||
		newConf.HLSAddress != p.conf.HLSAddress ||
		newConf.HLSAlwaysRemux != p.conf.HLSAlwaysRemux ||
		newConf.HLSSegmentCount != p.conf.HLSSegmentCount ||
		newConf.HLSSegmentDuration != p.conf.HLSSegmentDuration ||
		newConf.HLSAllowOrigin != p.conf.HLSAllowOrigin ||
		newConf.ReadBufferCount != p.conf.ReadBufferCount ||
		closeStats ||
		closePathMan {
		closeServerHLS = true
	}

	if closeServerTLS && p.rtspServerTLS != nil {
		p.rtspServerTLS.close()
		p.rtspServerTLS = nil
	}

	if closeServerPlain && p.rtspServerPlain != nil {
		p.rtspServerPlain.close()
		p.rtspServerPlain = nil
	}

	if closePathMan && p.pathMan != nil {
		p.pathMan.close()
		p.pathMan = nil
	}

	if closeServerHLS && p.hlsServer != nil {
		p.hlsServer.close()
		p.hlsServer = nil
	}

	if closeServerRTMP && p.rtmpServer != nil {
		p.rtmpServer.close()
		p.rtmpServer = nil
	}

	if closePPROF && p.pprof != nil {
		p.pprof.close()
		p.pprof = nil
	}

	if closeMetrics && p.metrics != nil {
		p.metrics.close()
		p.metrics = nil
	}

	if closeLogger && p.logger != nil {
		p.logger.Close()
		p.logger = nil
	}

	if closeStats && p.stats != nil {
		p.stats.close()
	}
}

func (p *Core) reloadConf() error {
	p.Log(logger.Info, "reloading configuration")

	newConf, _, err := conf.Load(p.confPath)
	if err != nil {
		return err
	}

	p.closeResources(newConf)

	p.conf = newConf
	return p.createResources(false)
}

// OnPathSourceReady is called by pathManager.
func (p *Core) OnPathSourceReady(pa *path) {
	select {
	case p.pathSourceReady <- pa:
	case <-p.done:
	}
}
