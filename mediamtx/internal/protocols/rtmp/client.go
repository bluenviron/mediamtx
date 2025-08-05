// Package rtmp provides RTMP connectivity.
package rtmp

import (
	"context"
	ctls "crypto/tls"
	"errors"
	"fmt"
	"net"
	"net/url"
	"strings"

	"github.com/bluenviron/mediamtx/internal/protocols/rtmp/amf0"
	"github.com/bluenviron/mediamtx/internal/protocols/rtmp/bytecounter"
	"github.com/bluenviron/mediamtx/internal/protocols/rtmp/handshake"
	"github.com/bluenviron/mediamtx/internal/protocols/rtmp/message"
	"github.com/google/uuid"
)

var errAuth = errors.New("auth")

func resultIsOK1(res *message.CommandAMF0) bool {
	if len(res.Arguments) < 2 {
		return false
	}

	ma, ok := objectOrArray(res.Arguments[1])
	if !ok {
		return false
	}

	v, ok := ma.Get("level")
	if !ok {
		return false
	}

	return (v == "status")
}

func resultIsOK2(res *message.CommandAMF0) bool {
	if len(res.Arguments) < 2 {
		return false
	}

	v, ok := res.Arguments[1].(float64)
	if !ok {
		return false
	}

	return v == 1
}

func splitPath(u *url.URL) (string, string) {
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

func getTcURL(u *url.URL) string {
	app, _ := splitPath(u)
	nu, _ := url.Parse(u.String()) // perform a deep copy
	nu.RawQuery = ""
	nu.Path = "/"
	return nu.String() + app
}

func readCommand(mrw *message.ReadWriter) (*message.CommandAMF0, error) {
	for {
		msg, err := mrw.Read()
		if err != nil {
			return nil, err
		}

		if cmd, ok := msg.(*message.CommandAMF0); ok {
			return cmd, nil
		}
	}
}

func readCommandResult(
	mrw *message.ReadWriter,
	commandID int,
) (*message.CommandAMF0, error) {
	for {
		msg, err := mrw.Read()
		if err != nil {
			return nil, err
		}

		if cmd, ok := msg.(*message.CommandAMF0); ok {
			if cmd.CommandID == commandID || cmd.CommandID == 0 {
				return cmd, nil
			}
		}
	}
}

type dialer interface {
	DialContext(ctx context.Context, network, address string) (net.Conn, error)
}

// Client is a client-side RTMP connection.
type Client struct {
	URL       *url.URL
	TLSConfig *ctls.Config
	Publish   bool

	nconn         net.Conn
	bc            *bytecounter.ReadWriter
	mrw           *message.ReadWriter
	authState     int
	authSalt      string
	authChallenge string
}

// Initialize initializes Client.
func (c *Client) Initialize(ctx context.Context) error {
	for {
		err := c.initialize2(ctx)
		if errors.Is(err, errAuth) {
			c.authState++
			continue
		}
		return err
	}
}

func (c *Client) initialize2(ctx context.Context) error {
	var dial dialer
	if c.URL.Scheme == "rtmp" {
		dial = &net.Dialer{}
	} else {
		dial = &ctls.Dialer{Config: c.TLSConfig}
	}

	var err error
	c.nconn, err = dial.DialContext(ctx, "tcp", c.URL.Host)
	if err != nil {
		return err
	}

	closerDone := make(chan struct{})
	defer func() { <-closerDone }()

	closerTerminate := make(chan struct{})
	defer close(closerTerminate)

	nc := c.nconn
	go func() {
		defer close(closerDone)

		select {
		case <-closerTerminate:
		case <-ctx.Done():
			nc.Close()
		}
	}()

	err = c.initialize3()
	if err != nil {
		c.nconn.Close()
		return err
	}

	return nil
}

func (c *Client) initialize3() error {
	c.bc = bytecounter.NewReadWriter(c.nconn)

	_, _, err := handshake.DoClient(c.bc, false, false)
	if err != nil {
		return err
	}

	c.mrw = message.NewReadWriter(c.bc, c.bc, false)

	err = c.mrw.Write(&message.SetWindowAckSize{
		Value: 2500000,
	})
	if err != nil {
		return err
	}

	err = c.mrw.Write(&message.SetPeerBandwidth{
		Value: 2500000,
		Type:  2,
	})
	if err != nil {
		return err
	}

	err = c.mrw.Write(&message.SetChunkSize{
		Value: 65536,
	})
	if err != nil {
		return err
	}

	cleanURL := &url.URL{
		Scheme:      c.URL.Scheme,
		Opaque:      c.URL.Opaque,
		Host:        c.URL.Host,
		Path:        c.URL.Path,
		RawPath:     c.URL.RawPath,
		OmitHost:    c.URL.OmitHost,
		ForceQuery:  c.URL.ForceQuery,
		RawQuery:    c.URL.RawQuery,
		Fragment:    c.URL.Fragment,
		RawFragment: c.URL.RawFragment,
	}
	app, streamKey := splitPath(cleanURL)
	tcURL := getTcURL(cleanURL)

	switch c.authState {
	case 1:
		user := c.URL.User.Username()

		app += "?authmod=adobe&user=" + user
		tcURL += "?authmod=adobe&user=" + user

	case 2:
		user := c.URL.User.Username()
		pass, _ := c.URL.User.Password()

		clientChallenge := strings.ReplaceAll(uuid.New().String(), "-", "")
		response := authResponse(user, pass, c.authSalt, "", c.authChallenge, clientChallenge)

		app += fmt.Sprintf("?authmod=adobe&user=myuser&challenge=%s&response=%s", clientChallenge, response)
		tcURL += fmt.Sprintf("?authmod=adobe&user=myuser&challenge=%s&response=%s", clientChallenge, response)
	}

	connectArg := amf0.Object{
		{Key: "app", Value: app},
		{Key: "flashVer", Value: "LNX 9,0,124,2"},
		{Key: "tcUrl", Value: tcURL},
	}

	if !c.Publish {
		connectArg = append(connectArg,
			amf0.ObjectEntry{Key: "fpad", Value: false},
			amf0.ObjectEntry{Key: "capabilities", Value: float64(15)},
			amf0.ObjectEntry{Key: "audioCodecs", Value: float64(4071)},
			amf0.ObjectEntry{Key: "videoCodecs", Value: float64(252)},
			amf0.ObjectEntry{Key: "videoFunction", Value: float64(1)},
		)
	}

	err = c.mrw.Write(&message.CommandAMF0{
		ChunkStreamID: 3,
		Name:          "connect",
		CommandID:     1,
		Arguments:     []interface{}{connectArg},
	})
	if err != nil {
		return err
	}

	res, err := readCommandResult(c.mrw, 1)
	if err != nil {
		return err
	}

	switch res.Name {
	case "_result":

	case "_error":
		if len(res.Arguments) < 2 {
			return fmt.Errorf("bad result: %v", res)
		}

		ma, ok := objectOrArray(res.Arguments[1])
		if !ok {
			return fmt.Errorf("bad result: %v", res)
		}

		desc, ok := ma.GetString("description")
		if !ok {
			return fmt.Errorf("bad result: %v", res)
		}

		if desc == "code=403 need auth; authmod=adobe" {
			if c.URL.User == nil {
				return fmt.Errorf("credentials are required")
			}

			if c.authState != 0 {
				return fmt.Errorf("authentication error")
			}

			return errAuth
		}

		if !strings.HasPrefix(desc, "authmod=adobe ?") {
			return fmt.Errorf("bad result: %v", res)
		}

		desc = desc[len("authmod=adobe ?"):]
		vals := queryDecode(desc)

		reason := vals["reason"]
		c.authSalt = vals["salt"]
		c.authChallenge = vals["challenge"]

		if reason != "needauth" || c.authSalt == "" || c.authChallenge == "" {
			return fmt.Errorf("bad result: %v", res)
		}

		if c.authState != 1 {
			return fmt.Errorf("authentication error")
		}

		return errAuth

	default:
		return fmt.Errorf("bad result: %v", res)
	}

	if !c.Publish {
		err = c.mrw.Write(&message.CommandAMF0{
			ChunkStreamID: 3,
			Name:          "createStream",
			CommandID:     2,
			Arguments: []interface{}{
				nil,
			},
		})
		if err != nil {
			return err
		}

		res, err = readCommandResult(c.mrw, 2)
		if err != nil {
			return err
		}

		if res.Name != "_result" || !resultIsOK2(res) {
			return fmt.Errorf("bad result: %v", res)
		}

		err = c.mrw.Write(&message.UserControlSetBufferLength{
			BufferLength: 0x64,
		})
		if err != nil {
			return err
		}

		err = c.mrw.Write(&message.CommandAMF0{
			ChunkStreamID:   4,
			MessageStreamID: 0x1000000,
			Name:            "play",
			CommandID:       3,
			Arguments: []interface{}{
				nil,
				streamKey,
			},
		})
		if err != nil {
			return err
		}

		res, err = readCommandResult(c.mrw, 3)
		if err != nil {
			return err
		}

		if res.Name != "onStatus" || !resultIsOK1(res) {
			return fmt.Errorf("bad result: %v", res)
		}
	} else {
		err = c.mrw.Write(&message.CommandAMF0{
			ChunkStreamID: 3,
			Name:          "releaseStream",
			CommandID:     2,
			Arguments: []interface{}{
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
			Arguments: []interface{}{
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
			Arguments: []interface{}{
				nil,
			},
		})
		if err != nil {
			return err
		}

		res, err = readCommandResult(c.mrw, 4)
		if err != nil {
			return err
		}

		if res.Name != "_result" || !resultIsOK2(res) {
			return fmt.Errorf("bad result: %v", res)
		}

		err = c.mrw.Write(&message.CommandAMF0{
			ChunkStreamID:   4,
			MessageStreamID: 0x1000000,
			Name:            "publish",
			CommandID:       5,
			Arguments: []interface{}{
				nil,
				streamKey,
				app,
			},
		})
		if err != nil {
			return err
		}

		res, err = readCommandResult(c.mrw, 5)
		if err != nil {
			return err
		}

		if res.Name != "onStatus" || !resultIsOK1(res) {
			return fmt.Errorf("bad result: %v", res)
		}
	}

	return nil
}

// Close closes the connection.
func (c *Client) Close() {
	c.nconn.Close()
}

// NetConn returns the underlying net.Conn.
func (c *Client) NetConn() net.Conn {
	return c.nconn
}

// BytesReceived returns the number of bytes received.
func (c *Client) BytesReceived() uint64 {
	return c.bc.Reader.Count()
}

// BytesSent returns the number of bytes sent.
func (c *Client) BytesSent() uint64 {
	return c.bc.Writer.Count()
}

// Read reads a message.
func (c *Client) Read() (message.Message, error) {
	return c.mrw.Read()
}

// Write writes a message.
func (c *Client) Write(msg message.Message) error {
	return c.mrw.Write(msg)
}
