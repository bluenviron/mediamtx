package push

import (
	"context"
	"crypto/tls"
	"fmt"
	"net"
	"net/url"
	"strings"
	"time"

	"github.com/bluenviron/gortmplib/pkg/amf0"
	"github.com/bluenviron/gortmplib/pkg/bytecounter"
	"github.com/bluenviron/gortmplib/pkg/handshake"
	"github.com/bluenviron/gortmplib/pkg/message"
)

const rtmpPublishTypeLive = "live"

type rtmpPublishClient struct {
	URL       *url.URL
	TLSConfig *tls.Config

	DialContext func(ctx context.Context, network string, address string) (net.Conn, error)

	nconn net.Conn
	bc    *bytecounter.ReadWriter
	mrw   *message.ReadWriter
}

func rtmpSplitPath(u *url.URL) (string, string) {
	nu := *u
	nu.ForceQuery = false
	pathsegs := strings.Split(nu.RequestURI(), "/")

	var app string
	var streamKey string

	switch {
	case len(pathsegs) == 2:
		app = pathsegs[1]

	case len(pathsegs) == 3:
		app = pathsegs[1]
		streamKey = pathsegs[2]

	case len(pathsegs) > 3:
		app = strings.Join(pathsegs[1:3], "/")
		streamKey = strings.Join(pathsegs[3:], "/")
	}

	return app, streamKey
}

func rtmpTCURL(u *url.URL) string {
	app, _ := rtmpSplitPath(u)
	nu, _ := url.Parse(u.String())
	nu.RawQuery = ""
	nu.Path = "/"
	return nu.String() + app
}

func rtmpObject(in any) (amf0.Object, bool) {
	switch o := in.(type) {
	case amf0.Object:
		return o, true

	case amf0.ECMAArray:
		return amf0.Object(o), true

	default:
		return nil, false
	}
}

func rtmpCommandResultIsOK1(res *message.CommandAMF0) bool {
	if len(res.Arguments) < 2 {
		return false
	}

	ma, ok := rtmpObject(res.Arguments[1])
	if !ok {
		return false
	}

	v, ok := ma.Get("level")
	return ok && v == "status"
}

func rtmpCommandResultIsOK2(res *message.CommandAMF0) bool {
	if len(res.Arguments) < 2 {
		return false
	}

	v, ok := res.Arguments[1].(float64)
	return ok && v == 1
}

func rtmpReadCommandResult(
	mrw *message.ReadWriter,
	commandID int,
) (*message.CommandAMF0, error) {
	for {
		msg, err := mrw.Read()
		if err != nil {
			return nil, err
		}

		if cmd, ok := msg.(*message.CommandAMF0); ok {
			if cmd.CommandID == commandID ||
				(cmd.CommandID == 0 && (cmd.Name == "_result" || cmd.Name == "_error")) {
				return cmd, nil
			}
		}
	}
}

func rtmpWaitOnStatus(
	mrw *message.ReadWriter,
	commandID int,
) (*message.CommandAMF0, error) {
	for {
		msg, err := mrw.Read()
		if err != nil {
			return nil, err
		}

		if cmd, ok := msg.(*message.CommandAMF0); ok {
			if cmd.CommandID == commandID || (cmd.CommandID == 0 && cmd.Name == "onStatus") {
				return cmd, nil
			}
		}
	}
}

func (c *rtmpPublishClient) Initialize(ctx context.Context) error {
	if c.DialContext == nil {
		c.DialContext = (&net.Dialer{}).DialContext
	}

	switch c.URL.Scheme {
	case "rtmp", "rtmps":
	default:
		return fmt.Errorf("unsupported scheme: %s", c.URL.Scheme)
	}

	var err error
	c.nconn, err = c.DialContext(ctx, "tcp", c.URL.Host)
	if err != nil {
		return err
	}

	if c.URL.Scheme == "rtmps" {
		tlsConfig := c.TLSConfig
		if tlsConfig == nil {
			tlsConfig = &tls.Config{} //nolint:gosec
		} else {
			tlsConfig = tlsConfig.Clone()
		}

		if tlsConfig.ServerName == "" {
			host, _, _ := net.SplitHostPort(c.URL.Host)
			tlsConfig.ServerName = host
		}

		c.nconn = tls.Client(c.nconn, tlsConfig)
	}

	closerDone := make(chan struct{})
	defer func() { <-closerDone }()

	closerTerminate := make(chan struct{})
	defer close(closerTerminate)

	nconn := c.nconn
	go func() {
		defer close(closerDone)

		select {
		case <-closerTerminate:
		case <-ctx.Done():
			nconn.Close()
		}
	}()

	err = c.initialize()
	if err != nil {
		c.nconn.Close()
		return err
	}

	return nil
}

func (c *rtmpPublishClient) initialize() error {
	c.bc = bytecounter.NewReadWriter(c.nconn)

	_, _, err := handshake.DoClient(c.bc, false, false)
	if err != nil {
		return err
	}

	c.mrw = message.NewReadWriter(c.bc, c.bc, false)

	for _, msg := range []message.Message{
		&message.SetWindowAckSize{Value: 2500000},
		&message.SetPeerBandwidth{Value: 2500000, Type: 2},
		&message.SetChunkSize{Value: 65536},
	} {
		err = c.mrw.Write(msg)
		if err != nil {
			return err
		}
	}

	app, streamKey := rtmpSplitPath(c.URL)

	err = c.mrw.Write(&message.CommandAMF0{
		ChunkStreamID: 3,
		Name:          "connect",
		CommandID:     1,
		Arguments: []any{
			amf0.Object{
				{Key: "app", Value: app},
				{Key: "flashVer", Value: "LNX 9,0,124,2"},
				{Key: "tcUrl", Value: rtmpTCURL(c.URL)},
				{Key: "objectEncoding", Value: float64(0)},
			},
		},
	})
	if err != nil {
		return err
	}

	res, err := rtmpReadCommandResult(c.mrw, 1)
	if err != nil {
		return err
	}

	if res.Name != "_result" {
		return fmt.Errorf("bad result: %v", res)
	}

	err = c.mrw.Write(&message.CommandAMF0{
		ChunkStreamID: 3,
		Name:          "releaseStream",
		CommandID:     2,
		Arguments: []any{
			nil,
			streamKey,
		},
	})
	if err != nil {
		return err
	}

	err = c.mrw.Write(&message.CommandAMF0{
		ChunkStreamID: 3,
		Name:          "FCPublish",
		CommandID:     3,
		Arguments: []any{
			nil,
			streamKey,
		},
	})
	if err != nil {
		return err
	}

	err = c.mrw.Write(&message.CommandAMF0{
		ChunkStreamID: 3,
		Name:          "createStream",
		CommandID:     4,
		Arguments: []any{
			nil,
		},
	})
	if err != nil {
		return err
	}

	res, err = rtmpReadCommandResult(c.mrw, 4)
	if err != nil {
		return err
	}

	if res.Name != "_result" || !rtmpCommandResultIsOK2(res) {
		return fmt.Errorf("bad result: %v", res)
	}

	err = c.mrw.Write(&message.CommandAMF0{
		ChunkStreamID:   4,
		MessageStreamID: 0x1000000,
		Name:            "publish",
		CommandID:       5,
		Arguments: []any{
			nil,
			streamKey,
			rtmpPublishTypeLive,
		},
	})
	if err != nil {
		return err
	}

	res, err = rtmpWaitOnStatus(c.mrw, 5)
	if err != nil {
		return err
	}

	if res.Name != "onStatus" || !rtmpCommandResultIsOK1(res) {
		return fmt.Errorf("bad result: %v", res)
	}

	return nil
}

func (c *rtmpPublishClient) Close() {
	if c.nconn != nil {
		c.nconn.Close()
	}
}

func (c *rtmpPublishClient) NetConn() net.Conn {
	return c.nconn
}

func (c *rtmpPublishClient) BytesReceived() uint64 {
	return c.bc.Reader.Count()
}

func (c *rtmpPublishClient) BytesSent() uint64 {
	return c.bc.Writer.Count()
}

func (c *rtmpPublishClient) Read() (message.Message, error) {
	return c.mrw.Read()
}

func (c *rtmpPublishClient) Write(msg message.Message) error {
	return c.mrw.Write(msg)
}

func (c *rtmpPublishClient) SetReadDeadline(t time.Time) error {
	return c.nconn.SetReadDeadline(t)
}
