package readpublisher

import (
	"fmt"

	"github.com/aler9/gortsplib"
	"github.com/aler9/gortsplib/pkg/base"
	"github.com/aler9/gortsplib/pkg/headers"

	"github.com/aler9/rtsp-simple-server/internal/conf"
	"github.com/aler9/rtsp-simple-server/internal/streamproc"
)

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
	*base.Response
}

// Error implements the error interface.
func (ErrAuthCritical) Error() string {
	return "critical authentication error"
}

// Path is implemented by path.Path.
type Path interface {
	Name() string
	Conf() *conf.PathConf
	OnReadPublisherRemove(RemoveReq)
	OnReadPublisherPlay(PlayReq)
	OnReadPublisherRecord(RecordReq)
	OnReadPublisherPause(PauseReq)
}

// DescribeRes is a describe response.
type DescribeRes struct {
	SDP      []byte
	Redirect string
	Err      error
}

// DescribeReq is a describe request.
type DescribeReq struct {
	ReadPublisher ReadPublisher
	PathName      string
	Data          *base.Request
	Res           chan DescribeRes
}

// SetupPlayRes is a setup/play response.
type SetupPlayRes struct {
	Path   Path
	Tracks gortsplib.Tracks
	Err    error
}

// SetupPlayReq is a setup/play request.
type SetupPlayReq struct {
	ReadPublisher ReadPublisher
	PathName      string
	Data          interface{}
	Res           chan SetupPlayRes
}

// AnnounceRes is a announce response.
type AnnounceRes struct {
	Path Path
	Err  error
}

// AnnounceReq is a announce request.
type AnnounceReq struct {
	ReadPublisher ReadPublisher
	PathName      string
	Tracks        gortsplib.Tracks
	Data          interface{}
	Res           chan AnnounceRes
}

// RemoveReq is a remove request.
type RemoveReq struct {
	ReadPublisher ReadPublisher
	Res           chan struct{}
}

// PlayRes is a play response.
type PlayRes struct {
	TrackInfos []streamproc.TrackInfo
}

// PlayReq is a play request.
type PlayReq struct {
	ReadPublisher ReadPublisher
	Res           chan PlayRes
}

// RecordRes is a record response.
type RecordRes struct {
	SP  *streamproc.StreamProc
	Err error
}

// RecordReq is a record request.
type RecordReq struct {
	ReadPublisher ReadPublisher
	Res           chan RecordRes
}

// PauseReq is a pause request.
type PauseReq struct {
	ReadPublisher ReadPublisher
	Res           chan struct{}
}

// ReadPublisher is an entity that can read/publish from/to a path.
type ReadPublisher interface {
	IsReadPublisher()
	IsSource()
	Close()
	CloseRequest()
	Authenticate([]headers.AuthMethod,
		string, []interface{},
		string, string, interface{}) error
	OnFrame(int, gortsplib.StreamType, []byte)
}
