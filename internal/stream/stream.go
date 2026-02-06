// Package stream contains the Stream object.
package stream

import (
	"os"
	"sync"
	"sync/atomic"
	"time"

	"github.com/bluenviron/gortsplib/v5"
	"github.com/bluenviron/gortsplib/v5/pkg/description"
	"github.com/bluenviron/gortsplib/v5/pkg/format"
	"github.com/bluenviron/mediacommon/v2/pkg/codecs/mpeg4audio"
	"github.com/bluenviron/mediacommon/v2/pkg/formats/mp4/codecs"
	"github.com/bluenviron/mediacommon/v2/pkg/formats/pmp4"
	"github.com/pion/rtp"

	"github.com/bluenviron/mediamtx/internal/conf"
	"github.com/bluenviron/mediamtx/internal/errordumper"
	"github.com/bluenviron/mediamtx/internal/logger"
)

func mediasFromAlwaysAvailableFile(alwaysAvailableFile string) ([]*description.Media, error) {
	f, err := os.Open(alwaysAvailableFile)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	var presentation pmp4.Presentation
	err = presentation.Unmarshal(f)
	if err != nil {
		return nil, err
	}

	var medias []*description.Media

	for _, track := range presentation.Tracks {
		switch codec := track.Codec.(type) {
		case *codecs.AV1:
			medias = append(medias, &description.Media{
				Type: description.MediaTypeVideo,
				Formats: []format.Format{&format.AV1{
					PayloadTyp: 96,
				}},
			})

		case *codecs.VP9:
			medias = append(medias, &description.Media{
				Type: description.MediaTypeVideo,
				Formats: []format.Format{&format.VP9{
					PayloadTyp: 96,
				}},
			})

		case *codecs.H265:
			medias = append(medias, &description.Media{
				Type: description.MediaTypeVideo,
				Formats: []format.Format{&format.H265{
					PayloadTyp: 96,
				}},
			})

		case *codecs.H264:
			medias = append(medias, &description.Media{
				Type: description.MediaTypeVideo,
				Formats: []format.Format{&format.H264{
					PayloadTyp:        96,
					PacketizationMode: 1,
				}},
			})

		case *codecs.Opus:
			medias = append(medias, &description.Media{
				Type: description.MediaTypeAudio,
				Formats: []format.Format{&format.Opus{
					PayloadTyp:   96,
					ChannelCount: 2,
				}},
			})

		case *codecs.MPEG4Audio:
			medias = append(medias, &description.Media{
				Type: description.MediaTypeAudio,
				Formats: []format.Format{&format.MPEG4Audio{
					PayloadTyp:       96,
					SizeLength:       13,
					IndexLength:      3,
					IndexDeltaLength: 3,
					Config: &mpeg4audio.AudioSpecificConfig{
						Type:          mpeg4audio.ObjectTypeAACLC,
						SampleRate:    codec.Config.SampleRate,
						ChannelConfig: codec.Config.ChannelConfig,
					},
				}},
			})

		case *codecs.LPCM:
			medias = append(medias, &description.Media{
				Type: description.MediaTypeAudio,
				Formats: []format.Format{&format.LPCM{
					PayloadTyp:   96,
					BitDepth:     codec.BitDepth,
					SampleRate:   codec.SampleRate,
					ChannelCount: codec.ChannelCount,
				}},
			})
		}
	}

	return medias, nil
}

func mediasFromAlwaysAvailableTracks(alwaysAvailableTracks []conf.AlwaysAvailableTrack) []*description.Media {
	var medias []*description.Media

	for _, track := range alwaysAvailableTracks {
		switch track.Codec {
		case conf.CodecAV1:
			medias = append(medias, &description.Media{
				Type: description.MediaTypeVideo,
				Formats: []format.Format{&format.AV1{
					PayloadTyp: 96,
				}},
			})

		case conf.CodecVP9:
			medias = append(medias, &description.Media{
				Type: description.MediaTypeVideo,
				Formats: []format.Format{&format.VP9{
					PayloadTyp: 96,
				}},
			})

		case conf.CodecH265:
			medias = append(medias, &description.Media{
				Type: description.MediaTypeVideo,
				Formats: []format.Format{&format.H265{
					PayloadTyp: 96,
				}},
			})

		case conf.CodecH264:
			medias = append(medias, &description.Media{
				Type: description.MediaTypeVideo,
				Formats: []format.Format{&format.H264{
					PayloadTyp:        96,
					PacketizationMode: 1,
				}},
			})

		case conf.CodecOpus:
			medias = append(medias, &description.Media{
				Type: description.MediaTypeAudio,
				Formats: []format.Format{&format.Opus{
					PayloadTyp:   96,
					ChannelCount: 2,
				}},
			})

		case conf.CodecMPEG4Audio:
			medias = append(medias, &description.Media{
				Type: description.MediaTypeAudio,
				Formats: []format.Format{&format.MPEG4Audio{
					PayloadTyp:       96,
					SizeLength:       13,
					IndexLength:      3,
					IndexDeltaLength: 3,
					Config: &mpeg4audio.AudioSpecificConfig{
						Type:          mpeg4audio.ObjectTypeAACLC,
						SampleRate:    track.SampleRate,
						ChannelConfig: uint8(track.ChannelCount),
						ChannelCount:  track.ChannelCount,
					},
				}},
			})

		case conf.CodecG711:
			medias = append(medias, &description.Media{
				Type: description.MediaTypeAudio,
				Formats: []format.Format{&format.G711{
					PayloadTyp: func() uint8 {
						switch {
						case track.ChannelCount == 1 && track.MULaw:
							return 0
						case track.ChannelCount == 1 && !track.MULaw:
							return 8
						default:
							return 96
						}
					}(),
					MULaw:        track.MULaw,
					SampleRate:   track.SampleRate,
					ChannelCount: track.ChannelCount,
				}},
			})

		case conf.CodecLPCM:
			medias = append(medias, &description.Media{
				Type: description.MediaTypeAudio,
				Formats: []format.Format{&format.LPCM{
					PayloadTyp:   96,
					BitDepth:     16,
					SampleRate:   track.SampleRate,
					ChannelCount: track.ChannelCount,
				}},
			})
		}
	}

	return medias
}

// Stream is a media stream.
// It stores tracks, readers and allows to write data to readers, remuxing it when needed.
type Stream struct {
	Desc                  *description.Session
	AlwaysAvailable       bool
	AlwaysAvailableFile   string
	AlwaysAvailableTracks []conf.AlwaysAvailableTrack
	WriteQueueSize        int
	RTPMaxPayloadSize     int
	ReplaceNTP            bool
	Parent                logger.Writer

	mutex            sync.RWMutex
	subStream        *SubStream
	offlineSubStream *offlineSubStream
	bytesReceived    *uint64
	bytesSent        *uint64
	medias           map[*description.Media]*streamMedia
	rtspStream       *gortsplib.ServerStream
	rtspsStream      *gortsplib.ServerStream
	readers          map[*Reader]struct{}
	processingErrors *errordumper.Dumper

	hasReaders chan struct{}
}

// Initialize initializes a Stream.
func (s *Stream) Initialize() error {
	if s.AlwaysAvailable {
		if s.Desc != nil {
			panic("should not happen")
		}
		if !s.ReplaceNTP {
			panic("should not happen")
		}

		var medias []*description.Media

		if s.AlwaysAvailableFile != "" {
			var err error
			medias, err = mediasFromAlwaysAvailableFile(s.AlwaysAvailableFile)
			if err != nil {
				return err
			}
		} else {
			medias = mediasFromAlwaysAvailableTracks(s.AlwaysAvailableTracks)
		}

		s.Desc = &description.Session{
			Medias: medias,
		}
	}

	s.bytesReceived = new(uint64)
	s.bytesSent = new(uint64)
	s.medias = make(map[*description.Media]*streamMedia)
	s.readers = make(map[*Reader]struct{})
	s.hasReaders = make(chan struct{})

	s.processingErrors = &errordumper.Dumper{
		OnReport: func(val uint64, last error) {
			if val == 1 {
				s.Parent.Log(logger.Warn, "processing error: %v", last)
			} else {
				s.Parent.Log(logger.Warn, "%d processing errors, last was: %v", val, last)
			}
		},
	}
	s.processingErrors.Start()

	for _, media := range s.Desc.Medias {
		sm := &streamMedia{
			media:             media,
			alwaysAvailable:   s.AlwaysAvailable,
			rtpMaxPayloadSize: s.RTPMaxPayloadSize,
			replaceNTP:        s.ReplaceNTP,
			onBytesReceived:   s.onBytesReceived,
			onBytesSent:       s.onBytesSent,
			writeRTSP:         s.writeRTSP,
			processingErrors:  s.processingErrors,
			parent:            s.Parent,
		}
		err := sm.initialize()
		if err != nil {
			return err
		}
		s.medias[media] = sm
	}

	if s.AlwaysAvailable {
		err := s.StartOfflineSubStream()
		if err != nil {
			return err
		}
	}

	return nil
}

// Close closes all resources of the stream.
func (s *Stream) Close() {
	if s.offlineSubStream != nil {
		s.offlineSubStream.close(false)
	}

	s.processingErrors.Stop()

	if s.rtspStream != nil {
		s.rtspStream.Close()
	}
	if s.rtspsStream != nil {
		s.rtspsStream.Close()
	}
}

// StartOfflineSubStream starts the offline substream.
func (s *Stream) StartOfflineSubStream() error {
	if !s.AlwaysAvailable {
		panic("should not happen")
	}

	oss := &offlineSubStream{
		stream: s,
	}
	err := oss.initialize()
	if err != nil {
		return err
	}

	if s.offlineSubStream != nil {
		s.Parent.Log(logger.Info, "stream is offline")
	}

	s.offlineSubStream = oss

	return nil
}

// BytesReceived returns received bytes.
func (s *Stream) BytesReceived() uint64 {
	return atomic.LoadUint64(s.bytesReceived)
}

// BytesSent returns sent bytes.
func (s *Stream) BytesSent() uint64 {
	bytesSent := atomic.LoadUint64(s.bytesSent)

	s.mutex.RLock()
	defer s.mutex.RUnlock()

	if s.rtspStream != nil {
		stats := s.rtspStream.Stats()
		bytesSent += stats.BytesSent
	}
	if s.rtspsStream != nil {
		stats := s.rtspsStream.Stats()
		bytesSent += stats.BytesSent
	}

	return bytesSent
}

// RTSPStream returns the RTSP stream.
func (s *Stream) RTSPStream(server *gortsplib.Server) *gortsplib.ServerStream {
	s.mutex.Lock()
	defer s.mutex.Unlock()

	if s.rtspStream == nil {
		s.rtspStream = &gortsplib.ServerStream{
			Server: server,
			Desc:   s.Desc,
		}
		err := s.rtspStream.Initialize()
		if err != nil {
			panic(err)
		}
	}
	return s.rtspStream
}

// RTSPSStream returns the RTSPS stream.
func (s *Stream) RTSPSStream(server *gortsplib.Server) *gortsplib.ServerStream {
	s.mutex.Lock()
	defer s.mutex.Unlock()

	if s.rtspsStream == nil {
		s.rtspsStream = &gortsplib.ServerStream{
			Server: server,
			Desc:   s.Desc,
		}
		err := s.rtspsStream.Initialize()
		if err != nil {
			panic(err)
		}
	}
	return s.rtspsStream
}

// AddReader adds a reader.
// Used by all protocols except RTSP.
func (s *Stream) AddReader(r *Reader) {
	s.mutex.Lock()
	defer s.mutex.Unlock()

	s.readers[r] = struct{}{}

	for medi, formats := range r.onDatas {
		sm := s.medias[medi]

		for forma, onData := range formats {
			sf := sm.formats[forma]
			sf.onDatas[r] = onData
		}
	}

	r.queueSize = s.WriteQueueSize
	r.start()

	select {
	case <-s.hasReaders:
	default:
		close(s.hasReaders)
	}
}

// RemoveReader removes a reader.
// Used by all protocols except RTSP.
func (s *Stream) RemoveReader(r *Reader) {
	s.mutex.Lock()
	defer s.mutex.Unlock()

	r.stop()

	for medi, formats := range r.onDatas {
		sm := s.medias[medi]

		for forma := range formats {
			sf := sm.formats[forma]
			delete(sf.onDatas, r)
		}
	}

	delete(s.readers, r)
}

// WaitForReaders waits for the stream to have at least one reader.
func (s *Stream) WaitForReaders() {
	<-s.hasReaders
}

func (s *Stream) onBytesReceived(v uint64) {
	atomic.AddUint64(s.bytesReceived, v)
}

func (s *Stream) onBytesSent(v uint64) {
	atomic.AddUint64(s.bytesSent, v)
}

func (s *Stream) writeRTSP(medi *description.Media, pkts []*rtp.Packet, ntp time.Time) {
	if s.rtspStream != nil {
		for _, pkt := range pkts {
			s.rtspStream.WritePacketRTPWithNTP(medi, pkt, ntp) //nolint:errcheck
		}
	}

	if s.rtspsStream != nil {
		for _, pkt := range pkts {
			s.rtspsStream.WritePacketRTPWithNTP(medi, pkt, ntp) //nolint:errcheck
		}
	}
}
