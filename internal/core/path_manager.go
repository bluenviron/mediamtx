package core

import (
	"context"
	"fmt"
	"sync"

	"github.com/aler9/rtsp-simple-server/internal/conf"
	"github.com/aler9/rtsp-simple-server/internal/externalcmd"
	"github.com/aler9/rtsp-simple-server/internal/logger"
)

func pathConfCanBeUpdated(oldPathConf *conf.PathConf, newPathConf *conf.PathConf) bool {
	var copy conf.PathConf
	cloneStruct(&copy, oldPathConf)

	copy.RPICameraBrightness = newPathConf.RPICameraBrightness
	copy.RPICameraContrast = newPathConf.RPICameraContrast
	copy.RPICameraSaturation = newPathConf.RPICameraSaturation
	copy.RPICameraSharpness = newPathConf.RPICameraSharpness
	copy.RPICameraExposure = newPathConf.RPICameraExposure
	copy.RPICameraAWB = newPathConf.RPICameraAWB
	copy.RPICameraDenoise = newPathConf.RPICameraDenoise
	copy.RPICameraMetering = newPathConf.RPICameraMetering
	copy.RPICameraShutter = newPathConf.RPICameraShutter
	copy.RPICameraEV = newPathConf.RPICameraEV
	copy.RPICameraFPS = newPathConf.RPICameraFPS

	return newPathConf.Equal(&copy)
}

type pathManagerHLSServer interface {
	pathSourceReady(*path)
	pathSourceNotReady(*path)
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

	ctx         context.Context
	ctxCancel   func()
	wg          sync.WaitGroup
	hlsServer   pathManagerHLSServer
	paths       map[string]*path
	pathsByConf map[string]map[*path]struct{}

	// in
	chConfReload         chan map[string]*conf.PathConf
	chPathClose          chan *path
	chPathSourceReady    chan *path
	chPathSourceNotReady chan *path
	chDescribe           chan pathDescribeReq
	chReaderAdd          chan pathReaderAddReq
	chPublisherAdd       chan pathPublisherAddReq
	chHLSServerSet       chan pathManagerHLSServer
	chAPIPathsList       chan pathAPIPathsListReq
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
		rtspAddress:          rtspAddress,
		readTimeout:          readTimeout,
		writeTimeout:         writeTimeout,
		readBufferCount:      readBufferCount,
		pathConfs:            pathConfs,
		externalCmdPool:      externalCmdPool,
		metrics:              metrics,
		parent:               parent,
		ctx:                  ctx,
		ctxCancel:            ctxCancel,
		paths:                make(map[string]*path),
		pathsByConf:          make(map[string]map[*path]struct{}),
		chConfReload:         make(chan map[string]*conf.PathConf),
		chPathClose:          make(chan *path),
		chPathSourceReady:    make(chan *path),
		chPathSourceNotReady: make(chan *path),
		chDescribe:           make(chan pathDescribeReq),
		chReaderAdd:          make(chan pathReaderAddReq),
		chPublisherAdd:       make(chan pathPublisherAddReq),
		chHLSServerSet:       make(chan pathManagerHLSServer),
		chAPIPathsList:       make(chan pathAPIPathsListReq),
	}

	for pathConfName, pathConf := range pm.pathConfs {
		if pathConf.Regexp == nil {
			pm.createPath(pathConfName, pathConf, pathConfName, nil)
		}
	}

	if pm.metrics != nil {
		pm.metrics.pathManagerSet(pm)
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
		case newPathConfs := <-pm.chConfReload:
			for confName, pathConf := range pm.pathConfs {
				if newPathConf, ok := newPathConfs[confName]; ok {
					// configuration has changed
					if !newPathConf.Equal(pathConf) {
						if pathConfCanBeUpdated(pathConf, newPathConf) { // paths associated with the configuration can be updated
							for pa := range pm.pathsByConf[confName] {
								go pa.reloadConf(newPathConf)
							}
						} else { // paths associated with the configuration must be recreated
							for pa := range pm.pathsByConf[confName] {
								pm.removePath(pa)
								pa.close()
								pa.wait() // avoid conflicts between sources
							}
						}
					}
				} else {
					// configuration has been deleted, remove associated paths
					for pa := range pm.pathsByConf[confName] {
						pm.removePath(pa)
						pa.close()
						pa.wait() // avoid conflicts between sources
					}
				}
			}

			pm.pathConfs = newPathConfs

			// add new paths
			for pathConfName, pathConf := range pm.pathConfs {
				if _, ok := pm.paths[pathConfName]; !ok && pathConf.Regexp == nil {
					pm.createPath(pathConfName, pathConf, pathConfName, nil)
				}
			}

		case pa := <-pm.chPathClose:
			if pmpa, ok := pm.paths[pa.name]; !ok || pmpa != pa {
				continue
			}
			pm.removePath(pa)

		case pa := <-pm.chPathSourceReady:
			if pm.hlsServer != nil {
				pm.hlsServer.pathSourceReady(pa)
			}

		case pa := <-pm.chPathSourceNotReady:
			if pm.hlsServer != nil {
				pm.hlsServer.pathSourceNotReady(pa)
			}

		case req := <-pm.chDescribe:
			pathConfName, pathConf, pathMatches, err := pm.findPathConf(req.pathName)
			if err != nil {
				req.res <- pathDescribeRes{err: err}
				continue
			}

			if req.authenticate != nil {
				err = req.authenticate(
					pathConf.ReadIPs,
					pathConf.ReadUser,
					pathConf.ReadPass)
				if err != nil {
					req.res <- pathDescribeRes{err: err}
					continue
				}
			}

			// create path if it doesn't exist
			if _, ok := pm.paths[req.pathName]; !ok {
				pm.createPath(pathConfName, pathConf, req.pathName, pathMatches)
			}

			req.res <- pathDescribeRes{path: pm.paths[req.pathName]}

		case req := <-pm.chReaderAdd:
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

		case req := <-pm.chPublisherAdd:
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

		case s := <-pm.chHLSServerSet:
			pm.hlsServer = s

		case req := <-pm.chAPIPathsList:
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
		pm.metrics.pathManagerSet(nil)
	}
}

func (pm *pathManager) createPath(
	pathConfName string,
	pathConf *conf.PathConf,
	name string,
	matches []string,
) {
	pa := newPath(
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

	pm.paths[name] = pa

	if _, ok := pm.pathsByConf[pathConfName]; !ok {
		pm.pathsByConf[pathConfName] = make(map[*path]struct{})
	}
	pm.pathsByConf[pathConfName][pa] = struct{}{}
}

func (pm *pathManager) removePath(pa *path) {
	delete(pm.pathsByConf[pa.confName], pa)
	if len(pm.pathsByConf[pa.confName]) == 0 {
		delete(pm.pathsByConf, pa.confName)
	}
	delete(pm.paths, pa.name)
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

// confReload is called by core.
func (pm *pathManager) confReload(pathConfs map[string]*conf.PathConf) {
	select {
	case pm.chConfReload <- pathConfs:
	case <-pm.ctx.Done():
	}
}

// pathSourceReady is called by path.
func (pm *pathManager) pathSourceReady(pa *path) {
	select {
	case pm.chPathSourceReady <- pa:
	case <-pm.ctx.Done():
	case <-pa.ctx.Done(): // in case pathManager is closing the path
	}
}

// pathSourceNotReady is called by path.
func (pm *pathManager) pathSourceNotReady(pa *path) {
	select {
	case pm.chPathSourceNotReady <- pa:
	case <-pm.ctx.Done():
	case <-pa.ctx.Done(): // in case pathManager is closing the path
	}
}

// onPathClose is called by path.
func (pm *pathManager) onPathClose(pa *path) {
	select {
	case pm.chPathClose <- pa:
	case <-pm.ctx.Done():
	case <-pa.ctx.Done(): // in case pathManager is closing the path
	}
}

// describe is called by a reader or publisher.
func (pm *pathManager) describe(req pathDescribeReq) pathDescribeRes {
	req.res = make(chan pathDescribeRes)
	select {
	case pm.chDescribe <- req:
		res1 := <-req.res
		if res1.err != nil {
			return res1
		}

		res2 := res1.path.describe(req)
		if res2.err != nil {
			return res2
		}

		res2.path = res1.path
		return res2

	case <-pm.ctx.Done():
		return pathDescribeRes{err: fmt.Errorf("terminated")}
	}
}

// publisherAnnounce is called by a publisher.
func (pm *pathManager) publisherAdd(req pathPublisherAddReq) pathPublisherAnnounceRes {
	req.res = make(chan pathPublisherAnnounceRes)
	select {
	case pm.chPublisherAdd <- req:
		res := <-req.res
		if res.err != nil {
			return res
		}

		return res.path.publisherAdd(req)

	case <-pm.ctx.Done():
		return pathPublisherAnnounceRes{err: fmt.Errorf("terminated")}
	}
}

// readerSetupPlay is called by a reader.
func (pm *pathManager) readerAdd(req pathReaderAddReq) pathReaderSetupPlayRes {
	req.res = make(chan pathReaderSetupPlayRes)
	select {
	case pm.chReaderAdd <- req:
		res := <-req.res
		if res.err != nil {
			return res
		}

		return res.path.readerAdd(req)

	case <-pm.ctx.Done():
		return pathReaderSetupPlayRes{err: fmt.Errorf("terminated")}
	}
}

// hlsServerSet is called by hlsServer.
func (pm *pathManager) hlsServerSet(s pathManagerHLSServer) {
	select {
	case pm.chHLSServerSet <- s:
	case <-pm.ctx.Done():
	}
}

// apiPathsList is called by api.
func (pm *pathManager) apiPathsList() pathAPIPathsListRes {
	req := pathAPIPathsListReq{
		res: make(chan pathAPIPathsListRes),
	}

	select {
	case pm.chAPIPathsList <- req:
		res := <-req.res

		res.data = &pathAPIPathsListData{
			Items: make(map[string]pathAPIPathsListItem),
		}

		for _, pa := range res.paths {
			pa.apiPathsList(pathAPIPathsListSubReq{data: res.data})
		}

		return res

	case <-pm.ctx.Done():
		return pathAPIPathsListRes{err: fmt.Errorf("terminated")}
	}
}
