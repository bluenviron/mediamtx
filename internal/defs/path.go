package defs

import (
	"fmt"
	"net"
	"net/http"

	"github.com/bluenviron/gortsplib/v4/pkg/base"
	"github.com/bluenviron/gortsplib/v4/pkg/description"
	"github.com/google/uuid"

	"github.com/bluenviron/mediamtx/internal/auth"
	"github.com/bluenviron/mediamtx/internal/conf"
	"github.com/bluenviron/mediamtx/internal/externalcmd"
	"github.com/bluenviron/mediamtx/internal/stream"
)

// PathNoOnePublishingError is returned when no one is publishing.
type PathNoOnePublishingError struct {
	PathName string
}

// Error implements the error interface.
func (e PathNoOnePublishingError) Error() string {
	return fmt.Sprintf("no one is publishing to path '%s'", e.PathName)
}

// Path is a path.
type Path interface {
	Name() string
	SafeConf() *conf.Path
	ExternalCmdEnv() externalcmd.Environment
	StartPublisher(req PathStartPublisherReq) (*stream.Stream, error)
	StopPublisher(req PathStopPublisherReq)
	RemovePublisher(req PathRemovePublisherReq)
	RemoveReader(req PathRemoveReaderReq)
}

// PathAccessRequest is a path access request.
type PathAccessRequest struct {
	Name     string
	Query    string
	Publish  bool
	SkipAuth bool

	// only if skipAuth = false
	User  string
	Pass  string
	IP    net.IP
	Proto auth.Protocol
	ID    *uuid.UUID

	// RTSP only
	RTSPRequest *base.Request
	RTSPNonce   string

	// HTTP only
	HTTPRequest *http.Request
}

// ToAuthRequest converts a path access request into an authentication request.
func (r *PathAccessRequest) ToAuthRequest() *auth.Request {
	return &auth.Request{
		User: r.User,
		Pass: r.Pass,
		IP:   r.IP,
		Action: func() conf.AuthAction {
			if r.Publish {
				return conf.AuthActionPublish
			}
			return conf.AuthActionRead
		}(),
		Path:        r.Name,
		Protocol:    r.Proto,
		ID:          r.ID,
		Query:       r.Query,
		RTSPRequest: r.RTSPRequest,
		RTSPNonce:   r.RTSPNonce,
		HTTPRequest: r.HTTPRequest,
	}
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
	Path Path
	Err  error
}

// PathAddPublisherReq contains arguments of AddPublisher().
type PathAddPublisherReq struct {
	Author        Publisher
	AccessRequest PathAccessRequest
	Res           chan PathAddPublisherRes
}

// PathRemovePublisherReq contains arguments of RemovePublisher().
type PathRemovePublisherReq struct {
	Author Publisher
	Res    chan struct{}
}

// PathStartPublisherRes contains the response of StartPublisher().
type PathStartPublisherRes struct {
	Stream *stream.Stream
	Err    error
}

// PathStartPublisherReq contains arguments of StartPublisher().
type PathStartPublisherReq struct {
	Author             Publisher
	Desc               *description.Session
	GenerateRTPPackets bool
	Res                chan PathStartPublisherRes
}

// PathStopPublisherReq contains arguments of StopPublisher().
type PathStopPublisherReq struct {
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
