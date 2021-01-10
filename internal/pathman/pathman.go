package pathman

import (
	"fmt"
	"sync"
	"time"

	"github.com/aler9/gortsplib"
	"github.com/aler9/gortsplib/pkg/base"
	"github.com/aler9/gortsplib/pkg/headers"

	"github.com/aler9/rtsp-simple-server/internal/client"
	"github.com/aler9/rtsp-simple-server/internal/conf"
	"github.com/aler9/rtsp-simple-server/internal/logger"
	"github.com/aler9/rtsp-simple-server/internal/path"
	"github.com/aler9/rtsp-simple-server/internal/stats"
)

// Parent is implemented by program.
type Parent interface {
	Log(logger.Level, string, ...interface{})
}

// PathManager is a path.Path manager.
type PathManager struct {
	rtspPort        int
	readTimeout     time.Duration
	writeTimeout    time.Duration
	readBufferCount uint64
	authMethods     []headers.AuthMethod
	pathConfs       map[string]*conf.PathConf
	stats           *stats.Stats
	parent          Parent

	paths map[string]*path.Path
	wg    sync.WaitGroup

	// in
	confReload      chan map[string]*conf.PathConf
	pathClose       chan *path.Path
	clientDescribe  chan path.ClientDescribeReq
	clientAnnounce  chan path.ClientAnnounceReq
	clientSetupPlay chan path.ClientSetupPlayReq
	terminate       chan struct{}

	// out
	clientClose chan *client.Client
	done        chan struct{}
}

// New allocates a PathManager.
func New(
	rtspPort int,
	readTimeout time.Duration,
	writeTimeout time.Duration,
	readBufferCount uint64,
	authMethods []headers.AuthMethod,
	pathConfs map[string]*conf.PathConf,
	stats *stats.Stats,
	parent Parent) *PathManager {

	pm := &PathManager{
		rtspPort:        rtspPort,
		readTimeout:     readTimeout,
		writeTimeout:    writeTimeout,
		readBufferCount: readBufferCount,
		authMethods:     authMethods,
		pathConfs:       pathConfs,
		stats:           stats,
		parent:          parent,
		paths:           make(map[string]*path.Path),
		confReload:      make(chan map[string]*conf.PathConf),
		pathClose:       make(chan *path.Path),
		clientDescribe:  make(chan path.ClientDescribeReq),
		clientAnnounce:  make(chan path.ClientAnnounceReq),
		clientSetupPlay: make(chan path.ClientSetupPlayReq),
		terminate:       make(chan struct{}),
		clientClose:     make(chan *client.Client),
		done:            make(chan struct{}),
	}

	pm.createPaths()

	go pm.run()
	return pm
}

// Close closes a PathManager.
func (pm *PathManager) Close() {
	go func() {
		for range pm.clientClose {
		}
	}()
	close(pm.terminate)
	<-pm.done
}

// Log is the main logging function.
func (pm *PathManager) Log(level logger.Level, format string, args ...interface{}) {
	pm.parent.Log(level, format, args...)
}

func (pm *PathManager) run() {
	defer close(pm.done)

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
			if _, ok := pm.paths[pa.Name()]; !ok {
				continue
			}
			delete(pm.paths, pa.Name())
			pa.Close()

		case req := <-pm.clientDescribe:
			pathName, pathConf, err := pm.findPathConf(req.PathName)
			if err != nil {
				req.Res <- path.ClientDescribeRes{nil, err} //nolint:govet
				continue
			}

			err = req.Client.Authenticate(pm.authMethods, pathConf.ReadIpsParsed,
				pathConf.ReadUser, pathConf.ReadPass, req.Req)
			if err != nil {
				req.Res <- path.ClientDescribeRes{nil, err} //nolint:govet
				continue
			}

			// create path if it doesn't exist
			if _, ok := pm.paths[req.PathName]; !ok {
				pa := path.New(
					pm.rtspPort,
					pm.readTimeout,
					pm.writeTimeout,
					pm.readBufferCount,
					pathName,
					pathConf,
					req.PathName,
					&pm.wg,
					pm.stats,
					pm)
				pm.paths[req.PathName] = pa
			}

			pm.paths[req.PathName].OnPathManDescribe(req)

		case req := <-pm.clientAnnounce:
			pathName, pathConf, err := pm.findPathConf(req.PathName)
			if err != nil {
				req.Res <- path.ClientAnnounceRes{nil, err} //nolint:govet
				continue
			}

			err = req.Client.Authenticate(pm.authMethods,
				pathConf.PublishIpsParsed, pathConf.PublishUser, pathConf.PublishPass, req.Req)
			if err != nil {
				req.Res <- path.ClientAnnounceRes{nil, err} //nolint:govet
				continue
			}

			// create path if it doesn't exist
			if _, ok := pm.paths[req.PathName]; !ok {
				pa := path.New(
					pm.rtspPort,
					pm.readTimeout,
					pm.writeTimeout,
					pm.readBufferCount,
					pathName,
					pathConf,
					req.PathName,
					&pm.wg,
					pm.stats,
					pm)
				pm.paths[req.PathName] = pa
			}

			pm.paths[req.PathName].OnPathManAnnounce(req)

		case req := <-pm.clientSetupPlay:
			if _, ok := pm.paths[req.PathName]; !ok {
				req.Res <- path.ClientSetupPlayRes{nil, fmt.Errorf("no one is publishing to path '%s'", req.PathName)} //nolint:govet
				continue
			}

			_, pathConf, err := pm.findPathConf(req.PathName)
			if err != nil {
				req.Res <- path.ClientSetupPlayRes{nil, err} //nolint:govet
				continue
			}

			err = req.Client.Authenticate(pm.authMethods,
				pathConf.ReadIpsParsed, pathConf.ReadUser, pathConf.ReadPass, req.Req)
			if err != nil {
				req.Res <- path.ClientSetupPlayRes{nil, err} //nolint:govet
				continue
			}

			pm.paths[req.PathName].OnPathManSetupPlay(req)

		case <-pm.terminate:
			break outer
		}
	}

	go func() {
		for {
			select {
			case _, ok := <-pm.confReload:
				if !ok {
					return
				}

			case _, ok := <-pm.pathClose:
				if !ok {
					return
				}

			case req := <-pm.clientDescribe:
				req.Res <- path.ClientDescribeRes{nil, fmt.Errorf("terminated")} //nolint:govet

			case req := <-pm.clientAnnounce:
				req.Res <- path.ClientAnnounceRes{nil, fmt.Errorf("terminated")} //nolint:govet

			case req := <-pm.clientSetupPlay:
				req.Res <- path.ClientSetupPlayRes{nil, fmt.Errorf("terminated")} //nolint:govet
			}
		}
	}()

	for _, pa := range pm.paths {
		pa.Close()
	}
	pm.wg.Wait()

	close(pm.confReload)
	close(pm.clientClose)
	close(pm.pathClose)
	close(pm.clientDescribe)
	close(pm.clientAnnounce)
	close(pm.clientSetupPlay)
}

func (pm *PathManager) createPaths() {
	for pathName, pathConf := range pm.pathConfs {
		if _, ok := pm.paths[pathName]; !ok && pathConf.Regexp == nil {
			pa := path.New(
				pm.rtspPort,
				pm.readTimeout,
				pm.writeTimeout,
				pm.readBufferCount,
				pathName,
				pathConf,
				pathName,
				&pm.wg,
				pm.stats,
				pm)
			pm.paths[pathName] = pa
		}
	}
}

func (pm *PathManager) findPathConf(name string) (string, *conf.PathConf, error) {
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

// OnProgramConfReload is called by program.
func (pm *PathManager) OnProgramConfReload(pathConfs map[string]*conf.PathConf) {
	pm.confReload <- pathConfs
}

// OnPathClose is called by path.Path.
func (pm *PathManager) OnPathClose(pa *path.Path) {
	pm.pathClose <- pa
}

// OnPathClientClose is called by path.Path.
func (pm *PathManager) OnPathClientClose(c *client.Client) {
	pm.clientClose <- c
}

// OnClientDescribe is called by client.Client.
func (pm *PathManager) OnClientDescribe(c *client.Client, pathName string, req *base.Request) (client.Path, error) {
	res := make(chan path.ClientDescribeRes)
	pm.clientDescribe <- path.ClientDescribeReq{res, c, pathName, req} //nolint:govet
	re := <-res
	return re.Path, re.Err
}

// OnClientAnnounce is called by client.Client.
func (pm *PathManager) OnClientAnnounce(c *client.Client, pathName string, tracks gortsplib.Tracks, req *base.Request) (client.Path, error) {
	res := make(chan path.ClientAnnounceRes)
	pm.clientAnnounce <- path.ClientAnnounceReq{res, c, pathName, tracks, req} //nolint:govet
	re := <-res
	return re.Path, re.Err
}

// OnClientSetupPlay is called by client.Client.
func (pm *PathManager) OnClientSetupPlay(c *client.Client, pathName string, trackID int, req *base.Request) (client.Path, error) {
	res := make(chan path.ClientSetupPlayRes)
	pm.clientSetupPlay <- path.ClientSetupPlayReq{res, c, pathName, trackID, req} //nolint:govet
	re := <-res
	return re.Path, re.Err
}

// ClientClose is called by client.Client.
func (pm *PathManager) ClientClose() chan *client.Client {
	return pm.clientClose
}
