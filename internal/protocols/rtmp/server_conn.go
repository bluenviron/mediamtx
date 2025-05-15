package rtmp

import (
	"crypto/md5"
	"encoding/base64"
	"fmt"
	"io"
	"net/url"
	"strings"

	"github.com/bluenviron/mediamtx/internal/protocols/rtmp/amf0"
	"github.com/bluenviron/mediamtx/internal/protocols/rtmp/bytecounter"
	"github.com/bluenviron/mediamtx/internal/protocols/rtmp/handshake"
	"github.com/bluenviron/mediamtx/internal/protocols/rtmp/message"
)

const (
	serverSalt      = "testsalt"
	serverChallenge = "testchallenge"
)

func queryDecode(enc string) map[string]string {
	// do not use url.ParseQuery since values are not URL-encoded
	vals := make(map[string]string)

	for _, kv := range strings.Split(enc, "&") {
		tmp := strings.SplitN(kv, "=", 2)
		if len(tmp) == 2 {
			vals[tmp[0]] = tmp[1]
		}
	}

	return vals
}

func queryEncode(dec map[string]string) string {
	tmp := make([]string, len(dec))
	i := 0

	for k, v := range dec {
		tmp[i] = k + "=" + v
		i++
	}

	return strings.Join(tmp, "&")
}

func authResponse(user, pass, salt, opaque, challenge, challenge2 string) string {
	h := md5.New()
	h.Write([]byte(user))
	h.Write([]byte(salt))
	h.Write([]byte(pass))
	str := base64.StdEncoding.EncodeToString(h.Sum(nil))

	h = md5.New()
	h.Write([]byte(str))
	if opaque != "" {
		h.Write([]byte(opaque))
	} else {
		h.Write([]byte(challenge))
	}
	h.Write([]byte(challenge2))
	return base64.StdEncoding.EncodeToString(h.Sum(nil))
}

func buildURL(tcURL string, app string, streamKey string) (*url.URL, error) {
	raw := "/" + app
	if streamKey != "" {
		raw += "/" + streamKey
	}

	u, err := url.ParseRequestURI(raw)
	if err != nil {
		return nil, err
	}

	tu, err := url.Parse(tcURL)
	if err != nil {
		return nil, err
	}

	if tu.Host == "" {
		return nil, fmt.Errorf("invalid host")
	}
	u.Host = tu.Host

	if tu.Scheme == "" {
		return nil, fmt.Errorf("invalid scheme")
	}
	u.Scheme = tu.Scheme

	return u, nil
}

func objectOrArray(in interface{}) (amf0.Object, bool) {
	switch o := in.(type) {
	case amf0.Object:
		return o, true

	case amf0.ECMAArray:
		return amf0.Object(o), true

	default:
		return nil, false
	}
}

// ServerConn is a server-side RTMP connection.
type ServerConn struct {
	RW io.ReadWriter

	// filled by Initialize
	connectCmd    *message.CommandAMF0
	connectObject amf0.Object
	app           string
	tcURL         string

	// filled by Accept
	URL     *url.URL
	Publish bool

	bc  *bytecounter.ReadWriter
	mrw *message.ReadWriter
}

// Initialize initializes ServerConn.
func (c *ServerConn) Initialize() error {
	c.bc = bytecounter.NewReadWriter(c.RW)

	keyIn, keyOut, err := handshake.DoServer(c.bc, false)
	if err != nil {
		return err
	}

	var rw io.ReadWriter
	if keyIn != nil {
		rw, err = newRC4ReadWriter(c.bc, keyIn, keyOut)
		if err != nil {
			return err
		}
	} else {
		rw = c.bc
	}

	c.mrw = message.NewReadWriter(rw, c.bc, false)

	c.connectCmd, err = readCommand(c.mrw)
	if err != nil {
		return err
	}

	if c.connectCmd.Name != "connect" {
		return fmt.Errorf("unexpected command: %+v", c.connectCmd)
	}

	if len(c.connectCmd.Arguments) < 1 {
		return fmt.Errorf("invalid connect command: %+v", c.connectCmd)
	}

	var ok bool
	c.connectObject, ok = objectOrArray(c.connectCmd.Arguments[0])
	if !ok {
		return fmt.Errorf("invalid connect command: %+v", c.connectCmd)
	}

	c.app, ok = c.connectObject.GetString("app")
	if !ok {
		return fmt.Errorf("invalid connect command: %+v", c.connectCmd)
	}

	c.tcURL, ok = c.connectObject.GetString("tcUrl")
	if !ok {
		c.tcURL, ok = c.connectObject.GetString("tcurl")
		if !ok {
			return fmt.Errorf("invalid connect command: %+v", c.connectCmd)
		}
	}

	c.tcURL = strings.Trim(c.tcURL, "'")

	return nil
}

// CheckCredentials checks credentials.
func (c *ServerConn) CheckCredentials(expectedUser string, expectedPass string) error {
	i := strings.Index(c.app, "?authmod=adobe")
	if i < 0 {
		err := c.mrw.Write(&message.CommandAMF0{
			ChunkStreamID: c.connectCmd.ChunkStreamID,
			Name:          "_error",
			CommandID:     c.connectCmd.CommandID,
			Arguments: []interface{}{
				nil,
				amf0.Object{
					{Key: "level", Value: "error"},
					{Key: "code", Value: "NetConnection.Connect.Rejected"},
					{Key: "description", Value: "code=403 need auth; authmod=adobe"},
				},
			},
		})
		if err != nil {
			return err
		}

		return fmt.Errorf("need auth")
	}

	authParams := c.app[i+1:]
	vals := queryDecode(authParams)

	user := vals["user"]
	if user == "" {
		return fmt.Errorf("user not provided")
	}

	clientChallenge := vals["challenge"]
	response := vals["response"]

	if clientChallenge == "" || response == "" {
		err := c.mrw.Write(&message.CommandAMF0{
			ChunkStreamID: c.connectCmd.ChunkStreamID,
			Name:          "_error",
			CommandID:     c.connectCmd.CommandID,
			Arguments: []interface{}{
				nil,
				amf0.Object{
					{Key: "level", Value: "error"},
					{Key: "code", Value: "NetConnection.Connect.Rejected"},
					{
						Key: "description",
						Value: fmt.Sprintf("authmod=adobe ?reason=needauth&user=%s&salt=%s&challenge=%s",
							user, serverSalt, serverChallenge),
					},
				},
			},
		})
		if err != nil {
			return err
		}

		return fmt.Errorf("need auth 2")
	}

	expectedResponse := authResponse(expectedUser, expectedPass, serverSalt, "", serverChallenge, clientChallenge)
	if expectedResponse != response {
		err := c.mrw.Write(&message.CommandAMF0{
			ChunkStreamID: c.connectCmd.ChunkStreamID,
			Name:          "_error",
			CommandID:     c.connectCmd.CommandID,
			Arguments: []interface{}{
				nil,
				amf0.Object{
					{Key: "level", Value: "error"},
					{Key: "code", Value: "NetConnection.Connect.Rejected"},
					{Key: "description", Value: "authmod=adobe ?reason=authfailed"},
				},
			},
		})
		if err != nil {
			return err
		}

		return fmt.Errorf("authentication failed")
	}

	// remove auth parameters from app
	c.app = c.app[:i]
	delete(vals, "authmod")
	delete(vals, "user")
	delete(vals, "challenge")
	delete(vals, "response")
	q := queryEncode(vals)
	if q != "" {
		c.app += "?" + q
	}

	return nil
}

// Accept accepts the connection.
func (c *ServerConn) Accept() error {
	err := c.mrw.Write(&message.SetWindowAckSize{
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

	oe, _ := c.connectObject.GetFloat64("objectEncoding")

	err = c.mrw.Write(&message.CommandAMF0{
		ChunkStreamID: c.connectCmd.ChunkStreamID,
		Name:          "_result",
		CommandID:     c.connectCmd.CommandID,
		Arguments: []interface{}{
			amf0.Object{
				{Key: "fmsVer", Value: "LNX 9,0,124,2"},
				{Key: "capabilities", Value: float64(31)},
			},
			amf0.Object{
				{Key: "level", Value: "status"},
				{Key: "code", Value: "NetConnection.Connect.Success"},
				{Key: "description", Value: "Connection succeeded."},
				{Key: "objectEncoding", Value: oe},
			},
		},
	})
	if err != nil {
		return err
	}

	for {
		cmd, err := readCommand(c.mrw)
		if err != nil {
			return err
		}

		switch cmd.Name {
		case "createStream":
			err = c.mrw.Write(&message.CommandAMF0{
				ChunkStreamID: cmd.ChunkStreamID,
				Name:          "_result",
				CommandID:     cmd.CommandID,
				Arguments: []interface{}{
					nil,
					float64(1),
				},
			})
			if err != nil {
				return err
			}

		case "play":
			if len(cmd.Arguments) < 2 {
				return fmt.Errorf("invalid play command arguments")
			}

			streamKey, ok := cmd.Arguments[1].(string)
			if !ok {
				return fmt.Errorf("invalid play command arguments")
			}

			c.URL, err = buildURL(c.tcURL, c.app, streamKey)
			if err != nil {
				return err
			}

			err = c.mrw.Write(&message.UserControlStreamIsRecorded{
				StreamID: 1,
			})
			if err != nil {
				return err
			}

			err = c.mrw.Write(&message.UserControlStreamBegin{
				StreamID: 1,
			})
			if err != nil {
				return err
			}

			err = c.mrw.Write(&message.CommandAMF0{
				ChunkStreamID:   5,
				MessageStreamID: 0x1000000,
				Name:            "onStatus",
				CommandID:       cmd.CommandID,
				Arguments: []interface{}{
					nil,
					amf0.Object{
						{Key: "level", Value: "status"},
						{Key: "code", Value: "NetStream.Play.Reset"},
						{Key: "description", Value: "play reset"},
					},
				},
			})
			if err != nil {
				return err
			}

			err = c.mrw.Write(&message.CommandAMF0{
				ChunkStreamID:   5,
				MessageStreamID: 0x1000000,
				Name:            "onStatus",
				CommandID:       cmd.CommandID,
				Arguments: []interface{}{
					nil,
					amf0.Object{
						{Key: "level", Value: "status"},
						{Key: "code", Value: "NetStream.Play.Start"},
						{Key: "description", Value: "play start"},
					},
				},
			})
			if err != nil {
				return err
			}

			err = c.mrw.Write(&message.CommandAMF0{
				ChunkStreamID:   5,
				MessageStreamID: 0x1000000,
				Name:            "onStatus",
				CommandID:       cmd.CommandID,
				Arguments: []interface{}{
					nil,
					amf0.Object{
						{Key: "level", Value: "status"},
						{Key: "code", Value: "NetStream.Data.Start"},
						{Key: "description", Value: "data start"},
					},
				},
			})
			if err != nil {
				return err
			}

			err = c.mrw.Write(&message.CommandAMF0{
				ChunkStreamID:   5,
				MessageStreamID: 0x1000000,
				Name:            "onStatus",
				CommandID:       cmd.CommandID,
				Arguments: []interface{}{
					nil,
					amf0.Object{
						{Key: "level", Value: "status"},
						{Key: "code", Value: "NetStream.Play.PublishNotify"},
						{Key: "description", Value: "publish notify"},
					},
				},
			})
			if err != nil {
				return err
			}

			c.Publish = false
			return nil

		case "publish":
			if len(cmd.Arguments) < 2 {
				return fmt.Errorf("invalid publish command arguments")
			}

			streamKey, ok := cmd.Arguments[1].(string)
			if !ok {
				return fmt.Errorf("invalid publish command arguments")
			}

			c.URL, err = buildURL(c.tcURL, c.app, streamKey)
			if err != nil {
				return err
			}

			err = c.mrw.Write(&message.CommandAMF0{
				ChunkStreamID:   5,
				Name:            "onStatus",
				CommandID:       cmd.CommandID,
				MessageStreamID: 0x1000000,
				Arguments: []interface{}{
					nil,
					amf0.Object{
						{Key: "level", Value: "status"},
						{Key: "code", Value: "NetStream.Publish.Start"},
						{Key: "description", Value: "publish start"},
					},
				},
			})
			if err != nil {
				return err
			}

			c.Publish = true
			return nil
		}
	}
}

// BytesReceived returns the number of bytes received.
func (c *ServerConn) BytesReceived() uint64 {
	return c.bc.Reader.Count()
}

// BytesSent returns the number of bytes sent.
func (c *ServerConn) BytesSent() uint64 {
	return c.bc.Writer.Count()
}

// Read reads a message.
func (c *ServerConn) Read() (message.Message, error) {
	return c.mrw.Read()
}

// Write writes a message.
func (c *ServerConn) Write(msg message.Message) error {
	return c.mrw.Write(msg)
}
