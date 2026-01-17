package stream

import (
	"bytes"
	"context"
	_ "embed"
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

//go:embed offline_h264.mp4
var offlineH264 []byte

type offlineSubStreamTrack struct {
	ctx       context.Context
	subStream *SubStream
	media     *description.Media
	format    format.Format
}

func (t *offlineSubStreamTrack) initialize() {
	go t.run()
}

func (t *offlineSubStreamTrack) run() {
	const audioWritesPerSecond = 10
	var pts int64
	systemTime := time.Now()

	switch forma := t.format.(type) {
	case *format.Opus:
		unitsPerWrite := (forma.ClockRate() / 960) / audioWritesPerSecond
		writeDuration := 960 * int64(unitsPerWrite)
		writeDurationGo := multiplyAndDivide2(time.Duration(writeDuration), time.Second, 48000)

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
			systemTime = systemTime.Add(writeDurationGo)

			select {
			case <-time.After(time.Until(systemTime)):
			case <-t.ctx.Done():
				return
			}
		}

	case *format.MPEG4Audio:
		unitsPerWrite := (forma.ClockRate() / mpeg4audio.SamplesPerAccessUnit) / audioWritesPerSecond
		writeDuration := mpeg4audio.SamplesPerAccessUnit * int64(unitsPerWrite)
		writeDurationGo := multiplyAndDivide2(time.Duration(writeDuration), time.Second, time.Duration(forma.ClockRate()))

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
			systemTime = systemTime.Add(writeDurationGo)

			select {
			case <-time.After(time.Until(systemTime)):
			case <-t.ctx.Done():
				return
			}
		}

	case *format.G711:
		samplesPerWrite := forma.ClockRate() / audioWritesPerSecond
		writeDuration := samplesPerWrite
		writeDurationGo := multiplyAndDivide2(time.Duration(writeDuration), time.Second, time.Duration(forma.ClockRate()))

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
			systemTime = systemTime.Add(writeDurationGo)

			select {
			case <-time.After(time.Until(systemTime)):
			case <-t.ctx.Done():
				return
			}
		}

	case *format.LPCM:
		samplesPerWrite := forma.ClockRate() / audioWritesPerSecond
		writeDuration := samplesPerWrite
		writeDurationGo := multiplyAndDivide2(time.Duration(writeDuration), time.Second, time.Duration(forma.ClockRate()))

		for {
			payload := make(unit.PayloadLPCM, samplesPerWrite*forma.ChannelCount*(forma.BitDepth/8))

			t.subStream.WriteUnit(t.media, t.format, &unit.Unit{
				PTS:     pts,
				NTP:     time.Time{},
				Payload: payload,
			})

			pts += int64(writeDuration)
			systemTime = systemTime.Add(writeDurationGo)

			select {
			case <-time.After(time.Until(systemTime)):
			case <-t.ctx.Done():
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

		t.runFile(pts, systemTime, buf)
	}
}

func (t *offlineSubStreamTrack) runFile(pts int64, systemTime time.Time, buf []byte) {
	var presentation pmp4.Presentation
	err := presentation.Unmarshal(bytes.NewReader(buf))
	if err != nil {
		panic(err)
	}

	track := presentation.Tracks[0]

	for {
		switch codec := track.Codec.(type) {
		case *mcodecs.H265:
			t.subStream.WriteUnit(t.media, t.format, &unit.Unit{
				PTS:     pts,
				NTP:     time.Time{},
				Payload: unit.PayloadH265([][]byte{codec.SPS, codec.PPS, codec.VPS}),
			})

		case *mcodecs.H264:
			t.subStream.WriteUnit(t.media, t.format, &unit.Unit{
				PTS:     pts,
				NTP:     time.Time{},
				Payload: unit.PayloadH264([][]byte{codec.SPS, codec.PPS}),
			})
		}

		for _, sample := range track.Samples {
			var payload []byte
			payload, err = sample.GetPayload()
			if err != nil {
				panic(err)
			}

			switch track.Codec.(type) {
			case *mcodecs.AV1:
				var bs av1.Bitstream
				err = bs.Unmarshal(payload)
				if err != nil {
					panic(err)
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
					panic(err)
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
					panic(err)
				}

				t.subStream.WriteUnit(t.media, t.format, &unit.Unit{
					PTS:     pts,
					NTP:     time.Time{},
					Payload: unit.PayloadH264(avcc),
				})
			}

			pts += multiplyAndDivide(int64(sample.Duration), int64(t.format.ClockRate()), int64(track.TimeScale))
			durationGo := multiplyAndDivide2(time.Duration(sample.Duration), time.Second, time.Duration(track.TimeScale))
			systemTime = systemTime.Add(durationGo)

			select {
			case <-time.After(time.Until(systemTime)):
			case <-t.ctx.Done():
				return
			}
		}
	}
}
