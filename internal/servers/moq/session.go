package moq

import (
	"context"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/bluenviron/gortsplib/v5/pkg/description"
	"github.com/google/uuid"
	"github.com/quic-go/quic-go"
	"github.com/quic-go/webtransport-go"
	"golang.org/x/sync/errgroup"

	"github.com/bluenviron/mediamtx/internal/auth"
	"github.com/bluenviron/mediamtx/internal/defs"
	"github.com/bluenviron/mediamtx/internal/logger"
	"github.com/bluenviron/mediamtx/internal/protocols/moq"
	"github.com/bluenviron/mediamtx/internal/protocols/moq/catalog"
	"github.com/bluenviron/mediamtx/internal/protocols/moq/controlmessage"
	"github.com/bluenviron/mediamtx/internal/protocols/moq/parameter"
	"github.com/bluenviron/mediamtx/internal/protocols/moq/property"
	"github.com/bluenviron/mediamtx/internal/protocols/moq/reorderer"
	"github.com/bluenviron/mediamtx/internal/protocols/moq/subgroup"
	"github.com/bluenviron/mediamtx/internal/protocols/moq/version"
	"github.com/bluenviron/mediamtx/internal/stream"
)

const (
	maxCatalogTracks = 50
)

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

func isCatalogTrack(name string) bool {
	return name == "catalog" || name == ".catalog"
}

type sessionParent interface {
	closeSession(sx *session)
	logger.Writer
}

type session struct {
	quicConn    *quic.Conn
	wtSession   *webtransport.Session
	version     version.Version
	wg          *sync.WaitGroup
	pathName    string
	query       string
	userAgent   string
	pathManager serverPathManager
	parent      sessionParent

	conn            *moq.Conn
	ctx             context.Context
	ctxCancel       context.CancelFunc
	created         time.Time
	uuid            uuid.UUID
	mutex           sync.Mutex
	state           defs.APIMoQSessionState
	path            defs.Path
	user            string
	expiresAt       *time.Time
	stream          *stream.Stream           // read only
	setupTracks     []moq.SetupTrackFunc     // read only
	catalogReceived chan *catalog.Catalog    // publish only
	publishReady    chan struct{}            // publish only
	inboundTracks   map[uint64]*inboundTrack // publish only

	inboundBytes  atomic.Uint64
	outboundBytes atomic.Uint64

	setupReceived chan struct{}
	done          chan struct{}
}

func (s *session) initialize() {
	if s.quicConn != nil {
		s.conn = &moq.Conn{Raw: &moq.RawConnQUIC{Conn: s.quicConn}, Server: true, Log: s, Version: s.version}
	} else {
		s.conn = &moq.Conn{Raw: &moq.RawConnWebTransport{Session: s.wtSession}, Server: true, Log: s, Version: s.version}
	}

	s.ctx, s.ctxCancel = context.WithCancel(context.Background())
	s.created = time.Now()
	s.uuid = uuid.New()
	s.state = defs.APIMoQSessionStateIdle

	s.catalogReceived = make(chan *catalog.Catalog, 1)
	s.publishReady = make(chan struct{})

	s.setupReceived = make(chan struct{})
	s.done = make(chan struct{})

	s.Log(logger.Info, "created by %s", s.conn.RemoteAddr())

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

	s.ctxCancel()

	s.parent.closeSession(s)

	s.Log(logger.Info, "closed: %v", err)
}

func (s *session) runInner() error {
	errGroup, errGroupCtx := errgroup.WithContext(context.Background())

	errGroup.Go(func() error {
		select {
		case <-s.ctx.Done():
			return fmt.Errorf("terminated")
		case <-errGroupCtx.Done():
			return fmt.Errorf("terminated")
		}
	})

	errGroup.Go(func() error {
		<-errGroupCtx.Done()
		s.conn.CloseWithError(0, "")
		return errors.New("terminated")
	})

	errGroup.Go(func() error {
		return s.runUniStreamAcceptor(errGroupCtx, errGroup)
	})

	errGroup.Go(func() error {
		return s.runBidiStreamAcceptor(errGroupCtx, errGroup)
	})

	if s.conn.Version != version.Draft16 {
		errGroup.Go(func() error {
			return s.runSetupWriter()
		})
	}

	return errGroup.Wait()
}

func (s *session) runSetupWriter() error {
	wstream, err := s.conn.OpenUniStreamSync(context.Background())
	if err != nil {
		return err
	}
	defer wstream.Close() //nolint:errcheck

	return wstream.WriteMessage(&controlmessage.Setup{})
}

func (s *session) runUniStreamAcceptor(errGroupCtx context.Context, errGroup *errgroup.Group) error {
	for {
		stream, err := s.conn.AcceptUniStream(context.Background())
		if err != nil {
			return fmt.Errorf("AcceptUniStream returned: %w", err)
		}

		errGroup.Go(func() error {
			return s.runUniStream(errGroupCtx, stream)
		})
	}
}

func (s *session) runBidiStreamAcceptor(errGroupCtx context.Context, errGroup *errgroup.Group) error {
	for {
		stream, err := s.conn.AcceptStream(context.Background())
		if err != nil {
			return fmt.Errorf("AcceptStream returned: %w", err)
		}

		errGroup.Go(func() error {
			return s.runBidiStream(errGroupCtx, errGroup, stream)
		})
	}
}

func (s *session) runUniStream(errGroupCtx context.Context, stream *moq.IncomingUniStream) error {
	isSubGroup, err := stream.IsSubGroup()
	if err != nil {
		return err
	}

	if isSubGroup {
		return s.onUniSubGroup(errGroupCtx, stream)
	}

	return s.onUniMessage(stream)
}

func (s *session) onUniMessage(stream *moq.IncomingUniStream) error {
	msg, err := stream.ReadMessage()
	if err != nil {
		return err
	}

	switch m := msg.(type) {
	case *controlmessage.Setup:
		if s.conn.Version == version.Draft16 {
			return fmt.Errorf("received SETUP over unidirectional stream with draft-16")
		}

		err = s.processSetupMessage(m)
		if err != nil {
			return err
		}

		return stream.Drain()

	default:
		return fmt.Errorf("unsupported stream type: %T", msg)
	}
}

func (s *session) transport() defs.APIMoQSessionTransport {
	if s.conn.IsQUIC() {
		return defs.APIMoQSessionTransportQUIC
	}

	return defs.APIMoQSessionTransportWebTransport
}

func (s *session) processSetupMessage(m *controlmessage.Setup) error {
	s.mutex.Lock()
	defer s.mutex.Unlock()

	switch s.transport() {
	case defs.APIMoQSessionTransportWebTransport:
		if m.Path != "" {
			return fmt.Errorf("received PATH setup option over WebTransport")
		}
		if m.Authority != "" {
			return fmt.Errorf("received AUTHORITY setup option over WebTransport")
		}

	case defs.APIMoQSessionTransportQUIC:
		if s.pathName == "" {
			pathWithQuery := m.Path
			if pathWithQuery == "" {
				return fmt.Errorf("missing PATH setup option")
			}

			u, err := url.ParseRequestURI(pathWithQuery)
			if err != nil {
				return fmt.Errorf("invalid PATH setup option: %w", err)
			}

			pathName := strings.Trim(u.Path, "/")
			if pathName == "" {
				return fmt.Errorf("invalid PATH setup option: empty path")
			}

			s.pathName = pathName
			s.query = u.RawQuery
		}
	}

	select {
	case <-s.setupReceived:
		return fmt.Errorf("SETUP stream is already present")
	default:
		close(s.setupReceived)
		return nil
	}
}

func (s *session) runBidiStream(
	errGroupCtx context.Context,
	errGroup *errgroup.Group,
	wstream *moq.ControlStream,
) error {
	if s.conn.Version == version.Draft16 {
		select {
		case <-s.setupReceived:
		default:
			msg, err := wstream.ReadMessage()
			if err != nil {
				return err
			}

			setupMsg, ok := msg.(*controlmessage.ClientSetup)
			if !ok {
				return fmt.Errorf("expected CLIENT_SETUP as first message on draft-16 bidirectional stream")
			}

			err = s.processSetupMessage((*controlmessage.Setup)(setupMsg))
			if err != nil {
				return err
			}

			err = wstream.WriteMessage(&controlmessage.ServerSetup{})
			if err != nil {
				return err
			}

			wstream.Close() //nolint:errcheck
			return wstream.Drain()
		}
	}

	select {
	case <-s.setupReceived:
	case <-errGroupCtx.Done():
		return fmt.Errorf("terminated")
	}

	msg, err := wstream.ReadMessage()
	if err != nil {
		return err
	}

	switch m := msg.(type) {
	case *controlmessage.Subscribe:
		if isCatalogTrack(m.TrackName) {
			return s.onSubscribeCatalog(errGroupCtx, errGroup, wstream, m)
		}

		return s.onSubscribeTrack(errGroupCtx, wstream, m)

	case *controlmessage.Publish:
		if isCatalogTrack(m.TrackName) {
			return s.onPublishCatalog(errGroupCtx, errGroup, wstream, m)
		}

		return s.onPublishTrack(wstream)

	default:
		return fmt.Errorf("unsupported message type: %T", msg)
	}
}

func (s *session) getPathNameAndQuery() (string, string) {
	s.mutex.Lock()
	defer s.mutex.Unlock()

	return s.pathName, s.query
}

func (s *session) onSubscribeCatalog(
	errGroupCtx context.Context,
	errGroup *errgroup.Group,
	wstream *moq.ControlStream,
	m *controlmessage.Subscribe,
) error {
	s.mutex.Lock()
	if s.state != defs.APIMoQSessionStateIdle {
		s.mutex.Unlock()
		return fmt.Errorf("unexpected SUBSCRIBE in state %s", s.state)
	}
	s.state = defs.APIMoQSessionStateRead
	s.mutex.Unlock()

	pathName, query := s.getPathNameAndQuery()

	remoteHost, _, _ := net.SplitHostPort(s.conn.RemoteAddr().String())
	addRes, err := s.pathManager.AddReader(defs.PathAddReaderReq{
		Author: s,
		AccessRequest: defs.PathAccessRequest{
			Name:                 pathName,
			Query:                query,
			Proto:                auth.ProtocolMoQ,
			ID:                   &s.uuid,
			Credentials:          credentialsFromAuthorizationToken(findAuthorizationToken(m.Parameters)),
			IP:                   net.ParseIP(remoteHost),
			UserAgent:            s.userAgent,
			EnableAskCredentials: false,
		},
	})
	if err != nil {
		var code controlmessage.RequestErrorCode
		if _, ok := errors.AsType[*auth.Error](err); ok {
			code = controlmessage.RequestErrorCodeUnauthorized
		} else if _, ok = errors.AsType[*defs.PathNoStreamAvailableError](err); ok {
			code = controlmessage.RequestErrorCodeDoesNotExist
		} else {
			code = controlmessage.RequestErrorCodeNotSupported
		}

		wstream.WriteMessage(&controlmessage.RequestError{ //nolint:errcheck
			Code:   code,
			Reason: err.Error(),
		})

		// wait for the client to read the error
		wstream.Close() //nolint:errcheck
		wstream.Drain() //nolint:errcheck

		return err
	}

	if addRes.AuthRes.ExpiresAt != nil {
		errGroup.Go(func() error {
			select {
			case <-time.After(time.Until(*addRes.AuthRes.ExpiresAt)):
				return fmt.Errorf("session expired")
			case <-errGroupCtx.Done():
				return nil
			}
		})
	}

	cat, setupTracks, err := moq.FromStream(addRes.Stream.OrigDesc)
	if err != nil {
		addRes.Path.RemoveReader(defs.PathRemoveReaderReq{Author: s})
		return err
	}

	s.mutex.Lock()
	s.path = addRes.Path
	s.user = addRes.AuthRes.User
	s.stream = addRes.Stream
	s.setupTracks = setupTracks
	s.expiresAt = addRes.AuthRes.ExpiresAt
	s.mutex.Unlock()

	s.Log(logger.Info, "is reading from path %s", pathName)

	err = wstream.WriteMessage(&controlmessage.SubscribeOk{TrackAlias: m.RequestID})
	if err != nil {
		return err
	}

	enc, err := json.Marshal(cat)
	if err != nil {
		return err
	}

	dataWStream, err := s.conn.OpenUniStreamSync(context.Background())
	if err != nil {
		return err
	}
	defer dataWStream.Close() //nolint:errcheck

	err = dataWStream.WriteSubGroup(&subgroup.SubGroup{
		Header: subgroup.Header{
			HasProperties: false,
			IsFirstObject: true,
			TrackAlias:    m.RequestID,
			GroupID:       0,
		},
		Objects: []subgroup.Object{{
			Payload: enc,
		}},
	})
	if err != nil {
		return err
	}

	wstream.Close() //nolint:errcheck
	wstream.Drain() //nolint:errcheck

	return fmt.Errorf("SUBSCRIBE catalog stream closed")
}

func (s *session) onSubscribeTrack(
	errGroupCtx context.Context,
	wstream *moq.ControlStream,
	m *controlmessage.Subscribe,
) error {
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

		if trackID >= len(s.setupTracks) {
			return fmt.Errorf("track index %d out of range", trackID)
		}

		return nil
	}()
	if err != nil {
		return err
	}

	r := &stream.Reader{Parent: s}

	groupID := uint64(0)

	writeData := func(payload []byte, pts int64) error {
		wstream, err2 := s.conn.OpenUniStreamSync(context.Background())
		if err2 != nil {
			return err2
		}
		defer wstream.Close() //nolint:errcheck

		sg := &subgroup.SubGroup{
			Header: subgroup.Header{
				HasProperties: true,
				IsFirstObject: true,
				TrackAlias:    m.RequestID,
				GroupID:       groupID,
			},
			Objects: []subgroup.Object{{
				Properties: property.Properties{
					new(property.Timestamp(pts)),
				},
				Payload: payload,
			}},
		}
		groupID++

		err2 = wstream.WriteSubGroup(sg)
		if err2 == nil {
			s.outboundBytes.Add(uint64(len(payload)))
		}
		return err2
	}

	s.setupTracks[trackID](r, writeData)

	s.stream.AddReader(r)
	defer s.stream.RemoveReader(r)

	err = wstream.WriteMessage(&controlmessage.SubscribeOk{TrackAlias: m.RequestID})
	if err != nil {
		return err
	}

	wstream.Close() //nolint:errcheck

	streamClosed := make(chan struct{})
	go func() {
		wstream.Drain() //nolint:errcheck
		close(streamClosed)
	}()

	select {
	case err = <-r.Error():
		return err

	case <-streamClosed:
		return nil

	case <-errGroupCtx.Done():
		return fmt.Errorf("terminated")
	}
}

func (s *session) onPublishCatalog(
	errGroupCtx context.Context,
	errGroup *errgroup.Group,
	wstream *moq.ControlStream,
	m *controlmessage.Publish,
) error {
	s.mutex.Lock()
	if s.state != defs.APIMoQSessionStateIdle {
		s.mutex.Unlock()
		return fmt.Errorf("unexpected PUBLISH in state %s", s.state)
	}
	s.state = defs.APIMoQSessionStatePublish
	s.mutex.Unlock()

	pathName, query := s.getPathNameAndQuery()

	select {
	case cat := <-s.catalogReceived:
		var subStream *stream.SubStream

		medias, writeFuncs, err := moq.ToStream(cat, &subStream)
		if err != nil {
			return err
		}

		s.inboundTracks = make(map[uint64]*inboundTrack)

		orchestrator := &reorderer.Orchestrator{Parent: s}
		orchestrator.Initialize()

		for i := range cat.Tracks {
			trackAlias := uint64(i + 1)
			tr := &inboundTrack{
				onSubGroup:   writeFuncs[trackAlias],
				orchestrator: orchestrator,
			}
			tr.initialize()
			s.inboundTracks[trackAlias] = tr
		}

		remoteHost, _, _ := net.SplitHostPort(s.conn.RemoteAddr().String())
		addRes, err := s.pathManager.AddPublisher(defs.PathAddPublisherReq{
			Author:        s,
			Desc:          &description.Session{Medias: medias},
			UseRTPPackets: false,
			ReplaceNTP:    true,
			AccessRequest: defs.PathAccessRequest{
				Name:                 pathName,
				Query:                query,
				Publish:              true,
				Proto:                auth.ProtocolMoQ,
				ID:                   &s.uuid,
				Credentials:          credentialsFromAuthorizationToken(findAuthorizationToken(m.Parameters)),
				IP:                   net.ParseIP(remoteHost),
				UserAgent:            s.userAgent,
				EnableAskCredentials: false,
			},
		})
		if err != nil {
			var code controlmessage.RequestErrorCode
			if _, ok := errors.AsType[*auth.Error](err); ok {
				code = controlmessage.RequestErrorCodeUnauthorized
			} else {
				code = controlmessage.RequestErrorCodeUninterested
			}

			wstream.WriteMessage(&controlmessage.RequestError{ //nolint:errcheck
				Code:   code,
				Reason: err.Error(),
			})

			// wait for the client to read the error
			wstream.Close() //nolint:errcheck
			wstream.Drain() //nolint:errcheck

			return err
		}

		if addRes.AuthRes.ExpiresAt != nil {
			errGroup.Go(func() error {
				select {
				case <-time.After(time.Until(*addRes.AuthRes.ExpiresAt)):
					return fmt.Errorf("session expired")
				case <-errGroupCtx.Done():
					return nil
				}
			})
		}

		s.mutex.Lock()
		s.path = addRes.Path
		s.user = addRes.AuthRes.User
		s.expiresAt = addRes.AuthRes.ExpiresAt
		s.mutex.Unlock()

		subStream = addRes.SubStream

		close(s.publishReady)

	case <-s.publishReady:
		return fmt.Errorf("catalog already received")

	case <-errGroupCtx.Done():
		return fmt.Errorf("terminated")
	}

	var ack controlmessage.Message
	if s.conn.Version == version.Draft16 || s.conn.Version == version.Draft17 {
		ack = &controlmessage.PublishOk{}
	} else {
		ack = &controlmessage.RequestOk{}
	}

	err := wstream.WriteMessage(ack)
	if err != nil {
		return err
	}

	wstream.Close() //nolint:errcheck
	wstream.Drain() //nolint:errcheck

	return fmt.Errorf("PUBLISH catalog stream closed")
}

func (s *session) onPublishTrack(wstream *moq.ControlStream) error {
	s.mutex.Lock()
	if s.state != defs.APIMoQSessionStatePublish {
		s.mutex.Unlock()
		return fmt.Errorf("unexpected PUBLISH in state %s", s.state)
	}
	s.mutex.Unlock()

	var ack controlmessage.Message
	if s.conn.Version == version.Draft16 || s.conn.Version == version.Draft17 {
		ack = &controlmessage.PublishOk{}
	} else {
		ack = &controlmessage.RequestOk{}
	}

	err := wstream.WriteMessage(ack)
	if err != nil {
		return err
	}

	wstream.Close() //nolint:errcheck
	wstream.Drain() //nolint:errcheck

	return fmt.Errorf("PUBLISH track stream closed")
}

func (s *session) onUniSubGroup(errGroupCtx context.Context, r *moq.IncomingUniStream) error {
	sg, err := r.ReadSubGroup()
	if err != nil {
		return err
	}

	if sg.Header.TrackAlias == 0 {
		return s.onDataCatalog(r, sg)
	}

	return s.onDataTrack(errGroupCtx, r, sg)
}

func (s *session) onDataCatalog(r *moq.IncomingUniStream, sg *subgroup.SubGroup) error {
	var cat catalog.Catalog
	err := json.Unmarshal(sg.Objects[0].Payload, &cat)
	if err != nil {
		return fmt.Errorf("failed to parse catalog JSON: %w", err)
	}

	if len(cat.Tracks) > maxCatalogTracks {
		return fmt.Errorf("too many catalog tracks: %d", len(cat.Tracks))
	}

	select {
	case s.catalogReceived <- &cat:
	default:
		return fmt.Errorf("catalog already received")
	}

	r.Drain() //nolint:errcheck
	return nil
}

func (s *session) onDataTrack(errGroupCtx context.Context, r *moq.IncomingUniStream, sg *subgroup.SubGroup) error {
	select {
	case <-s.publishReady:
	case <-errGroupCtx.Done():
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

	r.Drain() //nolint:errcheck
	return nil
}

func (s *session) apiItem() defs.APIMoQSession {
	s.mutex.Lock()
	state := s.state
	pathName := s.pathName
	query := s.query
	user := s.user
	expiresAt := s.expiresAt
	s.mutex.Unlock()

	return defs.APIMoQSession{
		ID:            s.uuid,
		Created:       s.created,
		RemoteAddr:    s.conn.RemoteAddr().String(),
		State:         state,
		Path:          pathName,
		Query:         query,
		User:          user,
		ExpiresAt:     expiresAt,
		UserAgent:     s.userAgent,
		Transport:     s.transport(),
		Version:       defs.APIMoQVersion(s.conn.Version.String()),
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
