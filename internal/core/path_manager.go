package core

import (
	"context"
	"fmt"
	"sync"

	"github.com/aler9/rtsp-simple-server/internal/conf"
	"github.com/aler9/rtsp-simple-server/internal/externalcmd"
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
	pathConfs       map[string]*conf.PathConf
	externalCmdPool *externalcmd.Pool
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
	pathConfs map[string]*conf.PathConf,
	externalCmdPool *externalcmd.Pool,
	metrics *metrics,
	parent pathManagerParent,
) *pathManager {
	ctx, ctxCancel := context.WithCancel(parentCtx)

	pm := &pathManager{
		rtspAddress:       rtspAddress,
		readTimeout:       readTimeout,
		writeTimeout:      writeTimeout,
		readBufferCount:   readBufferCount,
		pathConfs:         pathConfs,
		externalCmdPool:   externalCmdPool,
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

	for pathConfName, pathConf := range pm.pathConfs {
		if pathConf.Regexp == nil {
			pm.createPath(pathConfName, pathConf, pathConfName, nil)
		}
	}

	if pm.metrics != nil {
		pm.metrics.onPathManagerSet(pm)
	}

	pm.log(logger.Debug, "path manager created")

	pm.wg.Add(1)
	go pm.run()

	return pm
}

func (pm *pathManager) close() {
	pm.log(logger.Debug, "path manager is shutting down")
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
			for pathConfName := range pm.pathConfs {
				if _, ok := pathConfs[pathConfName]; !ok {
					delete(pm.pathConfs, pathConfName)
				}
			}

			// update confs
			for pathConfName, oldConf := range pm.pathConfs {
				if !oldConf.Equal(pathConfs[pathConfName]) {
					pm.pathConfs[pathConfName] = pathConfs[pathConfName]
				}
			}

			// add confs
			for pathConfName, pathConf := range pathConfs {
				if _, ok := pm.pathConfs[pathConfName]; !ok {
					pm.pathConfs[pathConfName] = pathConf
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
			for pathConfName, pathConf := range pm.pathConfs {
				if _, ok := pm.paths[pathConfName]; !ok && pathConf.Regexp == nil {
					pm.createPath(pathConfName, pathConf, pathConfName, nil)
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
			pathConfName, pathConf, pathMatches, err := pm.findPathConf(req.pathName)
			if err != nil {
				req.res <- pathDescribeRes{err: err}
				continue
			}

			err = req.authenticate(
				pathConf.ReadIPs,
				pathConf.ReadUser,
				pathConf.ReadPass)
			if err != nil {
				req.res <- pathDescribeRes{err: err}
				continue
			}

			// create path if it doesn't exist
			if _, ok := pm.paths[req.pathName]; !ok {
				pm.createPath(pathConfName, pathConf, req.pathName, pathMatches)
			}

			req.res <- pathDescribeRes{path: pm.paths[req.pathName]}

		case req := <-pm.readerSetupPlay:
			pathConfName, pathConf, pathMatches, err := pm.findPathConf(req.pathName)
			if err != nil {
				req.res <- pathReaderSetupPlayRes{err: err}
				continue
			}

			if req.authenticate != nil {
				err = req.authenticate(
					pathConf.ReadIPs,
					pathConf.ReadUser,
					pathConf.ReadPass)
				if err != nil {
					req.res <- pathReaderSetupPlayRes{err: err}
					continue
				}
			}

			// create path if it doesn't exist
			if _, ok := pm.paths[req.pathName]; !ok {
				pm.createPath(pathConfName, pathConf, req.pathName, pathMatches)
			}

			req.res <- pathReaderSetupPlayRes{path: pm.paths[req.pathName]}

		case req := <-pm.publisherAnnounce:
			pathConfName, pathConf, pathMatches, err := pm.findPathConf(req.pathName)
			if err != nil {
				req.res <- pathPublisherAnnounceRes{err: err}
				continue
			}

			err = req.authenticate(
				pathConf.PublishIPs,
				pathConf.PublishUser,
				pathConf.PublishPass)
			if err != nil {
				req.res <- pathPublisherAnnounceRes{err: err}
				continue
			}

			// create path if it doesn't exist
			if _, ok := pm.paths[req.pathName]; !ok {
				pm.createPath(pathConfName, pathConf, req.pathName, pathMatches)
			}

			req.res <- pathPublisherAnnounceRes{path: pm.paths[req.pathName]}

		case s := <-pm.hlsServerSet:
			pm.hlsServer = s

		case req := <-pm.apiPathsList:
			paths := make(map[string]*path)

			for name, pa := range pm.paths {
				paths[name] = pa
			}

			req.res <- pathAPIPathsListRes{
				paths: paths,
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

func (pm *pathManager) createPath(
	pathConfName string,
	pathConf *conf.PathConf,
	name string,
	matches []string,
) {
	pm.paths[name] = newPath(
		pm.ctx,
		pm.rtspAddress,
		pm.readTimeout,
		pm.writeTimeout,
		pm.readBufferCount,
		pathConfName,
		pathConf,
		name,
		matches,
		&pm.wg,
		pm.externalCmdPool,
		pm)
}

func (pm *pathManager) findPathConf(name string) (string, *conf.PathConf, []string, error) {
	err := conf.IsValidPathName(name)
	if err != nil {
		return "", nil, nil, fmt.Errorf("invalid path name: %s (%s)", err, name)
	}

	// normal path
	if pathConf, ok := pm.pathConfs[name]; ok {
		return name, pathConf, nil, nil
	}

	// regular expression path
	for pathConfName, pathConf := range pm.pathConfs {
		if pathConf.Regexp != nil {
			m := pathConf.Regexp.FindStringSubmatch(name)
			if m != nil {
				return pathConfName, pathConf, m, nil
			}
		}
	}

	return "", nil, nil, fmt.Errorf("path '%s' is not configured", name)
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
	req.res = make(chan pathDescribeRes)
	select {
	case pm.describe <- req:
		res := <-req.res
		if res.err != nil {
			return res
		}

		return res.path.onDescribe(req)

	case <-pm.ctx.Done():
		return pathDescribeRes{err: fmt.Errorf("terminated")}
	}
}

// onPublisherAnnounce is called by a publisher.
func (pm *pathManager) onPublisherAnnounce(req pathPublisherAnnounceReq) pathPublisherAnnounceRes {
	req.res = make(chan pathPublisherAnnounceRes)
	select {
	case pm.publisherAnnounce <- req:
		res := <-req.res
		if res.err != nil {
			return res
		}

		return res.path.onPublisherAnnounce(req)

	case <-pm.ctx.Done():
		return pathPublisherAnnounceRes{err: fmt.Errorf("terminated")}
	}
}

// onReaderSetupPlay is called by a reader.
func (pm *pathManager) onReaderSetupPlay(req pathReaderSetupPlayReq) pathReaderSetupPlayRes {
	req.res = make(chan pathReaderSetupPlayRes)
	select {
	case pm.readerSetupPlay <- req:
		res := <-req.res
		if res.err != nil {
			return res
		}

		return res.path.onReaderSetupPlay(req)

	case <-pm.ctx.Done():
		return pathReaderSetupPlayRes{err: fmt.Errorf("terminated")}
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
	req.res = make(chan pathAPIPathsListRes)
	select {
	case pm.apiPathsList <- req:
		res := <-req.res

		res.data = &pathAPIPathsListData{
			Items: make(map[string]pathAPIPathsListItem),
		}

		for _, pa := range res.paths {
			pa.onAPIPathsList(pathAPIPathsListSubReq{data: res.data})
		}

		return res

	case <-pm.ctx.Done():
		return pathAPIPathsListRes{err: fmt.Errorf("terminated")}
	}
}
