package test

import (
	"github.com/bluenviron/mediamtx/internal/defs"
)

// PathManager is a dummy path manager.
type PathManager struct {
	FindPathConfImpl func(req defs.PathFindPathConfReq) (*defs.PathFindPathConfRes, error)
	DescribeImpl     func(req defs.PathDescribeReq) defs.PathDescribeRes
	AddPublisherImpl func(req defs.PathAddPublisherReq) (*defs.PathAddPublisherRes, error)
	AddReaderImpl    func(req defs.PathAddReaderReq) (*defs.PathAddReaderRes, error)
}

// FindPathConf implements PathManager.
func (pm *PathManager) FindPathConf(req defs.PathFindPathConfReq) (*defs.PathFindPathConfRes, error) {
	return pm.FindPathConfImpl(req)
}

// Describe implements PathManager.
func (pm *PathManager) Describe(req defs.PathDescribeReq) defs.PathDescribeRes {
	return pm.DescribeImpl(req)
}

// AddPublisher implements PathManager.
func (pm *PathManager) AddPublisher(req defs.PathAddPublisherReq) (*defs.PathAddPublisherRes, error) {
	return pm.AddPublisherImpl(req)
}

// AddReader implements PathManager.
func (pm *PathManager) AddReader(req defs.PathAddReaderReq) (*defs.PathAddReaderRes, error) {
	return pm.AddReaderImpl(req)
}
