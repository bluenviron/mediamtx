package stream

import (
	"bytes"
	_ "embed"
	"time"

	"github.com/bluenviron/mediacommon/v2/pkg/codecs/h264"
	mcodecs "github.com/bluenviron/mediacommon/v2/pkg/formats/mp4/codecs"
	"github.com/bluenviron/mediacommon/v2/pkg/formats/pmp4"
	"github.com/bluenviron/mediamtx/internal/unit"
)

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
	// <-o.terminate

	for {
		ok := o.runLoop()
		if !ok {
			return
		}
	}
}

func (o *offlineSubStream) runLoop() bool {
	var presentation pmp4.Presentation
	err := presentation.Unmarshal(bytes.NewReader(offlineH264))
	if err != nil {
		panic(err)
	}

	track := presentation.Tracks[0]
	codecH264 := track.Codec.(*mcodecs.H264)
	media := o.subStream.CurDesc.Medias[0]
	forma := o.subStream.CurDesc.Medias[0].Formats[0]

	o.subStream.WriteUnit(media, forma, &unit.Unit{
		PTS:        o.pts,
		NTP:        time.Time{},
		RTPPackets: nil,
		Payload:    unit.PayloadH264([][]byte{codecH264.SPS, codecH264.PPS}),
	})

	for _, sample := range track.Samples {
		buf, err := sample.GetPayload()
		if err != nil {
			panic(err)
		}

		var avcc h264.AVCC
		err = avcc.Unmarshal(buf)
		if err != nil {
			panic(err)
		}

		o.subStream.WriteUnit(media, forma, &unit.Unit{
			PTS:        o.pts,
			NTP:        time.Time{},
			RTPPackets: nil,
			Payload:    unit.PayloadH264(avcc),
		})

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
