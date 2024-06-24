// Package hls contains the HLS static source.
package hls

import (
	"net/http"
	"time"

	"github.com/bluenviron/gohlslib"
	"github.com/bluenviron/gohlslib/pkg/codecs"
	"github.com/bluenviron/gortsplib/v4/pkg/description"
	"github.com/bluenviron/gortsplib/v4/pkg/format"

	"github.com/bluenviron/mediamtx/internal/conf"
	"github.com/bluenviron/mediamtx/internal/defs"
	"github.com/bluenviron/mediamtx/internal/logger"
	"github.com/bluenviron/mediamtx/internal/protocols/tls"
	"github.com/bluenviron/mediamtx/internal/stream"
	"github.com/bluenviron/mediamtx/internal/unit"
)

// Source is a HLS static source.
type Source struct {
	ReadTimeout conf.StringDuration
	Parent      defs.StaticSourceParent
}

// Log implements logger.Writer.
func (s *Source) Log(level logger.Level, format string, args ...interface{}) {
	s.Parent.Log(level, "[HLS source] "+format, args...)
}

// Run implements StaticSource.
func (s *Source) Run(params defs.StaticSourceRunParams) error {
	var stream *stream.Stream

	defer func() {
		if stream != nil {
			s.Parent.SetNotReady(defs.PathSourceStaticSetNotReadyReq{})
		}
	}()

	decodeErrLogger := logger.NewLimitedLogger(s)

	tr := &http.Transport{
		TLSClientConfig: tls.ConfigForFingerprint(params.Conf.SourceFingerprint),
	}
	defer tr.CloseIdleConnections()

	var c *gohlslib.Client
	c = &gohlslib.Client{
		URI: params.ResolvedSource,
		HTTPClient: &http.Client{
			Timeout:   time.Duration(s.ReadTimeout),
			Transport: tr,
		},
		OnDownloadPrimaryPlaylist: func(u string) {
			s.Log(logger.Debug, "downloading primary playlist %v", u)
		},
		OnDownloadStreamPlaylist: func(u string) {
			s.Log(logger.Debug, "downloading stream playlist %v", u)
		},
		OnDownloadSegment: func(u string) {
			s.Log(logger.Debug, "downloading segment %v", u)
		},
		OnDownloadPart: func(u string) {
			s.Log(logger.Debug, "downloading part %v", u)
		},
		OnDecodeError: func(err error) {
			decodeErrLogger.Log(logger.Warn, err.Error())
		},
		OnTracks: func(tracks []*gohlslib.Track) error {
			var medias []*description.Media

			for _, track := range tracks {
				var medi *description.Media

				switch tcodec := track.Codec.(type) {
				case *codecs.AV1:
					medi = &description.Media{
						Type: description.MediaTypeVideo,
						Formats: []format.Format{&format.AV1{
							PayloadTyp: 96,
						}},
					}

					c.OnDataAV1(track, func(pts time.Duration, tu [][]byte) {
						stream.WriteUnit(medi, medi.Formats[0], &unit.AV1{
							Base: unit.Base{
								NTP: time.Now(),
								PTS: pts,
							},
							TU: tu,
						})
					})

				case *codecs.VP9:
					medi = &description.Media{
						Type: description.MediaTypeVideo,
						Formats: []format.Format{&format.VP9{
							PayloadTyp: 96,
						}},
					}

					c.OnDataVP9(track, func(pts time.Duration, frame []byte) {
						stream.WriteUnit(medi, medi.Formats[0], &unit.VP9{
							Base: unit.Base{
								NTP: time.Now(),
								PTS: pts,
							},
							Frame: frame,
						})
					})

				case *codecs.H264:
					medi = &description.Media{
						Type: description.MediaTypeVideo,
						Formats: []format.Format{&format.H264{
							PayloadTyp:        96,
							PacketizationMode: 1,
							SPS:               tcodec.SPS,
							PPS:               tcodec.PPS,
						}},
					}

					c.OnDataH26x(track, func(pts time.Duration, _ time.Duration, au [][]byte) {
						stream.WriteUnit(medi, medi.Formats[0], &unit.H264{
							Base: unit.Base{
								NTP: time.Now(),
								PTS: pts,
							},
							AU: au,
						})
					})

				case *codecs.H265:
					medi = &description.Media{
						Type: description.MediaTypeVideo,
						Formats: []format.Format{&format.H265{
							PayloadTyp: 96,
							VPS:        tcodec.VPS,
							SPS:        tcodec.SPS,
							PPS:        tcodec.PPS,
						}},
					}

					c.OnDataH26x(track, func(pts time.Duration, _ time.Duration, au [][]byte) {
						stream.WriteUnit(medi, medi.Formats[0], &unit.H265{
							Base: unit.Base{
								NTP: time.Now(),
								PTS: pts,
							},
							AU: au,
						})
					})

				case *codecs.MPEG4Audio:
					medi = &description.Media{
						Type: description.MediaTypeAudio,
						Formats: []format.Format{&format.MPEG4Audio{
							PayloadTyp:       96,
							SizeLength:       13,
							IndexLength:      3,
							IndexDeltaLength: 3,
							Config:           &tcodec.Config,
						}},
					}

					c.OnDataMPEG4Audio(track, func(pts time.Duration, aus [][]byte) {
						stream.WriteUnit(medi, medi.Formats[0], &unit.MPEG4Audio{
							Base: unit.Base{
								NTP: time.Now(),
								PTS: pts,
							},
							AUs: aus,
						})
					})

				case *codecs.Opus:
					medi = &description.Media{
						Type: description.MediaTypeAudio,
						Formats: []format.Format{&format.Opus{
							PayloadTyp:   96,
							ChannelCount: tcodec.ChannelCount,
						}},
					}

					c.OnDataOpus(track, func(pts time.Duration, packets [][]byte) {
						stream.WriteUnit(medi, medi.Formats[0], &unit.Opus{
							Base: unit.Base{
								NTP: time.Now(),
								PTS: pts,
							},
							Packets: packets,
						})
					})
				}

				medias = append(medias, medi)
			}

			res := s.Parent.SetReady(defs.PathSourceStaticSetReadyReq{
				Desc:               &description.Session{Medias: medias},
				GenerateRTPPackets: true,
			})
			if res.Err != nil {
				return res.Err
			}

			stream = res.Stream

			return nil
		},
	}

	err := c.Start()
	if err != nil {
		return err
	}

	for {
		select {
		case err := <-c.Wait():
			c.Close()
			return err

		case <-params.ReloadConf:

		case <-params.Context.Done():
			c.Close()
			<-c.Wait()
			return nil
		}
	}
}

// APISourceDescribe implements StaticSource.
func (*Source) APISourceDescribe() defs.APIPathSourceOrReader {
	return defs.APIPathSourceOrReader{
		Type: "hlsSource",
		ID:   "",
	}
}
