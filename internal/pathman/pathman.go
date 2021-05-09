package pathman

import (
	"fmt"
	"net"
	"sync"
	"time"

	"github.com/aler9/gortsplib/pkg/base"
	"github.com/aler9/gortsplib/pkg/headers"

	"github.com/aler9/rtsp-simple-server/internal/conf"
	"github.com/aler9/rtsp-simple-server/internal/logger"
	"github.com/aler9/rtsp-simple-server/internal/path"
	"github.com/aler9/rtsp-simple-server/internal/readpublisher"
	"github.com/aler9/rtsp-simple-server/internal/stats"
)

func ipEqualOrInRange(ip net.IP, ips []interface{}) bool {
	for _, item := range ips {
		switch titem := item.(type) {
		case net.IP:
			if titem.Equal(ip) {
				return true
			}

		case *net.IPNet:
			if titem.Contains(ip) {
				return true
			}
		}
	}
	return false
}

// Parent is implemented by program.
type Parent interface {
	Log(logger.Level, string, ...interface{})
}

// PathManager is a path.Path manager.
type PathManager struct {
	rtspAddress     string
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
	confReload  chan map[string]*conf.PathConf
	pathClose   chan *path.Path
	rpDescribe  chan readpublisher.DescribeReq
	rpSetupPlay chan readpublisher.SetupPlayReq
	rpAnnounce  chan readpublisher.AnnounceReq
	terminate   chan struct{}

	// out
	done chan struct{}
}

// New allocates a PathManager.
func New(
	rtspAddress string,
	readTimeout time.Duration,
	writeTimeout time.Duration,
	readBufferCount int,
	readBufferSize int,
	authMethods []headers.AuthMethod,
	pathConfs map[string]*conf.PathConf,
	stats *stats.Stats,
	parent Parent) *PathManager {

	pm := &PathManager{
		rtspAddress:     rtspAddress,
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
		rpDescribe:      make(chan readpublisher.DescribeReq),
		rpSetupPlay:     make(chan readpublisher.SetupPlayReq),
		rpAnnounce:      make(chan readpublisher.AnnounceReq),
		terminate:       make(chan struct{}),
		done:            make(chan struct{}),
	}

	pm.createPaths()

	go pm.run()
	return pm
}

// Close closes a PathManager.
func (pm *PathManager) Close() {
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

		case req := <-pm.rpDescribe:
			pathName, pathConf, err := pm.findPathConf(req.PathName)
			if err != nil {
				req.Res <- readpublisher.DescribeRes{nil, "", err} //nolint:govet
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
				req.Res <- readpublisher.DescribeRes{nil, "", err} //nolint:govet
				continue
			}

			// create path if it doesn't exist
			if _, ok := pm.paths[req.PathName]; !ok {
				pm.createPath(pathName, pathConf, req.PathName)
			}

			pm.paths[req.PathName].OnPathManDescribe(req)

		case req := <-pm.rpSetupPlay:
			pathName, pathConf, err := pm.findPathConf(req.PathName)
			if err != nil {
				req.Res <- readpublisher.SetupPlayRes{nil, nil, err} //nolint:govet
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
				req.Res <- readpublisher.SetupPlayRes{nil, nil, err} //nolint:govet
				continue
			}

			// create path if it doesn't exist
			if _, ok := pm.paths[req.PathName]; !ok {
				pm.createPath(pathName, pathConf, req.PathName)
			}

			pm.paths[req.PathName].OnPathManSetupPlay(req)

		case req := <-pm.rpAnnounce:
			pathName, pathConf, err := pm.findPathConf(req.PathName)
			if err != nil {
				req.Res <- readpublisher.AnnounceRes{nil, err} //nolint:govet
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
				req.Res <- readpublisher.AnnounceRes{nil, err} //nolint:govet
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

			case req, ok := <-pm.rpDescribe:
				if !ok {
					return
				}
				req.Res <- readpublisher.DescribeRes{nil, "", fmt.Errorf("terminated")} //nolint:govet

			case req, ok := <-pm.rpSetupPlay:
				if !ok {
					return
				}
				req.Res <- readpublisher.SetupPlayRes{nil, nil, fmt.Errorf("terminated")} //nolint:govet

			case req, ok := <-pm.rpAnnounce:
				if !ok {
					return
				}
				req.Res <- readpublisher.AnnounceRes{nil, fmt.Errorf("terminated")} //nolint:govet
			}
		}
	}()

	for _, pa := range pm.paths {
		pa.Close()
	}
	pm.wg.Wait()

	close(pm.confReload)
	close(pm.pathClose)
	close(pm.rpDescribe)
	close(pm.rpSetupPlay)
	close(pm.rpAnnounce)
}

func (pm *PathManager) createPath(confName string, conf *conf.PathConf, name string) {
	pm.paths[name] = path.New(
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

// OnReadPublisherDescribe is called by a ReadPublisher.
func (pm *PathManager) OnReadPublisherDescribe(req readpublisher.DescribeReq) {
	pm.rpDescribe <- req
}

// OnReadPublisherAnnounce is called by a ReadPublisher.
func (pm *PathManager) OnReadPublisherAnnounce(req readpublisher.AnnounceReq) {
	pm.rpAnnounce <- req
}

// OnReadPublisherSetupPlay is called by a ReadPublisher.
func (pm *PathManager) OnReadPublisherSetupPlay(req readpublisher.SetupPlayReq) {
	pm.rpSetupPlay <- req
}

func (pm *PathManager) authenticate(
	ip net.IP,
	validateCredentials func(authMethods []headers.AuthMethod, pathUser string, pathPass string) error,
	pathName string,
	pathIPs []interface{},
	pathUser string,
	pathPass string,
) error {

	// validate ip
	if pathIPs != nil && ip != nil {
		if !ipEqualOrInRange(ip, pathIPs) {
			return readpublisher.ErrAuthCritical{
				Message: fmt.Sprintf("IP '%s' not allowed", ip),
				Response: &base.Response{
					StatusCode: base.StatusUnauthorized,
				},
			}
		}
	}

	// validate user
	if pathUser != "" && validateCredentials != nil {
		err := validateCredentials(pm.authMethods, pathUser, pathPass)
		if err != nil {
			return err
		}
	}

	return nil
}
