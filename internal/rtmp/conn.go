package rtmp

import (
	"errors"
	"fmt"
	"net"
	"net/url"
	"strings"
	"time"

	"github.com/aler9/gortsplib"
	"github.com/aler9/gortsplib/pkg/aac"
	"github.com/notedit/rtmp/format/flv/flvio"

	"github.com/aler9/rtsp-simple-server/internal/rtmp/bytecounter"
	"github.com/aler9/rtsp-simple-server/internal/rtmp/h264conf"
	"github.com/aler9/rtsp-simple-server/internal/rtmp/handshake"
	"github.com/aler9/rtsp-simple-server/internal/rtmp/message"
)

const (
	codecH264 = 7
	codecAAC  = 10
)

func resultIsOK1(res *message.MsgCommandAMF0) bool {
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

func resultIsOK2(res *message.MsgCommandAMF0) bool {
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

func createURL(tcurl, app, play string) (*url.URL, error) {
	u, err := url.ParseRequestURI("/" + app + "/" + play)
	if err != nil {
		return nil, err
	}

	tu, err := url.Parse(tcurl)
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

// Conn is a RTMP connection.
type Conn struct {
	bc  *bytecounter.ReadWriter
	mrw *message.ReadWriter
}

// NewConn initializes a connection.
func NewConn(nconn net.Conn) *Conn {
	c := &Conn{}
	c.bc = bytecounter.NewReadWriter(nconn)
	c.mrw = message.NewReadWriter(c.bc, false)
	return c
}

func (c *Conn) readCommand() (*message.MsgCommandAMF0, error) {
	for {
		msg, err := c.mrw.Read()
		if err != nil {
			return nil, err
		}

		if cmd, ok := msg.(*message.MsgCommandAMF0); ok {
			return cmd, nil
		}
	}
}

func (c *Conn) readCommandResult(commandName string, isValid func(*message.MsgCommandAMF0) bool) error {
	for {
		msg, err := c.mrw.Read()
		if err != nil {
			return err
		}

		if cmd, ok := msg.(*message.MsgCommandAMF0); ok {
			if cmd.Name == commandName {
				if !isValid(cmd) {
					return fmt.Errorf("server refused connect request")
				}

				return nil
			}
		}
	}
}

// InitializeClient performs the initialization of a client-side connection.
func (c *Conn) InitializeClient(u *url.URL, isPlaying bool) error {
	connectpath, actionpath := splitPath(u)

	err := handshake.DoClient(c.bc, false)
	if err != nil {
		return err
	}

	err = c.mrw.Write(&message.MsgSetWindowAckSize{
		Value: 2500000,
	})
	if err != nil {
		return err
	}

	err = c.mrw.Write(&message.MsgSetPeerBandwidth{
		Value: 2500000,
		Type:  2,
	})
	if err != nil {
		return err
	}

	err = c.mrw.Write(&message.MsgSetChunkSize{
		Value: 65536,
	})
	if err != nil {
		return err
	}

	err = c.mrw.Write(&message.MsgCommandAMF0{
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

	err = c.readCommandResult("_result", resultIsOK1)
	if err != nil {
		return err
	}

	if isPlaying {
		err = c.mrw.Write(&message.MsgCommandAMF0{
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

		err = c.readCommandResult("_result", resultIsOK2)
		if err != nil {
			return err
		}

		err = c.mrw.Write(&message.MsgUserControlSetBufferLength{
			BufferLength: 0x64,
		})
		if err != nil {
			return err
		}

		err = c.mrw.Write(&message.MsgCommandAMF0{
			ChunkStreamID:   4,
			MessageStreamID: 0x1000000,
			Name:            "play",
			CommandID:       0,
			Arguments: []interface{}{
				nil,
				actionpath,
			},
		})
		if err != nil {
			return err
		}

		err = c.readCommandResult("onStatus", resultIsOK1)
		if err != nil {
			return err
		}
	} else {
		err = c.mrw.Write(&message.MsgCommandAMF0{
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

		err = c.mrw.Write(&message.MsgCommandAMF0{
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

		err = c.mrw.Write(&message.MsgCommandAMF0{
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

		err = c.readCommandResult("_result", resultIsOK2)
		if err != nil {
			return err
		}

		err = c.mrw.Write(&message.MsgCommandAMF0{
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

		err = c.readCommandResult("onStatus", resultIsOK1)
		if err != nil {
			return err
		}
	}

	return nil
}

// InitializeServer performs the initialization of a server-side connection.
func (c *Conn) InitializeServer() (*url.URL, bool, error) {
	err := handshake.DoServer(c.bc, false)
	if err != nil {
		return nil, false, err
	}

	cmd, err := c.readCommand()
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

	err = c.mrw.Write(&message.MsgSetWindowAckSize{
		Value: 2500000,
	})
	if err != nil {
		return nil, false, err
	}

	err = c.mrw.Write(&message.MsgSetPeerBandwidth{
		Value: 2500000,
		Type:  2,
	})
	if err != nil {
		return nil, false, err
	}

	err = c.mrw.Write(&message.MsgSetChunkSize{
		Value: 65536,
	})
	if err != nil {
		return nil, false, err
	}

	oe, _ := ma.GetFloat64("objectEncoding")

	err = c.mrw.Write(&message.MsgCommandAMF0{
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
		cmd, err := c.readCommand()
		if err != nil {
			return nil, false, err
		}

		switch cmd.Name {
		case "createStream":
			err = c.mrw.Write(&message.MsgCommandAMF0{
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

			err = c.mrw.Write(&message.MsgUserControlStreamIsRecorded{
				StreamID: 1,
			})
			if err != nil {
				return nil, false, err
			}

			err = c.mrw.Write(&message.MsgUserControlStreamBegin{
				StreamID: 1,
			})
			if err != nil {
				return nil, false, err
			}

			err = c.mrw.Write(&message.MsgCommandAMF0{
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

			err = c.mrw.Write(&message.MsgCommandAMF0{
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

			err = c.mrw.Write(&message.MsgCommandAMF0{
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

			err = c.mrw.Write(&message.MsgCommandAMF0{
				ChunkStreamID:   5,
				MessageStreamID: 0x1000000,
				Name:            "onStatus",
				CommandID:       4,
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

			return u, true, nil

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

			err = c.mrw.Write(&message.MsgCommandAMF0{
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

			return u, false, nil
		}
	}
}

// ReadMessage reads a message.
func (c *Conn) ReadMessage() (message.Message, error) {
	return c.mrw.Read()
}

// WriteMessage writes a message.
func (c *Conn) WriteMessage(msg message.Message) error {
	return c.mrw.Write(msg)
}

func trackFromH264DecoderConfig(data []byte) (*gortsplib.TrackH264, error) {
	var conf h264conf.Conf
	err := conf.Unmarshal(data)
	if err != nil {
		return nil, fmt.Errorf("unable to parse H264 config: %v", err)
	}

	return &gortsplib.TrackH264{
		PayloadType: 96,
		SPS:         conf.SPS,
		PPS:         conf.PPS,
	}, nil
}

func trackFromAACDecoderConfig(data []byte) (*gortsplib.TrackAAC, error) {
	var mpegConf aac.MPEG4AudioConfig
	err := mpegConf.Unmarshal(data)
	if err != nil {
		return nil, err
	}

	return &gortsplib.TrackAAC{
		PayloadType:      96,
		Config:           &mpegConf,
		SizeLength:       13,
		IndexLength:      3,
		IndexDeltaLength: 3,
	}, nil
}

var errEmptyMetadata = errors.New("metadata is empty")

func (c *Conn) readTracksFromMetadata(payload []interface{}) (*gortsplib.TrackH264, *gortsplib.TrackAAC, error) {
	if len(payload) != 1 {
		return nil, nil, fmt.Errorf("invalid metadata")
	}

	md, ok := payload[0].(flvio.AMFMap)
	if !ok {
		return nil, nil, fmt.Errorf("invalid metadata")
	}

	hasVideo, err := func() (bool, error) {
		v, ok := md.GetV("videocodecid")
		if !ok {
			return false, nil
		}

		switch vt := v.(type) {
		case float64:
			switch vt {
			case 0:
				return false, nil

			case codecH264:
				return true, nil
			}

		case string:
			if vt == "avc1" {
				return true, nil
			}
		}

		return false, fmt.Errorf("unsupported video codec %v", v)
	}()
	if err != nil {
		return nil, nil, err
	}

	hasAudio, err := func() (bool, error) {
		v, ok := md.GetV("audiocodecid")
		if !ok {
			return false, nil
		}

		switch vt := v.(type) {
		case float64:
			switch vt {
			case 0:
				return false, nil

			case codecAAC:
				return true, nil
			}

		case string:
			if vt == "mp4a" {
				return true, nil
			}
		}

		return false, fmt.Errorf("unsupported audio codec %v", v)
	}()
	if err != nil {
		return nil, nil, err
	}

	if !hasVideo && !hasAudio {
		return nil, nil, errEmptyMetadata
	}

	var videoTrack *gortsplib.TrackH264
	var audioTrack *gortsplib.TrackAAC

	for {
		msg, err := c.ReadMessage()
		if err != nil {
			return nil, nil, err
		}

		switch tmsg := msg.(type) {
		case *message.MsgVideo:
			if tmsg.H264Type == flvio.AVC_SEQHDR {
				if !hasVideo {
					return nil, nil, fmt.Errorf("unexpected video packet")
				}

				if videoTrack != nil {
					return nil, nil, fmt.Errorf("video track setupped twice")
				}

				videoTrack, err = trackFromH264DecoderConfig(tmsg.Payload)
				if err != nil {
					return nil, nil, err
				}
			}

		case *message.MsgAudio:
			if tmsg.AACType == flvio.AVC_SEQHDR {
				if !hasAudio {
					return nil, nil, fmt.Errorf("unexpected audio packet")
				}

				if audioTrack != nil {
					return nil, nil, fmt.Errorf("audio track setupped twice")
				}

				audioTrack, err = trackFromAACDecoderConfig(tmsg.Payload)
				if err != nil {
					return nil, nil, err
				}
			}
		}

		if (!hasVideo || videoTrack != nil) &&
			(!hasAudio || audioTrack != nil) {
			return videoTrack, audioTrack, nil
		}
	}
}

func (c *Conn) readTracksFromMessages(msg message.Message) (*gortsplib.TrackH264, *gortsplib.TrackAAC, error) {
	var startTime *time.Duration
	var videoTrack *gortsplib.TrackH264
	var audioTrack *gortsplib.TrackAAC

	// analyze 1 second of packets
outer:
	for {
		switch tmsg := msg.(type) {
		case *message.MsgVideo:
			if startTime == nil {
				v := tmsg.DTS
				startTime = &v
			}

			if tmsg.H264Type == flvio.AVC_SEQHDR {
				if videoTrack == nil {
					var err error
					videoTrack, err = trackFromH264DecoderConfig(tmsg.Payload)
					if err != nil {
						return nil, nil, err
					}

					// stop the analysis if both tracks are found
					if videoTrack != nil && audioTrack != nil {
						return videoTrack, audioTrack, nil
					}
				}
			}

			if (tmsg.DTS - *startTime) >= 1*time.Second {
				break outer
			}

		case *message.MsgAudio:
			if startTime == nil {
				v := tmsg.DTS
				startTime = &v
			}

			if tmsg.AACType == flvio.AVC_SEQHDR {
				if audioTrack == nil {
					var err error
					audioTrack, err = trackFromAACDecoderConfig(tmsg.Payload)
					if err != nil {
						return nil, nil, err
					}

					// stop the analysis if both tracks are found
					if videoTrack != nil && audioTrack != nil {
						return videoTrack, audioTrack, nil
					}
				}
			}

			if (tmsg.DTS - *startTime) >= 1*time.Second {
				break outer
			}
		}

		var err error
		msg, err = c.ReadMessage()
		if err != nil {
			return nil, nil, err
		}
	}

	if videoTrack == nil && audioTrack == nil {
		return nil, nil, fmt.Errorf("no tracks found")
	}

	return videoTrack, audioTrack, nil
}

// ReadTracks reads track informations.
func (c *Conn) ReadTracks() (*gortsplib.TrackH264, *gortsplib.TrackAAC, error) {
	msg, err := c.ReadMessage()
	if err != nil {
		return nil, nil, err
	}

	if data, ok := msg.(*message.MsgDataAMF0); ok && len(data.Payload) >= 1 {
		payload := data.Payload

		// skip packet
		if s, ok := payload[0].(string); ok && s == "|RtmpSampleAccess" {
			return c.ReadTracks()
		}

		if s, ok := payload[0].(string); ok && s == "@setDataFrame" {
			payload = payload[1:]
		}

		if len(payload) >= 1 {
			if s, ok := payload[0].(string); ok && s == "onMetaData" {
				videoTrack, audioTrack, err := c.readTracksFromMetadata(payload[1:])
				if err != nil {
					if err == errEmptyMetadata {
						msg, err := c.ReadMessage()
						if err != nil {
							return nil, nil, err
						}

						return c.readTracksFromMessages(msg)
					}

					return nil, nil, err
				}

				return videoTrack, audioTrack, nil
			}
		}
	}

	return c.readTracksFromMessages(msg)
}

// WriteTracks writes track informations.
func (c *Conn) WriteTracks(videoTrack *gortsplib.TrackH264, audioTrack *gortsplib.TrackAAC) error {
	err := c.WriteMessage(&message.MsgDataAMF0{
		ChunkStreamID:   4,
		MessageStreamID: 0x1000000,
		Payload: []interface{}{
			"@setDataFrame",
			"onMetaData",
			flvio.AMFMap{
				{
					K: "videodatarate",
					V: float64(0),
				},
				{
					K: "videocodecid",
					V: func() float64 {
						if videoTrack != nil {
							return codecH264
						}
						return 0
					}(),
				},
				{
					K: "audiodatarate",
					V: float64(0),
				},
				{
					K: "audiocodecid",
					V: func() float64 {
						if audioTrack != nil {
							return codecAAC
						}
						return 0
					}(),
				},
			},
		},
	})
	if err != nil {
		return err
	}

	// write decoder config only if SPS and PPS are available.
	// if they're not available yet, they're sent later.
	if videoTrack != nil && videoTrack.SafeSPS() != nil && videoTrack.SafePPS() != nil {
		buf, _ := h264conf.Conf{
			SPS: videoTrack.SafeSPS(),
			PPS: videoTrack.SafePPS(),
		}.Marshal()

		err = c.WriteMessage(&message.MsgVideo{
			ChunkStreamID:   6,
			MessageStreamID: 0x1000000,
			IsKeyFrame:      true,
			H264Type:        flvio.AVC_SEQHDR,
			Payload:         buf,
		})
		if err != nil {
			return err
		}
	}

	if audioTrack != nil {
		enc, err := audioTrack.Config.Marshal()
		if err != nil {
			return err
		}

		err = c.WriteMessage(&message.MsgAudio{
			ChunkStreamID:   4,
			MessageStreamID: 0x1000000,
			Rate:            flvio.SOUND_44Khz,
			Depth:           flvio.SOUND_16BIT,
			Channels:        flvio.SOUND_STEREO,
			AACType:         flvio.AAC_SEQHDR,
			Payload:         enc,
		})
		if err != nil {
			return err
		}
	}

	return nil
}
