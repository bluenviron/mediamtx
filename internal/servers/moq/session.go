package moq

import (
	"bufio"
	"context"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/bluenviron/gortsplib/v5/pkg/description"
	"github.com/bluenviron/mediamtx/internal/auth"
	"github.com/bluenviron/mediamtx/internal/defs"
	"github.com/bluenviron/mediamtx/internal/logger"
	"github.com/bluenviron/mediamtx/internal/protocols/moq"
	"github.com/bluenviron/mediamtx/internal/protocols/moq/catalog"
	"github.com/bluenviron/mediamtx/internal/protocols/moq/controlmessage"
	"github.com/bluenviron/mediamtx/internal/protocols/moq/parameter"
	"github.com/bluenviron/mediamtx/internal/protocols/moq/property"
	"github.com/bluenviron/mediamtx/internal/protocols/moq/subgroup"
	"github.com/bluenviron/mediamtx/internal/stream"
	"github.com/bluenviron/mediamtx/internal/unit"
	"github.com/google/uuid"
	"github.com/quic-go/webtransport-go"
	"golang.org/x/sync/errgroup"
)

const maxReorderedSubGroups = 50

func findAuthorizationToken(parameters []parameter.Parameter) *parameter.AuthorizationToken {
	for _, pa := range parameters {
		if auth, ok := pa.(*parameter.AuthorizationToken); ok {
			return auth
		}
	}
	return nil
}

func credentialsFromAuthorizationToken(authorization *parameter.AuthorizationToken) *auth.Credentials {
	if authorization != nil {
		if s, ok := strings.CutPrefix(string(authorization.TokenValue), "Basic "); ok {
			decoded, err := base64.StdEncoding.DecodeString(s)
			if err != nil {
				return &auth.Credentials{}
			}
			var user, pass string
			user, pass, ok = strings.Cut(string(decoded), ":")
			if !ok {
				return &auth.Credentials{}
			}
			return &auth.Credentials{
				User: user,
				Pass: pass,
			}
		}

		if s, ok := strings.CutPrefix(string(authorization.TokenValue), "Bearer "); ok {
			return &auth.Credentials{
				Token: s,
			}
		}
	}

	return &auth.Credentials{}
}

func tracksToCatalog(tracks []*moq.Track) (catalog.Catalog, error) {
	cat := catalog.Catalog{
		Version: 1,
		Tracks:  make([]catalog.Track, len(tracks)),
	}

	for i, track := range tracks {
		ct := catalog.Track{
			Name:       strconv.Itoa(i),
			Packaging:  "loc",
			IsLive:     true,
			Codec:      track.Codec,
			Samplerate: track.Samplerate,
			Channels:   track.Channels,
			InitData:   track.InitData,
		}
		cat.Tracks[i] = ct
	}

	return cat, nil
}

func isSubGroupStream(b byte) bool {
	return (b & 0x90) == 0x10
}

type sessionParent interface {
	closeSession(sx *session)
	logger.Writer
}
type session struct {
	wt          *webtransport.Session
	wg          *sync.WaitGroup
	pathName    string
	query       string
	userAgent   string
	pathManager serverPathManager
	parent      sessionParent

	ctx                context.Context
	ctxCancel          context.CancelFunc
	created            time.Time
	uuid               uuid.UUID
	mutex              sync.Mutex
	state              defs.APIMoQSessionState
	path               defs.Path
	stream             *stream.Stream           // read only
	tracks             []*moq.Track             // read only
	trackSubscriptions map[int]struct{}         // read only
	catalogReceived    chan []byte              // publish only
	publishReady       chan struct{}            // publish only
	inboundTracks      map[uint64]*inboundTrack // publish only

	inboundBytes  atomic.Uint64
	outboundBytes atomic.Uint64

	setupReceived chan struct{}
	done          chan struct{}
}

func (s *session) initialize() {
	s.ctx, s.ctxCancel = context.WithCancel(context.Background())
	s.created = time.Now()
	s.uuid = uuid.New()
	s.state = defs.APIMoQSessionStateIdle
	s.trackSubscriptions = make(map[int]struct{})

	s.catalogReceived = make(chan []byte, 1)
	s.publishReady = make(chan struct{})

	s.setupReceived = make(chan struct{})
	s.done = make(chan struct{})

	s.Log(logger.Info, "created by %s", s.wt.RemoteAddr())

	s.wg.Add(1)
	go s.run()
}

// Log implements logger.Writer.
func (s *session) Log(level logger.Level, format string, args ...any) {
	id := hex.EncodeToString(s.uuid[:4])
	s.parent.Log(level, "[session %v] "+format, append([]any{id}, args...)...)
}

// Close implements defs.Reader.
func (s *session) Close() {
	s.ctxCancel()
}

func (s *session) run() {
	defer s.wg.Done()
	defer close(s.done)

	err := s.runInner()

	switch s.state {
	case defs.APIMoQSessionStatePublish:
		if s.path != nil {
			s.path.RemovePublisher(defs.PathRemovePublisherReq{Author: s})
		}

	case defs.APIMoQSessionStateRead:
		if s.path != nil {
			s.path.RemoveReader(defs.PathRemoveReaderReq{Author: s})
		}
	}

	s.parent.closeSession(s)

	s.Log(logger.Info, "closed: %v", err)
}

func (s *session) runInner() error {
	errGroup, errGroupCtx := errgroup.WithContext(context.Background())

	errGroup.Go(func() error {
		return s.runUniStreamAcceptor(errGroup)
	})

	errGroup.Go(func() error {
		return s.runBidiStreamAcceptor(errGroup)
	})

	errGroup.Go(func() error {
		return s.runSetupWriter()
	})

	select {
	case <-s.ctx.Done():
		s.wt.CloseWithError(0, "") //nolint:errcheck
		errGroup.Wait()            //nolint:errcheck
		return fmt.Errorf("terminated")

	case <-errGroupCtx.Done():
		s.ctxCancel()
		s.wt.CloseWithError(0, "") //nolint:errcheck
		return errGroup.Wait()
	}
}

func (s *session) runSetupWriter() error {
	wstream, err := s.wt.OpenUniStreamSync(context.Background())
	if err != nil {
		return err
	}

	_, err = wstream.Write(controlmessage.Setup{}.Marshal())
	if err != nil {
		return err
	}

	return nil
}

func (s *session) runUniStreamAcceptor(errGroup *errgroup.Group) error {
	for {
		stream, err := s.wt.AcceptUniStream(context.Background())
		if err != nil {
			return fmt.Errorf("AcceptUniStream returned: %w", err)
		}

		errGroup.Go(func() error {
			return s.runUniStream(stream)
		})
	}
}

func (s *session) runBidiStreamAcceptor(errGroup *errgroup.Group) error {
	for {
		stream, err := s.wt.AcceptStream(context.Background())
		if err != nil {
			return fmt.Errorf("AcceptStream returned: %w", err)
		}

		errGroup.Go(func() error {
			return s.runBidiStream(stream)
		})
	}
}

func (s *session) runUniStream(wstream *webtransport.ReceiveStream) error {
	br := bufio.NewReader(wstream)
	firstByte, err := br.Peek(1)
	if err != nil {
		return err
	}

	if isSubGroupStream(firstByte[0]) {
		return s.onUniSubGroup(br)
	}

	return s.onUniMessage(br)
}

func (s *session) onUniMessage(r io.Reader) error {
	msg, err := controlmessage.Read(r)
	if err != nil {
		return err
	}

	switch msg.(type) {
	case *controlmessage.Setup:
		err = func() error {
			s.mutex.Lock()
			defer s.mutex.Unlock()

			select {
			case <-s.setupReceived:
				return fmt.Errorf("SETUP stream is already present")
			default:
				close(s.setupReceived)
				return nil
			}
		}()
		if err != nil {
			return err
		}

		io.Copy(io.Discard, r)
		return fmt.Errorf("SETUP stream closed")

	default:
		return fmt.Errorf("unsupported stream type: %T", msg)
	}
}

func (s *session) runBidiStream(wstream *webtransport.Stream) error {
	select {
	case <-s.setupReceived:
	case <-s.ctx.Done():
		return fmt.Errorf("terminated")
	}

	msg, err := controlmessage.Read(wstream)
	if err != nil {
		return err
	}

	switch m := msg.(type) {
	case *controlmessage.Subscribe:
		s.Log(logger.Debug, "SUBSCRIBE track=%s", m.TrackName)

		if m.TrackName == ".catalog" {
			return s.onSubscribeCatalog(wstream, m)
		}

		return s.onSubscribeTrack(wstream, m)

	case *controlmessage.Publish:
		s.Log(logger.Debug, "PUBLISH track=%s alias=%d", m.TrackName, m.TrackAlias)

		if m.TrackName == ".catalog" {
			return s.onPublishCatalog(wstream, m)
		}

		return s.onPublishTrack(wstream)

	default:
		return fmt.Errorf("unsupported message type: %T", msg)
	}
}

func (s *session) onSubscribeCatalog(wstream *webtransport.Stream, m *controlmessage.Subscribe) error {
	s.mutex.Lock()
	if s.state != defs.APIMoQSessionStateIdle {
		s.mutex.Unlock()
		return fmt.Errorf("unexpected SUBSCRIBE in state %s", s.state)
	}
	s.state = defs.APIMoQSessionStateRead
	s.mutex.Unlock()

	remoteHost, _, _ := net.SplitHostPort(s.wt.RemoteAddr().String())
	addRes, err := s.pathManager.AddReader(defs.PathAddReaderReq{
		Author: s,
		AccessRequest: defs.PathAccessRequest{
			Name:        s.pathName,
			Query:       s.query,
			Proto:       auth.ProtocolMoQ,
			ID:          &s.uuid,
			Credentials: credentialsFromAuthorizationToken(findAuthorizationToken(m.Parameters)),
			IP:          net.ParseIP(remoteHost),
			UserAgent:   s.userAgent,
		},
	})
	if err != nil {
		if _, ok := errors.AsType[*auth.Error](err); ok {
			// wait some seconds to delay brute force attacks
			<-time.After(auth.PauseAfterError)
		}

		var code controlmessage.RequestErrorCode
		if _, ok := errors.AsType[*auth.Error](err); ok {
			code = controlmessage.RequestErrorCodeUnauthorized
		} else if _, ok = errors.AsType[*defs.PathNoStreamAvailableError](err); ok {
			code = controlmessage.RequestErrorCodeDoesNotExist
		} else {
			code = controlmessage.RequestErrorCodeNotSupported
		}

		wstream.Write(controlmessage.RequestError{ //nolint:errcheck
			Code:   code,
			Reason: err.Error(),
		}.Marshal())

		// wait for the client to read the error
		io.Copy(io.Discard, wstream)

		return err
	}

	tracks, err := moq.FromStream(
		addRes.Stream.Desc,
	)
	if err != nil {
		addRes.Path.RemoveReader(defs.PathRemoveReaderReq{Author: s})
		return err
	}

	s.mutex.Lock()
	s.path = addRes.Path
	s.stream = addRes.Stream
	s.tracks = tracks
	s.mutex.Unlock()

	s.Log(logger.Info, "is reading from path %s", s.pathName)

	_, err = wstream.Write(controlmessage.SubscribeOk{TrackAlias: m.RequestID}.Marshal())
	if err != nil {
		return err
	}

	cat, err := tracksToCatalog(tracks)
	if err != nil {
		return err
	}

	enc, err := json.Marshal(cat)
	if err != nil {
		return err
	}

	dataWStream, err := s.wt.OpenUniStreamSync(context.Background())
	if err != nil {
		return err
	}
	defer dataWStream.Close() //nolint:errcheck

	sg := &subgroup.SubGroup{
		Header: subgroup.Header{
			Properties:  false,
			FirstObject: true,
			TrackAlias:  m.RequestID,
			GroupID:     0,
		},
		Objects: []subgroup.Object{{
			Payload: enc,
		}},
	}
	buf := sg.Marshal()

	_, err = dataWStream.Write(buf)
	if err != nil {
		return err
	}

	io.Copy(io.Discard, wstream)
	return fmt.Errorf("SUBSCRIBE catalog stream closed")
}

func (s *session) onSubscribeTrack(wstream *webtransport.Stream, m *controlmessage.Subscribe) error {
	trackID, err := strconv.Atoi(m.TrackName)
	if err != nil || trackID < 0 {
		return fmt.Errorf("invalid track name: %s", m.TrackName)
	}

	err = func() error {
		s.mutex.Lock()
		defer s.mutex.Unlock()

		if s.state != defs.APIMoQSessionStateRead {
			return fmt.Errorf("unexpected SUBSCRIBE in state %s", s.state)
		}

		if s.stream == nil {
			return fmt.Errorf("stream not ready")
		}

		if trackID >= len(s.tracks) {
			return fmt.Errorf("track index %d out of range", trackID)
		}

		_, ok := s.trackSubscriptions[trackID]
		if ok {
			return fmt.Errorf("already subscribed to track %d", trackID)
		}

		s.trackSubscriptions[trackID] = struct{}{}

		return nil
	}()
	if err != nil {
		return err
	}

	r := &stream.Reader{Parent: s}

	track := s.tracks[trackID]
	groupID := uint64(0)

	wrapped := func(payload []byte, pts int64) error {
		wstream, err2 := s.wt.OpenUniStreamSync(context.Background())
		if err2 != nil {
			return err2
		}
		defer wstream.Close() //nolint:errcheck

		sg := &subgroup.SubGroup{
			Header: subgroup.Header{
				Properties:  true,
				FirstObject: true,
				TrackAlias:  m.RequestID,
				GroupID:     groupID,
			},
			Objects: []subgroup.Object{{
				Properties: property.Properties{
					new(property.Timestamp(pts)),
				},
				Payload: payload,
			}},
		}
		buf := sg.Marshal()
		groupID++

		_, err2 = wstream.Write(buf)
		if err2 == nil {
			s.outboundBytes.Add(uint64(len(payload)))
		}
		return err2
	}

	r.OnData(track.Media, track.Format, func(u *unit.Unit) error {
		return track.OnData(u, wrapped)
	})

	s.stream.AddReader(r)
	defer s.stream.RemoveReader(r)

	_, err = wstream.Write(controlmessage.SubscribeOk{TrackAlias: m.RequestID}.Marshal())
	if err != nil {
		return err
	}

	streamClosed := make(chan struct{})
	go func() {
		io.Copy(io.Discard, wstream)
		close(streamClosed)
	}()

	select {
	case err = <-r.Error():
		return err
	case <-streamClosed:
		return fmt.Errorf("SUBSCRIBE track stream closed")
	case <-s.ctx.Done():
		return fmt.Errorf("terminated")
	}
}

func (s *session) onPublishCatalog(wstream *webtransport.Stream, m *controlmessage.Publish) error {
	s.mutex.Lock()
	if s.state != defs.APIMoQSessionStateIdle {
		s.mutex.Unlock()
		return fmt.Errorf("unexpected PUBLISH in state %s", s.state)
	}
	s.state = defs.APIMoQSessionStatePublish
	s.mutex.Unlock()

	select {
	case catalogData := <-s.catalogReceived:
		var cat catalog.Catalog
		err := json.Unmarshal(catalogData, &cat)
		if err != nil {
			return fmt.Errorf("failed to parse catalog JSON: %w", err)
		}

		var subStream *stream.SubStream

		medias, writeFuncs, err := moq.ToStream(&cat, &subStream)
		if err != nil {
			return err
		}

		s.inboundTracks = make(map[uint64]*inboundTrack)

		for i := range cat.Tracks {
			trackAlias := uint64(i + 1)
			tr := &inboundTrack{
				onSubGroup: writeFuncs[trackAlias],
				parent:     s,
			}
			tr.initialize()
			s.inboundTracks[trackAlias] = tr
		}

		remoteHost, _, _ := net.SplitHostPort(s.wt.RemoteAddr().String())
		addRes, err := s.pathManager.AddPublisher(defs.PathAddPublisherReq{
			Author:        s,
			Desc:          &description.Session{Medias: medias},
			UseRTPPackets: false,
			ReplaceNTP:    true,
			AccessRequest: defs.PathAccessRequest{
				Name:        s.pathName,
				Query:       s.query,
				Publish:     true,
				Proto:       auth.ProtocolMoQ,
				ID:          &s.uuid,
				Credentials: credentialsFromAuthorizationToken(findAuthorizationToken(m.Parameters)),
				IP:          net.ParseIP(remoteHost),
				UserAgent:   s.userAgent,
			},
		})
		if err != nil {
			if _, ok := errors.AsType[*auth.Error](err); ok {
				// wait some seconds to delay brute force attacks
				<-time.After(auth.PauseAfterError)
			}

			var code controlmessage.RequestErrorCode
			if _, ok := errors.AsType[*auth.Error](err); ok {
				code = controlmessage.RequestErrorCodeUnauthorized
			} else {
				code = controlmessage.RequestErrorCodeUninterested
			}

			wstream.Write(controlmessage.RequestError{ //nolint:errcheck
				Code:   code,
				Reason: err.Error(),
			}.Marshal()) //nolint:errcheck

			// wait for the client to read the error
			io.Copy(io.Discard, wstream)

			return err
		}

		s.mutex.Lock()
		s.path = addRes.Path
		s.mutex.Unlock()

		subStream = addRes.SubStream

		close(s.publishReady)

	case <-s.publishReady:
		return fmt.Errorf("catalog already received")

	case <-s.ctx.Done():
		return fmt.Errorf("terminated")
	}

	_, err := wstream.Write(controlmessage.RequestOk{}.Marshal())
	if err != nil {
		return err
	}

	io.Copy(io.Discard, wstream)
	return fmt.Errorf("PUBLISH catalog stream closed")
}

func (s *session) onPublishTrack(wstream *webtransport.Stream) error {
	s.mutex.Lock()
	if s.state != defs.APIMoQSessionStatePublish {
		s.mutex.Unlock()
		return fmt.Errorf("unexpected PUBLISH in state %s", s.state)
	}
	s.mutex.Unlock()

	_, err := wstream.Write(controlmessage.RequestOk{}.Marshal())
	if err != nil {
		return err
	}

	io.Copy(io.Discard, wstream)
	return fmt.Errorf("PUBLISH track stream closed")
}

func (s *session) onUniSubGroup(r io.Reader) error {
	var sg subgroup.SubGroup
	err := sg.Read(r)
	if err != nil {
		return err
	}

	if sg.Header.TrackAlias == 0 {
		return s.onDataCatalog(r, &sg)
	}

	return s.onDataTrack(r, &sg)
}

func (s *session) onDataCatalog(r io.Reader, sg *subgroup.SubGroup) error {
	select {
	case s.catalogReceived <- sg.Objects[0].Payload:
	default:
		return fmt.Errorf("catalog already received")
	}

	io.Copy(io.Discard, r)
	return nil
}

func (s *session) onDataTrack(r io.Reader, sg *subgroup.SubGroup) error {
	select {
	case <-s.publishReady:
	case <-s.ctx.Done():
		return fmt.Errorf("terminated")
	}

	track, ok := s.inboundTracks[sg.Header.TrackAlias]
	if !ok {
		return fmt.Errorf("track %d not found", sg.Header.TrackAlias)
	}

	for _, obj := range sg.Objects {
		s.inboundBytes.Add(uint64(len(obj.Payload)))
	}

	err := track.push(sg)
	if err != nil {
		return err
	}

	io.Copy(io.Discard, r)
	return nil
}

func (s *session) apiItem() defs.APIMoQSession {
	s.mutex.Lock()
	state := s.state
	s.mutex.Unlock()

	return defs.APIMoQSession{
		ID:            s.uuid,
		Created:       s.created,
		RemoteAddr:    s.wt.RemoteAddr().String(),
		State:         state,
		Path:          s.pathName,
		Query:         s.query,
		UserAgent:     s.userAgent,
		InboundBytes:  s.inboundBytes.Load(),
		OutboundBytes: s.outboundBytes.Load(),
	}
}

// APIReaderDescribe implements defs.Reader.
func (s *session) APIReaderDescribe() *defs.APIPathReader {
	return &defs.APIPathReader{
		Type: defs.APIPathReaderTypeMoQSession,
		ID:   s.uuid.String(),
	}
}

// APISourceDescribe implements defs.Source.
func (s *session) APISourceDescribe() *defs.APIPathSource {
	return &defs.APIPathSource{
		Type: defs.APIPathSourceTypeMoQSession,
		ID:   s.uuid.String(),
	}
}
