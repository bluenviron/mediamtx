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

// Version can be overridden by build flags.
var Version = "v0.0.0"

type program struct {
	confPath      string
	conf          *conf.Conf
	stats         *stats.Stats
	logHandler    *loghandler.LogHandler
	metrics       *metrics.Metrics
	pprof         *pprof.Pprof
	serverUdpRtp  *serverudp.Server
	serverUdpRtcp *serverudp.Server
	serverTcp     *servertcp.Server
	pathMan       *pathman.PathManager
	clientMan     *clientman.ClientManager
	confWatcher   *confwatcher.ConfWatcher

	terminate chan struct{}
	done      chan struct{}
}

func newProgram(args []string) (*program, error) {
	k := kingpin.New("rtsp-simple-server",
		"rtsp-simple-server "+Version+"\n\nRTSP server.")

	argVersion := k.Flag("version", "print version").Bool()
	argConfPath := k.Arg("confpath", "path to a config file. The default is rtsp-simple-server.yml.").Default("rtsp-simple-server.yml").String()

	kingpin.MustParse(k.Parse(args))

	if *argVersion == true {
		fmt.Println(Version)
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

	err = p.createResources(true)
	if err != nil {
		p.closeResources()
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

	log.Printf(fmt.Sprintf("[%d/%d/%d] "+format, append([]interface{}{countClients,
		countPublishers, countReaders}, args...)...))
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

	p.closeResources()
}

func (p *program) createResources(initial bool) error {
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
		p.Log("rtsp-simple-server %s", Version)
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
		if p.serverUdpRtp == nil {
			p.serverUdpRtp, err = serverudp.New(p.conf.WriteTimeout,
				p.conf.RtpPort, gortsplib.StreamTypeRtp, p)
			if err != nil {
				return err
			}
		}

		if p.serverUdpRtcp == nil {
			p.serverUdpRtcp, err = serverudp.New(p.conf.WriteTimeout,
				p.conf.RtcpPort, gortsplib.StreamTypeRtcp, p)
			if err != nil {
				return err
			}
		}
	}

	if p.serverTcp == nil {
		p.serverTcp, err = servertcp.New(p.conf.RtspPort, p)
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
			p.conf.ProtocolsParsed, p.stats, p.serverUdpRtp, p.serverUdpRtcp,
			p.pathMan, p.serverTcp, p)
	}

	if p.confWatcher == nil {
		p.confWatcher, err = confwatcher.New(p.confPath)
		if err != nil {
			return err
		}
	}

	return nil
}

func (p *program) closeResources() {
	if p.confWatcher != nil {
		p.confWatcher.Close()
	}

	if p.clientMan != nil {
		p.clientMan.Close()
	}

	if p.pathMan != nil {
		p.pathMan.Close()
	}

	if p.serverTcp != nil {
		p.serverTcp.Close()
	}

	if p.serverUdpRtcp != nil {
		p.serverUdpRtcp.Close()
	}

	if p.serverUdpRtp != nil {
		p.serverUdpRtp.Close()
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

	// always recreate confWatcher to avoid reloading twice
	p.confWatcher.Close()
	p.confWatcher = nil

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

	closeServerUdpRtp := false
	if conf.WriteTimeout != p.conf.WriteTimeout ||
		conf.RtpPort != p.conf.RtpPort {
		closeServerUdpRtp = true
	}

	closeServerUdpRtcp := false
	if conf.WriteTimeout != p.conf.WriteTimeout ||
		conf.RtcpPort != p.conf.RtcpPort {
		closeServerUdpRtcp = true
	}

	closeServerTcp := false
	if conf.RtspPort != p.conf.RtspPort {
		closeServerTcp = true
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
	if closeServerUdpRtp ||
		closeServerUdpRtcp ||
		closeServerTcp ||
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

	if closeServerTcp {
		p.serverTcp.Close()
		p.serverTcp = nil
	}

	if closeServerUdpRtcp && p.serverUdpRtcp != nil {
		p.serverUdpRtcp.Close()
		p.serverUdpRtcp = nil
	}

	if closeServerUdpRtp && p.serverUdpRtp != nil {
		p.serverUdpRtp.Close()
		p.serverUdpRtp = nil
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
	return p.createResources(false)
}

func main() {
	p, err := newProgram(os.Args[1:])
	if err != nil {
		log.Fatal("ERR: ", err)
	}

	<-p.done
}
