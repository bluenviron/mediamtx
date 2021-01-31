package rtmpinfo

import (
	"fmt"
	"net"
	"time"

	"github.com/aler9/gortsplib"
	"github.com/notedit/rtmp/av"
	"github.com/notedit/rtmp/codec/h264"
	"github.com/notedit/rtmp/format/flv/flvio"
	"github.com/notedit/rtmp/format/rtmp"
)

const (
	codecH264 = 7
	codecAAC  = 10
)

func readMetadata(rconn *rtmp.Conn) (flvio.AMFMap, error) {
	pkt, err := rconn.ReadPacket()
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

// Info extracts track informations from a RTMP connection.
func Info(rconn *rtmp.Conn, nconn net.Conn, readTimeout time.Duration) (
	*gortsplib.Track, *gortsplib.Track, error) {
	var videoTrack *gortsplib.Track
	var audioTrack *gortsplib.Track

	// configuration must be completed within readTimeout
	nconn.SetReadDeadline(time.Now().Add(readTimeout))

	md, err := readMetadata(rconn)
	if err != nil {
		return nil, nil, err
	}

	hasVideo := false
	if v, ok := md.GetFloat64("videocodecid"); ok {
		switch v {
		case codecH264:
			hasVideo = true
		case 0:
		default:
			return nil, nil, fmt.Errorf("unsupported video codec %v", v)
		}

	}

	hasAudio := false
	if v, ok := md.GetFloat64("audiocodecid"); ok {
		switch v {
		case codecAAC:
			hasAudio = true
		case 0:
		default:
			return nil, nil, fmt.Errorf("unsupported audio codec %v", v)
		}
	}

	if !hasVideo && !hasAudio {
		return nil, nil, fmt.Errorf("stream has no tracks")
	}

	for {
		var pkt av.Packet
		pkt, err = rconn.ReadPacket()
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
