package rtmp

import (
	"bytes"
	"errors"
	"fmt"
	"time"

	"github.com/abema/go-mp4"
	"github.com/bluenviron/gortsplib/v3/pkg/formats"
	"github.com/bluenviron/mediacommon/pkg/codecs/av1"
	"github.com/bluenviron/mediacommon/pkg/codecs/h264"
	"github.com/bluenviron/mediacommon/pkg/codecs/h265"
	"github.com/bluenviron/mediacommon/pkg/codecs/mpeg4audio"
	"github.com/notedit/rtmp/format/flv/flvio"

	"github.com/bluenviron/mediamtx/internal/rtmp/h264conf"
	"github.com/bluenviron/mediamtx/internal/rtmp/message"
)

// OnDataAV1Func is the prototype of the callback passed to OnDataAV1().
type OnDataAV1Func func(pts time.Duration, tu [][]byte)

// OnDataH26xFunc is the prototype of the callback passed to OnDataH26x().
type OnDataH26xFunc func(pts time.Duration, au [][]byte)

// OnDataMPEG4AudioFunc is the prototype of the callback passed to OnDataMPEG4Audio().
type OnDataMPEG4AudioFunc func(pts time.Duration, au []byte)

// OnDataMPEG1AudioFunc is the prototype of the callback passed to OnDataMPEG1Audio().
type OnDataMPEG1AudioFunc func(pts time.Duration, frame []byte)

func h265FindNALU(array []mp4.HEVCNaluArray, typ h265.NALUType) []byte {
	for _, entry := range array {
		if entry.NaluType == byte(typ) && entry.NumNalus == 1 &&
			h265.NALUType((entry.Nalus[0].NALUnit[0]>>1)&0b111111) == typ {
			return entry.Nalus[0].NALUnit
		}
	}
	return nil
}

func trackFromH264DecoderConfig(data []byte) (formats.Format, error) {
	var conf h264conf.Conf
	err := conf.Unmarshal(data)
	if err != nil {
		return nil, fmt.Errorf("unable to parse H264 config: %v", err)
	}

	return &formats.H264{
		PayloadTyp:        96,
		SPS:               conf.SPS,
		PPS:               conf.PPS,
		PacketizationMode: 1,
	}, nil
}

func trackFromAACDecoderConfig(data []byte) (formats.Format, error) {
	var mpegConf mpeg4audio.Config
	err := mpegConf.Unmarshal(data)
	if err != nil {
		return nil, err
	}

	return &formats.MPEG4Audio{
		PayloadTyp:       96,
		Config:           &mpegConf,
		SizeLength:       13,
		IndexLength:      3,
		IndexDeltaLength: 3,
	}, nil
}

var errEmptyMetadata = errors.New("metadata is empty")

func tracksFromMetadata(conn *Conn, payload []interface{}) (formats.Format, formats.Format, error) {
	if len(payload) != 1 {
		return nil, nil, fmt.Errorf("invalid metadata")
	}

	md, ok := payload[0].(flvio.AMFMap)
	if !ok {
		return nil, nil, fmt.Errorf("invalid metadata")
	}

	var videoTrack formats.Format
	var audioTrack formats.Format

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

			case message.CodecMPEG1Audio:
				audioTrack = &formats.MPEG1Audio{}
				return true, nil

			case message.CodecMPEG4Audio:
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

	for {
		if (!hasVideo || videoTrack != nil) &&
			(!hasAudio || audioTrack != nil) {
			return videoTrack, audioTrack, nil
		}

		msg, err := conn.Read()
		if err != nil {
			return nil, nil, err
		}

		switch tmsg := msg.(type) {
		case *message.Video:
			if !hasVideo {
				return nil, nil, fmt.Errorf("unexpected video packet")
			}

			if videoTrack == nil {
				if tmsg.Type == message.VideoTypeConfig {
					videoTrack, err = trackFromH264DecoderConfig(tmsg.Payload)
					if err != nil {
						return nil, nil, err
					}

					// format used by OBS < 29.1 to publish H265
				} else if tmsg.Type == message.VideoTypeAU && tmsg.IsKeyFrame {
					nalus, err := h264.AVCCUnmarshal(tmsg.Payload)
					if err != nil {
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
						videoTrack = &formats.H265{
							PayloadTyp: 96,
							VPS:        vps,
							SPS:        sps,
							PPS:        pps,
						}
					}
				}
			}

		case *message.ExtendedSequenceStart:
			if videoTrack == nil {
				switch tmsg.FourCC {
				case message.FourCCHEVC:
					var hvcc mp4.HvcC
					_, err := mp4.Unmarshal(bytes.NewReader(tmsg.Config), uint64(len(tmsg.Config)), &hvcc, mp4.Context{})
					if err != nil {
						return nil, nil, fmt.Errorf("invalid H265 configuration: %v", err)
					}

					vps := h265FindNALU(hvcc.NaluArrays, h265.NALUType_VPS_NUT)
					sps := h265FindNALU(hvcc.NaluArrays, h265.NALUType_SPS_NUT)
					pps := h265FindNALU(hvcc.NaluArrays, h265.NALUType_PPS_NUT)
					if vps == nil || sps == nil || pps == nil {
						return nil, nil, fmt.Errorf("H265 parameters are missing")
					}

					videoTrack = &formats.H265{
						PayloadTyp: 96,
						VPS:        vps,
						SPS:        sps,
						PPS:        pps,
					}

				case message.FourCCAV1:
					var av1c mp4.Av1C
					_, err := mp4.Unmarshal(bytes.NewReader(tmsg.Config), uint64(len(tmsg.Config)), &av1c, mp4.Context{})
					if err != nil {
						return nil, nil, fmt.Errorf("invalid AV1 configuration: %v", err)
					}

					// parse sequence header and metadata contained in ConfigOBUs, but do not use them
					_, err = av1.BitstreamUnmarshal(av1c.ConfigOBUs, false)
					if err != nil {
						return nil, nil, fmt.Errorf("invalid AV1 configuration: %v", err)
					}

					videoTrack = &formats.AV1{}

				default: // VP9
					return nil, nil, fmt.Errorf("VP9 is not supported yet")
				}
			}

		case *message.Audio:
			if !hasAudio {
				return nil, nil, fmt.Errorf("unexpected audio packet")
			}

			if audioTrack == nil &&
				tmsg.Codec == message.CodecMPEG4Audio &&
				tmsg.AACType == message.AudioAACTypeConfig {
				audioTrack, err = trackFromAACDecoderConfig(tmsg.Payload)
				if err != nil {
					return nil, nil, err
				}
			}
		}
	}
}

func tracksFromMessages(conn *Conn, msg message.Message) (formats.Format, formats.Format, error) {
	var startTime *time.Duration
	var videoTrack formats.Format
	var audioTrack formats.Format

	// analyze 1 second of packets
outer:
	for {
		switch tmsg := msg.(type) {
		case *message.Video:
			if startTime == nil {
				v := tmsg.DTS
				startTime = &v
			}

			if tmsg.Type == message.VideoTypeConfig {
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

		case *message.Audio:
			if startTime == nil {
				v := tmsg.DTS
				startTime = &v
			}

			if tmsg.AACType == message.AudioAACTypeConfig {
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
	videoTrack  formats.Format
	audioTrack  formats.Format
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

func (r *Reader) readTracks() (formats.Format, formats.Format, error) {
	msg, err := func() (message.Message, error) {
		for {
			msg, err := r.conn.Read()
			if err != nil {
				return nil, err
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

			return msg, nil
		}
	}()
	if err != nil {
		return nil, nil, err
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
					if err == errEmptyMetadata {
						msg, err := r.conn.Read()
						if err != nil {
							return nil, nil, err
						}

						return tracksFromMessages(r.conn, msg)
					}

					return nil, nil, err
				}

				return videoTrack, audioTrack, nil
			}
		}
	}

	return tracksFromMessages(r.conn, msg)
}

// Tracks returns detected tracks
func (r *Reader) Tracks() (formats.Format, formats.Format) {
	return r.videoTrack, r.audioTrack
}

// OnDataAV1 sets a callback that is called when AV1 data is received.
func (r *Reader) OnDataAV1(cb OnDataAV1Func) {
	r.onDataVideo = func(msg message.Message) error {
		if msg, ok := msg.(*message.ExtendedCodedFrames); ok {
			tu, err := av1.BitstreamUnmarshal(msg.Payload, true)
			if err != nil {
				return fmt.Errorf("unable to decode bitstream: %v", err)
			}

			cb(msg.DTS, tu)
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
				return fmt.Errorf("unable to decode AVCC: %v", err)
			}

			cb(msg.DTS+msg.PTSDelta, au)

		case *message.ExtendedFramesX:
			au, err := h264.AVCCUnmarshal(msg.Payload)
			if err != nil {
				return fmt.Errorf("unable to decode AVCC: %v", err)
			}

			cb(msg.DTS, au)

		case *message.ExtendedCodedFrames:
			au, err := h264.AVCCUnmarshal(msg.Payload)
			if err != nil {
				return fmt.Errorf("unable to decode AVCC: %v", err)
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
					return fmt.Errorf("unable to parse H264 config: %v", err)
				}

				au := [][]byte{
					conf.SPS,
					conf.PPS,
				}

				cb(msg.DTS+msg.PTSDelta, au)

			case message.VideoTypeAU:
				au, err := h264.AVCCUnmarshal(msg.Payload)
				if err != nil {
					return fmt.Errorf("unable to decode AVCC: %v", err)
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
