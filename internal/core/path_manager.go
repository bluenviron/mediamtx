package core

import (
	"context"
	"fmt"
	"sort"
	"sync"

	"github.com/bluenviron/mediamtx/internal/auth"
	"github.com/bluenviron/mediamtx/internal/conf"
	"github.com/bluenviron/mediamtx/internal/defs"
	"github.com/bluenviron/mediamtx/internal/externalcmd"
	"github.com/bluenviron/mediamtx/internal/logger"
	"github.com/bluenviron/mediamtx/internal/metrics"
	"github.com/bluenviron/mediamtx/internal/servers/hls"
	"github.com/bluenviron/mediamtx/internal/stream"
)

func pathConfCanBeUpdated(oldPathConf *conf.Path, newPathConf *conf.Path) bool {
	clone := oldPathConf.Clone()

	clone.Name = newPathConf.Name
	clone.Regexp = newPathConf.Regexp

	clone.Record = newPathConf.Record
	clone.RecordPath = newPathConf.RecordPath
	clone.RecordFormat = newPathConf.RecordFormat
	clone.RecordPartDuration = newPathConf.RecordPartDuration
	clone.RecordMaxPartSize = newPathConf.RecordMaxPartSize
	clone.RecordSegmentDuration = newPathConf.RecordSegmentDuration
	clone.RecordDeleteAfter = newPathConf.RecordDeleteAfter

	clone.RPICameraBrightness = newPathConf.RPICameraBrightness
	clone.RPICameraContrast = newPathConf.RPICameraContrast
	clone.RPICameraSaturation = newPathConf.RPICameraSaturation
	clone.RPICameraSharpness = newPathConf.RPICameraSharpness
	clone.RPICameraExposure = newPathConf.RPICameraExposure
	clone.RPICameraFlickerPeriod = newPathConf.RPICameraFlickerPeriod
	clone.RPICameraAWB = newPathConf.RPICameraAWB
	clone.RPICameraAWBGains = newPathConf.RPICameraAWBGains
	clone.RPICameraDenoise = newPathConf.RPICameraDenoise
	clone.RPICameraShutter = newPathConf.RPICameraShutter
	clone.RPICameraMetering = newPathConf.RPICameraMetering
	clone.RPICameraGain = newPathConf.RPICameraGain
	clone.RPICameraEV = newPathConf.RPICameraEV
	clone.RPICameraFPS = newPathConf.RPICameraFPS
	clone.RPICameraIDRPeriod = newPathConf.RPICameraIDRPeriod
	clone.RPICameraBitrate = newPathConf.RPICameraBitrate

	return newPathConf.Equal(clone)
}

type pathSetHLSServerRes struct {
	readyPaths []defs.Path
}

type pathSetHLSServerReq struct {
	s   *hls.Server
	res chan pathSetHLSServerRes
}

type pathData struct {
	path     *path
	ready    bool
	confName string
}

type pathManagerParent interface {
	logger.Writer
}

type pathManager struct {
	logLevel          conf.LogLevel
	authManager       *auth.Manager
	rtspAddress       string
	readTimeout       conf.Duration
	writeTimeout      conf.Duration
	writeQueueSize    int
	udpReadBufferSize uint
	rtpMaxPayloadSize int
	pathConfs         map[string]*conf.Path
	externalCmdPool   *externalcmd.Pool
	metrics           *metrics.Metrics
	parent            pathManagerParent

	ctx       context.Context
	ctxCancel func()
	wg        sync.WaitGroup
	hlsServer *hls.Server
	paths     map[string]*pathData

	// in
	chReloadConf   chan map[string]*conf.Path
	chSetHLSServer chan pathSetHLSServerReq
	chClosePath    chan *path
	chPathReady    chan *path
	chPathNotReady chan *path
	chFindPathConf chan defs.PathFindPathConfReq
	chDescribe     chan defs.PathDescribeReq
	chAddReader    chan defs.PathAddReaderReq
	chAddPublisher chan defs.PathAddPublisherReq
	chAPIPathsList chan pathAPIPathsListReq
	chAPIPathsGet  chan pathAPIPathsGetReq
}

func (pm *pathManager) initialize() {
	ctx, ctxCancel := context.WithCancel(context.Background())

	pm.ctx = ctx
	pm.ctxCancel = ctxCancel
	pm.paths = make(map[string]*pathData)
	pm.chReloadConf = make(chan map[string]*conf.Path)
	pm.chSetHLSServer = make(chan pathSetHLSServerReq)
	pm.chClosePath = make(chan *path)
	pm.chPathReady = make(chan *path)
	pm.chPathNotReady = make(chan *path)
	pm.chFindPathConf = make(chan defs.PathFindPathConfReq)
	pm.chDescribe = make(chan defs.PathDescribeReq)
	pm.chAddReader = make(chan defs.PathAddReaderReq)
	pm.chAddPublisher = make(chan defs.PathAddPublisherReq)
	pm.chAPIPathsList = make(chan pathAPIPathsListReq)
	pm.chAPIPathsGet = make(chan pathAPIPathsGetReq)

	for _, pathConf := range pm.pathConfs {
		if pathConf.Regexp == nil {
			pm.createPath(pathConf, pathConf.Name, nil)
		}
	}

	pm.Log(logger.Debug, "path manager created")

	pm.wg.Add(1)
	go pm.run()

	if pm.metrics != nil {
		pm.metrics.SetPathManager(pm)
	}
}

func (pm *pathManager) close() {
	pm.Log(logger.Debug, "path manager is shutting down")

	if pm.metrics != nil {
		pm.metrics.SetPathManager(nil)
	}

	pm.ctxCancel()
	pm.wg.Wait()
}

// Log implements logger.Writer.
func (pm *pathManager) Log(level logger.Level, format string, args ...any) {
	pm.parent.Log(level, format, args...)
}

func (pm *pathManager) run() {
	defer pm.wg.Done()

outer:
	for {
		select {
		case newPaths := <-pm.chReloadConf:
			pm.doReloadConf(newPaths)

		case req := <-pm.chSetHLSServer:
			readyPaths := pm.doSetHLSServer(req.s)
			req.res <- pathSetHLSServerRes{readyPaths: readyPaths}

		case pa := <-pm.chClosePath:
			pm.doClosePath(pa)

		case pa := <-pm.chPathReady:
			pm.doPathReady(pa)

		case pa := <-pm.chPathNotReady:
			pm.doPathNotReady(pa)

		case req := <-pm.chFindPathConf:
			pm.doFindPathConf(req)

		case req := <-pm.chDescribe:
			pm.doDescribe(req)

		case req := <-pm.chAddReader:
			pm.doAddReader(req)

		case req := <-pm.chAddPublisher:
			pm.doAddPublisher(req)

		case req := <-pm.chAPIPathsList:
			pm.doAPIPathsList(req)

		case req := <-pm.chAPIPathsGet:
			pm.doAPIPathsGet(req)

		case <-pm.ctx.Done():
			break outer
		}
	}

	pm.ctxCancel()
}

func (pm *pathManager) doReloadConf(newPaths map[string]*conf.Path) {
	confsToRecreate := make(map[string]struct{})
	confsToReload := make(map[string]struct{})

	for confName, pathConf := range pm.pathConfs {
		if newPath, ok := newPaths[confName]; ok {
			if !newPath.Equal(pathConf) {
				if pathConfCanBeUpdated(pathConf, newPath) {
					confsToReload[confName] = struct{}{}
				} else {
					confsToRecreate[confName] = struct{}{}
				}
			}
		}
	}

	// process existing paths
	for pathName, pathData := range pm.paths {
		path := pathData.path
		newPathConf, _, err := conf.FindPathConf(newPaths, pathName)
		// path does not have a config anymore: delete it
		if err != nil {
			pm.removeAndClosePath(path)
			continue
		}

		// path now belongs to a different config
		if newPathConf.Name != pathData.confName {
			// path config can be hot reloaded
			oldPathConf := pm.pathConfs[pathData.confName]
			if pathConfCanBeUpdated(oldPathConf, newPathConf) {
				pm.paths[path.name].confName = newPathConf.Name
				go path.reloadConf(newPathConf)
				continue
			}

			// Configuration cannot be hot reloaded: delete the path
			pm.removeAndClosePath(path)
			continue
		}

		// path configuration has changed and cannot be hot reloaded: delete path
		if _, ok := confsToRecreate[newPathConf.Name]; ok {
			pm.removeAndClosePath(path)
			continue
		}

		// path configuration has changed but can be hot reloaded: reload it
		if _, ok := confsToReload[newPathConf.Name]; ok {
			go path.reloadConf(newPathConf)
		}
	}

	pm.pathConfs = newPaths

	// create new static paths
	for pathConfName, pathConf := range newPaths {
		if pathConf.Regexp == nil {
			if _, ok := pm.paths[pathConfName]; !ok {
				pm.createPath(pathConf, pathConfName, nil)
			}
		}
	}
}

func (pm *pathManager) removeAndClosePath(path *path) {
	pm.removePath(path)
	path.close()
	path.wait() // avoid conflicts between sources
}

func (pm *pathManager) doSetHLSServer(m *hls.Server) []defs.Path {
	pm.hlsServer = m

	var ret []defs.Path

	for _, pd := range pm.paths {
		if pd.ready {
			ret = append(ret, pd.path)
		}
	}

	return ret
}

func (pm *pathManager) doClosePath(pa *path) {
	if pd, ok := pm.paths[pa.name]; !ok || pd.path != pa {
		return
	}
	pm.removePath(pa)
}

func (pm *pathManager) doPathReady(pa *path) {
	if pd, ok := pm.paths[pa.name]; !ok || pd.path != pa {
		return
	}

	pm.paths[pa.name].ready = true

	if pm.hlsServer != nil {
		pm.hlsServer.PathReady(pa)
	}
}

func (pm *pathManager) doPathNotReady(pa *path) {
	if pd, ok := pm.paths[pa.name]; !ok || pd.path != pa {
		return
	}

	pm.paths[pa.name].ready = false

	if pm.hlsServer != nil {
		pm.hlsServer.PathNotReady(pa)
	}
}

func (pm *pathManager) doFindPathConf(req defs.PathFindPathConfReq) {
	pathConf, _, err := conf.FindPathConf(pm.pathConfs, req.AccessRequest.Name)
	if err != nil {
		req.Res <- defs.PathFindPathConfRes{Err: err}
		return
	}

	err2 := pm.authManager.Authenticate(req.AccessRequest.ToAuthRequest())
	if err2 != nil {
		req.Res <- defs.PathFindPathConfRes{Err: err2}
		return
	}

	req.Res <- defs.PathFindPathConfRes{Conf: pathConf}
}

func (pm *pathManager) doDescribe(req defs.PathDescribeReq) {
	pathConf, pathMatches, err := conf.FindPathConf(pm.pathConfs, req.AccessRequest.Name)
	if err != nil {
		req.Res <- defs.PathDescribeRes{Err: err}
		return
	}

	err2 := pm.authManager.Authenticate(req.AccessRequest.ToAuthRequest())
	if err2 != nil {
		req.Res <- defs.PathDescribeRes{Err: err2}
		return
	}

	// create path if it doesn't exist
	if _, ok := pm.paths[req.AccessRequest.Name]; !ok {
		pm.createPath(pathConf, req.AccessRequest.Name, pathMatches)
	}

	pd := pm.paths[req.AccessRequest.Name]
	req.Res <- defs.PathDescribeRes{Path: pd.path}
}

func (pm *pathManager) doAddReader(req defs.PathAddReaderReq) {
	pathConf, pathMatches, err := conf.FindPathConf(pm.pathConfs, req.AccessRequest.Name)
	if err != nil {
		req.Res <- defs.PathAddReaderRes{Err: err}
		return
	}

	if !req.AccessRequest.SkipAuth {
		err2 := pm.authManager.Authenticate(req.AccessRequest.ToAuthRequest())
		if err2 != nil {
			req.Res <- defs.PathAddReaderRes{Err: err2}
			return
		}
	}

	// create path if it doesn't exist
	if _, ok := pm.paths[req.AccessRequest.Name]; !ok {
		pm.createPath(pathConf, req.AccessRequest.Name, pathMatches)
	}

	pd := pm.paths[req.AccessRequest.Name]
	req.Res <- defs.PathAddReaderRes{Path: pd.path}
}

func (pm *pathManager) doAddPublisher(req defs.PathAddPublisherReq) {
	pathConf, pathMatches, err := conf.FindPathConf(pm.pathConfs, req.AccessRequest.Name)
	if err != nil {
		req.Res <- defs.PathAddPublisherRes{Err: err}
		return
	}

	if req.ConfToCompare != nil && !pathConf.Equal(req.ConfToCompare) {
		req.Res <- defs.PathAddPublisherRes{Err: fmt.Errorf("configuration has changed")}
		return
	}

	if !req.AccessRequest.SkipAuth {
		err2 := pm.authManager.Authenticate(req.AccessRequest.ToAuthRequest())
		if err2 != nil {
			req.Res <- defs.PathAddPublisherRes{Err: err2}
			return
		}
	}

	// create path if it doesn't exist
	if _, ok := pm.paths[req.AccessRequest.Name]; !ok {
		pm.createPath(pathConf, req.AccessRequest.Name, pathMatches)
	}

	pd := pm.paths[req.AccessRequest.Name]
	req.Res <- defs.PathAddPublisherRes{Path: pd.path}
}

func (pm *pathManager) doAPIPathsList(req pathAPIPathsListReq) {
	paths := make(map[string]*path)

	for name, pd := range pm.paths {
		paths[name] = pd.path
	}

	req.res <- pathAPIPathsListRes{paths: paths}
}

func (pm *pathManager) doAPIPathsGet(req pathAPIPathsGetReq) {
	pd, ok := pm.paths[req.name]
	if !ok {
		req.res <- pathAPIPathsGetRes{err: conf.ErrPathNotFound}
		return
	}

	req.res <- pathAPIPathsGetRes{path: pd.path}
}

func (pm *pathManager) createPath(
	pathConf *conf.Path,
	name string,
	matches []string,
) {
	pa := &path{
		parentCtx:         pm.ctx,
		logLevel:          pm.logLevel,
		rtspAddress:       pm.rtspAddress,
		readTimeout:       pm.readTimeout,
		writeTimeout:      pm.writeTimeout,
		writeQueueSize:    pm.writeQueueSize,
		udpReadBufferSize: pm.udpReadBufferSize,
		rtpMaxPayloadSize: pm.rtpMaxPayloadSize,
		conf:              pathConf,
		name:              name,
		matches:           matches,
		wg:                &pm.wg,
		externalCmdPool:   pm.externalCmdPool,
		parent:            pm,
	}
	pa.initialize()

	pm.paths[name] = &pathData{
		path:     pa,
		confName: pathConf.Name,
	}
}

func (pm *pathManager) removePath(pa *path) {
	delete(pm.paths, pa.name)
}

// ReloadPathConfs is called by core.
func (pm *pathManager) ReloadPathConfs(pathConfs map[string]*conf.Path) {
	select {
	case pm.chReloadConf <- pathConfs:
	case <-pm.ctx.Done():
	}
}

// pathReady is called by path.
func (pm *pathManager) pathReady(pa *path) {
	select {
	case pm.chPathReady <- pa:
	case <-pm.ctx.Done():
	case <-pa.ctx.Done(): // in case pathManager is blocked by path.wait()
	}
}

// pathNotReady is called by path.
func (pm *pathManager) pathNotReady(pa *path) {
	select {
	case pm.chPathNotReady <- pa:
	case <-pm.ctx.Done():
	case <-pa.ctx.Done(): // in case pathManager is blocked by path.wait()
	}
}

// closePath is called by path.
func (pm *pathManager) closePath(pa *path) {
	select {
	case pm.chClosePath <- pa:
	case <-pm.ctx.Done():
	case <-pa.ctx.Done(): // in case pathManager is blocked by path.wait()
	}
}

// FindPathConf is called by a reader or publisher.
func (pm *pathManager) FindPathConf(req defs.PathFindPathConfReq) (*conf.Path, error) {
	req.Res = make(chan defs.PathFindPathConfRes)
	select {
	case pm.chFindPathConf <- req:
		res := <-req.Res
		return res.Conf, res.Err

	case <-pm.ctx.Done():
		return nil, fmt.Errorf("terminated")
	}
}

// Describe is called by a reader or publisher.
func (pm *pathManager) Describe(req defs.PathDescribeReq) defs.PathDescribeRes {
	req.Res = make(chan defs.PathDescribeRes)
	select {
	case pm.chDescribe <- req:
		res1 := <-req.Res
		if res1.Err != nil {
			return res1
		}

		res2 := res1.Path.(*path).describe(req)
		if res2.Err != nil {
			return res2
		}

		res2.Path = res1.Path
		return res2

	case <-pm.ctx.Done():
		return defs.PathDescribeRes{Err: fmt.Errorf("terminated")}
	}
}

// AddPublisher is called by a publisher.
func (pm *pathManager) AddPublisher(req defs.PathAddPublisherReq) (defs.Path, *stream.Stream, error) {
	req.Res = make(chan defs.PathAddPublisherRes)
	select {
	case pm.chAddPublisher <- req:
		res := <-req.Res
		if res.Err != nil {
			return nil, nil, res.Err
		}

		return res.Path.(*path).addPublisher(req)

	case <-pm.ctx.Done():
		return nil, nil, fmt.Errorf("terminated")
	}
}

// AddReader is called by a reader.
func (pm *pathManager) AddReader(req defs.PathAddReaderReq) (defs.Path, *stream.Stream, error) {
	req.Res = make(chan defs.PathAddReaderRes)
	select {
	case pm.chAddReader <- req:
		res := <-req.Res
		if res.Err != nil {
			return nil, nil, res.Err
		}

		return res.Path.(*path).addReader(req)

	case <-pm.ctx.Done():
		return nil, nil, fmt.Errorf("terminated")
	}
}

// SetHLSServer is called by hls.Server.
func (pm *pathManager) SetHLSServer(s *hls.Server) []defs.Path {
	req := pathSetHLSServerReq{
		s:   s,
		res: make(chan pathSetHLSServerRes),
	}

	select {
	case pm.chSetHLSServer <- req:
		res := <-req.res
		return res.readyPaths

	case <-pm.ctx.Done():
		return nil
	}
}

// APIPathsList is called by api.
func (pm *pathManager) APIPathsList() (*defs.APIPathList, error) {
	req := pathAPIPathsListReq{
		res: make(chan pathAPIPathsListRes),
	}

	select {
	case pm.chAPIPathsList <- req:
		res := <-req.res

		res.data = &defs.APIPathList{
			Items: []defs.APIPath{},
		}

		for _, pa := range res.paths {
			item, err := pa.APIPathsGet(pathAPIPathsGetReq{})
			if err == nil {
				res.data.Items = append(res.data.Items, *item)
			}
		}

		sort.Slice(res.data.Items, func(i, j int) bool {
			return res.data.Items[i].Name < res.data.Items[j].Name
		})

		return res.data, nil

	case <-pm.ctx.Done():
		return nil, fmt.Errorf("terminated")
	}
}

// APIPathsGet is called by api.
func (pm *pathManager) APIPathsGet(name string) (*defs.APIPath, error) {
	req := pathAPIPathsGetReq{
		name: name,
		res:  make(chan pathAPIPathsGetRes),
	}

	select {
	case pm.chAPIPathsGet <- req:
		res := <-req.res
		if res.err != nil {
			return nil, res.err
		}

		data, err := res.path.APIPathsGet(req)
		return data, err

	case <-pm.ctx.Done():
		return nil, fmt.Errorf("terminated")
	}
}
