package defs

import (
	"fmt"

	"github.com/bluenviron/gortsplib/v4/pkg/description"

	"github.com/bluenviron/mediamtx/internal/conf"
	"github.com/bluenviron/mediamtx/internal/externalcmd"
	"github.com/bluenviron/mediamtx/internal/stream"
)

// PathNoStreamAvailableError is returned when no one is publishing.
type PathNoStreamAvailableError struct {
	PathName string
}

// Error implements the error interface.
func (e PathNoStreamAvailableError) Error() string {
	return fmt.Sprintf("no stream is available on path '%s'", e.PathName)
}

// Path is a path.
type Path interface {
	Name() string
	SafeConf() *conf.Path
	ExternalCmdEnv() externalcmd.Environment
	RemovePublisher(req PathRemovePublisherReq)
	RemoveReader(req PathRemoveReaderReq)
}

// PathFindPathConfRes contains the response of FindPathConf().
type PathFindPathConfRes struct {
	Conf *conf.Path
	Err  error
}

// PathFindPathConfReq contains arguments of FindPathConf().
type PathFindPathConfReq struct {
	AccessRequest PathAccessRequest
	Res           chan PathFindPathConfRes
}

// PathDescribeRes contains the response of Describe().
type PathDescribeRes struct {
	Path     Path
	Stream   *stream.Stream
	Redirect string
	Err      error
}

// PathDescribeReq contains arguments of Describe().
type PathDescribeReq struct {
	AccessRequest PathAccessRequest
	Res           chan PathDescribeRes
}

// PathAddPublisherRes contains the response of AddPublisher().
type PathAddPublisherRes struct {
	Path   Path
	Stream *stream.Stream
	Err    error
}

// PathAddPublisherReq contains arguments of AddPublisher().
type PathAddPublisherReq struct {
	Author             Publisher
	Desc               *description.Session
	GenerateRTPPackets bool
	ConfToCompare      *conf.Path
	AccessRequest      PathAccessRequest
	Res                chan PathAddPublisherRes
}

// PathRemovePublisherReq contains arguments of RemovePublisher().
type PathRemovePublisherReq struct {
	Author Publisher
	Res    chan struct{}
}

// PathAddReaderRes contains the response of AddReader().
type PathAddReaderRes struct {
	Path   Path
	Stream *stream.Stream
	Err    error
}

// PathAddReaderReq contains arguments of AddReader().
type PathAddReaderReq struct {
	Author        Reader
	AccessRequest PathAccessRequest
	Res           chan PathAddReaderRes
}

// PathRemoveReaderReq contains arguments of RemoveReader().
type PathRemoveReaderReq struct {
	Author Reader
	Res    chan struct{}
}

// PathSourceStaticSetReadyRes contains the response of SetReadu().
type PathSourceStaticSetReadyRes struct {
	Stream *stream.Stream
	Err    error
}

// PathSourceStaticSetReadyReq contains arguments of SetReady().
type PathSourceStaticSetReadyReq struct {
	Desc               *description.Session
	GenerateRTPPackets bool
	Res                chan PathSourceStaticSetReadyRes
}

// PathSourceStaticSetNotReadyReq contains arguments of SetNotReady().
type PathSourceStaticSetNotReadyReq struct {
	Res chan struct{}
}
