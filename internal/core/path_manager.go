package core

import (
	"context"
	"fmt"
	"net"
	"sync"
	"time"

	"github.com/aler9/gortsplib/pkg/base"

	"github.com/aler9/rtsp-simple-server/internal/conf"
	"github.com/aler9/rtsp-simple-server/internal/logger"
)

type pathManagerParent interface {
	Log(logger.Level, string, ...interface{})
}

type pathManager struct {
	rtspAddress     string
	readTimeout     time.Duration
	writeTimeout    time.Duration
	readBufferCount int
	readBufferSize  int
	pathConfs       map[string]*conf.PathConf
	stats           *stats
	parent          pathManagerParent

	ctx       context.Context
	ctxCancel func()
	wg        sync.WaitGroup
	hlsServer *hlsServer
	paths     map[string]*path

	// in
	confReload        chan map[string]*conf.PathConf
	pathClose         chan *path
	pathSourceReady   chan *path
	describe          chan pathDescribeReq
	readerSetupPlay   chan pathReaderSetupPlayReq
	publisherAnnounce chan pathPublisherAnnounceReq
	hlsServerSet      chan *hlsServer
}

func newPathManager(
	parentCtx context.Context,
	rtspAddress string,
	readTimeout time.Duration,
	writeTimeout time.Duration,
	readBufferCount int,
	readBufferSize int,
	pathConfs map[string]*conf.PathConf,
	stats *stats,
	parent pathManagerParent) *pathManager {
	ctx, ctxCancel := context.WithCancel(parentCtx)

	pm := &pathManager{
		rtspAddress:       rtspAddress,
		readTimeout:       readTimeout,
		writeTimeout:      writeTimeout,
		readBufferCount:   readBufferCount,
		readBufferSize:    readBufferSize,
		pathConfs:         pathConfs,
		stats:             stats,
		parent:            parent,
		ctx:               ctx,
		ctxCancel:         ctxCancel,
		paths:             make(map[string]*path),
		confReload:        make(chan map[string]*conf.PathConf),
		pathClose:         make(chan *path),
		pathSourceReady:   make(chan *path),
		describe:          make(chan pathDescribeReq),
		readerSetupPlay:   make(chan pathReaderSetupPlayReq),
		publisherAnnounce: make(chan pathPublisherAnnounceReq),
		hlsServerSet:      make(chan *hlsServer),
	}

	pm.createPaths()

	pm.wg.Add(1)
	go pm.run()

	return pm
}

func (pm *pathManager) close() {
	pm.ctxCancel()
	pm.wg.Wait()
}

// Log is the main logging function.
func (pm *pathManager) Log(level logger.Level, format string, args ...interface{}) {
	pm.parent.Log(level, format, args...)
}

func (pm *pathManager) run() {
	defer pm.wg.Done()

outer:
	for {
		select {
		case pathConfs := <-pm.confReload:
			// remove confs
			for pathName := range pm.pathConfs {
				if _, ok := pathConfs[pathName]; !ok {
					delete(pm.pathConfs, pathName)
				}
			}

			// update confs
			for pathName, oldConf := range pm.pathConfs {
				if !oldConf.Equal(pathConfs[pathName]) {
					pm.pathConfs[pathName] = pathConfs[pathName]
				}
			}

			// add confs
			for pathName, pathConf := range pathConfs {
				if _, ok := pm.pathConfs[pathName]; !ok {
					pm.pathConfs[pathName] = pathConf
				}
			}

			// remove paths associated with a conf which doesn't exist anymore
			// or has changed
			for _, pa := range pm.paths {
				if pathConf, ok := pm.pathConfs[pa.ConfName()]; !ok || pathConf != pa.Conf() {
					delete(pm.paths, pa.Name())
					pa.Close()
				}
			}

			// add paths
			pm.createPaths()

		case pa := <-pm.pathClose:
			if pmpa, ok := pm.paths[pa.Name()]; !ok || pmpa != pa {
				continue
			}
			delete(pm.paths, pa.Name())
			pa.Close()

		case pa := <-pm.pathSourceReady:
			if pm.hlsServer != nil {
				pm.hlsServer.OnPathSourceReady(pa)
			}

		case req := <-pm.describe:
			pathName, pathConf, err := pm.findPathConf(req.PathName)
			if err != nil {
				req.Res <- pathDescribeRes{Err: err}
				continue
			}

			err = pm.authenticate(
				req.IP,
				req.ValidateCredentials,
				req.PathName,
				pathConf.ReadIPsParsed,
				pathConf.ReadUser,
				pathConf.ReadPass,
			)
			if err != nil {
				req.Res <- pathDescribeRes{Err: err}
				continue
			}

			// create path if it doesn't exist
			if _, ok := pm.paths[req.PathName]; !ok {
				pm.createPath(pathName, pathConf, req.PathName)
			}

			pm.paths[req.PathName].OnDescribe(req)

		case req := <-pm.readerSetupPlay:
			pathName, pathConf, err := pm.findPathConf(req.PathName)
			if err != nil {
				req.Res <- pathReaderSetupPlayRes{Err: err}
				continue
			}

			err = pm.authenticate(
				req.IP,
				req.ValidateCredentials,
				req.PathName,
				pathConf.ReadIPsParsed,
				pathConf.ReadUser,
				pathConf.ReadPass,
			)
			if err != nil {
				req.Res <- pathReaderSetupPlayRes{Err: err}
				continue
			}

			// create path if it doesn't exist
			if _, ok := pm.paths[req.PathName]; !ok {
				pm.createPath(pathName, pathConf, req.PathName)
			}

			pm.paths[req.PathName].OnReaderSetupPlay(req)

		case req := <-pm.publisherAnnounce:
			pathName, pathConf, err := pm.findPathConf(req.PathName)
			if err != nil {
				req.Res <- pathPublisherAnnounceRes{Err: err}
				continue
			}

			err = pm.authenticate(
				req.IP,
				req.ValidateCredentials,
				req.PathName,
				pathConf.PublishIPsParsed,
				pathConf.PublishUser,
				pathConf.PublishPass,
			)
			if err != nil {
				req.Res <- pathPublisherAnnounceRes{Err: err}
				continue
			}

			// create path if it doesn't exist
			if _, ok := pm.paths[req.PathName]; !ok {
				pm.createPath(pathName, pathConf, req.PathName)
			}

			pm.paths[req.PathName].OnPublisherAnnounce(req)

		case s := <-pm.hlsServerSet:
			pm.hlsServer = s

		case <-pm.ctx.Done():
			break outer
		}
	}

	pm.ctxCancel()
}

func (pm *pathManager) createPath(confName string, conf *conf.PathConf, name string) {
	pm.paths[name] = newPath(
		pm.ctx,
		pm.rtspAddress,
		pm.readTimeout,
		pm.writeTimeout,
		pm.readBufferCount,
		pm.readBufferSize,
		confName,
		conf,
		name,
		&pm.wg,
		pm.stats,
		pm)
}

func (pm *pathManager) createPaths() {
	for pathName, pathConf := range pm.pathConfs {
		if _, ok := pm.paths[pathName]; !ok && pathConf.Regexp == nil {
			pm.createPath(pathName, pathConf, pathName)
		}
	}
}

func (pm *pathManager) findPathConf(name string) (string, *conf.PathConf, error) {
	err := conf.CheckPathName(name)
	if err != nil {
		return "", nil, fmt.Errorf("invalid path name: %s (%s)", err, name)
	}

	// normal path
	if pathConf, ok := pm.pathConfs[name]; ok {
		return name, pathConf, nil
	}

	// regular expression path
	for pathName, pathConf := range pm.pathConfs {
		if pathConf.Regexp != nil && pathConf.Regexp.MatchString(name) {
			return pathName, pathConf, nil
		}
	}

	return "", nil, fmt.Errorf("unable to find a valid configuration for path '%s'", name)
}

func (pm *pathManager) authenticate(
	ip net.IP,
	validateCredentials func(pathUser string, pathPass string) error,
	pathName string,
	pathIPs []interface{},
	pathUser string,
	pathPass string,
) error {
	// validate ip
	if pathIPs != nil && ip != nil {
		if !ipEqualOrInRange(ip, pathIPs) {
			return pathErrAuthCritical{
				Message: fmt.Sprintf("IP '%s' not allowed", ip),
				Response: &base.Response{
					StatusCode: base.StatusUnauthorized,
				},
			}
		}
	}

	// validate user
	if pathUser != "" && validateCredentials != nil {
		err := validateCredentials(pathUser, pathPass)
		if err != nil {
			return err
		}
	}

	return nil
}

// OnConfReload is called by core.
func (pm *pathManager) OnConfReload(pathConfs map[string]*conf.PathConf) {
	select {
	case pm.confReload <- pathConfs:
	case <-pm.ctx.Done():
	}
}

// OnPathSourceReady is called by path.
func (pm *pathManager) OnPathSourceReady(pa *path) {
	select {
	case pm.pathSourceReady <- pa:
	case <-pm.ctx.Done():
	}
}

// OnPathClose is called by path.
func (pm *pathManager) OnPathClose(pa *path) {
	select {
	case pm.pathClose <- pa:
	case <-pm.ctx.Done():
	}
}

// OnDescribe is called by a reader or publisher.
func (pm *pathManager) OnDescribe(req pathDescribeReq) pathDescribeRes {
	req.Res = make(chan pathDescribeRes)
	select {
	case pm.describe <- req:
		return <-req.Res
	case <-pm.ctx.Done():
		return pathDescribeRes{Err: fmt.Errorf("terminated")}
	}
}

// OnPublisherAnnounce is called by a publisher.
func (pm *pathManager) OnPublisherAnnounce(req pathPublisherAnnounceReq) pathPublisherAnnounceRes {
	req.Res = make(chan pathPublisherAnnounceRes)
	select {
	case pm.publisherAnnounce <- req:
		return <-req.Res
	case <-pm.ctx.Done():
		return pathPublisherAnnounceRes{Err: fmt.Errorf("terminated")}
	}
}

// OnReaderSetupPlay is called by a reader.
func (pm *pathManager) OnReaderSetupPlay(req pathReaderSetupPlayReq) pathReaderSetupPlayRes {
	req.Res = make(chan pathReaderSetupPlayRes)
	select {
	case pm.readerSetupPlay <- req:
		return <-req.Res
	case <-pm.ctx.Done():
		return pathReaderSetupPlayRes{Err: fmt.Errorf("terminated")}
	}
}

// OnHLSServer is called by hlsServer.
func (pm *pathManager) OnHLSServer(s *hlsServer) {
	select {
	case pm.hlsServerSet <- s:
	case <-pm.ctx.Done():
	}
}
