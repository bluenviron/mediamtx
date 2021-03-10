package pathman

import (
	"fmt"
	"sync"
	"time"

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
	readBufferCount int
	readBufferSize  int
	authMethods     []headers.AuthMethod
	pathConfs       map[string]*conf.PathConf
	stats           *stats.Stats
	parent          Parent

	paths map[string]*path.Path
	wg    sync.WaitGroup

	// in
	confReload      chan map[string]*conf.PathConf
	pathClose       chan *path.Path
	clientDescribe  chan client.DescribeReq
	clientSetupPlay chan client.SetupPlayReq
	clientAnnounce  chan client.AnnounceReq
	terminate       chan struct{}

	// out
	clientClose chan client.Client
	done        chan struct{}
}

// New allocates a PathManager.
func New(
	rtspPort int,
	readTimeout time.Duration,
	writeTimeout time.Duration,
	readBufferCount int,
	readBufferSize int,
	authMethods []headers.AuthMethod,
	pathConfs map[string]*conf.PathConf,
	stats *stats.Stats,
	parent Parent) *PathManager {

	pm := &PathManager{
		rtspPort:        rtspPort,
		readTimeout:     readTimeout,
		writeTimeout:    writeTimeout,
		readBufferCount: readBufferCount,
		readBufferSize:  readBufferSize,
		authMethods:     authMethods,
		pathConfs:       pathConfs,
		stats:           stats,
		parent:          parent,
		paths:           make(map[string]*path.Path),
		confReload:      make(chan map[string]*conf.PathConf),
		pathClose:       make(chan *path.Path),
		clientDescribe:  make(chan client.DescribeReq),
		clientSetupPlay: make(chan client.SetupPlayReq),
		clientAnnounce:  make(chan client.AnnounceReq),
		terminate:       make(chan struct{}),
		clientClose:     make(chan client.Client),
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
				req.Res <- client.DescribeRes{nil, "", err} //nolint:govet
				continue
			}

			err = req.Client.Authenticate(
				pm.authMethods,
				req.PathName,
				pathConf.ReadIpsParsed,
				pathConf.ReadUser,
				pathConf.ReadPass,
				req.Data)
			if err != nil {
				req.Res <- client.DescribeRes{nil, "", err} //nolint:govet
				continue
			}

			// create path if it doesn't exist
			if _, ok := pm.paths[req.PathName]; !ok {
				pm.createPath(pathName, pathConf, req.PathName)
			}

			pm.paths[req.PathName].OnPathManDescribe(req)

		case req := <-pm.clientSetupPlay:
			pathName, pathConf, err := pm.findPathConf(req.PathName)
			if err != nil {
				req.Res <- client.SetupPlayRes{nil, nil, err} //nolint:govet
				continue
			}

			err = req.Client.Authenticate(
				pm.authMethods,
				req.PathName,
				pathConf.ReadIpsParsed,
				pathConf.ReadUser,
				pathConf.ReadPass,
				req.Data)
			if err != nil {
				req.Res <- client.SetupPlayRes{nil, nil, err} //nolint:govet
				continue
			}

			// create path if it doesn't exist
			if _, ok := pm.paths[req.PathName]; !ok {
				pm.createPath(pathName, pathConf, req.PathName)
			}

			pm.paths[req.PathName].OnPathManSetupPlay(req)

		case req := <-pm.clientAnnounce:
			pathName, pathConf, err := pm.findPathConf(req.PathName)
			if err != nil {
				req.Res <- client.AnnounceRes{nil, err} //nolint:govet
				continue
			}

			err = req.Client.Authenticate(
				pm.authMethods,
				req.PathName,
				pathConf.PublishIpsParsed,
				pathConf.PublishUser,
				pathConf.PublishPass,
				req.Data)
			if err != nil {
				req.Res <- client.AnnounceRes{nil, err} //nolint:govet
				continue
			}

			// create path if it doesn't exist
			if _, ok := pm.paths[req.PathName]; !ok {
				pm.createPath(pathName, pathConf, req.PathName)
			}

			pm.paths[req.PathName].OnPathManAnnounce(req)

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

			case req, ok := <-pm.clientDescribe:
				if !ok {
					return
				}
				req.Res <- client.DescribeRes{nil, "", fmt.Errorf("terminated")} //nolint:govet

			case req, ok := <-pm.clientSetupPlay:
				if !ok {
					return
				}
				req.Res <- client.SetupPlayRes{nil, nil, fmt.Errorf("terminated")} //nolint:govet

			case req, ok := <-pm.clientAnnounce:
				if !ok {
					return
				}
				req.Res <- client.AnnounceRes{nil, fmt.Errorf("terminated")} //nolint:govet
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
	close(pm.clientSetupPlay)
	close(pm.clientAnnounce)
}

func (pm *PathManager) createPath(confName string, conf *conf.PathConf, name string) {
	pm.paths[name] = path.New(
		pm.rtspPort,
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

func (pm *PathManager) createPaths() {
	for pathName, pathConf := range pm.pathConfs {
		if _, ok := pm.paths[pathName]; !ok && pathConf.Regexp == nil {
			pm.createPath(pathName, pathConf, pathName)
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
func (pm *PathManager) OnPathClientClose(c client.Client) {
	pm.clientClose <- c
}

// OnClientDescribe is called by clientman.ClientMan.
func (pm *PathManager) OnClientDescribe(req client.DescribeReq) {
	pm.clientDescribe <- req
}

// OnClientAnnounce is called by clientman.ClientMan.
func (pm *PathManager) OnClientAnnounce(req client.AnnounceReq) {
	pm.clientAnnounce <- req
}

// OnClientSetupPlay is called by clientman.ClientMan.
func (pm *PathManager) OnClientSetupPlay(req client.SetupPlayReq) {
	pm.clientSetupPlay <- req
}

// ClientClose is called by clientman.ClientMan.
func (pm *PathManager) ClientClose() chan client.Client {
	return pm.clientClose
}
