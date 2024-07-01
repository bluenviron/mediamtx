package rtmp

import (
	"bytes"
	"errors"
	"fmt"
	"time"

	"github.com/abema/go-mp4"
	"github.com/bluenviron/gortsplib/v4/pkg/format"
	"github.com/bluenviron/mediacommon/pkg/codecs/av1"
	"github.com/bluenviron/mediacommon/pkg/codecs/h264"
	"github.com/bluenviron/mediacommon/pkg/codecs/h265"
	"github.com/bluenviron/mediacommon/pkg/codecs/mpeg4audio"

	"github.com/bluenviron/mediamtx/internal/protocols/rtmp/amf0"
	"github.com/bluenviron/mediamtx/internal/protocols/rtmp/h264conf"
	"github.com/bluenviron/mediamtx/internal/protocols/rtmp/message"
)

const (
	analyzePeriod = 1 * time.Second
)

// OnDataAV1Func is the prototype of the callback passed to OnDataAV1().
type OnDataAV1Func func(pts time.Duration, tu [][]byte)

// OnDataVP9Func is the prototype of the callback passed to OnDataVP9().
type OnDataVP9Func func(pts time.Duration, frame []byte)

// OnDataH26xFunc is the prototype of the callback passed to OnDataH26x().
type OnDataH26xFunc func(pts time.Duration, au [][]byte)

// OnDataMPEG4AudioFunc is the prototype of the callback passed to OnDataMPEG4Audio().
type OnDataMPEG4AudioFunc func(pts time.Duration, au []byte)

// OnDataMPEG1AudioFunc is the prototype of the callback passed to OnDataMPEG1Audio().
type OnDataMPEG1AudioFunc func(pts time.Duration, frame []byte)

// OnDataG711Func is the prototype of the callback passed to OnDataG711().
type OnDataG711Func func(pts time.Duration, samples []byte)

// OnDataLPCMFunc is the prototype of the callback passed to OnDataLPCM().
type OnDataLPCMFunc func(pts time.Duration, samples []byte)

func hasVideo(md amf0.Object) (bool, error) {
	v, ok := md.Get("videocodecid")
	if !ok {
		// some Dahua cameras send width and height without videocodecid
		if v2, ok := md.Get("width"); ok {
			if vf, ok := v2.(float64); ok && vf != 0 {
				return true, nil
			}
		}
		return false, nil
	}

	switch vt := v.(type) {
	case float64:
		switch vt {
		case 0:
			return false, nil

		case message.CodecH264, float64(message.FourCCAV1),
			float64(message.FourCCVP9), float64(message.FourCCHEVC):
			return true, nil
		}

	case string:
		if vt == "avc1" || vt == "hvc1" || vt == "av01" {
			return true, nil
		}
	}

	return false, fmt.Errorf("unsupported video codec: %v", v)
}

func hasAudio(md amf0.Object, audioTrack *format.Format) (bool, error) {
	v, ok := md.Get("audiocodecid")
	if !ok {
		return false, nil
	}

	switch vt := v.(type) {
	case float64:
		switch vt {
		case 0:
			return false, nil

		case message.CodecMPEG4Audio, message.CodecLPCM:
			return true, nil

		case message.CodecMPEG1Audio:
			*audioTrack = &format.MPEG1Audio{}
			return true, nil

		case message.CodecPCMA:
			return true, nil

		case message.CodecPCMU:
			return true, nil
		}

	case string:
		if vt == "mp4a" {
			return true, nil
		}
	}

	return false, fmt.Errorf("unsupported audio codec: %v", v)
}

func h265FindNALU(array []mp4.HEVCNaluArray, typ h265.NALUType) []byte {
	for _, entry := range array {
		if entry.NaluType == byte(typ) && entry.NumNalus == 1 &&
			h265.NALUType((entry.Nalus[0].NALUnit[0]>>1)&0b111111) == typ {
			return entry.Nalus[0].NALUnit
		}
	}
	return nil
}

func trackFromH264DecoderConfig(data []byte) (format.Format, error) {
	var conf h264conf.Conf
	err := conf.Unmarshal(data)
	if err != nil {
		return nil, fmt.Errorf("unable to parse H264 config: %w", err)
	}

	return &format.H264{
		PayloadTyp:        96,
		SPS:               conf.SPS,
		PPS:               conf.PPS,
		PacketizationMode: 1,
	}, nil
}

func trackFromAACDecoderConfig(data []byte) (format.Format, error) {
	var mpegConf mpeg4audio.Config
	err := mpegConf.Unmarshal(data)
	if err != nil {
		return nil, err
	}

	return &format.MPEG4Audio{
		PayloadTyp:       96,
		Config:           &mpegConf,
		SizeLength:       13,
		IndexLength:      3,
		IndexDeltaLength: 3,
	}, nil
}

func tracksFromMetadata(conn *Conn, payload []interface{}) (format.Format, format.Format, error) {
	if len(payload) != 1 {
		return nil, nil, fmt.Errorf("invalid metadata")
	}

	var md amf0.Object
	switch pl := payload[0].(type) {
	case amf0.Object:
		md = pl

	case amf0.ECMAArray:
		md = amf0.Object(pl)

	default:
		return nil, nil, fmt.Errorf("invalid metadata")
	}

	var videoTrack format.Format
	var audioTrack format.Format

	hasVideo, err := hasVideo(md)
	if err != nil {
		return nil, nil, err
	}

	hasAudio, err := hasAudio(md, &audioTrack)
	if err != nil {
		return nil, nil, err
	}

	if !hasVideo && !hasAudio {
		return nil, nil, nil
	}

	firstReceived := false
	var startTime time.Duration

	for {
		if (!hasVideo || videoTrack != nil) &&
			(!hasAudio || audioTrack != nil) {
			return videoTrack, audioTrack, nil
		}

		msg, err := conn.Read()
		if err != nil {
			return nil, nil, err
		}

		switch msg := msg.(type) {
		case *message.Video:
			if !hasVideo {
				return nil, nil, fmt.Errorf("unexpected video packet")
			}

			if !firstReceived {
				firstReceived = true
				startTime = msg.DTS
			}

			if videoTrack == nil {
				if msg.Type == message.VideoTypeConfig {
					videoTrack, err = trackFromH264DecoderConfig(msg.Payload)
					if err != nil {
						return nil, nil, err
					}

					// format used by OBS < 29.1 to publish H265
				} else if msg.Type == message.VideoTypeAU && msg.IsKeyFrame {
					var nalus [][]byte
					nalus, err = h264.AVCCUnmarshal(msg.Payload)
					if err != nil {
						if errors.Is(err, h264.ErrAVCCNoNALUs) {
							continue
						}
						return nil, nil, err
					}

					var vps []byte
					var sps []byte
					var pps []byte

					for _, nalu := range nalus {
						typ := h265.NALUType((nalu[0] >> 1) & 0b111111)

						switch typ {
						case h265.NALUType_VPS_NUT:
							vps = nalu

						case h265.NALUType_SPS_NUT:
							sps = nalu

						case h265.NALUType_PPS_NUT:
							pps = nalu
						}
					}

					if vps != nil && sps != nil && pps != nil {
						videoTrack = &format.H265{
							PayloadTyp: 96,
							VPS:        vps,
							SPS:        sps,
							PPS:        pps,
						}
					}
				}
			}

			// video was found, but audio was not
			if videoTrack != nil && (msg.DTS-startTime) >= analyzePeriod {
				return videoTrack, nil, nil
			}

		case *message.ExtendedSequenceStart:
			if !hasVideo {
				return nil, nil, fmt.Errorf("unexpected video packet")
			}

			if videoTrack == nil {
				switch msg.FourCC {
				case message.FourCCHEVC:
					var hvcc mp4.HvcC
					_, err = mp4.Unmarshal(bytes.NewReader(msg.Config), uint64(len(msg.Config)), &hvcc, mp4.Context{})
					if err != nil {
						return nil, nil, fmt.Errorf("invalid H265 configuration: %w", err)
					}

					vps := h265FindNALU(hvcc.NaluArrays, h265.NALUType_VPS_NUT)
					sps := h265FindNALU(hvcc.NaluArrays, h265.NALUType_SPS_NUT)
					pps := h265FindNALU(hvcc.NaluArrays, h265.NALUType_PPS_NUT)
					if vps == nil || sps == nil || pps == nil {
						return nil, nil, fmt.Errorf("H265 parameters are missing")
					}

					videoTrack = &format.H265{
						PayloadTyp: 96,
						VPS:        vps,
						SPS:        sps,
						PPS:        pps,
					}

				case message.FourCCAV1:
					var av1c mp4.Av1C
					_, err = mp4.Unmarshal(bytes.NewReader(msg.Config), uint64(len(msg.Config)), &av1c, mp4.Context{})
					if err != nil {
						return nil, nil, fmt.Errorf("invalid AV1 configuration: %w", err)
					}

					// parse sequence header and metadata contained in ConfigOBUs, but do not use them
					_, err = av1.BitstreamUnmarshal(av1c.ConfigOBUs, false)
					if err != nil {
						return nil, nil, fmt.Errorf("invalid AV1 configuration: %w", err)
					}

					videoTrack = &format.AV1{
						PayloadTyp: 96,
					}

				default: // VP9
					var vpcc mp4.VpcC
					_, err = mp4.Unmarshal(bytes.NewReader(msg.Config), uint64(len(msg.Config)), &vpcc, mp4.Context{})
					if err != nil {
						return nil, nil, fmt.Errorf("invalid VP9 configuration: %w", err)
					}

					videoTrack = &format.VP9{
						PayloadTyp: 96,
					}
				}
			}

		case *message.Audio:
			if !hasAudio {
				return nil, nil, fmt.Errorf("unexpected audio packet")
			}

			if audioTrack == nil {
				if len(msg.Payload) == 0 {
					continue
				}
				switch {
				case msg.Codec == message.CodecMPEG4Audio &&
					msg.AACType == message.AudioAACTypeConfig:
					audioTrack, err = trackFromAACDecoderConfig(msg.Payload)
					if err != nil {
						return nil, nil, err
					}

				case msg.Codec == message.CodecPCMA:
					audioTrack = &format.G711{
						PayloadTyp: 8,
						MULaw:      false,
						SampleRate: 8000,
						ChannelCount: func() int {
							if msg.IsStereo {
								return 2
							}
							return 1
						}(),
					}

				case msg.Codec == message.CodecPCMU:
					audioTrack = &format.G711{
						PayloadTyp: 0,
						MULaw:      true,
						SampleRate: 8000,
						ChannelCount: func() int {
							if msg.IsStereo {
								return 2
							}
							return 1
						}(),
					}

				case msg.Codec == message.CodecLPCM:
					audioTrack = &format.LPCM{
						PayloadTyp: 96,
						BitDepth: func() int {
							if msg.Depth == message.Depth16 {
								return 16
							}
							return 8
						}(),
						SampleRate: audioRateRTMPToInt(msg.Rate),
						ChannelCount: func() int {
							if msg.IsStereo {
								return 2
							}
							return 1
						}(),
					}
				}
			}
		}
	}
}

func tracksFromMessages(conn *Conn, msg message.Message) (format.Format, format.Format, error) {
	firstReceived := false
	var startTime time.Duration
	var videoTrack format.Format
	var audioTrack format.Format

outer:
	for {
		switch msg := msg.(type) {
		case *message.Video:
			if !firstReceived {
				firstReceived = true
				startTime = msg.DTS
			}

			if msg.Type == message.VideoTypeConfig {
				if videoTrack == nil {
					var err error
					videoTrack, err = trackFromH264DecoderConfig(msg.Payload)
					if err != nil {
						return nil, nil, err
					}

					// stop the analysis if both tracks are found
					if videoTrack != nil && audioTrack != nil {
						return videoTrack, audioTrack, nil
					}
				}
			}

			if (msg.DTS - startTime) >= analyzePeriod {
				break outer
			}

		case *message.Audio:
			if !firstReceived {
				firstReceived = true
				startTime = msg.DTS
			}

			if msg.AACType == message.AudioAACTypeConfig {
				if audioTrack == nil {
					var err error
					audioTrack, err = trackFromAACDecoderConfig(msg.Payload)
					if err != nil {
						return nil, nil, err
					}

					// stop the analysis if both tracks are found
					if videoTrack != nil && audioTrack != nil {
						return videoTrack, audioTrack, nil
					}
				}
			}

			if (msg.DTS - startTime) >= analyzePeriod {
				break outer
			}
		}

		var err error
		msg, err = conn.Read()
		if err != nil {
			return nil, nil, err
		}
	}

	if videoTrack == nil && audioTrack == nil {
		return nil, nil, fmt.Errorf("no supported tracks found")
	}

	return videoTrack, audioTrack, nil
}

// Reader is a wrapper around Conn that provides utilities to demux incoming data.
type Reader struct {
	conn        *Conn
	videoTrack  format.Format
	audioTrack  format.Format
	onDataVideo func(message.Message) error
	onDataAudio func(*message.Audio) error
}

// NewReader allocates a Reader.
func NewReader(conn *Conn) (*Reader, error) {
	r := &Reader{
		conn: conn,
	}

	var err error
	r.videoTrack, r.audioTrack, err = r.readTracks()
	if err != nil {
		return nil, err
	}

	return r, nil
}

func (r *Reader) readTracks() (format.Format, format.Format, error) {
	for {
		msg, err := r.conn.Read()
		if err != nil {
			return nil, nil, err
		}

		// skip play start and data start
		if cmd, ok := msg.(*message.CommandAMF0); ok && cmd.Name == "onStatus" {
			continue
		}

		// skip RtmpSampleAccess
		if data, ok := msg.(*message.DataAMF0); ok && len(data.Payload) >= 1 {
			if s, ok := data.Payload[0].(string); ok && s == "|RtmpSampleAccess" {
				continue
			}
		}

		// skip SetChunkSize
		if _, ok := msg.(*message.SetChunkSize); ok {
			continue
		}

		if data, ok := msg.(*message.DataAMF0); ok && len(data.Payload) >= 1 {
			payload := data.Payload

			if s, ok := payload[0].(string); ok && s == "@setDataFrame" {
				payload = payload[1:]
			}

			if len(payload) >= 1 {
				if s, ok := payload[0].(string); ok && s == "onMetaData" {
					videoTrack, audioTrack, err := tracksFromMetadata(r.conn, payload[1:])
					if err != nil {
						return nil, nil, err
					}

					if videoTrack != nil || audioTrack != nil {
						return videoTrack, audioTrack, nil
					}
				}
			}
		}

		return tracksFromMessages(r.conn, msg)
	}
}

// Tracks returns detected tracks
func (r *Reader) Tracks() (format.Format, format.Format) {
	return r.videoTrack, r.audioTrack
}

// OnDataAV1 sets a callback that is called when AV1 data is received.
func (r *Reader) OnDataAV1(cb OnDataAV1Func) {
	r.onDataVideo = func(msg message.Message) error {
		if msg, ok := msg.(*message.ExtendedCodedFrames); ok {
			tu, err := av1.BitstreamUnmarshal(msg.Payload, true)
			if err != nil {
				return fmt.Errorf("unable to decode bitstream: %w", err)
			}

			cb(msg.DTS, tu)
		}
		return nil
	}
}

// OnDataVP9 sets a callback that is called when VP9 data is received.
func (r *Reader) OnDataVP9(cb OnDataVP9Func) {
	r.onDataVideo = func(msg message.Message) error {
		if msg, ok := msg.(*message.ExtendedCodedFrames); ok {
			cb(msg.DTS, msg.Payload)
		}
		return nil
	}
}

// OnDataH265 sets a callback that is called when H265 data is received.
func (r *Reader) OnDataH265(cb OnDataH26xFunc) {
	r.onDataVideo = func(msg message.Message) error {
		switch msg := msg.(type) {
		case *message.Video:
			au, err := h264.AVCCUnmarshal(msg.Payload)
			if err != nil {
				if errors.Is(err, h264.ErrAVCCNoNALUs) {
					return nil
				}
				return fmt.Errorf("unable to decode AVCC: %w", err)
			}

			cb(msg.DTS+msg.PTSDelta, au)

		case *message.ExtendedFramesX:
			au, err := h264.AVCCUnmarshal(msg.Payload)
			if err != nil {
				if errors.Is(err, h264.ErrAVCCNoNALUs) {
					return nil
				}
				return fmt.Errorf("unable to decode AVCC: %w", err)
			}

			cb(msg.DTS, au)

		case *message.ExtendedCodedFrames:
			au, err := h264.AVCCUnmarshal(msg.Payload)
			if err != nil {
				if errors.Is(err, h264.ErrAVCCNoNALUs) {
					return nil
				}
				return fmt.Errorf("unable to decode AVCC: %w", err)
			}

			cb(msg.DTS+msg.PTSDelta, au)
		}

		return nil
	}
}

// OnDataH264 sets a callback that is called when H264 data is received.
func (r *Reader) OnDataH264(cb OnDataH26xFunc) {
	r.onDataVideo = func(msg message.Message) error {
		if msg, ok := msg.(*message.Video); ok {
			switch msg.Type {
			case message.VideoTypeConfig:
				var conf h264conf.Conf
				err := conf.Unmarshal(msg.Payload)
				if err != nil {
					return fmt.Errorf("unable to parse H264 config: %w", err)
				}

				au := [][]byte{
					conf.SPS,
					conf.PPS,
				}

				cb(msg.DTS+msg.PTSDelta, au)

			case message.VideoTypeAU:
				au, err := h264.AVCCUnmarshal(msg.Payload)
				if err != nil {
					if errors.Is(err, h264.ErrAVCCNoNALUs) {
						return nil
					}
					return fmt.Errorf("unable to decode AVCC: %w", err)
				}

				cb(msg.DTS+msg.PTSDelta, au)
			}
		}

		return nil
	}
}

// OnDataMPEG4Audio sets a callback that is called when MPEG-4 Audio data is received.
func (r *Reader) OnDataMPEG4Audio(cb OnDataMPEG4AudioFunc) {
	r.onDataAudio = func(msg *message.Audio) error {
		if msg.AACType == message.AudioAACTypeAU {
			cb(msg.DTS, msg.Payload)
		}
		return nil
	}
}

// OnDataMPEG1Audio sets a callback that is called when MPEG-1 Audio data is received.
func (r *Reader) OnDataMPEG1Audio(cb OnDataMPEG1AudioFunc) {
	r.onDataAudio = func(msg *message.Audio) error {
		cb(msg.DTS, msg.Payload)
		return nil
	}
}

// OnDataG711 sets a callback that is called when G711 data is received.
func (r *Reader) OnDataG711(cb OnDataG711Func) {
	r.onDataAudio = func(msg *message.Audio) error {
		cb(msg.DTS, msg.Payload)
		return nil
	}
}

// OnDataLPCM sets a callback that is called when LPCM data is received.
func (r *Reader) OnDataLPCM(cb OnDataLPCMFunc) {
	bitDepth := r.audioTrack.(*format.LPCM).BitDepth

	if bitDepth == 16 {
		r.onDataAudio = func(msg *message.Audio) error {
			le := len(msg.Payload)
			if le%2 != 0 {
				return fmt.Errorf("invalid payload length: %d", le)
			}

			// convert from little endian to big endian
			for i := 0; i < le; i += 2 {
				msg.Payload[i], msg.Payload[i+1] = msg.Payload[i+1], msg.Payload[i]
			}

			cb(msg.DTS, msg.Payload)
			return nil
		}
	} else {
		r.onDataAudio = func(msg *message.Audio) error {
			cb(msg.DTS, msg.Payload)
			return nil
		}
	}
}

// Read reads data.
func (r *Reader) Read() error {
	msg, err := r.conn.Read()
	if err != nil {
		return err
	}

	switch msg := msg.(type) {
	case *message.Video, *message.ExtendedFramesX, *message.ExtendedCodedFrames:
		if r.onDataVideo == nil {
			return fmt.Errorf("received a video packet, but track is not set up")
		}

		return r.onDataVideo(msg)

	case *message.Audio:
		if r.onDataAudio == nil {
			return fmt.Errorf("received an audio packet, but track is not set up")
		}

		return r.onDataAudio(msg)
	}

	return nil
}
