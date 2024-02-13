package defs

import (
	"github.com/bluenviron/mediamtx/internal/conf"
	"github.com/bluenviron/mediamtx/internal/stream"
)

// PathManager is a path manager.
type PathManager interface {
	FindPathConf(req PathFindPathConfReq) (*conf.Path, error)
	Describe(req PathDescribeReq) PathDescribeRes
	AddPublisher(req PathAddPublisherReq) (Path, error)
	AddReader(req PathAddReaderReq) (Path, *stream.Stream, error)
}
