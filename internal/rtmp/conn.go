// Package rtmp provides RTMP connectivity.
package rtmp

import (
	"fmt"
	"io"
	"net/url"
	"strings"

	"github.com/notedit/rtmp/format/flv/flvio"

	"github.com/bluenviron/mediamtx/internal/rtmp/bytecounter"
	"github.com/bluenviron/mediamtx/internal/rtmp/handshake"
	"github.com/bluenviron/mediamtx/internal/rtmp/message"
)

func resultIsOK1(res *message.CommandAMF0) bool {
	if len(res.Arguments) < 2 {
		return false
	}

	ma, ok := res.Arguments[1].(flvio.AMFMap)
	if !ok {
		return false
	}

	v, ok := ma.GetString("level")
	if !ok {
		return false
	}

	return v == "status"
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

func splitPath(u *url.URL) (app, stream string) {
	nu := *u
	nu.ForceQuery = false

	pathsegs := strings.Split(nu.RequestURI(), "/")
	if len(pathsegs) == 2 {
		app = pathsegs[1]
	}
	if len(pathsegs) == 3 {
		app = pathsegs[1]
		stream = pathsegs[2]
	}
	if len(pathsegs) > 3 {
		app = strings.Join(pathsegs[1:3], "/")
		stream = strings.Join(pathsegs[3:], "/")
	}
	return
}

func getTcURL(u *url.URL) string {
	app, _ := splitPath(u)
	nu, _ := url.Parse(u.String()) // perform a deep copy
	nu.RawQuery = ""
	nu.Path = "/"
	return nu.String() + app
}

func createURL(tcURL string, app string, play string) (*url.URL, error) {
	u, err := url.ParseRequestURI("/" + app + "/" + play)
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
	commandName string,
	isValid func(*message.CommandAMF0) bool,
) error {
	for {
		msg, err := mrw.Read()
		if err != nil {
			return err
		}

		if cmd, ok := msg.(*message.CommandAMF0); ok {
			if cmd.CommandID == commandID && cmd.Name == commandName {
				if !isValid(cmd) {
					return fmt.Errorf("server refused connect request")
				}

				return nil
			}
		}
	}
}

// Conn is a RTMP connection.
type Conn struct {
	bc  *bytecounter.ReadWriter
	mrw *message.ReadWriter
}

// NewClientConn initializes a client-side connection.
func NewClientConn(rw io.ReadWriter, u *url.URL, publish bool) (*Conn, error) {
	c := &Conn{
		bc: bytecounter.NewReadWriter(rw),
	}

	err := c.initializeClient(u, publish)
	if err != nil {
		return nil, err
	}

	return c, nil
}

func (c *Conn) initializeClient(u *url.URL, publish bool) error {
	connectpath, actionpath := splitPath(u)

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

	err = c.mrw.Write(&message.CommandAMF0{
		ChunkStreamID: 3,
		Name:          "connect",
		CommandID:     1,
		Arguments: []interface{}{
			flvio.AMFMap{
				{K: "app", V: connectpath},
				{K: "flashVer", V: "LNX 9,0,124,2"},
				{K: "tcUrl", V: getTcURL(u)},
				{K: "fpad", V: false},
				{K: "capabilities", V: 15},
				{K: "audioCodecs", V: 4071},
				{K: "videoCodecs", V: 252},
				{K: "videoFunction", V: 1},
			},
		},
	})
	if err != nil {
		return err
	}

	err = readCommandResult(c.mrw, 1, "_result", resultIsOK1)
	if err != nil {
		return err
	}

	if !publish {
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

		err = readCommandResult(c.mrw, 2, "_result", resultIsOK2)
		if err != nil {
			return err
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
				actionpath,
			},
		})
		if err != nil {
			return err
		}

		return readCommandResult(c.mrw, 3, "onStatus", resultIsOK1)
	}

	err = c.mrw.Write(&message.CommandAMF0{
		ChunkStreamID: 3,
		Name:          "releaseStream",
		CommandID:     2,
		Arguments: []interface{}{
			nil,
			actionpath,
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
			actionpath,
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

	err = readCommandResult(c.mrw, 4, "_result", resultIsOK2)
	if err != nil {
		return err
	}

	err = c.mrw.Write(&message.CommandAMF0{
		ChunkStreamID:   4,
		MessageStreamID: 0x1000000,
		Name:            "publish",
		CommandID:       5,
		Arguments: []interface{}{
			nil,
			actionpath,
			connectpath,
		},
	})
	if err != nil {
		return err
	}

	return readCommandResult(c.mrw, 5, "onStatus", resultIsOK1)
}

// NewServerConn initializes a server-side connection.
func NewServerConn(rw io.ReadWriter) (*Conn, *url.URL, bool, error) {
	c := &Conn{
		bc: bytecounter.NewReadWriter(rw),
	}

	u, publish, err := c.initializeServer()
	if err != nil {
		return nil, nil, false, err
	}

	return c, u, publish, nil
}

func (c *Conn) initializeServer() (*url.URL, bool, error) {
	keyIn, keyOut, err := handshake.DoServer(c.bc, false)
	if err != nil {
		return nil, false, err
	}

	var rw io.ReadWriter
	if keyIn != nil {
		var err error
		rw, err = newRC4ReadWriter(c.bc, keyIn, keyOut)
		if err != nil {
			return nil, false, err
		}
	} else {
		rw = c.bc
	}

	c.mrw = message.NewReadWriter(rw, c.bc, false)

	cmd, err := readCommand(c.mrw)
	if err != nil {
		return nil, false, err
	}

	if cmd.Name != "connect" {
		return nil, false, fmt.Errorf("unexpected command: %+v", cmd)
	}

	if len(cmd.Arguments) < 1 {
		return nil, false, fmt.Errorf("invalid connect command: %+v", cmd)
	}

	ma, ok := cmd.Arguments[0].(flvio.AMFMap)
	if !ok {
		return nil, false, fmt.Errorf("invalid connect command: %+v", cmd)
	}

	connectpath, ok := ma.GetString("app")
	if !ok {
		return nil, false, fmt.Errorf("invalid connect command: %+v", cmd)
	}

	tcURL, ok := ma.GetString("tcUrl")
	if !ok {
		tcURL, ok = ma.GetString("tcurl")
		if !ok {
			return nil, false, fmt.Errorf("invalid connect command: %+v", cmd)
		}
	}

	tcURL = strings.Trim(tcURL, "'")

	err = c.mrw.Write(&message.SetWindowAckSize{
		Value: 2500000,
	})
	if err != nil {
		return nil, false, err
	}

	err = c.mrw.Write(&message.SetPeerBandwidth{
		Value: 2500000,
		Type:  2,
	})
	if err != nil {
		return nil, false, err
	}

	err = c.mrw.Write(&message.SetChunkSize{
		Value: 65536,
	})
	if err != nil {
		return nil, false, err
	}

	oe, _ := ma.GetFloat64("objectEncoding")

	err = c.mrw.Write(&message.CommandAMF0{
		ChunkStreamID: cmd.ChunkStreamID,
		Name:          "_result",
		CommandID:     cmd.CommandID,
		Arguments: []interface{}{
			flvio.AMFMap{
				{K: "fmsVer", V: "LNX 9,0,124,2"},
				{K: "capabilities", V: float64(31)},
			},
			flvio.AMFMap{
				{K: "level", V: "status"},
				{K: "code", V: "NetConnection.Connect.Success"},
				{K: "description", V: "Connection succeeded."},
				{K: "objectEncoding", V: oe},
			},
		},
	})
	if err != nil {
		return nil, false, err
	}

	for {
		cmd, err := readCommand(c.mrw)
		if err != nil {
			return nil, false, err
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
				return nil, false, err
			}

		case "play":
			if len(cmd.Arguments) < 2 {
				return nil, false, fmt.Errorf("invalid play command arguments")
			}

			actionpath, ok := cmd.Arguments[1].(string)
			if !ok {
				return nil, false, fmt.Errorf("invalid play command arguments")
			}

			u, err := createURL(tcURL, connectpath, actionpath)
			if err != nil {
				return nil, false, err
			}

			err = c.mrw.Write(&message.UserControlStreamIsRecorded{
				StreamID: 1,
			})
			if err != nil {
				return nil, false, err
			}

			err = c.mrw.Write(&message.UserControlStreamBegin{
				StreamID: 1,
			})
			if err != nil {
				return nil, false, err
			}

			err = c.mrw.Write(&message.CommandAMF0{
				ChunkStreamID:   5,
				MessageStreamID: 0x1000000,
				Name:            "onStatus",
				CommandID:       cmd.CommandID,
				Arguments: []interface{}{
					nil,
					flvio.AMFMap{
						{K: "level", V: "status"},
						{K: "code", V: "NetStream.Play.Reset"},
						{K: "description", V: "play reset"},
					},
				},
			})
			if err != nil {
				return nil, false, err
			}

			err = c.mrw.Write(&message.CommandAMF0{
				ChunkStreamID:   5,
				MessageStreamID: 0x1000000,
				Name:            "onStatus",
				CommandID:       cmd.CommandID,
				Arguments: []interface{}{
					nil,
					flvio.AMFMap{
						{K: "level", V: "status"},
						{K: "code", V: "NetStream.Play.Start"},
						{K: "description", V: "play start"},
					},
				},
			})
			if err != nil {
				return nil, false, err
			}

			err = c.mrw.Write(&message.CommandAMF0{
				ChunkStreamID:   5,
				MessageStreamID: 0x1000000,
				Name:            "onStatus",
				CommandID:       cmd.CommandID,
				Arguments: []interface{}{
					nil,
					flvio.AMFMap{
						{K: "level", V: "status"},
						{K: "code", V: "NetStream.Data.Start"},
						{K: "description", V: "data start"},
					},
				},
			})
			if err != nil {
				return nil, false, err
			}

			err = c.mrw.Write(&message.CommandAMF0{
				ChunkStreamID:   5,
				MessageStreamID: 0x1000000,
				Name:            "onStatus",
				CommandID:       cmd.CommandID,
				Arguments: []interface{}{
					nil,
					flvio.AMFMap{
						{K: "level", V: "status"},
						{K: "code", V: "NetStream.Play.PublishNotify"},
						{K: "description", V: "publish notify"},
					},
				},
			})
			if err != nil {
				return nil, false, err
			}

			return u, false, nil

		case "publish":
			if len(cmd.Arguments) < 2 {
				return nil, false, fmt.Errorf("invalid publish command arguments")
			}

			actionpath, ok := cmd.Arguments[1].(string)
			if !ok {
				return nil, false, fmt.Errorf("invalid publish command arguments")
			}

			u, err := createURL(tcURL, connectpath, actionpath)
			if err != nil {
				return nil, false, err
			}

			err = c.mrw.Write(&message.CommandAMF0{
				ChunkStreamID:   5,
				Name:            "onStatus",
				CommandID:       cmd.CommandID,
				MessageStreamID: 0x1000000,
				Arguments: []interface{}{
					nil,
					flvio.AMFMap{
						{K: "level", V: "status"},
						{K: "code", V: "NetStream.Publish.Start"},
						{K: "description", V: "publish start"},
					},
				},
			})
			if err != nil {
				return nil, false, err
			}

			return u, true, nil
		}
	}
}

func newNoHandshakeConn(rw io.ReadWriter) *Conn {
	c := &Conn{
		bc: bytecounter.NewReadWriter(rw),
	}

	c.mrw = message.NewReadWriter(c.bc, c.bc, false)

	return c
}

// BytesReceived returns the number of bytes received.
func (c *Conn) BytesReceived() uint64 {
	return c.bc.Reader.Count()
}

// BytesSent returns the number of bytes sent.
func (c *Conn) BytesSent() uint64 {
	return c.bc.Writer.Count()
}

// Read reads a message.
func (c *Conn) Read() (message.Message, error) {
	return c.mrw.Read()
}

// Write writes a message.
func (c *Conn) Write(msg message.Message) error {
	return c.mrw.Write(msg)
}
