// Package moq contains the MoQ static source.
package moq

import (
	"bufio"
	"context"
	"crypto/tls"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/url"
	"strings"

	"github.com/bluenviron/gortsplib/v5/pkg/description"
	"github.com/quic-go/quic-go"
	"github.com/quic-go/webtransport-go"

	"github.com/bluenviron/mediamtx/internal/conf"
	"github.com/bluenviron/mediamtx/internal/defs"
	"github.com/bluenviron/mediamtx/internal/logger"
	protomoq "github.com/bluenviron/mediamtx/internal/protocols/moq"
	"github.com/bluenviron/mediamtx/internal/protocols/moq/catalog"
	"github.com/bluenviron/mediamtx/internal/protocols/moq/controlmessage"
	"github.com/bluenviron/mediamtx/internal/protocols/moq/parameter"
	"github.com/bluenviron/mediamtx/internal/protocols/moq/reorderer"
	"github.com/bluenviron/mediamtx/internal/protocols/moq/subgroup"
	ptls "github.com/bluenviron/mediamtx/internal/protocols/tls"
	"github.com/bluenviron/mediamtx/internal/stream"
)

const maxReorderedSubGroups = 50

type parent interface {
	logger.Writer
	SetReady(req defs.PathSourceStaticSetReadyReq) defs.PathSourceStaticSetReadyRes
	SetNotReady(req defs.PathSourceStaticSetNotReadyReq)
}

type conn interface {
	OpenUniStreamSync(ctx context.Context) (io.WriteCloser, error)
	AcceptUniStream(ctx context.Context) (io.Reader, error)
	OpenStreamSync(ctx context.Context) (io.ReadWriteCloser, error)
	CloseWithError(code uint64, msg string) error
	Close() error
}

type connQUIC struct {
	conn *quic.Conn
}

func (c *connQUIC) OpenUniStreamSync(ctx context.Context) (io.WriteCloser, error) {
	return c.conn.OpenUniStreamSync(ctx)
}

func (c *connQUIC) AcceptUniStream(ctx context.Context) (io.Reader, error) {
	return c.conn.AcceptUniStream(ctx)
}

func (c *connQUIC) OpenStreamSync(ctx context.Context) (io.ReadWriteCloser, error) {
	return c.conn.OpenStreamSync(ctx)
}

func (c *connQUIC) CloseWithError(code uint64, msg string) error {
	return c.conn.CloseWithError(quic.ApplicationErrorCode(code), msg)
}

func (c *connQUIC) Close() error {
	return c.conn.CloseWithError(0, "")
}

type connWebTransport struct {
	session      *webtransport.Session
	transport    *webtransport.Transport
	responseBody io.Closer
}

func (c *connWebTransport) OpenUniStreamSync(ctx context.Context) (io.WriteCloser, error) {
	return c.session.OpenUniStreamSync(ctx)
}

func (c *connWebTransport) AcceptUniStream(ctx context.Context) (io.Reader, error) {
	return c.session.AcceptUniStream(ctx)
}

func (c *connWebTransport) OpenStreamSync(ctx context.Context) (io.ReadWriteCloser, error) {
	return c.session.OpenStreamSync(ctx)
}

func (c *connWebTransport) CloseWithError(code uint64, msg string) error {
	return c.session.CloseWithError(webtransport.SessionErrorCode(code), msg)
}

func (c *connWebTransport) Close() error {
	c.session.CloseWithError(0, "") //nolint:errcheck
	if c.responseBody != nil {
		c.responseBody.Close() //nolint:errcheck
	}
	if c.transport != nil {
		return c.transport.Close()
	}
	return nil
}

type inboundTrack struct {
	onSubGroup func(sg *subgroup.SubGroup) error
	parent     logger.Writer

	reorderer *reorderer.Reorderer
}

func (t *inboundTrack) initialize() {
	t.reorderer = &reorderer.Reorderer{
		MaxReordered: maxReorderedSubGroups,
		Parent:       t.parent,
	}
	t.reorderer.Initialize()
}

func (t *inboundTrack) push(sg *subgroup.SubGroup) error {
	sgs, err := t.reorderer.Push(sg)
	if err != nil {
		return err
	}

	for _, sg := range sgs {
		err = t.onSubGroup(sg)
		if err != nil {
			return err
		}
	}

	return nil
}

func encodeAuthorization(user *url.Userinfo) parameter.Parameters {
	if user == nil {
		return nil
	}

	username := user.Username()
	password, ok := user.Password()
	if username == "" || !ok {
		return nil
	}

	credentials := base64.StdEncoding.EncodeToString([]byte(username + ":" + password))

	return parameter.Parameters{
		&parameter.AuthorizationToken{
			AliasType:  parameter.AuthorizationTokenAliasTypeUseValue,
			TokenType:  1,
			TokenValue: []byte("Basic " + credentials),
		},
	}
}

func parseVersionFromWTHeader(v string) defs.APIMoQVersion {
	v = strings.Trim(v, "\"")

	switch defs.APIMoQVersion(v) {
	case defs.APIMoQVersionDraft19,
		defs.APIMoQVersionDraft18,
		defs.APIMoQVersionDraft17,
		defs.APIMoQVersionDraft16:
		return defs.APIMoQVersion(v)
	default:
		return ""
	}
}

func performSetup(ctx context.Context, c conn, version defs.APIMoQVersion, path string) error {
	if version == defs.APIMoQVersionDraft16 {
		setupBidi, err := c.OpenStreamSync(ctx)
		if err != nil {
			return err
		}
		defer setupBidi.Close() //nolint:errcheck

		_, err = setupBidi.Write(controlmessage.ClientSetup(controlmessage.Setup{Path: path}).Marshal())
		if err != nil {
			return err
		}

		msg, err := controlmessage.Read(setupBidi)
		if err != nil {
			return err
		}

		if _, ok := msg.(*controlmessage.ServerSetup); !ok {
			return fmt.Errorf("unexpected setup response: %T", msg)
		}

		return nil
	}

	setupStream, err := c.AcceptUniStream(ctx)
	if err != nil {
		return err
	}

	msg, err := controlmessage.Read(setupStream)
	if err != nil {
		return err
	}

	if _, ok := msg.(*controlmessage.Setup); !ok {
		return fmt.Errorf("unexpected setup response: %T", msg)
	}

	clientSetup, err := c.OpenUniStreamSync(ctx)
	if err != nil {
		return err
	}
	defer clientSetup.Close() //nolint:errcheck

	payload := controlmessage.Setup{}
	if path != "" {
		payload.Path = path
	}

	_, err = clientSetup.Write(payload.Marshal())
	return err
}

func subscribe(
	ctx context.Context,
	c conn,
	requestID uint64,
	trackName string,
	params parameter.Parameters,
) (uint64, io.ReadWriteCloser, error) {
	bidi, err := c.OpenStreamSync(ctx)
	if err != nil {
		return 0, nil, err
	}

	_, err = bidi.Write(controlmessage.Subscribe{
		RequestID:  requestID,
		TrackName:  trackName,
		Parameters: params,
	}.Marshal())
	if err != nil {
		bidi.Close() //nolint:errcheck
		return 0, nil, err
	}

	msg, err := controlmessage.Read(bidi)
	if err != nil {
		bidi.Close() //nolint:errcheck
		return 0, nil, err
	}

	switch msg := msg.(type) {
	case *controlmessage.SubscribeOk:
		return msg.TrackAlias, bidi, nil

	case *controlmessage.RequestError:
		bidi.Close() //nolint:errcheck
		return 0, nil, fmt.Errorf("subscribe failed: %s", msg.Reason)

	default:
		bidi.Close() //nolint:errcheck
		return 0, nil, fmt.Errorf("unexpected subscribe response: %T", msg)
	}
}

func readSubGroup(r io.Reader) (*subgroup.SubGroup, error) {
	br := bufio.NewReader(r)
	firstByte, err := br.Peek(1)
	if err != nil {
		return nil, err
	}
	if (firstByte[0] & 0x90) != 0x10 {
		return nil, fmt.Errorf("unexpected unidirectional stream")
	}

	var sg subgroup.SubGroup
	err = sg.Read(br)
	if err != nil {
		return nil, err
	}

	return &sg, nil
}

func dialQUIC(ctx context.Context, u *url.URL, tlsConfig *tls.Config) (conn, defs.APIMoQVersion, error) {
	host := u.Host
	if !strings.Contains(host, ":") {
		host += ":443"
	}

	cfg := &tls.Config{}
	if tlsConfig != nil {
		cfg = tlsConfig.Clone()
	}
	cfg.NextProtos = []string{
		string(defs.APIMoQVersionDraft19),
		string(defs.APIMoQVersionDraft18),
		string(defs.APIMoQVersionDraft17),
		string(defs.APIMoQVersionDraft16),
	}
	if cfg.ServerName == "" {
		hostname := u.Hostname()
		if hostname != "" && tlsConfig == nil {
			cfg.ServerName = hostname
		}
	}

	qconn, err := quic.DialAddr(ctx, host, cfg, &quic.Config{EnableDatagrams: true})
	if err != nil {
		return nil, "", err
	}

	version := defs.APIMoQVersion(qconn.ConnectionState().TLS.NegotiatedProtocol)
	if version == "" {
		qconn.CloseWithError(0, "") //nolint:errcheck
		return nil, "", fmt.Errorf("missing negotiated MoQ version")
	}

	return &connQUIC{conn: qconn}, version, nil
}

func dialWebTransport(ctx context.Context, u *url.URL, tlsConfig *tls.Config) (conn, defs.APIMoQVersion, error) {
	httpsURL := &url.URL{
		Scheme:   "https",
		Host:     u.Host,
		Path:     u.Path,
		RawQuery: u.RawQuery,
	}

	cfg := &tls.Config{}
	if tlsConfig != nil {
		cfg = tlsConfig.Clone()
	}
	if cfg.ServerName == "" {
		hostname := u.Hostname()
		if hostname != "" && tlsConfig == nil {
			cfg.ServerName = hostname
		}
	}

	transport := &webtransport.Transport{
		TLSClientConfig: cfg,
		QUICConfig: &quic.Config{
			EnableDatagrams:                  true,
			EnableStreamResetPartialDelivery: true,
		},
		ApplicationProtocols: []string{
			string(defs.APIMoQVersionDraft19),
			string(defs.APIMoQVersionDraft18),
			string(defs.APIMoQVersionDraft17),
			string(defs.APIMoQVersionDraft16),
		},
	}

	res, session, err := transport.Dial(ctx, httpsURL.String(), nil)
	if err != nil {
		transport.Close() //nolint:errcheck
		return nil, "", err
	}

	version := parseVersionFromWTHeader(res.Header.Get("WT-Protocol"))
	if version == "" {
		res.Body.Close()              //nolint:errcheck
		session.CloseWithError(0, "") //nolint:errcheck
		transport.Close()             //nolint:errcheck
		return nil, "", fmt.Errorf("missing negotiated MoQ version")
	}

	return &connWebTransport{session: session, transport: transport, responseBody: res.Body}, version, nil
}

// Source is a MoQ static source.
type Source struct {
	ReadTimeout conf.Duration
	Parent      parent
}

// Log implements logger.Writer.
func (s *Source) Log(level logger.Level, format string, args ...any) {
	s.Parent.Log(level, "[MoQ source] "+format, args...)
}

// Run implements StaticSource.
func (s *Source) Run(params defs.StaticSourceRunParams) error {
	u, err := url.Parse(params.ResolvedSource)
	if err != nil {
		return err
	}

	tlsConfig := ptls.MakeConfig(params.Conf.SourceFingerprint)

	var c conn
	var version defs.APIMoQVersion

	switch params.Conf.MoQTransport {
	case conf.MoQTransportWebTransport:
		c, version, err = dialWebTransport(params.Context, u, tlsConfig)
	default:
		c, version, err = dialQUIC(params.Context, u, tlsConfig)
	}
	if err != nil {
		return err
	}
	defer c.Close() //nolint:errcheck

	setupPath := ""
	if params.Conf.MoQTransport == conf.MoQTransportQUIC {
		setupPath = u.RequestURI()
		if setupPath == "" {
			setupPath = "/"
		}
	}

	err = performSetup(params.Context, c, version, setupPath)
	if err != nil {
		return err
	}

	authParams := encodeAuthorization(u.User)

	catalogAlias, catalogBidi, err := subscribe(params.Context, c, 1, ".catalog", authParams)
	if err != nil {
		return err
	}
	go io.Copy(io.Discard, catalogBidi) //nolint:errcheck

	catalogStream, err := c.AcceptUniStream(params.Context)
	if err != nil {
		return err
	}

	sg, err := readSubGroup(catalogStream)
	if err != nil {
		return err
	}
	if sg.Header.TrackAlias != catalogAlias {
		return fmt.Errorf("unexpected catalog track alias: expected %d, got %d", catalogAlias, sg.Header.TrackAlias)
	}
	if len(sg.Objects) == 0 {
		return fmt.Errorf("received empty catalog")
	}

	var cat catalog.Catalog
	err = json.Unmarshal(sg.Objects[0].Payload, &cat)
	if err != nil {
		return fmt.Errorf("failed to parse catalog JSON: %w", err)
	}

	var subStream *stream.SubStream
	medias, writeFuncs, err := protomoq.ToStream(&cat, &subStream)
	if err != nil {
		return err
	}

	res := s.Parent.SetReady(defs.PathSourceStaticSetReadyReq{
		Desc:          &description.Session{Medias: medias},
		UseRTPPackets: false,
		ReplaceNTP:    !params.Conf.UseAbsoluteTimestamp,
	})
	if res.Err != nil {
		return res.Err
	}
	defer s.Parent.SetNotReady(defs.PathSourceStaticSetNotReadyReq{})

	subStream = res.SubStream

	tracks := make(map[uint64]*inboundTrack)
	for i, track := range cat.Tracks {
		writer := writeFuncs[uint64(i+1)]
		if writer == nil {
			return fmt.Errorf("missing writer for track %s", track.Name)
		}

		alias, bidi, err2 := subscribe(params.Context, c, uint64(i+2), track.Name, authParams)
		if err2 != nil {
			return err2
		}
		go io.Copy(io.Discard, bidi) //nolint:errcheck

		tr := &inboundTrack{onSubGroup: writer, parent: s}
		tr.initialize()
		tracks[alias] = tr
	}

	readErr := make(chan error, 1)
	go func() {
		for {
			uni, err2 := c.AcceptUniStream(params.Context)
			if err2 != nil {
				if params.Context.Err() != nil {
					readErr <- nil
					return
				}
				readErr <- err2
				return
			}

			sg2, err2 := readSubGroup(uni)
			if err2 != nil {
				readErr <- err2
				return
			}

			track := tracks[sg2.Header.TrackAlias]
			if track == nil {
				continue
			}

			err2 = track.push(sg2)
			if err2 != nil {
				readErr <- err2
				return
			}
		}
	}()

	for {
		select {
		case err = <-readErr:
			return err

		case <-params.ReloadConf:

		case <-params.Context.Done():
			c.CloseWithError(0, "") //nolint:errcheck
			return nil
		}
	}
}

// APISourceDescribe implements StaticSource.
func (*Source) APISourceDescribe() *defs.APIPathSource {
	return &defs.APIPathSource{
		Type: defs.APIPathSourceTypeMoQSource,
		ID:   "",
	}
}
