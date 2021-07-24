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

// TSFile is a MPEG-TS file.
type TSFile struct {
	name               string
	buf                *multiAccessBuffer
	mux                *astits.Muxer
	pcrTrackIsVideo    bool
	pcr                time.Duration
	firstPacketWritten bool
	minPTS             time.Duration
	maxPTS             time.Duration
}

// NewTSFile allocates a TSFile.
func NewTSFile(videoTrack *gortsplib.Track, audioTrack *gortsplib.Track) *TSFile {
	t := &TSFile{
		buf:  newMultiAccessBuffer(),
		name: strconv.FormatInt(time.Now().Unix(), 10),
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

// Close closes a TSFile.
func (t *TSFile) Close() error {
	return t.buf.Close()
}

// Name returns the file name.
func (t *TSFile) Name() string {
	return t.name
}

// Duration returns the file duration.
func (t *TSFile) Duration() time.Duration {
	return t.maxPTS - t.minPTS
}

// FirstPacketWritten returns whether a packet ha been written into the file.
func (t *TSFile) FirstPacketWritten() bool {
	return t.firstPacketWritten
}

// SetPCR sets the PCR.
func (t *TSFile) SetPCR(pcr time.Duration) {
	t.pcr = pcr
}

// NewReader allocates a reader to read the file.
func (t *TSFile) NewReader() io.Reader {
	return t.buf.NewReader()
}

// WriteH264 writes H264 NALUs into the file.
func (t *TSFile) WriteH264(dts time.Duration, pts time.Duration, isIDR bool, nalus [][]byte) error {
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

// WriteAAC writes AAC AUs into the file.
func (t *TSFile) WriteAAC(sampleRate int, channelCount int, pts time.Duration, au []byte) error {
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
