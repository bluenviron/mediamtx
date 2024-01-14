package defs

// PathManager is a path manager.
type PathManager interface {
	FindPathConf(req PathFindPathConfReq) PathFindPathConfRes
	Describe(req PathDescribeReq) PathDescribeRes
	AddPublisher(req PathAddPublisherReq) PathAddPublisherRes
	AddReader(req PathAddReaderReq) PathAddReaderRes
}
