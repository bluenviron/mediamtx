package core

import (
	"fmt"
	"net"

	"github.com/aler9/gortsplib"
	"github.com/aler9/gortsplib/pkg/base"
	"github.com/aler9/gortsplib/pkg/headers"

	"github.com/aler9/rtsp-simple-server/internal/conf"
)

type readPublisherPath interface {
	Name() string
	Conf() *conf.PathConf
	OnReadPublisherRemove(readPublisherRemoveReq)
	OnReadPublisherPlay(readPublisherPlayReq)
	OnReadPublisherRecord(readPublisherRecordReq)
	OnReadPublisherPause(readPublisherPauseReq)
	OnSourceFrame(int, gortsplib.StreamType, []byte)
}

type readPublisherErrNoOnePublishing struct {
	PathName string
}

// Error implements the error interface.
func (e readPublisherErrNoOnePublishing) Error() string {
	return fmt.Sprintf("no one is publishing to path '%s'", e.PathName)
}

type readPublisherErrAuthNotCritical struct {
	*base.Response
}

// Error implements the error interface.
func (readPublisherErrAuthNotCritical) Error() string {
	return "non-critical authentication error"
}

type readPublisherErrAuthCritical struct {
	Message  string
	Response *base.Response
}

// Error implements the error interface.
func (readPublisherErrAuthCritical) Error() string {
	return "critical authentication error"
}

type readPublisher interface {
	IsReadPublisher()
	IsSource()
	Close()
	OnReaderAccepted()
	OnPublisherAccepted(tracksLen int)
	OnFrame(int, gortsplib.StreamType, []byte)
}

type readPublisherDescribeRes struct {
	Stream   *gortsplib.ServerStream
	Redirect string
	Err      error
}

type readPublisherDescribeReq struct {
	PathName            string
	URL                 *base.URL
	IP                  net.IP
	ValidateCredentials func(authMethods []headers.AuthMethod, pathUser string, pathPass string) error
	Res                 chan readPublisherDescribeRes
}

type readPublisherSetupPlayRes struct {
	Path   readPublisherPath
	Stream *gortsplib.ServerStream
	Err    error
}

type readPublisherSetupPlayReq struct {
	Author              readPublisher
	PathName            string
	IP                  net.IP
	ValidateCredentials func(authMethods []headers.AuthMethod, pathUser string, pathPass string) error
	Res                 chan readPublisherSetupPlayRes
}

type readPublisherAnnounceRes struct {
	Path readPublisherPath
	Err  error
}

type readPublisherAnnounceReq struct {
	Author              readPublisher
	PathName            string
	Tracks              gortsplib.Tracks
	IP                  net.IP
	ValidateCredentials func(authMethods []headers.AuthMethod, pathUser string, pathPass string) error
	Res                 chan readPublisherAnnounceRes
}

type readPublisherRemoveReq struct {
	Author readPublisher
	Res    chan struct{}
}

type readPublisherPlayRes struct{}

type readPublisherPlayReq struct {
	Author readPublisher
	Res    chan readPublisherPlayRes
}

type readPublisherRecordRes struct {
	Err error
}

type readPublisherRecordReq struct {
	Author readPublisher
	Res    chan readPublisherRecordRes
}

type readPublisherPauseReq struct {
	Author readPublisher
	Res    chan struct{}
}
