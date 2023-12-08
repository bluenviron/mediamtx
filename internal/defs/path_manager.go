package defs

// PathManager is a path manager.
type PathManager interface {
	GetConfForPath(req PathGetConfForPathReq) PathGetConfForPathRes
	Describe(req PathDescribeReq) PathDescribeRes
	AddPublisher(req PathAddPublisherReq) PathAddPublisherRes
	AddReader(req PathAddReaderReq) PathAddReaderRes
}
