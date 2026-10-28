// Package moq contains Media-over-QUIC utilities.
package moq

import (
	"context"
	"crypto/tls"
	"encoding/base64"
	"fmt"
	"io"
	"net"
	"net/url"
	"strings"
	"sync"

	"github.com/quic-go/quic-go"
	"github.com/quic-go/webtransport-go"

	"github.com/bluenviron/mediamtx/internal/conf"
	"github.com/bluenviron/mediamtx/internal/logger"
	"github.com/bluenviron/mediamtx/internal/protocols/moq/controlmessage"
	"github.com/bluenviron/mediamtx/internal/protocols/moq/parameter"
	"github.com/bluenviron/mediamtx/internal/protocols/moq/property"
	"github.com/bluenviron/mediamtx/internal/protocols/moq/subgroup"
	"github.com/bluenviron/mediamtx/internal/protocols/moq/version"
)

func encodeAuthorization(user *url.Userinfo) *parameter.AuthorizationToken {
	if user == nil {
		return nil
	}

	username := user.Username()
	password, passwordOK := user.Password()

	if username == "" || !passwordOK {
		return nil
	}

	credentials := base64.StdEncoding.EncodeToString([]byte(username + ":" + password))

	return &parameter.AuthorizationToken{
		AliasType:  parameter.AuthorizationTokenAliasTypeUseValue,
		TokenType:  1,
		TokenValue: []byte("Basic " + credentials),
	}
}

func expectedSetupPath(transport conf.MoQTransport, u *url.URL) string {
	if transport == conf.MoQTransportWebTransport {
		return ""
	}

	path := u.RequestURI()
	if path == "" {
		return "/"
	}

	return path
}

func protocolListToString(protocols []version.Version) []string {
	result := make([]string, len(protocols))
	for i, v := range protocols {
		result[i] = v.String()
	}
	return result
}

func dialQUIC(
	ctx context.Context,
	u *url.URL,
	tlsConfig *tls.Config,
	protocols []version.Version,
) (*quic.Conn, version.Version, error) {
	host := u.Host
	if !strings.Contains(host, ":") {
		host += ":443"
	}

	cfg := &tls.Config{}
	if tlsConfig != nil {
		cfg = tlsConfig.Clone()
	}
	cfg.NextProtos = protocolListToString(protocols)
	if cfg.ServerName == "" {
		hostname := u.Hostname()
		if hostname != "" && tlsConfig == nil {
			cfg.ServerName = hostname
		}
	}

	qconn, err := quic.DialAddr(ctx, host, cfg, &quic.Config{EnableDatagrams: true})
	if err != nil {
		return nil, 0, err
	}

	version, ok := version.Parse(qconn.ConnectionState().TLS.NegotiatedProtocol)
	if !ok {
		qconn.CloseWithError(0, "") //nolint:errcheck
		return nil, 0, fmt.Errorf("unsupported MoQ version: %s", qconn.ConnectionState().TLS.NegotiatedProtocol)
	}

	return qconn, version, nil
}

func dialWebTransport(
	ctx context.Context,
	u *url.URL,
	tlsConfig *tls.Config,
	protocols []version.Version,
) (*webtransport.Session, *webtransport.Transport, io.ReadCloser, version.Version, error) {
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
		ApplicationProtocols: protocolListToString(protocols),
	}

	res, session, err := transport.Dial(ctx, httpsURL.String(), nil)
	if err != nil {
		transport.Close() //nolint:errcheck
		return nil, nil, nil, 0, err
	}

	version, ok := version.Parse(strings.Trim(res.Header.Get("WT-Protocol"), `"`))
	if !ok {
		res.Body.Close()              //nolint:errcheck
		session.CloseWithError(0, "") //nolint:errcheck
		transport.Close()             //nolint:errcheck
		return nil, nil, nil, 0, fmt.Errorf("unsupported MoQ version: %s", res.Header.Get("WT-Protocol"))
	}

	return session, transport, res.Body, version, nil
}

// Client is a MoQ client.
//
// Subscribe, Publish and Close must not be called concurrently.
type Client struct {
	URL             *url.URL
	Transport       conf.MoQTransport
	TLSConfig       *tls.Config
	ClientProtocols []version.Version
	Log             logger.Writer

	conn            *Conn
	nextRequestID   uint64
	acceptorStarted bool

	tracksMutex  sync.RWMutex
	tracks       map[uint64]func(sg *subgroup.SubGroup) error
	err          chan error
	acceptorDone chan struct{}
}

func (c *Client) performSetup(ctx context.Context, path string) error {
	if c.conn.Version == version.Draft16 {
		bidiStream, err := c.conn.OpenStreamSync(ctx)
		if err != nil {
			return err
		}
		defer bidiStream.Close() //nolint:errcheck

		err = bidiStream.WriteMessage(&controlmessage.ClientSetup{Path: path})
		if err != nil {
			return err
		}

		msg, err := bidiStream.ReadMessage()
		if err != nil {
			return err
		}

		if _, ok := msg.(*controlmessage.ServerSetup); !ok {
			return fmt.Errorf("unexpected setup response: %T", msg)
		}

		return nil
	}

	clientStream, err := c.conn.OpenUniStreamSync(ctx)
	if err != nil {
		return err
	}
	defer clientStream.Close() //nolint:errcheck

	payload := controlmessage.Setup{}
	if path != "" {
		payload.Path = path
	}

	err = clientStream.WriteMessage(&payload)
	if err != nil {
		return err
	}

	serverStream, err := c.conn.AcceptUniStream(ctx)
	if err != nil {
		return err
	}

	msg, err := serverStream.ReadMessage()
	if err != nil {
		return err
	}
	if _, ok := msg.(*controlmessage.Setup); !ok {
		return fmt.Errorf("unexpected setup response: %T", msg)
	}

	return nil
}

// RemoteAddr returns the remote address of the connection.
func (c *Client) RemoteAddr() net.Addr {
	return c.conn.RemoteAddr()
}

// Initialize initializes the Client.
func (c *Client) Initialize(ctx context.Context) error {
	transport := c.Transport
	if transport == "" {
		transport = conf.MoQTransportQUIC
	}

	protocols := c.ClientProtocols
	if c.ClientProtocols == nil {
		protocols = version.List
	}

	if transport == conf.MoQTransportQUIC {
		conn, version, err := dialQUIC(ctx, c.URL, c.TLSConfig, protocols)
		if err != nil {
			return err
		}
		c.conn = &Conn{
			Raw:     &RawConnQUIC{Conn: conn},
			Log:     c.Log,
			Version: version,
		}
	} else {
		session, transport, resBody, version, err := dialWebTransport(ctx, c.URL, c.TLSConfig, protocols)
		if err != nil {
			return err
		}

		c.conn = &Conn{
			Raw: &RawConnWebTransport{
				Session:      session,
				Transport:    transport,
				ResponseBody: resBody,
			},
			Log:     c.Log,
			Version: version,
		}
	}

	err := c.performSetup(ctx, expectedSetupPath(transport, c.URL))
	if err != nil {
		c.conn.CloseWithError(0, "")
		c.conn = nil
		return err
	}

	c.nextRequestID = 1
	c.tracks = make(map[uint64]func(sg *subgroup.SubGroup) error)
	c.err = make(chan error, 1)
	c.acceptorDone = make(chan struct{})

	return nil
}

// Close closes the Client.
func (c *Client) Close() error {
	if c.conn != nil {
		c.conn.CloseWithError(0, "")
	}

	if c.acceptorStarted {
		<-c.acceptorDone
	}

	c.conn = nil

	return nil
}

// Version returns the negotiated MoQ version.
func (c *Client) Version() version.Version {
	return c.conn.Version
}

func (c *Client) runUniStreamAcceptor() {
	defer close(c.acceptorDone)

	var wg sync.WaitGroup
	defer wg.Wait()

	for {
		uni, err := c.conn.AcceptUniStream(context.Background())
		if err != nil {
			c.sendError(err)
			return
		}

		wg.Go(func() {
			c.handleUniStream(uni)
		})
	}
}

func (c *Client) handleUniStream(uni *IncomingUniStream) {
	sg, err := uni.ReadSubGroup()
	if err != nil {
		c.sendError(err)
		return
	}

	c.tracksMutex.RLock()
	onSubGroup := c.tracks[sg.Header.TrackAlias]
	c.tracksMutex.RUnlock()

	if onSubGroup == nil {
		c.sendError(fmt.Errorf("received subgroup with unknown track alias: %d", sg.Header.TrackAlias))
		return
	}

	err = onSubGroup(sg)
	if err != nil {
		c.sendError(err)
	}
}

func (c *Client) sendError(err error) {
	select {
	case c.err <- err:
	default:
	}
}

// Wait waits until a fatal error occurs.
func (c *Client) Wait() error {
	return <-c.err
}

// Errors returns fatal client errors.
func (c *Client) Errors() <-chan error {
	return c.err
}

// Subscribe sends a SUBSCRIBE request.
func (c *Client) Subscribe(
	ctx context.Context,
	trackName string,
	onSubGroup func(sg *subgroup.SubGroup) error,
) error {
	requestID := c.nextRequestID
	c.nextRequestID++

	c.tracksMutex.Lock()
	defer c.tracksMutex.Unlock()

	bidi, err := c.conn.OpenStreamSync(ctx)
	if err != nil {
		return err
	}

	var params []parameter.Parameter
	auth := encodeAuthorization(c.URL.User)
	if auth != nil {
		params = []parameter.Parameter{auth}
	}

	err = bidi.WriteMessage(&controlmessage.Subscribe{
		RequestID:  requestID,
		TrackName:  trackName,
		Parameters: params,
	})
	if err != nil {
		bidi.Close() //nolint:errcheck
		return err
	}

	msg, err := bidi.ReadMessage()
	if err != nil {
		bidi.Close() //nolint:errcheck
		return err
	}

	switch msg := msg.(type) {
	case *controlmessage.SubscribeOk:
		c.tracks[msg.TrackAlias] = onSubGroup
		if !c.acceptorStarted {
			c.acceptorStarted = true
			go c.runUniStreamAcceptor()
		}

		// do not close the stream
		go bidi.Drain() //nolint:errcheck
		return nil

	case *controlmessage.RequestError:
		bidi.Close() //nolint:errcheck
		return fmt.Errorf("subscribe failed: %s", msg.Reason)

	default:
		bidi.Close() //nolint:errcheck
		return fmt.Errorf("unexpected subscribe response: %T", msg)
	}
}

// Publish sends a PUBLISH request.
func (c *Client) Publish(
	ctx context.Context,
	trackName string,
	trackAlias uint64,
	trackProperties property.Properties,
) error {
	bidi, err := c.conn.OpenStreamSync(ctx)
	if err != nil {
		return err
	}

	var params []parameter.Parameter
	auth := encodeAuthorization(c.URL.User)
	if auth != nil {
		params = []parameter.Parameter{auth}
	}

	requestID := c.nextRequestID
	c.nextRequestID++

	err = bidi.WriteMessage(&controlmessage.Publish{
		RequestID:       requestID,
		TrackName:       trackName,
		TrackAlias:      trackAlias,
		Parameters:      params,
		TrackProperties: trackProperties,
	})
	if err != nil {
		bidi.Close() //nolint:errcheck
		return err
	}

	msg, err := bidi.ReadMessage()
	if err != nil {
		bidi.Close() //nolint:errcheck
		return err
	}

	switch msg := msg.(type) {
	case *controlmessage.PublishOk, *controlmessage.RequestOk:
		// do not close the stream
		go bidi.Drain() //nolint:errcheck
		return nil

	case *controlmessage.RequestError:
		bidi.Close() //nolint:errcheck
		return fmt.Errorf("publish failed: %s", msg.Reason)

	default:
		bidi.Close() //nolint:errcheck
		return fmt.Errorf("unexpected publish response: %T", msg)
	}
}

// WriteSubGroup sends a subgroup.
func (c *Client) WriteSubGroup(
	ctx context.Context,
	trackAlias uint64,
	groupID uint64,
	props property.Properties,
	payload []byte,
) error {
	uni, err := c.conn.OpenUniStreamSync(ctx)
	if err != nil {
		return err
	}
	defer uni.Close() //nolint:errcheck

	sg := &subgroup.SubGroup{
		Header: subgroup.Header{
			HasProperties: len(props) != 0,
			IsFirstObject: true,
			TrackAlias:    trackAlias,
			GroupID:       groupID,
		},
		Objects: []subgroup.Object{{
			Properties: props,
			Payload:    payload,
		}},
	}

	return uni.WriteSubGroup(sg)
}
