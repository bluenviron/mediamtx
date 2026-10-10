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
	"sync"
	"syscall"
	"time"

	"github.com/alecthomas/kong"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"

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

// matchInternalUserIDs returns the IDs of newUsers. Users are matched by
// configuration and not by position, therefore a user that is unchanged,
// added, removed or moved keeps its ID, and only new users get a new one.
func matchInternalUserIDs(
	oldUsers []conf.AuthInternalUser,
	oldIDs []uuid.UUID,
	newUsers []conf.AuthInternalUser,
) []uuid.UUID {
	reused := make([]bool, len(oldUsers))
	ids := make([]uuid.UUID, len(newUsers))

	for i, user := range newUsers {
		for j, old := range oldUsers {
			if !reused[j] && j < len(oldIDs) && reflect.DeepEqual(old, user) {
				reused[j] = true
				ids[i] = oldIDs[j]
				break
			}
		}

		if ids[i] == uuid.Nil {
			ids[i] = uuid.New()
		}
	}

	return ids
}

var cli struct {
	Confpath     string `arg:"" default:""`
	Version      bool   `help:"print version"`
	CheckVersion bool   `help:"check whether a new version is available"`
	Upgrade      bool   `help:"upgrade executable to the latest version"`
	ValidateConf string `help:"check whether a configuration file is valid" placeholder:"path"`
}

// Core is an instance of MediaMTX.
type Core struct {
	ctx             context.Context
	ctxCancel       func()
	confPath        string
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

	confMutex       sync.RWMutex
	conf            *conf.Conf
	internalUserIDs []uuid.UUID

	// in
	chAPIConfigGlobalPatch         chan configGlobalPatchReq
	chAPIConfigPathDefaultsPatch   chan configPathDefaultsPatchReq
	chAPIConfigPathAdd             chan configPathAddReq
	chAPIConfigPathPatch           chan configPathPatchReq
	chAPIConfigPathReplace         chan configPathReplaceReq
	chAPIConfigPathDelete          chan configPathDeleteReq
	chAPIConfigInternalUserAdd     chan configInternalUserReq
	chAPIConfigInternalUserPatch   chan configInternalUserReq
	chAPIConfigInternalUserReplace chan configInternalUserReq
	chAPIConfigInternalUserDelete  chan configInternalUserReq

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
		ctx:                            ctx,
		ctxCancel:                      ctxCancel,
		chAPIConfigGlobalPatch:         make(chan configGlobalPatchReq),
		chAPIConfigPathDefaultsPatch:   make(chan configPathDefaultsPatchReq),
		chAPIConfigPathAdd:             make(chan configPathAddReq),
		chAPIConfigPathPatch:           make(chan configPathPatchReq),
		chAPIConfigPathReplace:         make(chan configPathReplaceReq),
		chAPIConfigPathDelete:          make(chan configPathDeleteReq),
		chAPIConfigInternalUserAdd:     make(chan configInternalUserReq),
		chAPIConfigInternalUserPatch:   make(chan configInternalUserReq),
		chAPIConfigInternalUserReplace: make(chan configInternalUserReq),
		chAPIConfigInternalUserDelete:  make(chan configInternalUserReq),
		done:                           make(chan struct{}),
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
	p.conf = loadedConf
	p.internalUserIDs = newInternalUserIDs(len(loadedConf.AuthInternalUsers))

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

			// reply before reloading, since the reload might cause the API server to close.
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

			var fatalErr error
			if err == nil {
				fatalErr = p.reloadConf(newConf)
				err = fatalErr
			}

			req.res <- err

			if fatalErr != nil {
				p.Log(logger.Error, "%s", fatalErr)
				break outer
			}

		case req := <-p.chAPIConfigPathAdd:
			newConf, err := p.doAPIConfigPathAdd(req.name, req.conf)

			var fatalErr error
			if err == nil {
				fatalErr = p.reloadConf(newConf)
				err = fatalErr
			}

			req.res <- err

			if fatalErr != nil {
				p.Log(logger.Error, "%s", fatalErr)
				break outer
			}

		case req := <-p.chAPIConfigPathPatch:
			newConf, err := p.doAPIConfigPathPatch(req.name, req.conf)

			var fatalErr error
			if err == nil {
				fatalErr = p.reloadConf(newConf)
				err = fatalErr
			}

			req.res <- err

			if fatalErr != nil {
				p.Log(logger.Error, "%s", fatalErr)
				break outer
			}

		case req := <-p.chAPIConfigPathReplace:
			newConf, err := p.doAPIConfigPathReplace(req.name, req.conf)

			var fatalErr error
			if err == nil {
				fatalErr = p.reloadConf(newConf)
				err = fatalErr
			}

			req.res <- err

			if fatalErr != nil {
				p.Log(logger.Error, "%s", fatalErr)
				break outer
			}

		case req := <-p.chAPIConfigPathDelete:
			newConf, err := p.doAPIConfigPathDelete(req.name)

			var fatalErr error
			if err == nil {
				fatalErr = p.reloadConf(newConf)
				err = fatalErr
			}

			req.res <- err

			if fatalErr != nil {
				p.Log(logger.Error, "%s", fatalErr)
				break outer
			}

		case req := <-p.chAPIConfigInternalUserAdd:
			newConf, newIDs, id, err := p.doAPIConfigInternalUserAdd(req)

			if err == nil {
				p.reloadInternalUsers(newConf, newIDs)
			}

			req.res <- configInternalUserRes{id: id, err: err}

		case req := <-p.chAPIConfigInternalUserPatch:
			newConf, newIDs, id, err := p.doAPIConfigInternalUserPatch(req)

			if err == nil {
				p.reloadInternalUsers(newConf, newIDs)
			}

			req.res <- configInternalUserRes{id: id, err: err}

		case req := <-p.chAPIConfigInternalUserReplace:
			newConf, newIDs, id, err := p.doAPIConfigInternalUserReplace(req)

			if err == nil {
				p.reloadInternalUsers(newConf, newIDs)
			}

			req.res <- configInternalUserRes{id: id, err: err}

		case req := <-p.chAPIConfigInternalUserDelete:
			newConf, newIDs, id, err := p.doAPIConfigInternalUserDelete(req)

			if err == nil {
				p.reloadInternalUsers(newConf, newIDs)
			}

			req.res <- configInternalUserRes{id: id, err: err}

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
	var err error

	if p.logger == nil {
		i := &logger.Logger{
			Level:        logger.Level(p.conf.LogLevel),
			Destinations: p.conf.LogDestinations.ToDestinations(),
			Structured:   p.conf.LogStructured,
			File:         p.conf.LogFile,
			SysLogPrefix: p.conf.SysLogPrefix,
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
			Method:             p.conf.AuthMethod,
			InternalUsers:      p.conf.AuthInternalUsers,
			HTTPAddress:        p.conf.AuthHTTPAddress,
			HTTPFingerprint:    p.conf.AuthHTTPFingerprint,
			HTTPExclude:        p.conf.AuthHTTPExclude,
			JWTJWKS:            p.conf.AuthJWTJWKS,
			JWTJWKSFingerprint: p.conf.AuthJWTJWKSFingerprint,
			JWTClaimKey:        p.conf.AuthJWTClaimKey,
			JWTExclude:         p.conf.AuthJWTExclude,
			JWTInHTTPQuery:     p.conf.AuthJWTInHTTPQuery,
			JWTIssuer:          p.conf.AuthJWTIssuer,
			JWTAudience:        p.conf.AuthJWTAudience,
			ReadTimeout:        time.Duration(p.conf.ReadTimeout),
			Parent:             p,
		}
		p.authManager.Initialize()
	}

	if p.conf.Metrics &&
		p.metrics == nil {
		i := &metrics.Metrics{
			Address:        p.conf.MetricsAddress,
			DumpPackets:    p.conf.DumpPackets,
			Encryption:     p.conf.MetricsEncryption,
			ServerKey:      p.conf.MetricsServerKey,
			ServerCert:     p.conf.MetricsServerCert,
			AllowOrigins:   p.conf.MetricsAllowOrigins,
			TrustedProxies: p.conf.MetricsTrustedProxies,
			ReadTimeout:    p.conf.ReadTimeout,
			WriteTimeout:   p.conf.WriteTimeout,
			AuthManager:    p.authManager,
			Parent:         p,
		}
		err = i.Initialize()
		if err != nil {
			return err
		}
		p.metrics = i
	}

	if p.conf.PPROF &&
		p.pprof == nil {
		i := &pprof.PPROF{
			Address:        p.conf.PPROFAddress,
			DumpPackets:    p.conf.DumpPackets,
			Encryption:     p.conf.PPROFEncryption,
			ServerKey:      p.conf.PPROFServerKey,
			ServerCert:     p.conf.PPROFServerCert,
			AllowOrigins:   p.conf.PPROFAllowOrigins,
			TrustedProxies: p.conf.PPROFTrustedProxies,
			ReadTimeout:    p.conf.ReadTimeout,
			WriteTimeout:   p.conf.WriteTimeout,
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
		atLeastOneRecordDeleteAfter(p.conf.Paths) {
		p.recordCleaner = &recordcleaner.Cleaner{
			PathConfs: p.conf.Paths,
			Parent:    p,
		}
		p.recordCleaner.Initialize()
	}

	if p.conf.Playback &&
		p.playbackServer == nil {
		i := &playback.Server{
			Address:        p.conf.PlaybackAddress,
			DumpPackets:    p.conf.DumpPackets,
			Encryption:     p.conf.PlaybackEncryption,
			ServerKey:      p.conf.PlaybackServerKey,
			ServerCert:     p.conf.PlaybackServerCert,
			AllowOrigins:   p.conf.PlaybackAllowOrigins,
			TrustedProxies: p.conf.PlaybackTrustedProxies,
			ReadTimeout:    p.conf.ReadTimeout,
			WriteTimeout:   p.conf.WriteTimeout,
			PathConfs:      p.conf.Paths,
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
		rtpMaxPayloadSize := getRTPMaxPayloadSize(p.conf.UDPMaxPayloadSize, p.conf.RTSPEncryption)

		p.pathManager = &pathManager{
			logLevel:          p.conf.LogLevel,
			dumpPackets:       p.conf.DumpPackets,
			rtspAddress:       p.conf.RTSPAddress,
			readTimeout:       p.conf.ReadTimeout,
			writeTimeout:      p.conf.WriteTimeout,
			writeQueueSize:    p.conf.WriteQueueSize,
			udpReadBufferSize: p.conf.UDPReadBufferSize,
			udpMaxPayloadSize: p.conf.UDPMaxPayloadSize,
			rtpMaxPayloadSize: rtpMaxPayloadSize,
			supportsIPv6:      p.supportsIPv6,
			pathConfs:         p.conf.Paths,
			authManager:       p.authManager,
			externalCmdPool:   p.externalCmdPool,
			metrics:           p.metrics,
			parent:            p,
		}
		p.pathManager.initialize()
	}

	if p.conf.RTSP &&
		(p.conf.RTSPEncryption == conf.EncryptionNo ||
			p.conf.RTSPEncryption == conf.EncryptionOptional) &&
		p.rtspServer == nil {
		udpReadBufferSize := p.conf.UDPReadBufferSize
		if p.conf.RTSPUDPReadBufferSize != nil {
			udpReadBufferSize = *p.conf.RTSPUDPReadBufferSize
		}

		i := &rtsp.Server{
			Address:             p.conf.RTSPAddress,
			AuthMethods:         p.conf.RTSPAuthMethods.ToAuthMethods(),
			DumpPackets:         p.conf.DumpPackets,
			UDPReadBufferSize:   udpReadBufferSize,
			ReadTimeout:         p.conf.ReadTimeout,
			WriteTimeout:        p.conf.WriteTimeout,
			WriteQueueSize:      p.conf.WriteQueueSize,
			RTSPTransports:      p.conf.RTSPTransports,
			RTPAddress:          p.conf.RTPAddress,
			RTCPAddress:         p.conf.RTCPAddress,
			MulticastIPRange:    p.conf.MulticastIPRange,
			MulticastRTPPort:    p.conf.MulticastRTPPort,
			MulticastRTCPPort:   p.conf.MulticastRTCPPort,
			Encryption:          false,
			ServerCert:          "",
			ServerKey:           "",
			RTSPAddress:         p.conf.RTSPAddress,
			TrustedProxies:      p.conf.RTSPTrustedProxies,
			Transports:          p.conf.RTSPTransports,
			RunOnConnect:        p.conf.RunOnConnect,
			RunOnConnectRestart: p.conf.RunOnConnectRestart,
			RunOnDisconnect:     p.conf.RunOnDisconnect,
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

	if p.conf.RTSP &&
		(p.conf.RTSPEncryption == conf.EncryptionStrict ||
			p.conf.RTSPEncryption == conf.EncryptionOptional) &&
		p.rtspsServer == nil {
		udpReadBufferSize := p.conf.UDPReadBufferSize
		if p.conf.RTSPUDPReadBufferSize != nil {
			udpReadBufferSize = *p.conf.RTSPUDPReadBufferSize
		}

		i := &rtsp.Server{
			Address:             p.conf.RTSPSAddress,
			AuthMethods:         p.conf.RTSPAuthMethods.ToAuthMethods(),
			DumpPackets:         p.conf.DumpPackets,
			UDPReadBufferSize:   udpReadBufferSize,
			ReadTimeout:         p.conf.ReadTimeout,
			WriteTimeout:        p.conf.WriteTimeout,
			WriteQueueSize:      p.conf.WriteQueueSize,
			RTSPTransports:      p.conf.RTSPTransports,
			RTPAddress:          p.conf.SRTPAddress,
			RTCPAddress:         p.conf.SRTCPAddress,
			MulticastIPRange:    p.conf.MulticastIPRange,
			MulticastRTPPort:    p.conf.MulticastSRTPPort,
			MulticastRTCPPort:   p.conf.MulticastSRTCPPort,
			Encryption:          true,
			ServerCert:          p.conf.RTSPServerCert,
			ServerKey:           p.conf.RTSPServerKey,
			RTSPAddress:         p.conf.RTSPAddress,
			TrustedProxies:      p.conf.RTSPTrustedProxies,
			Transports:          p.conf.RTSPTransports,
			RunOnConnect:        p.conf.RunOnConnect,
			RunOnConnectRestart: p.conf.RunOnConnectRestart,
			RunOnDisconnect:     p.conf.RunOnDisconnect,
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

	if p.conf.RTMP &&
		(p.conf.RTMPEncryption == conf.EncryptionNo ||
			p.conf.RTMPEncryption == conf.EncryptionOptional) &&
		p.rtmpServer == nil {
		i := &rtmp.Server{
			Address:             p.conf.RTMPAddress,
			DumpPackets:         p.conf.DumpPackets,
			ReadTimeout:         p.conf.ReadTimeout,
			WriteTimeout:        p.conf.WriteTimeout,
			Encryption:          false,
			ServerCert:          "",
			ServerKey:           "",
			RTSPAddress:         p.conf.RTSPAddress,
			TrustedProxies:      p.conf.RTMPTrustedProxies,
			RunOnConnect:        p.conf.RunOnConnect,
			RunOnConnectRestart: p.conf.RunOnConnectRestart,
			RunOnDisconnect:     p.conf.RunOnDisconnect,
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

	if p.conf.RTMP &&
		(p.conf.RTMPEncryption == conf.EncryptionStrict ||
			p.conf.RTMPEncryption == conf.EncryptionOptional) &&
		p.rtmpsServer == nil {
		i := &rtmp.Server{
			Address:             p.conf.RTMPSAddress,
			ReadTimeout:         p.conf.ReadTimeout,
			WriteTimeout:        p.conf.WriteTimeout,
			Encryption:          true,
			ServerCert:          p.conf.RTMPServerCert,
			ServerKey:           p.conf.RTMPServerKey,
			DumpPackets:         p.conf.DumpPackets,
			RTSPAddress:         p.conf.RTSPAddress,
			TrustedProxies:      p.conf.RTMPTrustedProxies,
			RunOnConnect:        p.conf.RunOnConnect,
			RunOnConnectRestart: p.conf.RunOnConnectRestart,
			RunOnDisconnect:     p.conf.RunOnDisconnect,
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

	if p.conf.HLS &&
		p.hlsServer == nil {
		i := &hls.Server{
			Address:         p.conf.HLSAddress,
			DumpPackets:     p.conf.DumpPackets,
			Encryption:      p.conf.HLSEncryption,
			ServerKey:       p.conf.HLSServerKey,
			ServerCert:      p.conf.HLSServerCert,
			AllowOrigins:    p.conf.HLSAllowOrigins,
			TrustedProxies:  p.conf.HLSTrustedProxies,
			AlwaysRemux:     p.conf.HLSAlwaysRemux,
			Variant:         p.conf.HLSVariant,
			SegmentCount:    p.conf.HLSSegmentCount,
			SegmentDuration: p.conf.HLSSegmentDuration,
			PartDuration:    p.conf.HLSPartDuration,
			SegmentMaxSize:  p.conf.HLSSegmentMaxSize,
			Directory:       p.conf.HLSDirectory,
			CDNSecret:       p.conf.HLSCDNSecret,
			ReadTimeout:     p.conf.ReadTimeout,
			WriteTimeout:    p.conf.WriteTimeout,
			MuxerCloseAfter: p.conf.HLSMuxerCloseAfter,
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

	if p.conf.WebRTC &&
		p.webRTCServer == nil {
		i := &webrtc.Server{
			Address:                      p.conf.WebRTCAddress,
			DumpPackets:                  p.conf.DumpPackets,
			Encryption:                   p.conf.WebRTCEncryption,
			ServerKey:                    p.conf.WebRTCServerKey,
			ServerCert:                   p.conf.WebRTCServerCert,
			AllowOrigins:                 p.conf.WebRTCAllowOrigins,
			TrustedProxies:               p.conf.WebRTCTrustedProxies,
			ReadTimeout:                  p.conf.ReadTimeout,
			WriteTimeout:                 p.conf.WriteTimeout,
			UDPReadBufferSize:            p.conf.UDPReadBufferSize,
			LocalUDPAddress:              p.conf.WebRTCLocalUDPAddress,
			LocalTCPAddress:              p.conf.WebRTCLocalTCPAddress,
			SupportsIPv6:                 p.supportsIPv6,
			IPsFromInterfaces:            p.conf.WebRTCIPsFromInterfaces,
			IPsFromInterfacesList:        p.conf.WebRTCIPsFromInterfacesList,
			IPsFromInterfacesExcludeList: p.conf.WebRTCIPsFromInterfacesExcludeList,
			AdditionalHosts:              p.conf.WebRTCAdditionalHosts,
			ICEServers:                   p.conf.WebRTCICEServers2,
			STUNGatherTimeout:            p.conf.WebRTCSTUNGatherTimeout,
			HandshakeTimeout:             p.conf.WebRTCHandshakeTimeout,
			TrackGatherTimeout:           p.conf.WebRTCTrackGatherTimeout,
			ExternalCmdPool:              p.externalCmdPool,
			Metrics:                      p.metrics,
			PathManager:                  p.pathManager,
			Parent:                       p,
		}
		err = i.Initialize()
		if err != nil {
			return err
		}
		p.webRTCServer = i
	}

	if p.conf.SRT &&
		p.srtServer == nil {
		i := &srt.Server{
			Address:             p.conf.SRTAddress,
			RTSPAddress:         p.conf.RTSPAddress,
			ReadTimeout:         p.conf.ReadTimeout,
			WriteTimeout:        p.conf.WriteTimeout,
			UDPMaxPayloadSize:   p.conf.UDPMaxPayloadSize,
			UDPReadBufferSize:   p.conf.UDPReadBufferSize,
			RunOnConnect:        p.conf.RunOnConnect,
			RunOnConnectRestart: p.conf.RunOnConnectRestart,
			RunOnDisconnect:     p.conf.RunOnDisconnect,
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

	if p.conf.MoQ &&
		p.moqServer == nil {
		i := &moq.Server{
			HTTP2Address:      p.conf.MoQHTTP2Address,
			HTTP3Address:      p.conf.MoQHTTP3Address,
			QUICAddress:       p.conf.MoQQUICAddress,
			ServerKey:         p.conf.MoQServerKey,
			ServerCert:        p.conf.MoQServerCert,
			AllowOrigins:      p.conf.MoQAllowOrigins,
			TrustedProxies:    p.conf.MoQTrustedProxies,
			UDPReadBufferSize: p.conf.UDPReadBufferSize,
			ReadTimeout:       p.conf.ReadTimeout,
			WriteTimeout:      p.conf.WriteTimeout,
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

	if p.conf.API &&
		p.api == nil {
		i := &api.API{
			Version:        string(version),
			Started:        started,
			Address:        p.conf.APIAddress,
			DumpPackets:    p.conf.DumpPackets,
			Encryption:     p.conf.APIEncryption,
			ServerKey:      p.conf.APIServerKey,
			ServerCert:     p.conf.APIServerCert,
			AllowOrigins:   p.conf.APIAllowOrigins,
			TrustedProxies: p.conf.APITrustedProxies,
			ReadTimeout:    p.conf.ReadTimeout,
			WriteTimeout:   p.conf.WriteTimeout,
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
	closeLogger := newConf == nil ||
		newConf.LogLevel != p.conf.LogLevel ||
		!reflect.DeepEqual(newConf.LogDestinations, p.conf.LogDestinations) ||
		newConf.LogFile != p.conf.LogFile ||
		newConf.SysLogPrefix != p.conf.SysLogPrefix ||
		newConf.LogStructured != p.conf.LogStructured

	closeAuthManager := newConf == nil ||
		newConf.AuthMethod != p.conf.AuthMethod ||
		newConf.AuthHTTPAddress != p.conf.AuthHTTPAddress ||
		newConf.AuthHTTPFingerprint != p.conf.AuthHTTPFingerprint ||
		!reflect.DeepEqual(newConf.AuthHTTPExclude, p.conf.AuthHTTPExclude) ||
		newConf.AuthJWTJWKS != p.conf.AuthJWTJWKS ||
		newConf.AuthJWTJWKSFingerprint != p.conf.AuthJWTJWKSFingerprint ||
		newConf.AuthJWTClaimKey != p.conf.AuthJWTClaimKey ||
		!reflect.DeepEqual(newConf.AuthJWTExclude, p.conf.AuthJWTExclude) ||
		!reflect.DeepEqual(newConf.AuthJWTInHTTPQuery, p.conf.AuthJWTInHTTPQuery) ||
		newConf.AuthJWTIssuer != p.conf.AuthJWTIssuer ||
		newConf.AuthJWTAudience != p.conf.AuthJWTAudience ||
		newConf.ReadTimeout != p.conf.ReadTimeout
	if !closeAuthManager && !reflect.DeepEqual(newConf.AuthInternalUsers, p.conf.AuthInternalUsers) {
		p.authManager.ReloadInternalUsers(newConf.AuthInternalUsers)
	}

	closeMetrics := newConf == nil ||
		newConf.Metrics != p.conf.Metrics ||
		newConf.MetricsAddress != p.conf.MetricsAddress ||
		newConf.MetricsEncryption != p.conf.MetricsEncryption ||
		newConf.MetricsServerKey != p.conf.MetricsServerKey ||
		newConf.MetricsServerCert != p.conf.MetricsServerCert ||
		!slices.Equal(newConf.MetricsAllowOrigins, p.conf.MetricsAllowOrigins) ||
		!reflect.DeepEqual(newConf.MetricsTrustedProxies, p.conf.MetricsTrustedProxies) ||
		newConf.ReadTimeout != p.conf.ReadTimeout ||
		newConf.WriteTimeout != p.conf.WriteTimeout ||
		newConf.DumpPackets != p.conf.DumpPackets ||
		closeAuthManager ||
		closeLogger

	closePPROF := newConf == nil ||
		newConf.PPROF != p.conf.PPROF ||
		newConf.PPROFAddress != p.conf.PPROFAddress ||
		newConf.PPROFEncryption != p.conf.PPROFEncryption ||
		newConf.PPROFServerKey != p.conf.PPROFServerKey ||
		newConf.PPROFServerCert != p.conf.PPROFServerCert ||
		!slices.Equal(newConf.PPROFAllowOrigins, p.conf.PPROFAllowOrigins) ||
		!reflect.DeepEqual(newConf.PPROFTrustedProxies, p.conf.PPROFTrustedProxies) ||
		newConf.ReadTimeout != p.conf.ReadTimeout ||
		newConf.WriteTimeout != p.conf.WriteTimeout ||
		newConf.DumpPackets != p.conf.DumpPackets ||
		closeAuthManager ||
		closeLogger

	closeRecorderCleaner := newConf == nil ||
		atLeastOneRecordDeleteAfter(newConf.Paths) != atLeastOneRecordDeleteAfter(p.conf.Paths) ||
		closeLogger
	if !closeRecorderCleaner && p.recordCleaner != nil && !reflect.DeepEqual(newConf.Paths, p.conf.Paths) {
		p.recordCleaner.ReloadPathConfs(newConf.Paths)
	}

	closePlaybackServer := newConf == nil ||
		newConf.Playback != p.conf.Playback ||
		newConf.PlaybackAddress != p.conf.PlaybackAddress ||
		newConf.PlaybackEncryption != p.conf.PlaybackEncryption ||
		newConf.PlaybackServerKey != p.conf.PlaybackServerKey ||
		newConf.PlaybackServerCert != p.conf.PlaybackServerCert ||
		!slices.Equal(newConf.PlaybackAllowOrigins, p.conf.PlaybackAllowOrigins) ||
		!reflect.DeepEqual(newConf.PlaybackTrustedProxies, p.conf.PlaybackTrustedProxies) ||
		newConf.ReadTimeout != p.conf.ReadTimeout ||
		newConf.WriteTimeout != p.conf.WriteTimeout ||
		newConf.DumpPackets != p.conf.DumpPackets ||
		closeAuthManager ||
		closeLogger
	if !closePlaybackServer && p.playbackServer != nil && !reflect.DeepEqual(newConf.Paths, p.conf.Paths) {
		p.playbackServer.ReloadPathConfs(newConf.Paths)
	}

	closePathManager := newConf == nil ||
		newConf.LogLevel != p.conf.LogLevel ||
		newConf.DumpPackets != p.conf.DumpPackets ||
		newConf.RTSPAddress != p.conf.RTSPAddress ||
		newConf.ReadTimeout != p.conf.ReadTimeout ||
		newConf.WriteTimeout != p.conf.WriteTimeout ||
		newConf.WriteQueueSize != p.conf.WriteQueueSize ||
		newConf.UDPReadBufferSize != p.conf.UDPReadBufferSize ||
		newConf.UDPMaxPayloadSize != p.conf.UDPMaxPayloadSize ||
		newConf.RTSPEncryption != p.conf.RTSPEncryption ||
		closeMetrics ||
		closeAuthManager ||
		closeLogger
	if !closePathManager && !reflect.DeepEqual(newConf.Paths, p.conf.Paths) {
		p.pathManager.ReloadPathConfs(newConf.Paths)
	}

	closeRTSPServer := newConf == nil ||
		newConf.RTSP != p.conf.RTSP ||
		newConf.RTSPEncryption != p.conf.RTSPEncryption ||
		newConf.RTSPAddress != p.conf.RTSPAddress ||
		!reflect.DeepEqual(newConf.RTSPAuthMethods, p.conf.RTSPAuthMethods) ||
		newConf.RTSPUDPReadBufferSize != p.conf.RTSPUDPReadBufferSize ||
		newConf.DumpPackets != p.conf.DumpPackets ||
		newConf.UDPReadBufferSize != p.conf.UDPReadBufferSize ||
		newConf.ReadTimeout != p.conf.ReadTimeout ||
		newConf.WriteTimeout != p.conf.WriteTimeout ||
		newConf.WriteQueueSize != p.conf.WriteQueueSize ||
		newConf.RTPAddress != p.conf.RTPAddress ||
		newConf.RTCPAddress != p.conf.RTCPAddress ||
		newConf.MulticastIPRange != p.conf.MulticastIPRange ||
		newConf.MulticastRTPPort != p.conf.MulticastRTPPort ||
		newConf.MulticastRTCPPort != p.conf.MulticastRTCPPort ||
		!reflect.DeepEqual(newConf.RTSPTransports, p.conf.RTSPTransports) ||
		!reflect.DeepEqual(newConf.RTSPTrustedProxies, p.conf.RTSPTrustedProxies) ||
		newConf.RunOnConnect != p.conf.RunOnConnect ||
		newConf.RunOnConnectRestart != p.conf.RunOnConnectRestart ||
		newConf.RunOnDisconnect != p.conf.RunOnDisconnect ||
		closeMetrics ||
		closePathManager ||
		closeLogger

	closeRTSPSServer := newConf == nil ||
		newConf.RTSP != p.conf.RTSP ||
		newConf.RTSPEncryption != p.conf.RTSPEncryption ||
		newConf.RTSPSAddress != p.conf.RTSPSAddress ||
		!reflect.DeepEqual(newConf.RTSPAuthMethods, p.conf.RTSPAuthMethods) ||
		newConf.RTSPUDPReadBufferSize != p.conf.RTSPUDPReadBufferSize ||
		newConf.DumpPackets != p.conf.DumpPackets ||
		newConf.UDPReadBufferSize != p.conf.UDPReadBufferSize ||
		newConf.ReadTimeout != p.conf.ReadTimeout ||
		newConf.WriteTimeout != p.conf.WriteTimeout ||
		newConf.WriteQueueSize != p.conf.WriteQueueSize ||
		newConf.RTSPServerCert != p.conf.RTSPServerCert ||
		newConf.RTSPServerKey != p.conf.RTSPServerKey ||
		newConf.RTSPAddress != p.conf.RTSPAddress ||
		!reflect.DeepEqual(newConf.RTSPTransports, p.conf.RTSPTransports) ||
		!reflect.DeepEqual(newConf.RTSPTrustedProxies, p.conf.RTSPTrustedProxies) ||
		newConf.RunOnConnect != p.conf.RunOnConnect ||
		newConf.RunOnConnectRestart != p.conf.RunOnConnectRestart ||
		newConf.RunOnDisconnect != p.conf.RunOnDisconnect ||
		closeMetrics ||
		closePathManager ||
		closeLogger

	closeRTMPServer := newConf == nil ||
		newConf.RTMP != p.conf.RTMP ||
		newConf.RTMPEncryption != p.conf.RTMPEncryption ||
		newConf.RTMPAddress != p.conf.RTMPAddress ||
		newConf.DumpPackets != p.conf.DumpPackets ||
		newConf.ReadTimeout != p.conf.ReadTimeout ||
		newConf.WriteTimeout != p.conf.WriteTimeout ||
		newConf.RTSPAddress != p.conf.RTSPAddress ||
		!reflect.DeepEqual(newConf.RTMPTrustedProxies, p.conf.RTMPTrustedProxies) ||
		newConf.RunOnConnect != p.conf.RunOnConnect ||
		newConf.RunOnConnectRestart != p.conf.RunOnConnectRestart ||
		newConf.RunOnDisconnect != p.conf.RunOnDisconnect ||
		closeMetrics ||
		closePathManager ||
		closeLogger

	closeRTMPSServer := newConf == nil ||
		newConf.RTMP != p.conf.RTMP ||
		newConf.RTMPEncryption != p.conf.RTMPEncryption ||
		newConf.RTMPSAddress != p.conf.RTMPSAddress ||
		newConf.DumpPackets != p.conf.DumpPackets ||
		newConf.ReadTimeout != p.conf.ReadTimeout ||
		newConf.WriteTimeout != p.conf.WriteTimeout ||
		newConf.RTMPServerCert != p.conf.RTMPServerCert ||
		newConf.RTMPServerKey != p.conf.RTMPServerKey ||
		newConf.RTSPAddress != p.conf.RTSPAddress ||
		!reflect.DeepEqual(newConf.RTMPTrustedProxies, p.conf.RTMPTrustedProxies) ||
		newConf.RunOnConnect != p.conf.RunOnConnect ||
		newConf.RunOnConnectRestart != p.conf.RunOnConnectRestart ||
		newConf.RunOnDisconnect != p.conf.RunOnDisconnect ||
		closeMetrics ||
		closePathManager ||
		closeLogger

	closeHLSServer := newConf == nil ||
		newConf.HLS != p.conf.HLS ||
		newConf.HLSAddress != p.conf.HLSAddress ||
		newConf.HLSEncryption != p.conf.HLSEncryption ||
		newConf.HLSServerKey != p.conf.HLSServerKey ||
		newConf.HLSServerCert != p.conf.HLSServerCert ||
		!slices.Equal(newConf.HLSAllowOrigins, p.conf.HLSAllowOrigins) ||
		!reflect.DeepEqual(newConf.HLSTrustedProxies, p.conf.HLSTrustedProxies) ||
		newConf.HLSAlwaysRemux != p.conf.HLSAlwaysRemux ||
		newConf.HLSVariant != p.conf.HLSVariant ||
		newConf.HLSSegmentCount != p.conf.HLSSegmentCount ||
		newConf.HLSSegmentDuration != p.conf.HLSSegmentDuration ||
		newConf.HLSPartDuration != p.conf.HLSPartDuration ||
		newConf.HLSSegmentMaxSize != p.conf.HLSSegmentMaxSize ||
		newConf.HLSDirectory != p.conf.HLSDirectory ||
		newConf.ReadTimeout != p.conf.ReadTimeout ||
		newConf.WriteTimeout != p.conf.WriteTimeout ||
		newConf.HLSMuxerCloseAfter != p.conf.HLSMuxerCloseAfter ||
		newConf.HLSCDNSecret != p.conf.HLSCDNSecret ||
		newConf.DumpPackets != p.conf.DumpPackets ||
		closePathManager ||
		closeMetrics ||
		closeLogger

	closeWebRTCServer := newConf == nil ||
		newConf.WebRTC != p.conf.WebRTC ||
		newConf.WebRTCAddress != p.conf.WebRTCAddress ||
		newConf.WebRTCEncryption != p.conf.WebRTCEncryption ||
		newConf.WebRTCServerKey != p.conf.WebRTCServerKey ||
		newConf.WebRTCServerCert != p.conf.WebRTCServerCert ||
		!slices.Equal(newConf.WebRTCAllowOrigins, p.conf.WebRTCAllowOrigins) ||
		!reflect.DeepEqual(newConf.WebRTCTrustedProxies, p.conf.WebRTCTrustedProxies) ||
		newConf.ReadTimeout != p.conf.ReadTimeout ||
		newConf.WriteTimeout != p.conf.WriteTimeout ||
		newConf.UDPReadBufferSize != p.conf.UDPReadBufferSize ||
		newConf.WebRTCLocalUDPAddress != p.conf.WebRTCLocalUDPAddress ||
		newConf.WebRTCLocalTCPAddress != p.conf.WebRTCLocalTCPAddress ||
		newConf.WebRTCIPsFromInterfaces != p.conf.WebRTCIPsFromInterfaces ||
		!reflect.DeepEqual(newConf.WebRTCIPsFromInterfacesExcludeList, p.conf.WebRTCIPsFromInterfacesExcludeList) ||
		!reflect.DeepEqual(newConf.WebRTCIPsFromInterfacesList, p.conf.WebRTCIPsFromInterfacesList) ||
		!reflect.DeepEqual(newConf.WebRTCAdditionalHosts, p.conf.WebRTCAdditionalHosts) ||
		!reflect.DeepEqual(newConf.WebRTCICEServers2, p.conf.WebRTCICEServers2) ||
		newConf.WebRTCSTUNGatherTimeout != p.conf.WebRTCSTUNGatherTimeout ||
		newConf.WebRTCHandshakeTimeout != p.conf.WebRTCHandshakeTimeout ||
		newConf.WebRTCTrackGatherTimeout != p.conf.WebRTCTrackGatherTimeout ||
		newConf.DumpPackets != p.conf.DumpPackets ||
		closeMetrics ||
		closePathManager ||
		closeLogger

	closeSRTServer := newConf == nil ||
		newConf.SRT != p.conf.SRT ||
		newConf.SRTAddress != p.conf.SRTAddress ||
		newConf.RTSPAddress != p.conf.RTSPAddress ||
		newConf.ReadTimeout != p.conf.ReadTimeout ||
		newConf.WriteTimeout != p.conf.WriteTimeout ||
		newConf.UDPMaxPayloadSize != p.conf.UDPMaxPayloadSize ||
		newConf.RunOnConnect != p.conf.RunOnConnect ||
		newConf.RunOnConnectRestart != p.conf.RunOnConnectRestart ||
		newConf.RunOnDisconnect != p.conf.RunOnDisconnect ||
		closeMetrics ||
		closePathManager ||
		closeLogger

	closeMoQServer := newConf == nil ||
		newConf.MoQ != p.conf.MoQ ||
		newConf.MoQHTTP2Address != p.conf.MoQHTTP2Address ||
		newConf.MoQHTTP3Address != p.conf.MoQHTTP3Address ||
		newConf.MoQQUICAddress != p.conf.MoQQUICAddress ||
		newConf.MoQServerKey != p.conf.MoQServerKey ||
		newConf.MoQServerCert != p.conf.MoQServerCert ||
		!slices.Equal(newConf.MoQAllowOrigins, p.conf.MoQAllowOrigins) ||
		!reflect.DeepEqual(newConf.MoQTrustedProxies, p.conf.MoQTrustedProxies) ||
		newConf.UDPReadBufferSize != p.conf.UDPReadBufferSize ||
		newConf.ReadTimeout != p.conf.ReadTimeout ||
		newConf.WriteTimeout != p.conf.WriteTimeout ||
		closeMetrics ||
		closePathManager ||
		closeLogger

	closeAPI := newConf == nil ||
		newConf.API != p.conf.API ||
		newConf.APIAddress != p.conf.APIAddress ||
		newConf.APIEncryption != p.conf.APIEncryption ||
		newConf.APIServerKey != p.conf.APIServerKey ||
		newConf.APIServerCert != p.conf.APIServerCert ||
		!slices.Equal(newConf.APIAllowOrigins, p.conf.APIAllowOrigins) ||
		!reflect.DeepEqual(newConf.APITrustedProxies, p.conf.APITrustedProxies) ||
		newConf.ReadTimeout != p.conf.ReadTimeout ||
		newConf.WriteTimeout != p.conf.WriteTimeout ||
		newConf.DumpPackets != p.conf.DumpPackets ||
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

// reloadInternalUsers applies a configuration that differs from the current one
// only by internal users, without considering anything else.
func (p *Core) reloadInternalUsers(newConf *conf.Conf, ids []uuid.UUID) {
	p.confMutex.Lock()
	p.conf = newConf
	p.internalUserIDs = ids
	p.confMutex.Unlock()

	p.authManager.ReloadInternalUsers(newConf.AuthInternalUsers)
}

func (p *Core) reloadConf(newConf *conf.Conf) error {
	ids := p.internalUserIDs

	if !reflect.DeepEqual(p.conf.AuthInternalUsers, newConf.AuthInternalUsers) {
		ids = matchInternalUserIDs(p.conf.AuthInternalUsers, p.internalUserIDs, newConf.AuthInternalUsers)
	}

	oldLogger := p.logger

	p.closeResources(newConf)

	p.confMutex.Lock()
	p.conf = newConf
	p.internalUserIDs = ids
	p.confMutex.Unlock()

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
