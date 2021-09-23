package rtmp

import (
	"fmt"

	"github.com/aler9/gortsplib"
	"github.com/aler9/gortsplib/pkg/aac"
	"github.com/notedit/rtmp/av"
	nh264 "github.com/notedit/rtmp/codec/h264"
	"github.com/notedit/rtmp/format/flv/flvio"
)

const (
	codecH264 = 7
	codecAAC  = 10
)

// ReadMetadata extracts track informations from a connection that is publishing.
func (c *Conn) ReadMetadata() (*gortsplib.Track, *gortsplib.Track, error) {
	var videoTrack *gortsplib.Track
	var audioTrack *gortsplib.Track

	md, err := func() (flvio.AMFMap, error) {
		pkt, err := c.ReadPacket()
		if err != nil {
			return nil, err
		}

		if pkt.Type != av.Metadata {
			return nil, fmt.Errorf("first packet must be metadata")
		}

		arr, err := flvio.ParseAMFVals(pkt.Data, false)
		if err != nil {
			return nil, err
		}

		if len(arr) != 1 {
			return nil, fmt.Errorf("invalid metadata")
		}

		ma, ok := arr[0].(flvio.AMFMap)
		if !ok {
			return nil, fmt.Errorf("invalid metadata")
		}

		return ma, nil
	}()
	if err != nil {
		return nil, nil, err
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
		return nil, nil, fmt.Errorf("stream doesn't contain tracks with supported codecs (H264 or AAC)")
	}

	for {
		var pkt av.Packet
		pkt, err = c.ReadPacket()
		if err != nil {
			return nil, nil, err
		}

		switch pkt.Type {
		case av.H264DecoderConfig:
			if !hasVideo {
				return nil, nil, fmt.Errorf("unexpected video packet")
			}

			if videoTrack != nil {
				return nil, nil, fmt.Errorf("video track setupped twice")
			}

			codec, err := nh264.FromDecoderConfig(pkt.Data)
			if err != nil {
				return nil, nil, err
			}

			videoTrack, err = gortsplib.NewTrackH264(96, &gortsplib.TrackConfigH264{SPS: codec.SPS[0], PPS: codec.PPS[0]})
			if err != nil {
				return nil, nil, err
			}

		case av.AACDecoderConfig:
			if !hasAudio {
				return nil, nil, fmt.Errorf("unexpected audio packet")
			}

			if audioTrack != nil {
				return nil, nil, fmt.Errorf("audio track setupped twice")
			}

			var mpegConf aac.MPEG4AudioConfig
			err := mpegConf.Decode(pkt.Data)
			if err != nil {
				return nil, nil, err
			}

			audioTrack, err = gortsplib.NewTrackAAC(96, &gortsplib.TrackConfigAAC{
				Type:              int(mpegConf.Type),
				SampleRate:        mpegConf.SampleRate,
				ChannelCount:      mpegConf.ChannelCount,
				AOTSpecificConfig: mpegConf.AOTSpecificConfig,
			})
			if err != nil {
				return nil, nil, err
			}
		}

		if (!hasVideo || videoTrack != nil) &&
			(!hasAudio || audioTrack != nil) {
			return videoTrack, audioTrack, nil
		}
	}
}

// WriteMetadata writes track informations to a connection that is reading.
func (c *Conn) WriteMetadata(videoTrack *gortsplib.Track, audioTrack *gortsplib.Track) error {
	err := c.WritePacket(av.Packet{
		Type: av.Metadata,
		Data: flvio.FillAMF0ValMalloc(flvio.AMFMap{
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
		}),
	})
	if err != nil {
		return err
	}

	if videoTrack != nil {
		conf, err := videoTrack.ExtractConfigH264()
		if err != nil {
			return err
		}

		codec := nh264.Codec{
			SPS: map[int][]byte{
				0: conf.SPS,
			},
			PPS: map[int][]byte{
				0: conf.PPS,
			},
		}
		b := make([]byte, 128)
		var n int
		codec.ToConfig(b, &n)
		b = b[:n]

		err = c.WritePacket(av.Packet{
			Type: av.H264DecoderConfig,
			Data: b,
		})
		if err != nil {
			return err
		}
	}

	if audioTrack != nil {
		conf, err := audioTrack.ExtractConfigAAC()
		if err != nil {
			return err
		}

		enc, err := aac.MPEG4AudioConfig{
			Type:              aac.MPEG4AudioType(conf.Type),
			SampleRate:        conf.SampleRate,
			ChannelCount:      conf.ChannelCount,
			AOTSpecificConfig: conf.AOTSpecificConfig,
		}.Encode()
		if err != nil {
			return err
		}

		err = c.WritePacket(av.Packet{
			Type: av.AACDecoderConfig,
			Data: enc,
		})
		if err != nil {
			return err
		}
	}

	return nil
}
