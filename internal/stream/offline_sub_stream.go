package stream

import (
	"bytes"
	_ "embed"
	"time"

	"github.com/bluenviron/gortsplib/v5/pkg/description"
	"github.com/bluenviron/gortsplib/v5/pkg/format"
	"github.com/bluenviron/mediacommon/v2/pkg/codecs/av1"
	"github.com/bluenviron/mediacommon/v2/pkg/codecs/h264"
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

func multiplyAndDivide2(v, m, d time.Duration) time.Duration {
	secs := v / d
	dec := v % d
	return (secs*m + dec*m/d)
}

type offlineSubStream struct {
	stream *Stream

	subStream  *SubStream
	pts        int64
	systemTime time.Time

	terminate chan struct{}
}

func (o *offlineSubStream) initialize() error {
	o.subStream = &SubStream{
		Stream:        o.stream,
		CurDesc:       o.stream.Desc,
		UseRTPPackets: false,
	}
	err := o.subStream.Initialize()
	if err != nil {
		return err
	}

	o.systemTime = time.Now()

	o.terminate = make(chan struct{})
	go o.run()

	return nil
}

func (o *offlineSubStream) close() {
	close(o.terminate)
}

func (o *offlineSubStream) run() {
	for _, media := range o.subStream.CurDesc.Medias {
		for _, forma := range media.Formats {
			go func(media *description.Media, forma format.Format) {
				for {
					ok := o.runLoop(media, forma)
					if !ok {
						return
					}
				}
			}(media, forma)
		}
	}
}

func (o *offlineSubStream) runLoop(media *description.Media, forma format.Format) bool {
	var buf []byte

	switch forma.(type) {
	case *format.AV1:
		buf = offlineAV1

	case *format.VP9:
		buf = offlineVP9

	case *format.H265:
		buf = offlineH265

	case *format.H264:
		buf = offlineH264
	}

	var presentation pmp4.Presentation
	err := presentation.Unmarshal(bytes.NewReader(buf))
	if err != nil {
		panic(err)
	}

	track := presentation.Tracks[0]

	switch codec := track.Codec.(type) {
	case *mcodecs.H265:
		o.subStream.WriteUnit(media, forma, &unit.Unit{
			PTS:     o.pts,
			NTP:     time.Time{},
			Payload: unit.PayloadH265([][]byte{codec.SPS, codec.PPS, codec.VPS}),
		})

	case *mcodecs.H264:
		o.subStream.WriteUnit(media, forma, &unit.Unit{
			PTS:     o.pts,
			NTP:     time.Time{},
			Payload: unit.PayloadH264([][]byte{codec.SPS, codec.PPS}),
		})
	}

	for _, sample := range track.Samples {
		buf, err := sample.GetPayload()
		if err != nil {
			panic(err)
		}

		switch track.Codec.(type) {
		case *mcodecs.AV1:
			var bs av1.Bitstream
			err = bs.Unmarshal(buf)
			if err != nil {
				panic(err)
			}

			o.subStream.WriteUnit(media, forma, &unit.Unit{
				PTS:     o.pts,
				NTP:     time.Time{},
				Payload: unit.PayloadAV1(bs),
			})

		case *mcodecs.VP9:
			o.subStream.WriteUnit(media, forma, &unit.Unit{
				PTS:     o.pts,
				NTP:     time.Time{},
				Payload: unit.PayloadVP9(buf),
			})

		case *mcodecs.H265:
			var avcc h264.AVCC
			err = avcc.Unmarshal(buf)
			if err != nil {
				panic(err)
			}

			o.subStream.WriteUnit(media, forma, &unit.Unit{
				PTS:     o.pts,
				NTP:     time.Time{},
				Payload: unit.PayloadH265(avcc),
			})

		case *mcodecs.H264:
			var avcc h264.AVCC
			err = avcc.Unmarshal(buf)
			if err != nil {
				panic(err)
			}

			o.subStream.WriteUnit(media, forma, &unit.Unit{
				PTS:     o.pts,
				NTP:     time.Time{},
				Payload: unit.PayloadH264(avcc),
			})
		}

		o.pts += multiplyAndDivide(int64(sample.Duration), int64(forma.ClockRate()), int64(track.TimeScale))
		durationGo := multiplyAndDivide2(time.Duration(sample.Duration), time.Second, time.Duration(track.TimeScale))
		o.systemTime = o.systemTime.Add(durationGo)

		select {
		case <-time.After(time.Until(o.systemTime)):
		case <-o.terminate:
			return false
		}
	}

	return true
}
