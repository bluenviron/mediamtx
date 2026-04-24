package hls

import (
	"encoding/hex"
	"net"
	"slices"
	"sync/atomic"
	"time"

	"github.com/bluenviron/mediamtx/internal/auth"
	"github.com/bluenviron/mediamtx/internal/defs"
	"github.com/bluenviron/mediamtx/internal/externalcmd"
	"github.com/bluenviron/mediamtx/internal/hooks"
	"github.com/bluenviron/mediamtx/internal/logger"
	"github.com/bluenviron/mediamtx/internal/protocols/httpp"
	"github.com/bluenviron/mediamtx/internal/stream"
	"github.com/bluenviron/mediamtx/internal/unit"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

type sessionServer interface {
	logger.Writer
	getMuxer(serverGetMuxerReq) (*muxer, error)
}

type session struct {
	remoteAddr      string
	pathName        string
	externalCmdPool *externalcmd.Pool
	pathManager     serverPathManager
	server          sessionServer

	uuid            uuid.UUID
	secret          uuid.UUID
	ip              string
	created         time.Time
	query           string
	user            string
	lastRequestTime atomic.Int64
	bytesSent       atomic.Uint64
	path            defs.Path
	stream          *stream.Stream
	muxer           *muxer
	reader          *stream.Reader
	onUnreadHook    func()
}

func (s *session) initialize(ctx *gin.Context) error {
	s.uuid = uuid.New()
	s.secret = uuid.New()
	s.ip, _, _ = net.SplitHostPort(s.remoteAddr)
	s.created = time.Now()
	s.query = ctx.Request.URL.RawQuery
	s.lastRequestTime.Store(time.Now().UnixNano())

	res, err := s.pathManager.AddReader(defs.PathAddReaderReq{
		Author: s,
		AccessRequest: defs.PathAccessRequest{
			Name:        s.pathName,
			Query:       s.query,
			Publish:     false,
			Proto:       auth.ProtocolHLS,
			ID:          &s.uuid,
			Credentials: httpp.Credentials(ctx.Request),
			IP:          net.ParseIP(ctx.ClientIP()),
		},
	})
	if err != nil {
		return err
	}

	s.path = res.Path
	s.stream = res.Stream
	s.user = res.User

	muxer, err := s.server.getMuxer(serverGetMuxerReq{
		path:           s.pathName,
		create:         true,
		remoteAddr:     s.remoteAddr,
		query:          s.query,
		sourceOnDemand: res.Path.SafeConf().SourceOnDemand,
	})
	if err != nil {
		s.path.RemoveReader(defs.PathRemoveReaderReq{Author: s})
		return err
	}

	s.muxer = muxer

	muxerFormats, err := s.muxer.addSession(s)
	if err != nil {
		s.path.RemoveReader(defs.PathRemoveReaderReq{Author: s})
		return err
	}

	s.reader = &stream.Reader{
		Parent: s,
	}

	// all of this is needed to allow Stream to increase outbound bytes for every HLS session
	for _, medi := range res.Stream.Desc.Medias {
		for _, forma := range medi.Formats {
			if slices.Contains(muxerFormats, forma) {
				s.reader.OnData(medi, forma, func(_ *unit.Unit) error {
					return nil
				})
			}
		}
	}

	res.Stream.AddReader(s.reader)

	s.Log(logger.Info, "created by %s, reading from muxer '%s'", s.remoteAddr, s.pathName)

	s.onUnreadHook = hooks.OnRead(hooks.OnReadParams{
		Logger:          s,
		ExternalCmdPool: s.externalCmdPool,
		Conf:            res.Path.SafeConf(),
		ExternalCmdEnv:  res.Path.ExternalCmdEnv(),
		Reader:          *s.APIReaderDescribe(),
		Query:           s.query,
	})

	return nil
}

// called by path or path manager.
// not implemented since closing the Muxer is enough to close every associated session.
func (s *session) Close() {
}

func (s *session) close2(err error) {
	s.stream.RemoveReader(s.reader)

	s.path.RemoveReader(defs.PathRemoveReaderReq{Author: s})

	s.onUnreadHook()

	s.Log(logger.Info, "closed: %v", err)
}

// Log implements logger.Writer.
func (s *session) Log(level logger.Level, format string, args ...any) {
	id := hex.EncodeToString(s.uuid[:4])
	s.server.Log(level, "[session %v] "+format, append([]any{id}, args...)...)
}

func (s *session) apiItem() *defs.APIHLSSession {
	outboundBytes := s.bytesSent.Load()

	return &defs.APIHLSSession{
		ID:            s.uuid,
		Created:       s.created,
		RemoteAddr:    s.remoteAddr,
		Path:          s.pathName,
		Query:         s.query,
		User:          s.user,
		OutboundBytes: outboundBytes,
	}
}

func (s *session) APIReaderDescribe() *defs.APIPathReader {
	return &defs.APIPathReader{
		Type: defs.APIPathReaderTypeHLSSession,
		ID:   s.uuid.String(),
	}
}
