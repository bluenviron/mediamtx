package core

import (
	"context"
	"fmt"
	"net"
	"sync"

	"github.com/aler9/gortsplib/pkg/base"

	"github.com/aler9/rtsp-simple-server/internal/conf"
	"github.com/aler9/rtsp-simple-server/internal/logger"
)

type pathManagerHLSServer interface {
	onPathSourceReady(pa *path)
}

type pathManagerParent interface {
	Log(logger.Level, string, ...interface{})
}

type pathManager struct {
	rtspAddress     string
	readTimeout     conf.StringDuration
	writeTimeout    conf.StringDuration
	readBufferCount int
	readBufferSize  int
	pathConfs       map[string]*conf.PathConf
	metrics         *metrics
	parent          pathManagerParent

	ctx       context.Context
	ctxCancel func()
	wg        sync.WaitGroup
	hlsServer pathManagerHLSServer
	paths     map[string]*path

	// in
	confReload        chan map[string]*conf.PathConf
	pathClose         chan *path
	pathSourceReady   chan *path
	describe          chan pathDescribeReq
	readerSetupPlay   chan pathReaderSetupPlayReq
	publisherAnnounce chan pathPublisherAnnounceReq
	hlsServerSet      chan pathManagerHLSServer
	apiPathsList      chan pathAPIPathsListReq
}

func newPathManager(
	parentCtx context.Context,
	rtspAddress string,
	readTimeout conf.StringDuration,
	writeTimeout conf.StringDuration,
	readBufferCount int,
	readBufferSize int,
	pathConfs map[string]*conf.PathConf,
	metrics *metrics,
	parent pathManagerParent) *pathManager {
	ctx, ctxCancel := context.WithCancel(parentCtx)

	pm := &pathManager{
		rtspAddress:       rtspAddress,
		readTimeout:       readTimeout,
		writeTimeout:      writeTimeout,
		readBufferCount:   readBufferCount,
		readBufferSize:    readBufferSize,
		pathConfs:         pathConfs,
		metrics:           metrics,
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
		hlsServerSet:      make(chan pathManagerHLSServer),
		apiPathsList:      make(chan pathAPIPathsListReq),
	}

	for pathName, pathConf := range pm.pathConfs {
		if pathConf.Regexp == nil {
			pm.createPath(pathName, pathConf, pathName)
		}
	}

	if pm.metrics != nil {
		pm.metrics.onPathManagerSet(pm)
	}

	pm.wg.Add(1)
	go pm.run()

	return pm
}

func (pm *pathManager) close() {
	pm.ctxCancel()
	pm.wg.Wait()
}

// Log is the main logging function.
func (pm *pathManager) log(level logger.Level, format string, args ...interface{}) {
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
					pa.close()
				}
			}

			// add new paths
			for pathName, pathConf := range pm.pathConfs {
				if _, ok := pm.paths[pathName]; !ok && pathConf.Regexp == nil {
					pm.createPath(pathName, pathConf, pathName)
				}
			}

		case pa := <-pm.pathClose:
			if pmpa, ok := pm.paths[pa.Name()]; !ok || pmpa != pa {
				continue
			}
			delete(pm.paths, pa.Name())
			pa.close()

		case pa := <-pm.pathSourceReady:
			if pm.hlsServer != nil {
				pm.hlsServer.onPathSourceReady(pa)
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
				pathConf.ReadIPs,
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

			req.Res <- pathDescribeRes{Path: pm.paths[req.PathName]}

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
				pathConf.ReadIPs,
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

			req.Res <- pathReaderSetupPlayRes{Path: pm.paths[req.PathName]}

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
				pathConf.PublishIPs,
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

			req.Res <- pathPublisherAnnounceRes{Path: pm.paths[req.PathName]}

		case s := <-pm.hlsServerSet:
			pm.hlsServer = s

		case req := <-pm.apiPathsList:
			paths := make(map[string]*path)

			for name, pa := range pm.paths {
				paths[name] = pa
			}

			req.Res <- pathAPIPathsListRes{
				Paths: paths,
			}

		case <-pm.ctx.Done():
			break outer
		}
	}

	pm.ctxCancel()

	if pm.metrics != nil {
		pm.metrics.onPathManagerSet(nil)
	}
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
		pm)
}

func (pm *pathManager) findPathConf(name string) (string, *conf.PathConf, error) {
	err := conf.IsValidPathName(name)
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
	validateCredentials func(pathUser conf.Credential, pathPass conf.Credential) error,
	pathName string,
	pathIPs []interface{},
	pathUser conf.Credential,
	pathPass conf.Credential,
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

// onConfReload is called by core.
func (pm *pathManager) onConfReload(pathConfs map[string]*conf.PathConf) {
	select {
	case pm.confReload <- pathConfs:
	case <-pm.ctx.Done():
	}
}

// onPathSourceReady is called by path.
func (pm *pathManager) onPathSourceReady(pa *path) {
	select {
	case pm.pathSourceReady <- pa:
	case <-pm.ctx.Done():
	}
}

// onPathClose is called by path.
func (pm *pathManager) onPathClose(pa *path) {
	select {
	case pm.pathClose <- pa:
	case <-pm.ctx.Done():
	}
}

// onDescribe is called by a reader or publisher.
func (pm *pathManager) onDescribe(req pathDescribeReq) pathDescribeRes {
	req.Res = make(chan pathDescribeRes)
	select {
	case pm.describe <- req:
		res := <-req.Res
		if res.Err != nil {
			return res
		}

		return res.Path.onDescribe(req)

	case <-pm.ctx.Done():
		return pathDescribeRes{Err: fmt.Errorf("terminated")}
	}
}

// onPublisherAnnounce is called by a publisher.
func (pm *pathManager) onPublisherAnnounce(req pathPublisherAnnounceReq) pathPublisherAnnounceRes {
	req.Res = make(chan pathPublisherAnnounceRes)
	select {
	case pm.publisherAnnounce <- req:
		res := <-req.Res
		if res.Err != nil {
			return res
		}

		return res.Path.onPublisherAnnounce(req)

	case <-pm.ctx.Done():
		return pathPublisherAnnounceRes{Err: fmt.Errorf("terminated")}
	}
}

// onReaderSetupPlay is called by a reader.
func (pm *pathManager) onReaderSetupPlay(req pathReaderSetupPlayReq) pathReaderSetupPlayRes {
	req.Res = make(chan pathReaderSetupPlayRes)
	select {
	case pm.readerSetupPlay <- req:
		res := <-req.Res
		if res.Err != nil {
			return res
		}

		return res.Path.onReaderSetupPlay(req)

	case <-pm.ctx.Done():
		return pathReaderSetupPlayRes{Err: fmt.Errorf("terminated")}
	}
}

// onHLSServerSet is called by hlsServer.
func (pm *pathManager) onHLSServerSet(s pathManagerHLSServer) {
	select {
	case pm.hlsServerSet <- s:
	case <-pm.ctx.Done():
	}
}

// onAPIPathsList is called by api.
func (pm *pathManager) onAPIPathsList(req pathAPIPathsListReq) pathAPIPathsListRes {
	req.Res = make(chan pathAPIPathsListRes)
	select {
	case pm.apiPathsList <- req:
		res := <-req.Res

		res.Data = &pathAPIPathsListData{
			Items: make(map[string]pathAPIPathsListItem),
		}

		for _, pa := range res.Paths {
			pa.onAPIPathsList(pathAPIPathsListSubReq{Data: res.Data})
		}

		return res

	case <-pm.ctx.Done():
		return pathAPIPathsListRes{Err: fmt.Errorf("terminated")}
	}
}
