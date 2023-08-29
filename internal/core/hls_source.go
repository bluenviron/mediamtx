package core

import (
	"context"
	"net/http"
	"time"

	"github.com/bluenviron/gohlslib"
	"github.com/bluenviron/gohlslib/pkg/codecs"
	"github.com/bluenviron/gortsplib/v4/pkg/description"
	"github.com/bluenviron/gortsplib/v4/pkg/format"

	"github.com/bluenviron/mediamtx/internal/conf"
	"github.com/bluenviron/mediamtx/internal/logger"
	"github.com/bluenviron/mediamtx/internal/stream"
	"github.com/bluenviron/mediamtx/internal/unit"
)

type hlsSourceParent interface {
	logger.Writer
	setReady(req pathSourceStaticSetReadyReq) pathSourceStaticSetReadyRes
	setNotReady(req pathSourceStaticSetNotReadyReq)
}

type hlsSource struct {
	parent hlsSourceParent
}

func newHLSSource(
	parent hlsSourceParent,
) *hlsSource {
	return &hlsSource{
		parent: parent,
	}
}

func (s *hlsSource) Log(level logger.Level, format string, args ...interface{}) {
	s.parent.Log(level, "[HLS source] "+format, args...)
}

// run implements sourceStaticImpl.
func (s *hlsSource) run(ctx context.Context, cnf *conf.PathConf, reloadConf chan *conf.PathConf) error {
	var stream *stream.Stream

	defer func() {
		if stream != nil {
			s.parent.setNotReady(pathSourceStaticSetNotReadyReq{})
		}
	}()

	decodeErrLogger := newLimitedLogger(s)

	var c *gohlslib.Client
	c = &gohlslib.Client{
		URI: cnf.Source,
		HTTPClient: &http.Client{
			Transport: &http.Transport{
				TLSClientConfig: tlsConfigForFingerprint(cnf.SourceFingerprint),
			},
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

					c.OnDataH26x(track, func(pts time.Duration, dts time.Duration, au [][]byte) {
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

					c.OnDataH26x(track, func(pts time.Duration, dts time.Duration, au [][]byte) {
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
						stream.WriteUnit(medi, medi.Formats[0], &unit.MPEG4AudioGeneric{
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
							PayloadTyp: 96,
							IsStereo:   (tcodec.ChannelCount == 2),
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

			res := s.parent.setReady(pathSourceStaticSetReadyReq{
				desc:               &description.Session{Medias: medias},
				generateRTPPackets: true,
			})
			if res.err != nil {
				return res.err
			}

			stream = res.stream

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

		case <-reloadConf:

		case <-ctx.Done():
			c.Close()
			<-c.Wait()
			return nil
		}
	}
}

// apiSourceDescribe implements sourceStaticImpl.
func (*hlsSource) apiSourceDescribe() pathAPISourceOrReader {
	return pathAPISourceOrReader{
		Type: "hlsSource",
		ID:   "",
	}
}
