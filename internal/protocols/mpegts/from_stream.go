package mpegts

import (
	"bufio"
	"fmt"
	"slices"
	"time"

	"github.com/bluenviron/gortsplib/v5/pkg/description"
	"github.com/bluenviron/gortsplib/v5/pkg/format"
	"github.com/bluenviron/mediacommon/v2/pkg/codecs/ac3"
	"github.com/bluenviron/mediacommon/v2/pkg/codecs/h264"
	"github.com/bluenviron/mediacommon/v2/pkg/codecs/h265"
	"github.com/bluenviron/mediacommon/v2/pkg/codecs/mpeg4audio"
	mcmpegts "github.com/bluenviron/mediacommon/v2/pkg/formats/mpegts"
	tscodecs "github.com/bluenviron/mediacommon/v2/pkg/formats/mpegts/codecs"
	srt "github.com/datarhei/gosrt"

	"github.com/bluenviron/mediamtx/internal/logger"
	"github.com/bluenviron/mediamtx/internal/stream"
	"github.com/bluenviron/mediamtx/internal/unit"
)

func multiplyAndDivide(v, m, d int64) int64 {
	secs := v / d
	dec := v % d
	return (secs*m + dec*m/d)
}

// FromStream maps a MediaMTX stream to a MPEG-TS writer.
func FromStream(
	desc *description.Session,
	r *stream.Reader,
	bw *bufio.Writer,
	sconn srt.Conn,
	writeTimeout time.Duration,
) error {
	var w *mcmpegts.Writer
	var tracks []*mcmpegts.Track

	addTrack := func(
		media *description.Media,
		forma format.Format,
		track *mcmpegts.Track,
		onData stream.OnDataFunc,
	) {
		tracks = append(tracks, track)
		r.OnData(media, forma, onData)
	}

	for _, media := range desc.Medias {
		for _, forma := range media.Formats {
			clockRate := forma.ClockRate()

			switch forma := forma.(type) {
			case *format.H265: //nolint:dupl
				track := &mcmpegts.Track{Codec: &tscodecs.H265{}}

				var dtsExtractor *h265.DTSExtractor

				addTrack(
					media,
					forma,
					track,
					func(u *unit.Unit) error {
						if u.NilPayload() {
							return nil
						}

						if dtsExtractor == nil {
							if !h265.IsRandomAccess(u.Payload.(unit.PayloadH265)) {
								return nil
							}
							dtsExtractor = &h265.DTSExtractor{}
							dtsExtractor.Initialize()
						}

						dts, err := dtsExtractor.Extract(u.Payload.(unit.PayloadH265), u.PTS)
						if err != nil {
							return err
						}

						sconn.SetWriteDeadline(time.Now().Add(writeTimeout))
						err = (*w).WriteH265(
							track,
							u.PTS, // no conversion is needed since clock rate is 90khz in both MPEG-TS and RTSP
							dts,
							u.Payload.(unit.PayloadH265))
						if err != nil {
							return err
						}
						return bw.Flush()
					})

			case *format.H264: //nolint:dupl
				track := &mcmpegts.Track{Codec: &tscodecs.H264{}}

				var dtsExtractor *h264.DTSExtractor

				addTrack(
					media,
					forma,
					track,
					func(u *unit.Unit) error {
						if u.NilPayload() {
							return nil
						}

						idrPresent := h264.IsRandomAccess(u.Payload.(unit.PayloadH264))

						if dtsExtractor == nil {
							if !idrPresent {
								return nil
							}
							dtsExtractor = &h264.DTSExtractor{}
							dtsExtractor.Initialize()
						}

						dts, err := dtsExtractor.Extract(u.Payload.(unit.PayloadH264), u.PTS)
						if err != nil {
							return err
						}

						sconn.SetWriteDeadline(time.Now().Add(writeTimeout))
						err = (*w).WriteH264(
							track,
							u.PTS, // no conversion is needed since clock rate is 90khz in both MPEG-TS and RTSP
							dts,
							u.Payload.(unit.PayloadH264))
						if err != nil {
							return err
						}
						return bw.Flush()
					})

			case *format.MPEG4Video:
				track := &mcmpegts.Track{Codec: &tscodecs.MPEG4Video{}}

				firstReceived := false
				var lastPTS int64

				addTrack(
					media,
					forma,
					track,
					func(u *unit.Unit) error {
						if u.NilPayload() {
							return nil
						}

						if !firstReceived {
							firstReceived = true
						} else if u.PTS < lastPTS {
							return fmt.Errorf("MPEG-4 Video streams with B-frames are not supported (yet)")
						}
						lastPTS = u.PTS

						sconn.SetWriteDeadline(time.Now().Add(writeTimeout))
						err := (*w).WriteMPEG4Video(
							track,
							u.PTS, // no conversion is needed since clock rate is 90khz in both MPEG-TS and RTSP
							u.Payload.(unit.PayloadMPEG4Video))
						if err != nil {
							return err
						}
						return bw.Flush()
					})

			case *format.MPEG1Video:
				track := &mcmpegts.Track{Codec: &tscodecs.MPEG1Video{}}

				firstReceived := false
				var lastPTS int64

				addTrack(
					media,
					forma,
					track,
					func(u *unit.Unit) error {
						if u.NilPayload() {
							return nil
						}

						if !firstReceived {
							firstReceived = true
						} else if u.PTS < lastPTS {
							return fmt.Errorf("MPEG-1 Video streams with B-frames are not supported (yet)")
						}
						lastPTS = u.PTS

						sconn.SetWriteDeadline(time.Now().Add(writeTimeout))
						err := (*w).WriteMPEG1Video(
							track,
							u.PTS, // no conversion is needed since clock rate is 90khz in both MPEG-TS and RTSP
							u.Payload.(unit.PayloadMPEG1Video))
						if err != nil {
							return err
						}
						return bw.Flush()
					})

			case *format.Opus:
				track := &mcmpegts.Track{Codec: &tscodecs.Opus{
					ChannelCount: forma.ChannelCount,
				}}

				addTrack(
					media,
					forma,
					track,
					func(u *unit.Unit) error {
						if u.NilPayload() {
							return nil
						}

						sconn.SetWriteDeadline(time.Now().Add(writeTimeout))
						err := (*w).WriteOpus(
							track,
							multiplyAndDivide(u.PTS, 90000, int64(clockRate)),
							u.Payload.(unit.PayloadOpus))
						if err != nil {
							return err
						}
						return bw.Flush()
					})

			case *format.KLV:
				track := &mcmpegts.Track{
					Codec: &tscodecs.KLV{
						Synchronous: true,
					},
				}

				addTrack(
					media,
					forma,
					track,
					func(u *unit.Unit) error {
						if u.NilPayload() {
							return nil
						}

						sconn.SetWriteDeadline(time.Now().Add(writeTimeout))
						err := (*w).WriteKLV(track, multiplyAndDivide(u.PTS, 90000, 90000), u.Payload.(unit.PayloadKLV))
						if err != nil {
							return err
						}
						return bw.Flush()
					})

			case *format.MPEG4Audio:
				track := &mcmpegts.Track{Codec: &tscodecs.MPEG4Audio{
					Config: *forma.Config,
				}}

				addTrack(
					media,
					forma,
					track,
					func(u *unit.Unit) error {
						if u.NilPayload() {
							return nil
						}

						sconn.SetWriteDeadline(time.Now().Add(writeTimeout))
						err := (*w).WriteMPEG4Audio(
							track,
							multiplyAndDivide(u.PTS, 90000, int64(clockRate)),
							u.Payload.(unit.PayloadMPEG4Audio))
						if err != nil {
							return err
						}
						return bw.Flush()
					})

			case *format.MPEG4AudioLATM:
				track := &mcmpegts.Track{Codec: &tscodecs.MPEG4AudioLATM{}}

				if !forma.CPresent {
					addTrack(
						media,
						forma,
						track,
						func(u *unit.Unit) error {
							if u.NilPayload() {
								return nil
							}

							var elIn mpeg4audio.AudioMuxElement
							elIn.MuxConfigPresent = false
							elIn.StreamMuxConfig = forma.StreamMuxConfig
							err := elIn.Unmarshal(u.Payload.(unit.PayloadMPEG4AudioLATM))
							if err != nil {
								return err
							}

							var elOut mpeg4audio.AudioMuxElement
							elOut.MuxConfigPresent = true
							elOut.StreamMuxConfig = forma.StreamMuxConfig
							elOut.UseSameStreamMux = false
							elOut.Payloads = elIn.Payloads
							buf, err := elOut.Marshal()
							if err != nil {
								return err
							}

							sconn.SetWriteDeadline(time.Now().Add(writeTimeout))
							err = (*w).WriteMPEG4AudioLATM(
								track,
								multiplyAndDivide(u.PTS, 90000, int64(clockRate)),
								[][]byte{buf})
							if err != nil {
								return err
							}
							return bw.Flush()
						})
				} else {
					addTrack(
						media,
						forma,
						track,
						func(u *unit.Unit) error {
							if u.NilPayload() {
								return nil
							}

							sconn.SetWriteDeadline(time.Now().Add(writeTimeout))
							err := (*w).WriteMPEG4AudioLATM(
								track,
								multiplyAndDivide(u.PTS, 90000, int64(clockRate)),
								[][]byte{u.Payload.(unit.PayloadMPEG4AudioLATM)})
							if err != nil {
								return err
							}
							return bw.Flush()
						})
				}

			case *format.MPEG1Audio:
				track := &mcmpegts.Track{Codec: &tscodecs.MPEG1Audio{}}

				addTrack(
					media,
					forma,
					track,
					func(u *unit.Unit) error {
						if u.NilPayload() {
							return nil
						}

						sconn.SetWriteDeadline(time.Now().Add(writeTimeout))
						err := (*w).WriteMPEG1Audio(
							track,
							u.PTS, // no conversion is needed since clock rate is 90khz in both MPEG-TS and RTSP
							u.Payload.(unit.PayloadMPEG1Audio))
						if err != nil {
							return err
						}
						return bw.Flush()
					})

			case *format.AC3:
				track := &mcmpegts.Track{Codec: &tscodecs.AC3{}}

				addTrack(
					media,
					forma,
					track,
					func(u *unit.Unit) error {
						if u.NilPayload() {
							return nil
						}

						for i, frame := range u.Payload.(unit.PayloadAC3) {
							framePTS := u.PTS + int64(i)*ac3.SamplesPerFrame

							sconn.SetWriteDeadline(time.Now().Add(writeTimeout))
							err := (*w).WriteAC3(
								track,
								multiplyAndDivide(framePTS, 90000, int64(clockRate)),
								frame)
							if err != nil {
								return err
							}
						}
						return bw.Flush()
					})
			}
		}
	}

	if len(tracks) == 0 {
		return errNoSupportedCodecs
	}

	setuppedFormats := r.Formats()

	n := 1
	for _, medi := range desc.Medias {
		for _, forma := range medi.Formats {
			if !slices.Contains(setuppedFormats, forma) {
				r.Parent.Log(logger.Warn, "skipping track %d (%s)", n, forma.Codec())
			}
			n++
		}
	}

	w = &mcmpegts.Writer{W: bw, Tracks: tracks}
	return w.Initialize()
}
