package stream

import (
	"bytes"
	"context"
	_ "embed"
	"io"
	"os"
	"sync"
	"time"

	"github.com/bluenviron/gortsplib/v5/pkg/description"
	"github.com/bluenviron/gortsplib/v5/pkg/format"
	"github.com/bluenviron/mediacommon/v2/pkg/codecs/av1"
	"github.com/bluenviron/mediacommon/v2/pkg/codecs/h264"
	"github.com/bluenviron/mediacommon/v2/pkg/codecs/mpeg4audio"
	mcodecs "github.com/bluenviron/mediacommon/v2/pkg/formats/mp4/codecs"
	"github.com/bluenviron/mediacommon/v2/pkg/formats/pmp4"
	"github.com/bluenviron/mediamtx/internal/unit"
)

//go:embed offline_av1.mp4
var offlineAV1 []byte

//go:embed offline_vp9.mp4
var offlineVP9 []byte

//go:embed offline_h265.mp4
var offlineH265 []byte

var offlineH265SPS = []byte{
	0x42, 0x01, 0x01, 0x01, 0x60, 0x00, 0x00, 0x03,
	0x00, 0x90, 0x00, 0x00, 0x03, 0x00, 0x00, 0x03,
	0x00, 0x78, 0xa0, 0x03, 0xc0, 0x80, 0x11, 0x07,
	0xcb, 0x96, 0xe9, 0x29, 0x30, 0xbc, 0x05, 0xa0,
	0x20, 0x00, 0x00, 0x03, 0x00, 0x20, 0x00, 0x00,
	0x03, 0x03, 0xc1,
}

var offlineH265PPS = []byte{
	0x44, 0x01, 0xc0, 0x73, 0xc1, 0x89,
}

var offlineH265VPS = []byte{
	0x40, 0x01, 0x0c, 0x01, 0xff, 0xff, 0x01, 0x60,
	0x00, 0x00, 0x03, 0x00, 0x90, 0x00, 0x00, 0x03,
	0x00, 0x00, 0x03, 0x00, 0x78, 0xba, 0x02, 0x40,
}

//go:embed offline_h264.mp4
var offlineH264 []byte

var offlineH264SPS = []byte{
	0x67, 0x42, 0xc0, 0x28, 0xda, 0x01, 0xe0, 0x08,
	0x9f, 0x97, 0x01, 0x10, 0x00, 0x00, 0x03, 0x00,
	0x10, 0x00, 0x00, 0x03, 0x03, 0xc0, 0xf1, 0x83,
	0x2a,
}

var offlineH264PPS = []byte{
	0x68, 0xce, 0x3c, 0x80,
}

type offlineSubStreamTrack struct {
	wg             *sync.WaitGroup
	file           string
	pos            int
	ctx            context.Context
	subStream      *SubStream
	media          *description.Media
	format         format.Format
	waitLastSample bool
}

func (t *offlineSubStreamTrack) initialize() {
	t.wg.Add(1)
	go t.run()
}

func (t *offlineSubStreamTrack) run() {
	defer t.wg.Done()

	if t.file != "" {
		f, err := os.Open(t.file)
		if err != nil {
			panic(err)
		}
		defer f.Close()

		err = t.runFile(f, t.pos)
		if err != nil {
			panic(err)
		}
		return
	}

	const audioWritesPerSecond = 10
	var pts int64
	startSystemTime := time.Now()

	switch forma := t.format.(type) {
	case *format.Opus:
		unitsPerWrite := (forma.ClockRate() / 960) / audioWritesPerSecond
		writeDuration := 960 * int64(unitsPerWrite)

		for {
			payload := make(unit.PayloadOpus, unitsPerWrite)
			for i := range payload {
				payload[i] = []byte{0xF8, 0xFF, 0xFE} // DTX frame
			}

			t.subStream.WriteUnit(t.media, t.format, &unit.Unit{
				PTS:     pts,
				NTP:     time.Time{},
				Payload: payload,
			})

			pts += writeDuration

			ptsGo := multiplyAndDivide2(time.Duration(pts), time.Second, 48000)
			systemTime := startSystemTime.Add(ptsGo)

			if !t.sleep(systemTime) {
				return
			}
		}

	case *format.MPEG4Audio:
		unitsPerWrite := (forma.ClockRate() / mpeg4audio.SamplesPerAccessUnit) / audioWritesPerSecond
		writeDuration := mpeg4audio.SamplesPerAccessUnit * int64(unitsPerWrite)

		for {
			var frame []byte
			switch forma.Config.ChannelConfig {
			case 1:
				frame = []byte{0x01, 0x18, 0x20, 0x07}

			default:
				frame = []byte{0x21, 0x10, 0x04, 0x60, 0x8c, 0x1c}
			}

			payload := make(unit.PayloadMPEG4Audio, unitsPerWrite)
			for i := range payload {
				payload[i] = frame
			}

			t.subStream.WriteUnit(t.media, t.format, &unit.Unit{
				PTS:     pts,
				NTP:     time.Time{},
				Payload: payload,
			})

			pts += writeDuration

			ptsGo := multiplyAndDivide2(time.Duration(pts), time.Second, time.Duration(forma.ClockRate()))
			systemTime := startSystemTime.Add(ptsGo)

			if !t.sleep(systemTime) {
				return
			}
		}

	case *format.G711:
		samplesPerWrite := forma.ClockRate() / audioWritesPerSecond
		writeDuration := samplesPerWrite

		for {
			var sample byte
			if forma.MULaw {
				sample = 0xFF
			} else {
				sample = 0xD5
			}

			payload := make(unit.PayloadG711, samplesPerWrite*forma.ChannelCount)
			for i := range payload {
				payload[i] = sample
			}

			t.subStream.WriteUnit(t.media, t.format, &unit.Unit{
				PTS:     pts,
				NTP:     time.Time{},
				Payload: payload,
			})

			pts += int64(writeDuration)

			ptsGo := multiplyAndDivide2(time.Duration(pts), time.Second, time.Duration(forma.ClockRate()))
			systemTime := startSystemTime.Add(ptsGo)

			if !t.sleep(systemTime) {
				return
			}
		}

	case *format.LPCM:
		samplesPerWrite := forma.ClockRate() / audioWritesPerSecond
		writeDuration := samplesPerWrite

		for {
			payload := make(unit.PayloadLPCM, samplesPerWrite*forma.ChannelCount*(forma.BitDepth/8))

			t.subStream.WriteUnit(t.media, t.format, &unit.Unit{
				PTS:     pts,
				NTP:     time.Time{},
				Payload: payload,
			})

			pts += int64(writeDuration)

			ptsGo := multiplyAndDivide2(time.Duration(pts), time.Second, time.Duration(forma.ClockRate()))
			systemTime := startSystemTime.Add(ptsGo)

			if !t.sleep(systemTime) {
				return
			}
		}

	default:
		var buf []byte

		switch t.format.(type) {
		case *format.AV1:
			buf = offlineAV1

		case *format.VP9:
			buf = offlineVP9

		case *format.H265:
			buf = offlineH265

		case *format.H264:
			buf = offlineH264

		default:
			panic("should not happen")
		}

		r := bytes.NewReader(buf)

		err := t.runFile(r, 0)
		if err != nil {
			panic(err)
		}
	}
}

func (t *offlineSubStreamTrack) runFile(r io.ReadSeeker, pos int) error {
	var presentation pmp4.Presentation
	err := presentation.Unmarshal(r)
	if err != nil {
		return err
	}

	track := presentation.Tracks[pos]
	var pts int64
	startSystemTime := time.Now()

	for {
		for _, sample := range track.Samples {
			var payload []byte
			payload, err = sample.GetPayload()
			if err != nil {
				return err
			}

			switch track.Codec.(type) {
			case *mcodecs.AV1:
				var bs av1.Bitstream
				err = bs.Unmarshal(payload)
				if err != nil {
					return err
				}

				t.subStream.WriteUnit(t.media, t.format, &unit.Unit{
					PTS:     pts,
					NTP:     time.Time{},
					Payload: unit.PayloadAV1(bs),
				})

			case *mcodecs.VP9:
				t.subStream.WriteUnit(t.media, t.format, &unit.Unit{
					PTS:     pts,
					NTP:     time.Time{},
					Payload: unit.PayloadVP9(payload),
				})

			case *mcodecs.H265:
				var avcc h264.AVCC
				err = avcc.Unmarshal(payload)
				if err != nil {
					return err
				}

				t.subStream.WriteUnit(t.media, t.format, &unit.Unit{
					PTS:     pts,
					NTP:     time.Time{},
					Payload: unit.PayloadH265(avcc),
				})

			case *mcodecs.H264:
				var avcc h264.AVCC
				err = avcc.Unmarshal(payload)
				if err != nil {
					return err
				}

				t.subStream.WriteUnit(t.media, t.format, &unit.Unit{
					PTS:     pts,
					NTP:     time.Time{},
					Payload: unit.PayloadH264(avcc),
				})
			}

			pts += multiplyAndDivide(int64(sample.Duration)+int64(sample.PTSOffset),
				int64(t.format.ClockRate()), int64(track.TimeScale))

			ptsGo := multiplyAndDivide2(time.Duration(pts), time.Second, time.Duration(t.format.ClockRate()))
			systemTime := startSystemTime.Add(ptsGo)

			if !t.sleep(systemTime) {
				return nil
			}
		}
	}
}

func (t *offlineSubStreamTrack) sleep(systemTime time.Time) bool {
	select {
	case <-time.After(time.Until(systemTime)):
	case <-t.ctx.Done():
		if t.waitLastSample {
			time.Sleep(time.Until(systemTime))
		}
		return false
	}
	return true
}
