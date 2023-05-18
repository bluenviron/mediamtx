// Package tracks contains functions to read and write track metadata.
package tracks

import (
	"bytes"
	"errors"
	"fmt"
	"time"

	gomp4 "github.com/abema/go-mp4"
	"github.com/bluenviron/gortsplib/v3/pkg/formats"
	"github.com/bluenviron/mediacommon/pkg/codecs/av1"
	"github.com/bluenviron/mediacommon/pkg/codecs/h264"
	"github.com/bluenviron/mediacommon/pkg/codecs/h265"
	"github.com/bluenviron/mediacommon/pkg/codecs/mpeg4audio"
	"github.com/notedit/rtmp/format/flv/flvio"

	"github.com/bluenviron/mediamtx/internal/rtmp/h264conf"
	"github.com/bluenviron/mediamtx/internal/rtmp/message"
)

func h265FindNALU(array []gomp4.HEVCNaluArray, typ h265.NALUType) []byte {
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

func readTracksFromMetadata(r *message.ReadWriter, payload []interface{}) (formats.Format, formats.Format, error) {
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

			case message.CodecH264:
				return true, nil
			}

		case string:
			if vt == "avc1" {
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

			case message.CodecMPEG2Audio:
				audioTrack = &formats.MPEG2Audio{}
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

		msg, err := r.Read()
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
					var hvcc gomp4.HvcC
					_, err := gomp4.Unmarshal(bytes.NewReader(tmsg.Config), uint64(len(tmsg.Config)), &hvcc, gomp4.Context{})
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
					var av1c Av1C
					_, err := gomp4.Unmarshal(bytes.NewReader(tmsg.Config), uint64(len(tmsg.Config)), &av1c, gomp4.Context{})
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

func readTracksFromMessages(r *message.ReadWriter, msg message.Message) (formats.Format, formats.Format, error) {
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
		msg, err = r.Read()
		if err != nil {
			return nil, nil, err
		}
	}

	if videoTrack == nil && audioTrack == nil {
		return nil, nil, fmt.Errorf("no tracks found")
	}

	return videoTrack, audioTrack, nil
}

// Read reads track informations.
// It returns the video track and the audio track.
func Read(r *message.ReadWriter) (formats.Format, formats.Format, error) {
	msg, err := func() (message.Message, error) {
		for {
			msg, err := r.Read()
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
				videoTrack, audioTrack, err := readTracksFromMetadata(r, payload[1:])
				if err != nil {
					if err == errEmptyMetadata {
						msg, err := r.Read()
						if err != nil {
							return nil, nil, err
						}

						return readTracksFromMessages(r, msg)
					}

					return nil, nil, err
				}

				return videoTrack, audioTrack, nil
			}
		}
	}

	return readTracksFromMessages(r, msg)
}
