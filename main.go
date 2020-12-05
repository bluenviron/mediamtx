package main

import (
	"fmt"
	"log"
	"os"
	"reflect"
	"sync/atomic"

	"github.com/aler9/gortsplib"
	"gopkg.in/alecthomas/kingpin.v2"

	"github.com/aler9/rtsp-simple-server/internal/clientman"
	"github.com/aler9/rtsp-simple-server/internal/conf"
	"github.com/aler9/rtsp-simple-server/internal/confwatcher"
	"github.com/aler9/rtsp-simple-server/internal/loghandler"
	"github.com/aler9/rtsp-simple-server/internal/metrics"
	"github.com/aler9/rtsp-simple-server/internal/pathman"
	"github.com/aler9/rtsp-simple-server/internal/pprof"
	"github.com/aler9/rtsp-simple-server/internal/servertcp"
	"github.com/aler9/rtsp-simple-server/internal/serverudp"
	"github.com/aler9/rtsp-simple-server/internal/stats"
)

var version = "v0.0.0"

type program struct {
	confPath      string
	conf          *conf.Conf
	stats         *stats.Stats
	logHandler    *loghandler.LogHandler
	metrics       *metrics.Metrics
	pprof         *pprof.Pprof
	serverUDPRtp  *serverudp.Server
	serverUDPRtcp *serverudp.Server
	serverTCP     *servertcp.Server
	pathMan       *pathman.PathManager
	clientMan     *clientman.ClientManager
	confWatcher   *confwatcher.ConfWatcher

	terminate chan struct{}
	done      chan struct{}
}

func newProgram(args []string) (*program, error) {
	k := kingpin.New("rtsp-simple-server",
		"rtsp-simple-server "+version+"\n\nRTSP server.")

	argVersion := k.Flag("version", "print version").Bool()
	argConfPath := k.Arg("confpath", "path to a config file. The default is rtsp-simple-server.yml.").Default("rtsp-simple-server.yml").String()

	kingpin.MustParse(k.Parse(args))

	if *argVersion {
		fmt.Println(version)
		os.Exit(0)
	}

	p := &program{
		confPath:  *argConfPath,
		terminate: make(chan struct{}),
		done:      make(chan struct{}),
	}

	var err error
	p.conf, err = conf.Load(p.confPath)
	if err != nil {
		return nil, err
	}

	err = p.createDynamicResources(true)
	if err != nil {
		p.closeAllResources()
		return nil, err
	}

	p.confWatcher, err = confwatcher.New(p.confPath)
	if err != nil {
		p.closeAllResources()
		return nil, err
	}

	go p.run()

	return p, nil
}

func (p *program) close() {
	close(p.terminate)
	<-p.done
}

func (p *program) Log(format string, args ...interface{}) {
	countClients := atomic.LoadInt64(p.stats.CountClients)
	countPublishers := atomic.LoadInt64(p.stats.CountPublishers)
	countReaders := atomic.LoadInt64(p.stats.CountReaders)

	log.Printf("[%d/%d/%d] "+format, append([]interface{}{countClients,
		countPublishers, countReaders}, args...)...)
}

func (p *program) run() {
	defer close(p.done)

outer:
	for {
		select {
		case <-p.confWatcher.Watch():
			err := p.reloadConf()
			if err != nil {
				p.Log("ERR: %s", err)
				break outer
			}

		case <-p.terminate:
			break outer
		}
	}

	p.closeAllResources()
}

func (p *program) createDynamicResources(initial bool) error {
	var err error

	if p.stats == nil {
		p.stats = stats.New()
	}

	if p.logHandler == nil {
		p.logHandler, err = loghandler.New(p.conf.LogDestinationsParsed, p.conf.LogFile)
		if err != nil {
			return err
		}
	}

	if initial {
		p.Log("rtsp-simple-server %s", version)
	}

	if p.conf.Metrics {
		if p.metrics == nil {
			p.metrics, err = metrics.New(p.stats, p)
			if err != nil {
				return err
			}
		}
	}

	if p.conf.Pprof {
		if p.pprof == nil {
			p.pprof, err = pprof.New(p)
			if err != nil {
				return err
			}
		}
	}

	if _, ok := p.conf.ProtocolsParsed[gortsplib.StreamProtocolUDP]; ok {
		if p.serverUDPRtp == nil {
			p.serverUDPRtp, err = serverudp.New(p.conf.WriteTimeout,
				p.conf.RtpPort, gortsplib.StreamTypeRtp, p)
			if err != nil {
				return err
			}
		}

		if p.serverUDPRtcp == nil {
			p.serverUDPRtcp, err = serverudp.New(p.conf.WriteTimeout,
				p.conf.RtcpPort, gortsplib.StreamTypeRtcp, p)
			if err != nil {
				return err
			}
		}
	}

	if p.serverTCP == nil {
		p.serverTCP, err = servertcp.New(p.conf.RtspPort, p)
		if err != nil {
			return err
		}
	}

	if p.pathMan == nil {
		p.pathMan = pathman.New(p.conf.RtspPort, p.conf.ReadTimeout,
			p.conf.WriteTimeout, p.conf.AuthMethodsParsed, p.conf.Paths,
			p.stats, p)
	}

	if p.clientMan == nil {
		p.clientMan = clientman.New(p.conf.RtspPort, p.conf.ReadTimeout,
			p.conf.WriteTimeout, p.conf.RunOnConnect, p.conf.RunOnConnectRestart,
			p.conf.ProtocolsParsed, p.stats, p.serverUDPRtp, p.serverUDPRtcp,
			p.pathMan, p.serverTCP, p)
	}

	return nil
}

func (p *program) closeAllResources() {
	if p.confWatcher != nil {
		p.confWatcher.Close()
	}

	if p.clientMan != nil {
		p.clientMan.Close()
	}

	if p.pathMan != nil {
		p.pathMan.Close()
	}

	if p.serverTCP != nil {
		p.serverTCP.Close()
	}

	if p.serverUDPRtcp != nil {
		p.serverUDPRtcp.Close()
	}

	if p.serverUDPRtp != nil {
		p.serverUDPRtp.Close()
	}

	if p.metrics != nil {
		p.metrics.Close()
	}

	if p.pprof != nil {
		p.pprof.Close()
	}

	if p.logHandler != nil {
		p.logHandler.Close()
	}
}

func (p *program) reloadConf() error {
	p.Log("reloading configuration")

	conf, err := conf.Load(p.confPath)
	if err != nil {
		return err
	}

	closeLogHandler := false
	if !reflect.DeepEqual(conf.LogDestinationsParsed, p.conf.LogDestinationsParsed) ||
		conf.LogFile != p.conf.LogFile {
		closeLogHandler = true
	}

	closeMetrics := false
	if conf.Metrics != p.conf.Metrics {
		closeMetrics = true
	}

	closePprof := false
	if conf.Pprof != p.conf.Pprof {
		closePprof = true
	}

	closeServerUDPRtp := false
	if !reflect.DeepEqual(conf.ProtocolsParsed, p.conf.ProtocolsParsed) ||
		conf.WriteTimeout != p.conf.WriteTimeout ||
		conf.RtpPort != p.conf.RtpPort {
		closeServerUDPRtp = true
	}

	closeServerUDPRtcp := false
	if !reflect.DeepEqual(conf.ProtocolsParsed, p.conf.ProtocolsParsed) ||
		conf.WriteTimeout != p.conf.WriteTimeout ||
		conf.RtcpPort != p.conf.RtcpPort {
		closeServerUDPRtcp = true
	}

	closeServerTCP := false
	if conf.RtspPort != p.conf.RtspPort {
		closeServerTCP = true
	}

	closePathMan := false
	if conf.RtspPort != p.conf.RtspPort ||
		conf.ReadTimeout != p.conf.ReadTimeout ||
		conf.WriteTimeout != p.conf.WriteTimeout ||
		!reflect.DeepEqual(conf.AuthMethodsParsed, p.conf.AuthMethodsParsed) {
		closePathMan = true
	} else if !reflect.DeepEqual(conf.Paths, p.conf.Paths) {
		p.pathMan.OnProgramConfReload(conf.Paths)
	}

	closeClientMan := false
	if closeServerUDPRtp ||
		closeServerUDPRtcp ||
		closeServerTCP ||
		closePathMan ||
		conf.RtspPort != p.conf.RtspPort ||
		conf.ReadTimeout != p.conf.ReadTimeout ||
		conf.WriteTimeout != p.conf.WriteTimeout ||
		conf.RunOnConnect != p.conf.RunOnConnect ||
		conf.RunOnConnectRestart != p.conf.RunOnConnectRestart ||
		!reflect.DeepEqual(conf.ProtocolsParsed, p.conf.ProtocolsParsed) {
		closeClientMan = true
	}

	if closeClientMan {
		p.clientMan.Close()
		p.clientMan = nil
	}

	if closePathMan {
		p.pathMan.Close()
		p.pathMan = nil
	}

	if closeServerTCP {
		p.serverTCP.Close()
		p.serverTCP = nil
	}

	if closeServerUDPRtcp && p.serverUDPRtcp != nil {
		p.serverUDPRtcp.Close()
		p.serverUDPRtcp = nil
	}

	if closeServerUDPRtp && p.serverUDPRtp != nil {
		p.serverUDPRtp.Close()
		p.serverUDPRtp = nil
	}

	if closePprof && p.pprof != nil {
		p.pprof.Close()
		p.pprof = nil
	}

	if closeMetrics && p.metrics != nil {
		p.metrics.Close()
		p.metrics = nil
	}

	if closeLogHandler {
		p.logHandler.Close()
		p.logHandler = nil
	}

	p.conf = conf
	return p.createDynamicResources(false)
}

func main() {
	p, err := newProgram(os.Args[1:])
	if err != nil {
		log.Fatal("ERR: ", err)
	}

	<-p.done
}
