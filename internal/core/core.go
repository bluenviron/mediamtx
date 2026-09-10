// Package core contains the main struct of the software.
package core

import (
	"context"
	_ "embed"
	"fmt"
	"net"
	"os"
	"os/signal"
	"path/filepath"
	"reflect"
	"runtime"
	"runtime/debug"
	"slices"
	"strings"
	"sync/atomic"
	"syscall"
	"time"

	"github.com/alecthomas/kong"
	"github.com/gin-gonic/gin"

	"github.com/bluenviron/mediamtx/internal/api"
	"github.com/bluenviron/mediamtx/internal/auth"
	"github.com/bluenviron/mediamtx/internal/conf"
	"github.com/bluenviron/mediamtx/internal/confwatcher"
	"github.com/bluenviron/mediamtx/internal/externalcmd"
	"github.com/bluenviron/mediamtx/internal/logger"
	"github.com/bluenviron/mediamtx/internal/metrics"
	"github.com/bluenviron/mediamtx/internal/playback"
	"github.com/bluenviron/mediamtx/internal/pprof"
	"github.com/bluenviron/mediamtx/internal/recordcleaner"
	"github.com/bluenviron/mediamtx/internal/rlimit"
	"github.com/bluenviron/mediamtx/internal/servers/hls"
	"github.com/bluenviron/mediamtx/internal/servers/moq"
	"github.com/bluenviron/mediamtx/internal/servers/rtmp"
	"github.com/bluenviron/mediamtx/internal/servers/rtsp"
	"github.com/bluenviron/mediamtx/internal/servers/srt"
	"github.com/bluenviron/mediamtx/internal/servers/webrtc"
	"github.com/bluenviron/mediamtx/internal/upgrade"
)

//go:generate go run ./versiongetter

//go:embed VERSION
var version []byte

var started = time.Now()

var defaultConfPaths = []string{
	"rtsp-simple-server.yml",
	"mediamtx.yml",
}

var defaultConfPathsNotWin = []string{
	"/usr/local/etc/mediamtx.yml",
	"/usr/etc/mediamtx.yml",
	"/etc/mediamtx/mediamtx.yml",
}

func currentDefaultConfPaths() []string {
	paths := append([]string(nil), defaultConfPaths...)
	if runtime.GOOS != "windows" {
		paths = append(paths, defaultConfPathsNotWin...)
	}
	return paths
}

func formatConfPaths(paths []string) []string {
	list := make([]string, len(paths))
	for i, pa := range paths {
		a, _ := filepath.Abs(pa)
		list[i] = a
	}
	return list
}

func newTempLogger() (*logger.Logger, error) {
	l := &logger.Logger{
		Level:        logger.Warn,
		Destinations: []logger.Destination{logger.DestinationStdout},
		Structured:   false,
		File:         "",
		SysLogPrefix: "",
	}
	return l, l.Initialize()
}

func validateConf(confPath string) bool {
	fmt.Printf("configuration file: %s\n", confPath)

	tempLogger, err := newTempLogger()
	if err != nil {
		fmt.Printf("ERR: %v\n", err)
		return false
	}
	defer tempLogger.Close()

	_, _, err = conf.Load(confPath, nil, tempLogger)
	if err != nil {
		fmt.Printf("ERR: %v\n", err)
		return false
	}

	fmt.Printf("configuration file is valid\n")

	return true
}

func goArm() string {
	bi, _ := debug.ReadBuildInfo()
	for _, bs := range bi.Settings {
		if bs.Key == "GOARM" {
			return bs.Value
		}
	}
	return ""
}

func getArch() string {
	var arch string
	if runtime.GOARCH == "arm" {
		arch = "armv" + goArm()
	} else {
		arch = runtime.GOARCH
	}
	return arch
}

func atLeastOneRecordDeleteAfter(pathConfs map[string]*conf.Path) bool {
	for _, e := range pathConfs {
		if e.RecordDeleteAfter != 0 {
			return true
		}
	}
	return false
}

func getRTPMaxPayloadSize(udpMaxPayloadSize int, rtspEncryption conf.Encryption) int {
	// UDP max payload size - 12 (RTP header)
	v := udpMaxPayloadSize - 12

	// 10 (SRTP HMAC SHA1 authentication tag)
	if rtspEncryption == conf.EncryptionOptional || rtspEncryption == conf.EncryptionStrict {
		v -= 10
	}

	return v
}

func supportsIPv6() bool {
	ln, err := net.ListenUDP("udp6", &net.UDPAddr{IP: net.IPv6unspecified, Port: 0})
	if err != nil {
		return false
	}
	defer ln.Close() //nolint:errcheck

	return true
}

var cli struct {
	Confpath     string `arg:"" default:""`
	Version      bool   `help:"print version"`
	CheckVersion bool   `help:"check whether a new version is available"`
	Upgrade      bool   `help:"upgrade executable to the latest version"`
	ValidateConf string `help:"check whether a configuration file is valid" placeholder:"path"`
}

type configGlobalPatchReq struct {
	conf conf.OptionalGlobal
	res  chan error
}

type configPathDefaultsPatchReq struct {
	conf conf.OptionalPath
	res  chan error
}

type configPathAddReq struct {
	name string
	conf conf.OptionalPath
	res  chan error
}

type configPathPatchReq struct {
	name string
	conf conf.OptionalPath
	res  chan error
}

type configPathReplaceReq struct {
	name string
	conf conf.OptionalPath
	res  chan error
}

type configPathDeleteReq struct {
	name string
	res  chan error
}

// Core is an instance of MediaMTX.
type Core struct {
	ctx             context.Context
	ctxCancel       func()
	confPath        string
	conf            atomic.Pointer[conf.Conf]
	supportsIPv6    bool
	logger          *logger.Logger
	externalCmdPool *externalcmd.Pool
	authManager     *auth.Manager
	metrics         *metrics.Metrics
	pprof           *pprof.PPROF
	recordCleaner   *recordcleaner.Cleaner
	playbackServer  *playback.Server
	pathManager     *pathManager
	rtspServer      *rtsp.Server
	rtspsServer     *rtsp.Server
	rtmpServer      *rtmp.Server
	rtmpsServer     *rtmp.Server
	hlsServer       *hls.Server
	webRTCServer    *webrtc.Server
	srtServer       *srt.Server
	moqServer       *moq.Server
	api             *api.API
	confWatcher     *confwatcher.ConfWatcher

	// in
	chAPIConfigGlobalPatch       chan configGlobalPatchReq
	chAPIConfigPathDefaultsPatch chan configPathDefaultsPatchReq
	chAPIConfigPathAdd           chan configPathAddReq
	chAPIConfigPathPatch         chan configPathPatchReq
	chAPIConfigPathReplace       chan configPathReplaceReq
	chAPIConfigPathDelete        chan configPathDeleteReq

	// out
	done chan struct{}
}

// New allocates a Core.
func New(args []string) (*Core, bool) {
	parser, err := kong.New(&cli,
		kong.Description("MediaMTX "+string(version)+", "+runtime.GOOS+", "+getArch()),
		kong.UsageOnError(),
		kong.ValueFormatter(func(value *kong.Value) string {
			switch value.Name {
			case "confpath":
				return "path to a config file. The default is mediamtx.yml."

			default:
				return kong.DefaultHelpValueFormatter(value)
			}
		}))
	if err != nil {
		panic(err)
	}

	_, err = parser.Parse(args)
	parser.FatalIfErrorf(err)

	oneShotCount := 0
	if cli.Version {
		oneShotCount++
	}
	if cli.CheckVersion {
		oneShotCount++
	}
	if cli.Upgrade {
		oneShotCount++
	}
	if cli.ValidateConf != "" {
		oneShotCount++
	}
	if oneShotCount > 1 {
		fmt.Printf("ERR: %v\n", "only one of --version, --check-version, --upgrade and --validate-conf can be used at a time")
		return nil, false
	}

	if cli.Version {
		fmt.Println(string(version))
		os.Exit(0)
	}

	if cli.CheckVersion {
		var newVersionAvailable bool
		newVersionAvailable, err = upgrade.CheckVersion(string(version), getArch())
		if err != nil {
			fmt.Printf("ERR: %v\n", err)
			os.Exit(1)
		}
		if newVersionAvailable {
			os.Exit(2)
		}
		os.Exit(0)
	}

	if cli.Upgrade {
		err = upgrade.Upgrade(string(version), getArch())
		if err != nil {
			fmt.Printf("ERR: %v\n", err)
			os.Exit(1)
		}
		os.Exit(0)
	}

	if cli.ValidateConf != "" {
		ok := validateConf(cli.ValidateConf)
		if !ok {
			os.Exit(1)
		}
		os.Exit(0)
	}

	ctx, ctxCancel := context.WithCancel(context.Background())

	p := &Core{
		ctx:                          ctx,
		ctxCancel:                    ctxCancel,
		chAPIConfigGlobalPatch:       make(chan configGlobalPatchReq),
		chAPIConfigPathDefaultsPatch: make(chan configPathDefaultsPatchReq),
		chAPIConfigPathAdd:           make(chan configPathAddReq),
		chAPIConfigPathPatch:         make(chan configPathPatchReq),
		chAPIConfigPathReplace:       make(chan configPathReplaceReq),
		chAPIConfigPathDelete:        make(chan configPathDeleteReq),
		done:                         make(chan struct{}),
	}

	tempLogger, err := newTempLogger()
	if err != nil {
		fmt.Printf("ERR: %v\n", err)
		return nil, false
	}
	defer tempLogger.Close()

	confPaths := currentDefaultConfPaths()

	loadedConf, confPath, err := conf.Load(cli.Confpath, confPaths, tempLogger)
	if err != nil {
		fmt.Printf("ERR: %s\n", err)
		return nil, false
	}

	p.confPath = confPath
	p.conf.Store(loadedConf)

	err = p.createResources(true)
	if err != nil {
		if p.logger != nil {
			p.Log(logger.Error, "%s", err)
		} else {
			fmt.Printf("ERR: %s\n", err)
		}
		p.closeResources(nil)
		return nil, false
	}

	go p.run()

	return p, true
}

// Close closes Core and waits for all goroutines to return.
func (p *Core) Close() {
	p.ctxCancel()
	<-p.done
}

// Wait waits for the Core to exit.
func (p *Core) Wait() {
	<-p.done
}

// Log implements logger.Writer.
func (p *Core) Log(level logger.Level, format string, args ...any) {
	p.logger.Log(level, format, args...)
}

func (p *Core) run() {
	defer close(p.done)

	confChanged := func() chan struct{} {
		if p.confWatcher != nil {
			return p.confWatcher.Watch()
		}
		return make(chan struct{})
	}()

	interrupt := make(chan os.Signal, 1)
	signal.Notify(interrupt, os.Interrupt)
	if runtime.GOOS == "linux" {
		signal.Notify(interrupt, syscall.SIGTERM)
	}

outer:
	for {
		select {
		case <-confChanged:
			p.Log(logger.Info, "reloading configuration (file changed)")

			newConf, _, err := conf.Load(p.confPath, nil, p.logger)
			if err != nil {
				p.Log(logger.Error, "%s", err)
				break outer
			}

			err = p.reloadConf(newConf)
			if err != nil {
				p.Log(logger.Error, "%s", err)
				break outer
			}

		case req := <-p.chAPIConfigGlobalPatch:
			newConf, err := p.doAPIConfigGlobalPatch(req.conf)
			req.res <- err

			if err == nil {
				err = p.reloadConf(newConf)
				if err != nil {
					p.Log(logger.Error, "%s", err)
					break outer
				}
			}

		case req := <-p.chAPIConfigPathDefaultsPatch:
			newConf, err := p.doAPIConfigPathDefaultsPatch(req.conf)
			req.res <- err

			if err == nil {
				err = p.reloadConf(newConf)
				if err != nil {
					p.Log(logger.Error, "%s", err)
					break outer
				}
			}

		case req := <-p.chAPIConfigPathAdd:
			newConf, err := p.doAPIConfigPathAdd(req.name, req.conf)
			req.res <- err

			if err == nil {
				err = p.reloadConf(newConf)
				if err != nil {
					p.Log(logger.Error, "%s", err)
					break outer
				}
			}

		case req := <-p.chAPIConfigPathPatch:
			newConf, err := p.doAPIConfigPathPatch(req.name, req.conf)
			req.res <- err

			if err == nil {
				err = p.reloadConf(newConf)
				if err != nil {
					p.Log(logger.Error, "%s", err)
					break outer
				}
			}

		case req := <-p.chAPIConfigPathReplace:
			newConf, err := p.doAPIConfigPathReplace(req.name, req.conf)
			req.res <- err

			if err == nil {
				err = p.reloadConf(newConf)
				if err != nil {
					p.Log(logger.Error, "%s", err)
					break outer
				}
			}

		case req := <-p.chAPIConfigPathDelete:
			newConf, err := p.doAPIConfigPathDelete(req.name)
			req.res <- err

			if err == nil {
				err = p.reloadConf(newConf)
				if err != nil {
					p.Log(logger.Error, "%s", err)
					break outer
				}
			}

		case <-interrupt:
			p.Log(logger.Info, "shutting down gracefully")
			break outer

		case <-p.ctx.Done():
			break outer
		}
	}

	p.ctxCancel()

	p.closeResources(nil)
}

func (p *Core) createResources(initial bool) error {
	currentConf := p.conf.Load()
	var err error

	if p.logger == nil {
		i := &logger.Logger{
			Level:        logger.Level(currentConf.LogLevel),
			Destinations: currentConf.LogDestinations.ToDestinations(),
			Structured:   currentConf.LogStructured,
			File:         currentConf.LogFile,
			SysLogPrefix: currentConf.SysLogPrefix,
		}
		err = i.Initialize()
		if err != nil {
			return err
		}
		p.logger = i
	}

	if initial {
		p.Log(logger.Info, "MediaMTX %s, %s, %s", string(version), runtime.GOOS, getArch())

		if p.confPath != "" {
			a, _ := filepath.Abs(p.confPath)
			p.Log(logger.Info, "configuration loaded from %s", a)
		} else {
			p.Log(logger.Warn,
				"configuration file not found (looked in %s), using an empty configuration",
				strings.Join(formatConfPaths(currentDefaultConfPaths()), ", "))
		}

		// on Linux, try to raise the number of file descriptors that can be opened
		// to allow the maximum possible number of clients.
		rlimit.Raise() //nolint:errcheck

		gin.SetMode(gin.ReleaseMode)

		p.supportsIPv6 = supportsIPv6()

		p.externalCmdPool = &externalcmd.Pool{}
		p.externalCmdPool.Initialize()
	}

	if p.authManager == nil {
		p.authManager = &auth.Manager{
			Method:             currentConf.AuthMethod,
			InternalUsers:      currentConf.AuthInternalUsers,
			HTTPAddress:        currentConf.AuthHTTPAddress,
			HTTPFingerprint:    currentConf.AuthHTTPFingerprint,
			HTTPExclude:        currentConf.AuthHTTPExclude,
			JWTJWKS:            currentConf.AuthJWTJWKS,
			JWTJWKSFingerprint: currentConf.AuthJWTJWKSFingerprint,
			JWTClaimKey:        currentConf.AuthJWTClaimKey,
			JWTExclude:         currentConf.AuthJWTExclude,
			JWTInHTTPQuery:     currentConf.AuthJWTInHTTPQuery,
			JWTIssuer:          currentConf.AuthJWTIssuer,
			JWTAudience:        currentConf.AuthJWTAudience,
			ReadTimeout:        time.Duration(currentConf.ReadTimeout),
		}
	}

	if currentConf.Metrics &&
		p.metrics == nil {
		i := &metrics.Metrics{
			Address:        currentConf.MetricsAddress,
			DumpPackets:    currentConf.DumpPackets,
			Encryption:     currentConf.MetricsEncryption,
			ServerKey:      currentConf.MetricsServerKey,
			ServerCert:     currentConf.MetricsServerCert,
			AllowOrigins:   currentConf.MetricsAllowOrigins,
			TrustedProxies: currentConf.MetricsTrustedProxies,
			ReadTimeout:    currentConf.ReadTimeout,
			WriteTimeout:   currentConf.WriteTimeout,
			AuthManager:    p.authManager,
			Parent:         p,
		}
		err = i.Initialize()
		if err != nil {
			return err
		}
		p.metrics = i
	}

	if currentConf.PPROF &&
		p.pprof == nil {
		i := &pprof.PPROF{
			Address:        currentConf.PPROFAddress,
			DumpPackets:    currentConf.DumpPackets,
			Encryption:     currentConf.PPROFEncryption,
			ServerKey:      currentConf.PPROFServerKey,
			ServerCert:     currentConf.PPROFServerCert,
			AllowOrigins:   currentConf.PPROFAllowOrigins,
			TrustedProxies: currentConf.PPROFTrustedProxies,
			ReadTimeout:    currentConf.ReadTimeout,
			WriteTimeout:   currentConf.WriteTimeout,
			AuthManager:    p.authManager,
			Parent:         p,
		}
		err = i.Initialize()
		if err != nil {
			return err
		}
		p.pprof = i
	}

	if p.recordCleaner == nil &&
		atLeastOneRecordDeleteAfter(currentConf.Paths) {
		p.recordCleaner = &recordcleaner.Cleaner{
			PathConfs: currentConf.Paths,
			Parent:    p,
		}
		p.recordCleaner.Initialize()
	}

	if currentConf.Playback &&
		p.playbackServer == nil {
		i := &playback.Server{
			Address:        currentConf.PlaybackAddress,
			DumpPackets:    currentConf.DumpPackets,
			Encryption:     currentConf.PlaybackEncryption,
			ServerKey:      currentConf.PlaybackServerKey,
			ServerCert:     currentConf.PlaybackServerCert,
			AllowOrigins:   currentConf.PlaybackAllowOrigins,
			TrustedProxies: currentConf.PlaybackTrustedProxies,
			ReadTimeout:    currentConf.ReadTimeout,
			WriteTimeout:   currentConf.WriteTimeout,
			PathConfs:      currentConf.Paths,
			AuthManager:    p.authManager,
			Parent:         p,
		}
		err = i.Initialize()
		if err != nil {
			return err
		}
		p.playbackServer = i
	}

	if p.pathManager == nil {
		rtpMaxPayloadSize := getRTPMaxPayloadSize(currentConf.UDPMaxPayloadSize, currentConf.RTSPEncryption)

		p.pathManager = &pathManager{
			logLevel:           currentConf.LogLevel,
			dumpPackets:        currentConf.DumpPackets,
			rtspAddress:        currentConf.RTSPAddress,
			readTimeout:        currentConf.ReadTimeout,
			writeTimeout:       currentConf.WriteTimeout,
			writeQueueSize:     currentConf.WriteQueueSize,
			udpReadBufferSize:  currentConf.UDPReadBufferSize,
			udpWriteBufferSize: currentConf.UDPWriteBufferSize,
			udpMaxPayloadSize:  currentConf.UDPMaxPayloadSize,
			rtpMaxPayloadSize:  rtpMaxPayloadSize,
			supportsIPv6:       p.supportsIPv6,
			pathConfs:          currentConf.Paths,
			authManager:        p.authManager,
			externalCmdPool:    p.externalCmdPool,
			metrics:            p.metrics,
			parent:             p,
		}
		p.pathManager.initialize()
	}

	if currentConf.RTSP &&
		(currentConf.RTSPEncryption == conf.EncryptionNo ||
			currentConf.RTSPEncryption == conf.EncryptionOptional) &&
		p.rtspServer == nil {
		udpReadBufferSize := currentConf.UDPReadBufferSize
		if currentConf.RTSPUDPReadBufferSize != nil {
			udpReadBufferSize = *currentConf.RTSPUDPReadBufferSize
		}

		i := &rtsp.Server{
			Address:             currentConf.RTSPAddress,
			AuthMethods:         currentConf.RTSPAuthMethods.ToAuthMethods(),
			DumpPackets:         currentConf.DumpPackets,
			UDPReadBufferSize:   udpReadBufferSize,
			ReadTimeout:         currentConf.ReadTimeout,
			WriteTimeout:        currentConf.WriteTimeout,
			WriteQueueSize:      currentConf.WriteQueueSize,
			RTSPTransports:      currentConf.RTSPTransports,
			RTPAddress:          currentConf.RTPAddress,
			RTCPAddress:         currentConf.RTCPAddress,
			MulticastIPRange:    currentConf.MulticastIPRange,
			MulticastRTPPort:    currentConf.MulticastRTPPort,
			MulticastRTCPPort:   currentConf.MulticastRTCPPort,
			Encryption:          false,
			ServerCert:          "",
			ServerKey:           "",
			RTSPAddress:         currentConf.RTSPAddress,
			TrustedProxies:      currentConf.RTSPTrustedProxies,
			Transports:          currentConf.RTSPTransports,
			RunOnConnect:        currentConf.RunOnConnect,
			RunOnConnectRestart: currentConf.RunOnConnectRestart,
			RunOnDisconnect:     currentConf.RunOnDisconnect,
			ExternalCmdPool:     p.externalCmdPool,
			Metrics:             p.metrics,
			PathManager:         p.pathManager,
			Parent:              p,
		}
		err = i.Initialize()
		if err != nil {
			return err
		}
		p.rtspServer = i
	}

	if currentConf.RTSP &&
		(currentConf.RTSPEncryption == conf.EncryptionStrict ||
			currentConf.RTSPEncryption == conf.EncryptionOptional) &&
		p.rtspsServer == nil {
		udpReadBufferSize := currentConf.UDPReadBufferSize
		if currentConf.RTSPUDPReadBufferSize != nil {
			udpReadBufferSize = *currentConf.RTSPUDPReadBufferSize
		}

		i := &rtsp.Server{
			Address:             currentConf.RTSPSAddress,
			AuthMethods:         currentConf.RTSPAuthMethods.ToAuthMethods(),
			DumpPackets:         currentConf.DumpPackets,
			UDPReadBufferSize:   udpReadBufferSize,
			ReadTimeout:         currentConf.ReadTimeout,
			WriteTimeout:        currentConf.WriteTimeout,
			WriteQueueSize:      currentConf.WriteQueueSize,
			RTSPTransports:      currentConf.RTSPTransports,
			RTPAddress:          currentConf.SRTPAddress,
			RTCPAddress:         currentConf.SRTCPAddress,
			MulticastIPRange:    currentConf.MulticastIPRange,
			MulticastRTPPort:    currentConf.MulticastSRTPPort,
			MulticastRTCPPort:   currentConf.MulticastSRTCPPort,
			Encryption:          true,
			ServerCert:          currentConf.RTSPServerCert,
			ServerKey:           currentConf.RTSPServerKey,
			RTSPAddress:         currentConf.RTSPAddress,
			TrustedProxies:      currentConf.RTSPTrustedProxies,
			Transports:          currentConf.RTSPTransports,
			RunOnConnect:        currentConf.RunOnConnect,
			RunOnConnectRestart: currentConf.RunOnConnectRestart,
			RunOnDisconnect:     currentConf.RunOnDisconnect,
			ExternalCmdPool:     p.externalCmdPool,
			Metrics:             p.metrics,
			PathManager:         p.pathManager,
			Parent:              p,
		}
		err = i.Initialize()
		if err != nil {
			return err
		}
		p.rtspsServer = i
	}

	if currentConf.RTMP &&
		(currentConf.RTMPEncryption == conf.EncryptionNo ||
			currentConf.RTMPEncryption == conf.EncryptionOptional) &&
		p.rtmpServer == nil {
		i := &rtmp.Server{
			Address:             currentConf.RTMPAddress,
			DumpPackets:         currentConf.DumpPackets,
			ReadTimeout:         currentConf.ReadTimeout,
			WriteTimeout:        currentConf.WriteTimeout,
			Encryption:          false,
			ServerCert:          "",
			ServerKey:           "",
			RTSPAddress:         currentConf.RTSPAddress,
			TrustedProxies:      currentConf.RTMPTrustedProxies,
			RunOnConnect:        currentConf.RunOnConnect,
			RunOnConnectRestart: currentConf.RunOnConnectRestart,
			RunOnDisconnect:     currentConf.RunOnDisconnect,
			ExternalCmdPool:     p.externalCmdPool,
			Metrics:             p.metrics,
			PathManager:         p.pathManager,
			Parent:              p,
		}
		err = i.Initialize()
		if err != nil {
			return err
		}
		p.rtmpServer = i
	}

	if currentConf.RTMP &&
		(currentConf.RTMPEncryption == conf.EncryptionStrict ||
			currentConf.RTMPEncryption == conf.EncryptionOptional) &&
		p.rtmpsServer == nil {
		i := &rtmp.Server{
			Address:             currentConf.RTMPSAddress,
			ReadTimeout:         currentConf.ReadTimeout,
			WriteTimeout:        currentConf.WriteTimeout,
			Encryption:          true,
			ServerCert:          currentConf.RTMPServerCert,
			ServerKey:           currentConf.RTMPServerKey,
			DumpPackets:         currentConf.DumpPackets,
			RTSPAddress:         currentConf.RTSPAddress,
			TrustedProxies:      currentConf.RTMPTrustedProxies,
			RunOnConnect:        currentConf.RunOnConnect,
			RunOnConnectRestart: currentConf.RunOnConnectRestart,
			RunOnDisconnect:     currentConf.RunOnDisconnect,
			ExternalCmdPool:     p.externalCmdPool,
			Metrics:             p.metrics,
			PathManager:         p.pathManager,
			Parent:              p,
		}
		err = i.Initialize()
		if err != nil {
			return err
		}
		p.rtmpsServer = i
	}

	if currentConf.HLS &&
		p.hlsServer == nil {
		i := &hls.Server{
			Address:         currentConf.HLSAddress,
			DumpPackets:     currentConf.DumpPackets,
			Encryption:      currentConf.HLSEncryption,
			ServerKey:       currentConf.HLSServerKey,
			ServerCert:      currentConf.HLSServerCert,
			AllowOrigins:    currentConf.HLSAllowOrigins,
			TrustedProxies:  currentConf.HLSTrustedProxies,
			AlwaysRemux:     currentConf.HLSAlwaysRemux,
			Variant:         currentConf.HLSVariant,
			SegmentCount:    currentConf.HLSSegmentCount,
			SegmentDuration: currentConf.HLSSegmentDuration,
			PartDuration:    currentConf.HLSPartDuration,
			SegmentMaxSize:  currentConf.HLSSegmentMaxSize,
			Directory:       currentConf.HLSDirectory,
			CDNSecret:       currentConf.HLSCDNSecret,
			ReadTimeout:     currentConf.ReadTimeout,
			WriteTimeout:    currentConf.WriteTimeout,
			MuxerCloseAfter: currentConf.HLSMuxerCloseAfter,
			ExternalCmdPool: p.externalCmdPool,
			Metrics:         p.metrics,
			PathManager:     p.pathManager,
			Parent:          p,
		}
		err = i.Initialize()
		if err != nil {
			return err
		}
		p.hlsServer = i
	}

	if currentConf.WebRTC &&
		p.webRTCServer == nil {
		i := &webrtc.Server{
			Address:               currentConf.WebRTCAddress,
			DumpPackets:           currentConf.DumpPackets,
			Encryption:            currentConf.WebRTCEncryption,
			ServerKey:             currentConf.WebRTCServerKey,
			ServerCert:            currentConf.WebRTCServerCert,
			AllowOrigins:          currentConf.WebRTCAllowOrigins,
			TrustedProxies:        currentConf.WebRTCTrustedProxies,
			ReadTimeout:           currentConf.ReadTimeout,
			WriteTimeout:          currentConf.WriteTimeout,
			UDPReadBufferSize:     currentConf.UDPReadBufferSize,
			UDPWriteBufferSize:    currentConf.UDPWriteBufferSize,
			LocalUDPAddress:       currentConf.WebRTCLocalUDPAddress,
			LocalTCPAddress:       currentConf.WebRTCLocalTCPAddress,
			SupportsIPv6:          p.supportsIPv6,
			IPsFromInterfaces:     currentConf.WebRTCIPsFromInterfaces,
			IPsFromInterfacesList: currentConf.WebRTCIPsFromInterfacesList,
			AdditionalHosts:       currentConf.WebRTCAdditionalHosts,
			ICEServers:            currentConf.WebRTCICEServers2,
			STUNGatherTimeout:     currentConf.WebRTCSTUNGatherTimeout,
			HandshakeTimeout:      currentConf.WebRTCHandshakeTimeout,
			TrackGatherTimeout:    currentConf.WebRTCTrackGatherTimeout,
			ExternalCmdPool:       p.externalCmdPool,
			Metrics:               p.metrics,
			PathManager:           p.pathManager,
			Parent:                p,
		}
		err = i.Initialize()
		if err != nil {
			return err
		}
		p.webRTCServer = i
	}

	if currentConf.SRT &&
		p.srtServer == nil {
		i := &srt.Server{
			Address:             currentConf.SRTAddress,
			RTSPAddress:         currentConf.RTSPAddress,
			ReadTimeout:         currentConf.ReadTimeout,
			WriteTimeout:        currentConf.WriteTimeout,
			UDPMaxPayloadSize:   currentConf.UDPMaxPayloadSize,
			UDPReadBufferSize:   currentConf.UDPReadBufferSize,
			RunOnConnect:        currentConf.RunOnConnect,
			RunOnConnectRestart: currentConf.RunOnConnectRestart,
			RunOnDisconnect:     currentConf.RunOnDisconnect,
			ExternalCmdPool:     p.externalCmdPool,
			Metrics:             p.metrics,
			PathManager:         p.pathManager,
			Parent:              p,
		}
		err = i.Initialize()
		if err != nil {
			return err
		}
		p.srtServer = i
	}

	if currentConf.MoQ &&
		p.moqServer == nil {
		i := &moq.Server{
			HTTP2Address:      currentConf.MoQHTTP2Address,
			HTTP3Address:      currentConf.MoQHTTP3Address,
			QUICAddress:       currentConf.MoQQUICAddress,
			ServerKey:         currentConf.MoQServerKey,
			ServerCert:        currentConf.MoQServerCert,
			AllowOrigins:      currentConf.MoQAllowOrigins,
			TrustedProxies:    currentConf.MoQTrustedProxies,
			UDPReadBufferSize: currentConf.UDPReadBufferSize,
			ReadTimeout:       currentConf.ReadTimeout,
			WriteTimeout:      currentConf.WriteTimeout,
			PathManager:       p.pathManager,
			Metrics:           p.metrics,
			Parent:            p,
		}
		err = i.Initialize()
		if err != nil {
			return err
		}
		p.moqServer = i
	}

	if currentConf.API &&
		p.api == nil {
		i := &api.API{
			Version:        string(version),
			Started:        started,
			Address:        currentConf.APIAddress,
			DumpPackets:    currentConf.DumpPackets,
			Encryption:     currentConf.APIEncryption,
			ServerKey:      currentConf.APIServerKey,
			ServerCert:     currentConf.APIServerCert,
			AllowOrigins:   currentConf.APIAllowOrigins,
			TrustedProxies: currentConf.APITrustedProxies,
			ReadTimeout:    currentConf.ReadTimeout,
			WriteTimeout:   currentConf.WriteTimeout,
			AuthManager:    p.authManager,
			PathManager:    p.pathManager,
			RTSPServer:     p.rtspServer,
			RTSPSServer:    p.rtspsServer,
			RTMPServer:     p.rtmpServer,
			RTMPSServer:    p.rtmpsServer,
			HLSServer:      p.hlsServer,
			WebRTCServer:   p.webRTCServer,
			SRTServer:      p.srtServer,
			MoQServer:      p.moqServer,
			Parent:         p,
		}
		err = i.Initialize()
		if err != nil {
			return err
		}
		p.api = i
	}

	if initial && p.confPath != "" {
		cf := &confwatcher.ConfWatcher{FilePath: p.confPath}
		err = cf.Initialize()
		if err != nil {
			return err
		}
		p.confWatcher = cf
	}

	return nil
}

func (p *Core) closeResources(newConf *conf.Conf) {
	currentConf := p.conf.Load()

	closeLogger := newConf == nil ||
		newConf.LogLevel != currentConf.LogLevel ||
		!reflect.DeepEqual(newConf.LogDestinations, currentConf.LogDestinations) ||
		newConf.LogFile != currentConf.LogFile ||
		newConf.SysLogPrefix != currentConf.SysLogPrefix ||
		newConf.LogStructured != currentConf.LogStructured

	closeAuthManager := newConf == nil ||
		newConf.AuthMethod != currentConf.AuthMethod ||
		newConf.AuthHTTPAddress != currentConf.AuthHTTPAddress ||
		newConf.AuthHTTPFingerprint != currentConf.AuthHTTPFingerprint ||
		!reflect.DeepEqual(newConf.AuthHTTPExclude, currentConf.AuthHTTPExclude) ||
		newConf.AuthJWTJWKS != currentConf.AuthJWTJWKS ||
		newConf.AuthJWTJWKSFingerprint != currentConf.AuthJWTJWKSFingerprint ||
		newConf.AuthJWTClaimKey != currentConf.AuthJWTClaimKey ||
		!reflect.DeepEqual(newConf.AuthJWTExclude, currentConf.AuthJWTExclude) ||
		!reflect.DeepEqual(newConf.AuthJWTInHTTPQuery, currentConf.AuthJWTInHTTPQuery) ||
		newConf.AuthJWTIssuer != currentConf.AuthJWTIssuer ||
		newConf.AuthJWTAudience != currentConf.AuthJWTAudience ||
		newConf.ReadTimeout != currentConf.ReadTimeout
	if !closeAuthManager && !reflect.DeepEqual(newConf.AuthInternalUsers, currentConf.AuthInternalUsers) {
		p.authManager.ReloadInternalUsers(newConf.AuthInternalUsers)
	}

	closeMetrics := newConf == nil ||
		newConf.Metrics != currentConf.Metrics ||
		newConf.MetricsAddress != currentConf.MetricsAddress ||
		newConf.MetricsEncryption != currentConf.MetricsEncryption ||
		newConf.MetricsServerKey != currentConf.MetricsServerKey ||
		newConf.MetricsServerCert != currentConf.MetricsServerCert ||
		!slices.Equal(newConf.MetricsAllowOrigins, currentConf.MetricsAllowOrigins) ||
		!reflect.DeepEqual(newConf.MetricsTrustedProxies, currentConf.MetricsTrustedProxies) ||
		newConf.ReadTimeout != currentConf.ReadTimeout ||
		newConf.WriteTimeout != currentConf.WriteTimeout ||
		newConf.DumpPackets != currentConf.DumpPackets ||
		closeAuthManager ||
		closeLogger

	closePPROF := newConf == nil ||
		newConf.PPROF != currentConf.PPROF ||
		newConf.PPROFAddress != currentConf.PPROFAddress ||
		newConf.PPROFEncryption != currentConf.PPROFEncryption ||
		newConf.PPROFServerKey != currentConf.PPROFServerKey ||
		newConf.PPROFServerCert != currentConf.PPROFServerCert ||
		!slices.Equal(newConf.PPROFAllowOrigins, currentConf.PPROFAllowOrigins) ||
		!reflect.DeepEqual(newConf.PPROFTrustedProxies, currentConf.PPROFTrustedProxies) ||
		newConf.ReadTimeout != currentConf.ReadTimeout ||
		newConf.WriteTimeout != currentConf.WriteTimeout ||
		newConf.DumpPackets != currentConf.DumpPackets ||
		closeAuthManager ||
		closeLogger

	closeRecorderCleaner := newConf == nil ||
		atLeastOneRecordDeleteAfter(newConf.Paths) != atLeastOneRecordDeleteAfter(currentConf.Paths) ||
		closeLogger
	if !closeRecorderCleaner && p.recordCleaner != nil && !reflect.DeepEqual(newConf.Paths, currentConf.Paths) {
		p.recordCleaner.ReloadPathConfs(newConf.Paths)
	}

	closePlaybackServer := newConf == nil ||
		newConf.Playback != currentConf.Playback ||
		newConf.PlaybackAddress != currentConf.PlaybackAddress ||
		newConf.PlaybackEncryption != currentConf.PlaybackEncryption ||
		newConf.PlaybackServerKey != currentConf.PlaybackServerKey ||
		newConf.PlaybackServerCert != currentConf.PlaybackServerCert ||
		!slices.Equal(newConf.PlaybackAllowOrigins, currentConf.PlaybackAllowOrigins) ||
		!reflect.DeepEqual(newConf.PlaybackTrustedProxies, currentConf.PlaybackTrustedProxies) ||
		newConf.ReadTimeout != currentConf.ReadTimeout ||
		newConf.WriteTimeout != currentConf.WriteTimeout ||
		newConf.DumpPackets != currentConf.DumpPackets ||
		closeAuthManager ||
		closeLogger
	if !closePlaybackServer && p.playbackServer != nil && !reflect.DeepEqual(newConf.Paths, currentConf.Paths) {
		p.playbackServer.ReloadPathConfs(newConf.Paths)
	}

	closePathManager := newConf == nil ||
		newConf.LogLevel != currentConf.LogLevel ||
		newConf.DumpPackets != currentConf.DumpPackets ||
		newConf.RTSPAddress != currentConf.RTSPAddress ||
		newConf.ReadTimeout != currentConf.ReadTimeout ||
		newConf.WriteTimeout != currentConf.WriteTimeout ||
		newConf.WriteQueueSize != currentConf.WriteQueueSize ||
		newConf.UDPReadBufferSize != currentConf.UDPReadBufferSize ||
		newConf.UDPWriteBufferSize != currentConf.UDPWriteBufferSize ||
		newConf.UDPMaxPayloadSize != currentConf.UDPMaxPayloadSize ||
		newConf.RTSPEncryption != currentConf.RTSPEncryption ||
		closeMetrics ||
		closeAuthManager ||
		closeLogger
	if !closePathManager && !reflect.DeepEqual(newConf.Paths, currentConf.Paths) {
		p.pathManager.ReloadPathConfs(newConf.Paths)
	}

	closeRTSPServer := newConf == nil ||
		newConf.RTSP != currentConf.RTSP ||
		newConf.RTSPEncryption != currentConf.RTSPEncryption ||
		newConf.RTSPAddress != currentConf.RTSPAddress ||
		!reflect.DeepEqual(newConf.RTSPAuthMethods, currentConf.RTSPAuthMethods) ||
		newConf.RTSPUDPReadBufferSize != currentConf.RTSPUDPReadBufferSize ||
		newConf.DumpPackets != currentConf.DumpPackets ||
		newConf.UDPReadBufferSize != currentConf.UDPReadBufferSize ||
		newConf.ReadTimeout != currentConf.ReadTimeout ||
		newConf.WriteTimeout != currentConf.WriteTimeout ||
		newConf.WriteQueueSize != currentConf.WriteQueueSize ||
		newConf.RTPAddress != currentConf.RTPAddress ||
		newConf.RTCPAddress != currentConf.RTCPAddress ||
		newConf.MulticastIPRange != currentConf.MulticastIPRange ||
		newConf.MulticastRTPPort != currentConf.MulticastRTPPort ||
		newConf.MulticastRTCPPort != currentConf.MulticastRTCPPort ||
		!reflect.DeepEqual(newConf.RTSPTransports, currentConf.RTSPTransports) ||
		!reflect.DeepEqual(newConf.RTSPTrustedProxies, currentConf.RTSPTrustedProxies) ||
		newConf.RunOnConnect != currentConf.RunOnConnect ||
		newConf.RunOnConnectRestart != currentConf.RunOnConnectRestart ||
		newConf.RunOnDisconnect != currentConf.RunOnDisconnect ||
		closeMetrics ||
		closePathManager ||
		closeLogger

	closeRTSPSServer := newConf == nil ||
		newConf.RTSP != currentConf.RTSP ||
		newConf.RTSPEncryption != currentConf.RTSPEncryption ||
		newConf.RTSPSAddress != currentConf.RTSPSAddress ||
		!reflect.DeepEqual(newConf.RTSPAuthMethods, currentConf.RTSPAuthMethods) ||
		newConf.RTSPUDPReadBufferSize != currentConf.RTSPUDPReadBufferSize ||
		newConf.DumpPackets != currentConf.DumpPackets ||
		newConf.UDPReadBufferSize != currentConf.UDPReadBufferSize ||
		newConf.ReadTimeout != currentConf.ReadTimeout ||
		newConf.WriteTimeout != currentConf.WriteTimeout ||
		newConf.WriteQueueSize != currentConf.WriteQueueSize ||
		newConf.RTSPServerCert != currentConf.RTSPServerCert ||
		newConf.RTSPServerKey != currentConf.RTSPServerKey ||
		newConf.RTSPAddress != currentConf.RTSPAddress ||
		!reflect.DeepEqual(newConf.RTSPTransports, currentConf.RTSPTransports) ||
		!reflect.DeepEqual(newConf.RTSPTrustedProxies, currentConf.RTSPTrustedProxies) ||
		newConf.RunOnConnect != currentConf.RunOnConnect ||
		newConf.RunOnConnectRestart != currentConf.RunOnConnectRestart ||
		newConf.RunOnDisconnect != currentConf.RunOnDisconnect ||
		closeMetrics ||
		closePathManager ||
		closeLogger

	closeRTMPServer := newConf == nil ||
		newConf.RTMP != currentConf.RTMP ||
		newConf.RTMPEncryption != currentConf.RTMPEncryption ||
		newConf.RTMPAddress != currentConf.RTMPAddress ||
		newConf.DumpPackets != currentConf.DumpPackets ||
		newConf.ReadTimeout != currentConf.ReadTimeout ||
		newConf.WriteTimeout != currentConf.WriteTimeout ||
		newConf.RTSPAddress != currentConf.RTSPAddress ||
		!reflect.DeepEqual(newConf.RTMPTrustedProxies, currentConf.RTMPTrustedProxies) ||
		newConf.RunOnConnect != currentConf.RunOnConnect ||
		newConf.RunOnConnectRestart != currentConf.RunOnConnectRestart ||
		newConf.RunOnDisconnect != currentConf.RunOnDisconnect ||
		closeMetrics ||
		closePathManager ||
		closeLogger

	closeRTMPSServer := newConf == nil ||
		newConf.RTMP != currentConf.RTMP ||
		newConf.RTMPEncryption != currentConf.RTMPEncryption ||
		newConf.RTMPSAddress != currentConf.RTMPSAddress ||
		newConf.DumpPackets != currentConf.DumpPackets ||
		newConf.ReadTimeout != currentConf.ReadTimeout ||
		newConf.WriteTimeout != currentConf.WriteTimeout ||
		newConf.RTMPServerCert != currentConf.RTMPServerCert ||
		newConf.RTMPServerKey != currentConf.RTMPServerKey ||
		newConf.RTSPAddress != currentConf.RTSPAddress ||
		!reflect.DeepEqual(newConf.RTMPTrustedProxies, currentConf.RTMPTrustedProxies) ||
		newConf.RunOnConnect != currentConf.RunOnConnect ||
		newConf.RunOnConnectRestart != currentConf.RunOnConnectRestart ||
		newConf.RunOnDisconnect != currentConf.RunOnDisconnect ||
		closeMetrics ||
		closePathManager ||
		closeLogger

	closeHLSServer := newConf == nil ||
		newConf.HLS != currentConf.HLS ||
		newConf.HLSAddress != currentConf.HLSAddress ||
		newConf.HLSEncryption != currentConf.HLSEncryption ||
		newConf.HLSServerKey != currentConf.HLSServerKey ||
		newConf.HLSServerCert != currentConf.HLSServerCert ||
		!slices.Equal(newConf.HLSAllowOrigins, currentConf.HLSAllowOrigins) ||
		!reflect.DeepEqual(newConf.HLSTrustedProxies, currentConf.HLSTrustedProxies) ||
		newConf.HLSAlwaysRemux != currentConf.HLSAlwaysRemux ||
		newConf.HLSVariant != currentConf.HLSVariant ||
		newConf.HLSSegmentCount != currentConf.HLSSegmentCount ||
		newConf.HLSSegmentDuration != currentConf.HLSSegmentDuration ||
		newConf.HLSPartDuration != currentConf.HLSPartDuration ||
		newConf.HLSSegmentMaxSize != currentConf.HLSSegmentMaxSize ||
		newConf.HLSDirectory != currentConf.HLSDirectory ||
		newConf.ReadTimeout != currentConf.ReadTimeout ||
		newConf.WriteTimeout != currentConf.WriteTimeout ||
		newConf.HLSMuxerCloseAfter != currentConf.HLSMuxerCloseAfter ||
		newConf.HLSCDNSecret != currentConf.HLSCDNSecret ||
		newConf.DumpPackets != currentConf.DumpPackets ||
		closePathManager ||
		closeMetrics ||
		closeLogger

	closeWebRTCServer := newConf == nil ||
		newConf.WebRTC != currentConf.WebRTC ||
		newConf.WebRTCAddress != currentConf.WebRTCAddress ||
		newConf.WebRTCEncryption != currentConf.WebRTCEncryption ||
		newConf.WebRTCServerKey != currentConf.WebRTCServerKey ||
		newConf.WebRTCServerCert != currentConf.WebRTCServerCert ||
		!slices.Equal(newConf.WebRTCAllowOrigins, currentConf.WebRTCAllowOrigins) ||
		!reflect.DeepEqual(newConf.WebRTCTrustedProxies, currentConf.WebRTCTrustedProxies) ||
		newConf.ReadTimeout != currentConf.ReadTimeout ||
		newConf.WriteTimeout != currentConf.WriteTimeout ||
		newConf.UDPReadBufferSize != currentConf.UDPReadBufferSize ||
		newConf.UDPWriteBufferSize != currentConf.UDPWriteBufferSize ||
		newConf.WebRTCLocalUDPAddress != currentConf.WebRTCLocalUDPAddress ||
		newConf.WebRTCLocalTCPAddress != currentConf.WebRTCLocalTCPAddress ||
		newConf.WebRTCIPsFromInterfaces != currentConf.WebRTCIPsFromInterfaces ||
		!reflect.DeepEqual(newConf.WebRTCIPsFromInterfacesList, currentConf.WebRTCIPsFromInterfacesList) ||
		!reflect.DeepEqual(newConf.WebRTCAdditionalHosts, currentConf.WebRTCAdditionalHosts) ||
		!reflect.DeepEqual(newConf.WebRTCICEServers2, currentConf.WebRTCICEServers2) ||
		newConf.WebRTCSTUNGatherTimeout != currentConf.WebRTCSTUNGatherTimeout ||
		newConf.WebRTCHandshakeTimeout != currentConf.WebRTCHandshakeTimeout ||
		newConf.WebRTCTrackGatherTimeout != currentConf.WebRTCTrackGatherTimeout ||
		newConf.DumpPackets != currentConf.DumpPackets ||
		closeMetrics ||
		closePathManager ||
		closeLogger

	closeSRTServer := newConf == nil ||
		newConf.SRT != currentConf.SRT ||
		newConf.SRTAddress != currentConf.SRTAddress ||
		newConf.RTSPAddress != currentConf.RTSPAddress ||
		newConf.ReadTimeout != currentConf.ReadTimeout ||
		newConf.WriteTimeout != currentConf.WriteTimeout ||
		newConf.UDPMaxPayloadSize != currentConf.UDPMaxPayloadSize ||
		newConf.RunOnConnect != currentConf.RunOnConnect ||
		newConf.RunOnConnectRestart != currentConf.RunOnConnectRestart ||
		newConf.RunOnDisconnect != currentConf.RunOnDisconnect ||
		closeMetrics ||
		closePathManager ||
		closeLogger

	closeMoQServer := newConf == nil ||
		newConf.MoQ != currentConf.MoQ ||
		newConf.MoQHTTP2Address != currentConf.MoQHTTP2Address ||
		newConf.MoQHTTP3Address != currentConf.MoQHTTP3Address ||
		newConf.MoQQUICAddress != currentConf.MoQQUICAddress ||
		newConf.MoQServerKey != currentConf.MoQServerKey ||
		newConf.MoQServerCert != currentConf.MoQServerCert ||
		!slices.Equal(newConf.MoQAllowOrigins, currentConf.MoQAllowOrigins) ||
		!reflect.DeepEqual(newConf.MoQTrustedProxies, currentConf.MoQTrustedProxies) ||
		newConf.UDPReadBufferSize != currentConf.UDPReadBufferSize ||
		newConf.ReadTimeout != currentConf.ReadTimeout ||
		newConf.WriteTimeout != currentConf.WriteTimeout ||
		closeMetrics ||
		closePathManager ||
		closeLogger

	closeAPI := newConf == nil ||
		newConf.API != currentConf.API ||
		newConf.APIAddress != currentConf.APIAddress ||
		newConf.APIEncryption != currentConf.APIEncryption ||
		newConf.APIServerKey != currentConf.APIServerKey ||
		newConf.APIServerCert != currentConf.APIServerCert ||
		!slices.Equal(newConf.APIAllowOrigins, currentConf.APIAllowOrigins) ||
		!reflect.DeepEqual(newConf.APITrustedProxies, currentConf.APITrustedProxies) ||
		newConf.ReadTimeout != currentConf.ReadTimeout ||
		newConf.WriteTimeout != currentConf.WriteTimeout ||
		newConf.DumpPackets != currentConf.DumpPackets ||
		closeAuthManager ||
		closePathManager ||
		closeRTSPServer ||
		closeRTSPSServer ||
		closeRTMPServer ||
		closeRTMPSServer ||
		closeHLSServer ||
		closeWebRTCServer ||
		closeSRTServer ||
		closeMoQServer ||
		closeLogger

	if newConf == nil && p.confWatcher != nil {
		p.confWatcher.Close()
		p.confWatcher = nil
	}

	if p.api != nil {
		if closeAPI {
			p.api.Close()
			p.api = nil
		}
	}

	if closeSRTServer && p.srtServer != nil {
		p.srtServer.Close()
		p.srtServer = nil
	}

	if closeMoQServer && p.moqServer != nil {
		p.moqServer.Close()
		p.moqServer = nil
	}

	if closeWebRTCServer && p.webRTCServer != nil {
		p.webRTCServer.Close()
		p.webRTCServer = nil
	}

	if closeHLSServer && p.hlsServer != nil {
		p.hlsServer.Close()
		p.hlsServer = nil
	}

	if closeRTMPSServer && p.rtmpsServer != nil {
		p.rtmpsServer.Close()
		p.rtmpsServer = nil
	}

	if closeRTMPServer && p.rtmpServer != nil {
		p.rtmpServer.Close()
		p.rtmpServer = nil
	}

	if closeRTSPSServer && p.rtspsServer != nil {
		p.rtspsServer.Close()
		p.rtspsServer = nil
	}

	if closeRTSPServer && p.rtspServer != nil {
		p.rtspServer.Close()
		p.rtspServer = nil
	}

	if closePathManager && p.pathManager != nil {
		p.pathManager.close()
		p.pathManager = nil
	}

	if closePlaybackServer && p.playbackServer != nil {
		p.playbackServer.Close()
		p.playbackServer = nil
	}

	if closeRecorderCleaner && p.recordCleaner != nil {
		p.recordCleaner.Close()
		p.recordCleaner = nil
	}

	if closePPROF && p.pprof != nil {
		p.pprof.Close()
		p.pprof = nil
	}

	if closeMetrics && p.metrics != nil {
		p.metrics.Close()
		p.metrics = nil
	}

	if closeAuthManager && p.authManager != nil {
		p.authManager = nil
	}

	if newConf == nil && p.externalCmdPool != nil {
		p.Log(logger.Info, "waiting for running hooks")
		p.externalCmdPool.Close()
	}

	if closeLogger && p.logger != nil {
		if newConf == nil {
			p.logger.Close()
		}
		p.logger = nil
	}
}

func (p *Core) reloadConf(newConf *conf.Conf) error {
	oldLogger := p.logger

	p.closeResources(newConf)

	p.conf.Store(newConf)

	err := p.createResources(false)
	if err != nil {
		p.logger = oldLogger
		return err
	}

	if p.logger != oldLogger {
		oldLogger.Close()
	}

	return nil
}

func (p *Core) apiConfigSnapshot() *conf.Conf {
	return p.conf.Load()
}

func (p *Core) doAPIConfigGlobalPatch(in conf.OptionalGlobal) (*conf.Conf, error) {
	newConf := p.conf.Load().Clone()
	newConf.PatchGlobal(&in)

	if err := newConf.Validate(nil); err != nil {
		return nil, err
	}

	p.Log(logger.Info, "reloading configuration (API request)")
	return newConf, nil
}

func (p *Core) doAPIConfigPathDefaultsPatch(in conf.OptionalPath) (*conf.Conf, error) {
	newConf := p.conf.Load().Clone()
	newConf.PatchPathDefaults(&in)

	if err := newConf.Validate(nil); err != nil {
		return nil, err
	}

	p.Log(logger.Info, "reloading configuration (API request)")
	return newConf, nil
}

func (p *Core) doAPIConfigPathAdd(name string, in conf.OptionalPath) (*conf.Conf, error) {
	newConf := p.conf.Load().Clone()

	if err := newConf.AddPath(name, &in); err != nil {
		return nil, err
	}

	if err := newConf.Validate(nil); err != nil {
		return nil, err
	}

	p.Log(logger.Info, "reloading configuration (API request)")
	return newConf, nil
}

func (p *Core) doAPIConfigPathPatch(name string, in conf.OptionalPath) (*conf.Conf, error) {
	newConf := p.conf.Load().Clone()

	if err := newConf.PatchPath(name, &in); err != nil {
		return nil, err
	}

	if err := newConf.Validate(nil); err != nil {
		return nil, err
	}

	p.Log(logger.Info, "reloading configuration (API request)")
	return newConf, nil
}

func (p *Core) doAPIConfigPathReplace(name string, in conf.OptionalPath) (*conf.Conf, error) {
	newConf := p.conf.Load().Clone()

	if err := newConf.ReplacePath(name, &in); err != nil {
		return nil, err
	}

	if err := newConf.Validate(nil); err != nil {
		return nil, err
	}

	p.Log(logger.Info, "reloading configuration (API request)")
	return newConf, nil
}

func (p *Core) doAPIConfigPathDelete(name string) (*conf.Conf, error) {
	newConf := p.conf.Load().Clone()

	if err := newConf.RemovePath(name); err != nil {
		return nil, err
	}

	if err := newConf.Validate(nil); err != nil {
		return nil, err
	}

	p.Log(logger.Info, "reloading configuration (API request)")
	return newConf, nil
}

// APIConfigSnapshot implements apiParent.
func (p *Core) APIConfigSnapshot() *conf.Conf {
	return p.apiConfigSnapshot()
}

// APIConfigGlobalPatch implements apiParent.
func (p *Core) APIConfigGlobalPatch(in conf.OptionalGlobal) error {
	res := make(chan error)
	select {
	case p.chAPIConfigGlobalPatch <- configGlobalPatchReq{conf: in, res: res}:
		return <-res
	case <-p.ctx.Done():
		return fmt.Errorf("terminated")
	}
}

// APIConfigPathDefaultsPatch implements apiParent.
func (p *Core) APIConfigPathDefaultsPatch(in conf.OptionalPath) error {
	res := make(chan error)
	select {
	case p.chAPIConfigPathDefaultsPatch <- configPathDefaultsPatchReq{conf: in, res: res}:
		return <-res
	case <-p.ctx.Done():
		return fmt.Errorf("terminated")
	}
}

// APIConfigPathsAdd implements apiParent.
func (p *Core) APIConfigPathsAdd(name string, in conf.OptionalPath) error {
	res := make(chan error)
	select {
	case p.chAPIConfigPathAdd <- configPathAddReq{name: name, conf: in, res: res}:
		return <-res
	case <-p.ctx.Done():
		return fmt.Errorf("terminated")
	}
}

// APIConfigPathsPatch implements apiParent.
func (p *Core) APIConfigPathsPatch(name string, in conf.OptionalPath) error {
	res := make(chan error)
	select {
	case p.chAPIConfigPathPatch <- configPathPatchReq{name: name, conf: in, res: res}:
		return <-res
	case <-p.ctx.Done():
		return fmt.Errorf("terminated")
	}
}

// APIConfigPathsReplace implements apiParent.
func (p *Core) APIConfigPathsReplace(name string, in conf.OptionalPath) error {
	res := make(chan error)
	select {
	case p.chAPIConfigPathReplace <- configPathReplaceReq{name: name, conf: in, res: res}:
		return <-res
	case <-p.ctx.Done():
		return fmt.Errorf("terminated")
	}
}

// APIConfigPathsDelete implements apiParent.
func (p *Core) APIConfigPathsDelete(name string) error {
	res := make(chan error)
	select {
	case p.chAPIConfigPathDelete <- configPathDeleteReq{name: name, res: res}:
		return <-res
	case <-p.ctx.Done():
		return fmt.Errorf("terminated")
	}
}
