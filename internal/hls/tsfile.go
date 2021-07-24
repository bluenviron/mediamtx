package hls

import (
	"context"
	"io"
	"strconv"
	"time"

	"github.com/asticode/go-astits"

	"github.com/aler9/rtsp-simple-server/internal/aac"
	"github.com/aler9/rtsp-simple-server/internal/h264"
)

type tsFile struct {
	name               string
	buf                *multiAccessBuffer
	mux                *astits.Muxer
	pcrTrackIsVideo    bool
	pcr                time.Duration
	firstPacketWritten bool
	minPTS             time.Duration
	maxPTS             time.Duration
}

func newTSFile(hasVideoTrack bool, hasAudioTrack bool) *tsFile {
	t := &tsFile{
		buf:  newMultiAccessBuffer(),
		name: strconv.FormatInt(time.Now().Unix(), 10),
	}

	t.mux = astits.NewMuxer(context.Background(), t.buf)

	if hasVideoTrack {
		t.mux.AddElementaryStream(astits.PMTElementaryStream{
			ElementaryPID: 256,
			StreamType:    astits.StreamTypeH264Video,
		})
	}

	if hasAudioTrack {
		t.mux.AddElementaryStream(astits.PMTElementaryStream{
			ElementaryPID: 257,
			StreamType:    astits.StreamTypeAACAudio,
		})
	}

	if hasVideoTrack {
		t.pcrTrackIsVideo = true
		t.mux.SetPCRPID(256)
	} else {
		t.pcrTrackIsVideo = false
		t.mux.SetPCRPID(257)
	}

	// write PMT at the beginning of every segment
	// so no packets are lost
	t.mux.WriteTables()

	return t
}

func (t *tsFile) close() error {
	return t.buf.Close()
}

func (t *tsFile) duration() time.Duration {
	return t.maxPTS - t.minPTS
}

func (t *tsFile) setPCR(pcr time.Duration) {
	t.pcr = pcr
}

func (t *tsFile) newReader() io.Reader {
	return t.buf.NewReader()
}

func (t *tsFile) writeH264(dts time.Duration, pts time.Duration, isIDR bool, nalus [][]byte) error {
	if t.pcrTrackIsVideo {
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

	enc, err := h264.EncodeAnnexB(nalus)
	if err != nil {
		return err
	}

	af := &astits.PacketAdaptationField{
		RandomAccessIndicator: isIDR,
	}

	if t.pcrTrackIsVideo {
		af.HasPCR = true
		af.PCR = &astits.ClockReference{Base: int64(t.pcr.Seconds() * 90000)}
	}

	_, err = t.mux.WriteData(&astits.MuxerData{
		PID:             256,
		AdaptationField: af,
		PES: &astits.PESData{
			Header: &astits.PESHeader{
				OptionalHeader: &astits.PESOptionalHeader{
					MarkerBits:      2,
					PTSDTSIndicator: astits.PTSDTSIndicatorBothPresent,
					DTS:             &astits.ClockReference{Base: int64(dts.Seconds() * 90000)},
					PTS:             &astits.ClockReference{Base: int64(pts.Seconds() * 90000)},
				},
				StreamID: 224, // = video
			},
			Data: enc,
		},
	})
	return err
}

func (t *tsFile) writeAAC(sampleRate int, channelCount int, pts time.Duration, au []byte) error {
	if !t.pcrTrackIsVideo {
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

	if !t.pcrTrackIsVideo {
		af.HasPCR = true
		af.PCR = &astits.ClockReference{Base: int64(t.pcr.Seconds() * 90000)}
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
