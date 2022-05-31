package hls

import (
	"bytes"
	"fmt"
	"math"
	"time"

	"github.com/aler9/gortsplib"
	"github.com/aler9/gortsplib/pkg/h264"
	"github.com/icza/bitio"
)

const (
	fmp4VideoTimescale = 90000
)

func readGolombUnsigned(br *bitio.Reader) (uint32, error) {
	leadingZeroBits := uint32(0)

	for {
		b, err := br.ReadBits(1)
		if err != nil {
			return 0, err
		}

		if b != 0 {
			break
		}

		leadingZeroBits++
	}

	codeNum := uint32(0)

	for n := leadingZeroBits; n > 0; n-- {
		b, err := br.ReadBits(1)
		if err != nil {
			return 0, err
		}

		codeNum |= uint32(b) << (n - 1)
	}

	codeNum = (1 << leadingZeroBits) - 1 + codeNum

	return codeNum, nil
}

func getPOC(buf []byte, sps *h264.SPS) (uint32, error) {
	buf = h264.AntiCompetitionRemove(buf[:10])

	isIDR := h264.NALUType(buf[0]&0x1F) == h264.NALUTypeIDR

	r := bytes.NewReader(buf[1:])
	br := bitio.NewReader(r)

	// first_mb_in_slice
	_, err := readGolombUnsigned(br)
	if err != nil {
		return 0, err
	}

	// slice_type
	_, err = readGolombUnsigned(br)
	if err != nil {
		return 0, err
	}

	// pic_parameter_set_id
	_, err = readGolombUnsigned(br)
	if err != nil {
		return 0, err
	}

	// frame_num
	_, err = br.ReadBits(uint8(sps.Log2MaxFrameNumMinus4 + 4))
	if err != nil {
		return 0, err
	}

	if !sps.FrameMbsOnlyFlag {
		return 0, fmt.Errorf("unsupported")
	}

	if isIDR {
		// idr_pic_id
		_, err := readGolombUnsigned(br)
		if err != nil {
			return 0, err
		}
	}

	var picOrderCntLsb uint64
	switch {
	case sps.PicOrderCntType == 0:
		picOrderCntLsb, err = br.ReadBits(uint8(sps.Log2MaxPicOrderCntLsbMinus4 + 4))
		if err != nil {
			return 0, err
		}

	default:
		return 0, fmt.Errorf("pic_order_cnt_type = 1 is unsupported")
	}

	return uint32(picOrderCntLsb), nil
}

func getNALUSPOC(nalus [][]byte, sps *h264.SPS) (uint32, error) {
	for _, nalu := range nalus {
		typ := h264.NALUType(nalu[0] & 0x1F)
		if typ == h264.NALUTypeIDR || typ == h264.NALUTypeNonIDR {
			poc, err := getPOC(nalu, sps)
			if err != nil {
				return 0, err
			}
			return poc, nil
		}
	}
	return 0, fmt.Errorf("POC not found")
}

func getPOCDiff(poc uint32, expectedPOC uint32, sps *h264.SPS) int32 {
	diff := int32(poc) - int32(expectedPOC)
	switch {
	case diff < -((1 << (sps.Log2MaxPicOrderCntLsbMinus4 + 3)) - 1):
		diff += (1 << (sps.Log2MaxPicOrderCntLsbMinus4 + 4))

	case diff > ((1 << (sps.Log2MaxPicOrderCntLsbMinus4 + 3)) - 1):
		diff -= (1 << (sps.Log2MaxPicOrderCntLsbMinus4 + 4))
	}
	return diff
}

type fmp4VideoSample struct {
	pts        time.Duration
	dts        time.Duration
	nalus      [][]byte
	avcc       []byte
	idrPresent bool
	next       *fmp4VideoSample
	pocDiff    int32
}

func (s *fmp4VideoSample) fillDTS(
	prev *fmp4VideoSample,
	sps *h264.SPS,
	expectedPOC *uint32,
) error {
	if s.idrPresent || sps.PicOrderCntType == 2 {
		s.dts = s.pts
		*expectedPOC = 0
	} else {
		*expectedPOC += 2
		*expectedPOC &= ((1 << (sps.Log2MaxPicOrderCntLsbMinus4 + 4)) - 1)

		poc, err := getNALUSPOC(s.nalus, sps)
		if err != nil {
			return err
		}

		s.pocDiff = getPOCDiff(poc, *expectedPOC, sps)

		if s.pocDiff == 0 {
			s.dts = s.pts
		} else {
			if prev.pocDiff == 0 {
				if s.pocDiff == -2 {
					return fmt.Errorf("invalid frame POC")
				}
				s.dts = prev.pts + time.Duration(math.Round(float64(s.pts-prev.pts)/float64(s.pocDiff/2+1)))
			} else {
				s.dts = s.pts + time.Duration(math.Round(float64(prev.dts-prev.pts)*float64(s.pocDiff)/float64(prev.pocDiff)))
			}
		}
	}

	return nil
}

func (s fmp4VideoSample) duration() time.Duration {
	return s.next.dts - s.dts
}

type fmp4AudioSample struct {
	pts  time.Duration
	au   []byte
	next *fmp4AudioSample
}

func (s fmp4AudioSample) duration() time.Duration {
	return s.next.pts - s.pts
}

type muxerVariantFMP4 struct {
	playlist  *muxerVariantFMP4Playlist
	segmenter *muxerVariantFMP4Segmenter
}

func newMuxerVariantFMP4(
	lowLatency bool,
	segmentCount int,
	segmentDuration time.Duration,
	partDuration time.Duration,
	segmentMaxSize uint64,
	videoTrack *gortsplib.TrackH264,
	audioTrack *gortsplib.TrackAAC,
) *muxerVariantFMP4 {
	v := &muxerVariantFMP4{}

	v.playlist = newMuxerVariantFMP4Playlist(
		lowLatency,
		segmentCount,
		videoTrack,
		audioTrack,
	)

	v.segmenter = newMuxerVariantFMP4Segmenter(
		lowLatency,
		segmentCount,
		segmentDuration,
		partDuration,
		segmentMaxSize,
		videoTrack,
		audioTrack,
		v.playlist.onSegmentFinalized,
		v.playlist.onPartFinalized,
	)

	return v
}

func (v *muxerVariantFMP4) close() {
	v.playlist.close()
}

func (v *muxerVariantFMP4) writeH264(pts time.Duration, nalus [][]byte) error {
	return v.segmenter.writeH264(pts, nalus)
}

func (v *muxerVariantFMP4) writeAAC(pts time.Duration, aus [][]byte) error {
	return v.segmenter.writeAAC(pts, aus)
}

func (v *muxerVariantFMP4) file(name string, msn string, part string, skip string) *MuxerFileResponse {
	return v.playlist.file(name, msn, part, skip)
}
