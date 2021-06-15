package readpublisher

import (
	"fmt"
	"net"

	"github.com/aler9/gortsplib"
	"github.com/aler9/gortsplib/pkg/base"
	"github.com/aler9/gortsplib/pkg/headers"

	"github.com/aler9/rtsp-simple-server/internal/conf"
)

// Path is implemented by path.Path.
type Path interface {
	Name() string
	Conf() *conf.PathConf
	OnReadPublisherRemove(RemoveReq)
	OnReadPublisherPlay(PlayReq)
	OnReadPublisherRecord(RecordReq)
	OnReadPublisherPause(PauseReq)
	OnFrame(int, gortsplib.StreamType, []byte)
}

// ErrNoOnePublishing is a "no one is publishing" error.
type ErrNoOnePublishing struct {
	PathName string
}

// Error implements the error interface.
func (e ErrNoOnePublishing) Error() string {
	return fmt.Sprintf("no one is publishing to path '%s'", e.PathName)
}

// ErrAuthNotCritical is a non-critical authentication error.
type ErrAuthNotCritical struct {
	*base.Response
}

// Error implements the error interface.
func (ErrAuthNotCritical) Error() string {
	return "non-critical authentication error"
}

// ErrAuthCritical is a critical authentication error.
type ErrAuthCritical struct {
	Message  string
	Response *base.Response
}

// Error implements the error interface.
func (ErrAuthCritical) Error() string {
	return "critical authentication error"
}

// ReadPublisher is an entity that can read/publish from/to a path.
type ReadPublisher interface {
	IsReadPublisher()
	IsSource()
	Close()
	OnFrame(int, gortsplib.StreamType, []byte)
}

// DescribeRes is a describe response.
type DescribeRes struct {
	Stream   *gortsplib.ServerStream
	Redirect string
	Err      error
}

// DescribeReq is a describe request.
type DescribeReq struct {
	PathName            string
	URL                 *base.URL
	IP                  net.IP
	ValidateCredentials func(authMethods []headers.AuthMethod, pathUser string, pathPass string) error
	Res                 chan DescribeRes
}

// SetupPlayRes is a setup/play response.
type SetupPlayRes struct {
	Path   Path
	Stream *gortsplib.ServerStream
	Err    error
}

// SetupPlayReq is a setup/play request.
type SetupPlayReq struct {
	Author              ReadPublisher
	PathName            string
	IP                  net.IP
	ValidateCredentials func(authMethods []headers.AuthMethod, pathUser string, pathPass string) error
	Res                 chan SetupPlayRes
}

// AnnounceRes is a announce response.
type AnnounceRes struct {
	Path Path
	Err  error
}

// AnnounceReq is a announce request.
type AnnounceReq struct {
	Author              ReadPublisher
	PathName            string
	Tracks              gortsplib.Tracks
	IP                  net.IP
	ValidateCredentials func(authMethods []headers.AuthMethod, pathUser string, pathPass string) error
	Res                 chan AnnounceRes
}

// RemoveReq is a remove request.
type RemoveReq struct {
	Author ReadPublisher
	Res    chan struct{}
}

// PlayRes is a play response.
type PlayRes struct{}

// PlayReq is a play request.
type PlayReq struct {
	Author ReadPublisher
	Res    chan PlayRes
}

// RecordRes is a record response.
type RecordRes struct {
	Err error
}

// RecordReq is a record request.
type RecordReq struct {
	Author ReadPublisher
	Res    chan RecordRes
}

// PauseReq is a pause request.
type PauseReq struct {
	Author ReadPublisher
	Res    chan struct{}
}
