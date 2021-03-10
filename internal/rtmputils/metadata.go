package rtmputils

import (
	"fmt"

	"github.com/aler9/gortsplib"
	"github.com/notedit/rtmp/av"
	"github.com/notedit/rtmp/codec/h264"
	"github.com/notedit/rtmp/format/flv/flvio"
)

const (
	codecH264 = 7
	codecAAC  = 10
)

func readMetadata(conn *Conn) (flvio.AMFMap, error) {
	pkt, err := conn.ReadPacket()
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
}

// ReadMetadata extracts track informations from a RTMP connection that is publishing.
func ReadMetadata(conn *Conn) (*gortsplib.Track, *gortsplib.Track, error) {
	var videoTrack *gortsplib.Track
	var audioTrack *gortsplib.Track

	md, err := readMetadata(conn)
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
			switch vt {
			case "avc1":
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
			switch vt {
			case "mp4a":
				return true, nil
			}
		}

		return false, fmt.Errorf("unsupported audio codec %v", v)
	}()
	if err != nil {
		return nil, nil, err
	}

	if !hasVideo && !hasAudio {
		return nil, nil, fmt.Errorf("stream has no tracks")
	}

	for {
		var pkt av.Packet
		pkt, err = conn.ReadPacket()
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

			codec, err := h264.FromDecoderConfig(pkt.Data)
			if err != nil {
				return nil, nil, err
			}

			videoTrack, err = gortsplib.NewTrackH264(96, codec.SPS[0], codec.PPS[0])
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

			audioTrack, err = gortsplib.NewTrackAAC(96, pkt.Data)
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

// WriteMetadata writes track informations to a RTMP connection that is reading.
func WriteMetadata(conn *Conn, videoTrack *gortsplib.Track, audioTrack *gortsplib.Track) error {
	return conn.WritePacket(av.Packet{
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
}
