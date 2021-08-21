package hls

import (
	"context"
	"io"
	"strconv"
	"time"

	"github.com/aler9/gortsplib"
	"github.com/asticode/go-astits"

	"github.com/aler9/rtsp-simple-server/internal/aac"
	"github.com/aler9/rtsp-simple-server/internal/h264"
)

type tsFile struct {
	videoTrack         *gortsplib.Track
	name               string
	buf                *multiAccessBuffer
	mux                *astits.Muxer
	firstPacketWritten bool
	minPTS             time.Duration
	maxPTS             time.Duration
	startPCR           time.Time
	pcrSendCounter     int
}

func newTSFile(videoTrack *gortsplib.Track, audioTrack *gortsplib.Track) *tsFile {
	t := &tsFile{
		videoTrack: videoTrack,
		name:       strconv.FormatInt(time.Now().Unix(), 10),
		buf:        newMultiAccessBuffer(),
	}

	t.mux = astits.NewMuxer(context.Background(), t.buf)

	if videoTrack != nil {
		t.mux.AddElementaryStream(astits.PMTElementaryStream{
			ElementaryPID: 256,
			StreamType:    astits.StreamTypeH264Video,
		})
	}

	if audioTrack != nil {
		t.mux.AddElementaryStream(astits.PMTElementaryStream{
			ElementaryPID: 257,
			StreamType:    astits.StreamTypeAACAudio,
		})
	}

	if videoTrack != nil {
		t.mux.SetPCRPID(256)
	} else {
		t.mux.SetPCRPID(257)
	}

	// WriteTable() is called automatically when WriteData() is called with
	// - PID == PCRPID
	// - AdaptationField != nil
	// - RandomAccessIndicator = true

	return t
}

func (t *tsFile) close() error {
	return t.buf.Close()
}

func (t *tsFile) duration() time.Duration {
	return t.maxPTS - t.minPTS
}

func (t *tsFile) setStartPCR(startPCR time.Time) {
	t.startPCR = startPCR
}

func (t *tsFile) newReader() io.Reader {
	return t.buf.NewReader()
}

func (t *tsFile) writeH264(
	h264SPS []byte,
	h264PPS []byte,
	dts time.Duration,
	pts time.Duration,
	isIDR bool,
	nalus [][]byte) error {
	if !t.firstPacketWritten {
		t.firstPacketWritten = true
		t.minPTS = pts
		t.maxPTS = pts
	} else {
		if pts < t.minPTS {
			t.minPTS = pts
		}
		if pts > t.maxPTS {
			t.maxPTS = pts
		}
	}

	filteredNALUs := [][]byte{
		// prepend an AUD. This is required by video.js and iOS
		{byte(h264.NALUTypeAccessUnitDelimiter), 240},
	}

	for _, nalu := range nalus {
		// remove existing SPS, PPS, AUD
		typ := h264.NALUType(nalu[0] & 0x1F)
		switch typ {
		case h264.NALUTypeSPS, h264.NALUTypePPS, h264.NALUTypeAccessUnitDelimiter:
			continue
		}

		// add SPS and PPS before IDR
		if typ == h264.NALUTypeIDR {
			filteredNALUs = append(filteredNALUs, h264SPS)
			filteredNALUs = append(filteredNALUs, h264PPS)
		}

		filteredNALUs = append(filteredNALUs, nalu)
	}

	enc, err := h264.EncodeAnnexB(filteredNALUs)
	if err != nil {
		return err
	}

	var af *astits.PacketAdaptationField

	if isIDR {
		if af == nil {
			af = &astits.PacketAdaptationField{}
		}
		af.RandomAccessIndicator = true
	}

	// send PCR once in a while
	if t.pcrSendCounter == 0 {
		if af == nil {
			af = &astits.PacketAdaptationField{}
		}
		af.HasPCR = true
		af.PCR = &astits.ClockReference{Base: int64(time.Since(t.startPCR).Seconds() * 90000)}
		t.pcrSendCounter = 3
	}
	t.pcrSendCounter--

	oh := &astits.PESOptionalHeader{
		MarkerBits: 2,
	}

	if dts == pts {
		oh.PTSDTSIndicator = astits.PTSDTSIndicatorOnlyPTS
		oh.PTS = &astits.ClockReference{Base: int64(pts.Seconds() * 90000)}
	} else {
		oh.PTSDTSIndicator = astits.PTSDTSIndicatorBothPresent
		oh.DTS = &astits.ClockReference{Base: int64(dts.Seconds() * 90000)}
		oh.PTS = &astits.ClockReference{Base: int64(pts.Seconds() * 90000)}
	}

	_, err = t.mux.WriteData(&astits.MuxerData{
		PID:             256,
		AdaptationField: af,
		PES: &astits.PESData{
			Header: &astits.PESHeader{
				OptionalHeader: oh,
				StreamID:       224, // = video
			},
			Data: enc,
		},
	})
	return err
}

func (t *tsFile) writeAAC(sampleRate int, channelCount int, pts time.Duration, au []byte) error {
	if t.videoTrack == nil {
		if !t.firstPacketWritten {
			t.firstPacketWritten = true
			t.minPTS = pts
			t.maxPTS = pts
		} else {
			if pts < t.minPTS {
				t.minPTS = pts
			}
			if pts > t.maxPTS {
				t.maxPTS = pts
			}
		}
	}

	adtsPkt, err := aac.EncodeADTS([]*aac.ADTSPacket{
		{
			SampleRate:   sampleRate,
			ChannelCount: channelCount,
			Frame:        au,
		},
	})
	if err != nil {
		return err
	}

	af := &astits.PacketAdaptationField{
		RandomAccessIndicator: true,
	}

	// if audio is the only track
	if t.videoTrack == nil {
		// send PCR once in a while
		if t.pcrSendCounter == 0 {
			af.HasPCR = true
			af.PCR = &astits.ClockReference{Base: int64(time.Since(t.startPCR).Seconds() * 90000)}
			t.pcrSendCounter = 3
		}
	}

	_, err = t.mux.WriteData(&astits.MuxerData{
		PID:             257,
		AdaptationField: af,
		PES: &astits.PESData{
			Header: &astits.PESHeader{
				OptionalHeader: &astits.PESOptionalHeader{
					MarkerBits:      2,
					PTSDTSIndicator: astits.PTSDTSIndicatorOnlyPTS,
					PTS:             &astits.ClockReference{Base: int64(pts.Seconds() * 90000)},
				},
				PacketLength: uint16(len(adtsPkt) + 8),
				StreamID:     192, // = audio
			},
			Data: adtsPkt,
		},
	})
	return err
}
