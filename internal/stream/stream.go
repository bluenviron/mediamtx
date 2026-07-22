// Package stream contains the Stream object.
package stream

import (
	"os"
	"reflect"
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
					VPS:        codec.VPS,
					SPS:        codec.SPS,
					PPS:        codec.PPS,
				}},
			})

		case *codecs.H264:
			medias = append(medias, &description.Media{
				Type: description.MediaTypeVideo,
				Formats: []format.Format{&format.H264{
					PayloadTyp:        96,
					PacketizationMode: 1,
					SPS:               codec.SPS,
					PPS:               codec.PPS,
				}},
			})

		case *codecs.Opus:
			medias = append(medias, &description.Media{
				Type: description.MediaTypeAudio,
				Formats: []format.Format{&format.Opus{
					PayloadTyp:   96,
					ChannelCount: codec.ChannelCount,
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
						Type:          codec.Config.Type,
						SampleRate:    codec.Config.SampleRate,
						ChannelConfig: codec.Config.ChannelConfig,
						ChannelCount:  codec.Config.ChannelCount, //nolint:staticcheck
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
					VPS:        offlineH265VPS,
					SPS:        offlineH265SPS,
					PPS:        offlineH265PPS,
				}},
			})

		case conf.CodecH264:
			medias = append(medias, &description.Media{
				Type: description.MediaTypeVideo,
				Formats: []format.Format{&format.H264{
					PayloadTyp:        96,
					PacketizationMode: 1,
					SPS:               offlineH264SPS,
					PPS:               offlineH264PPS,
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

func buildOfflineDesc(
	alwaysAvailableTracks []conf.AlwaysAvailableTrack,
	alwaysAvailableFile string,
) (*description.Session, error) {
	out := &description.Session{}

	if alwaysAvailableFile != "" {
		var err error
		out.Medias, err = mediasFromAlwaysAvailableFile(alwaysAvailableFile)
		if err != nil {
			return nil, err
		}
	} else {
		out.Medias = mediasFromAlwaysAvailableTracks(alwaysAvailableTracks)
	}

	return out, nil
}

func cloneFormatShallow(forma format.Format) format.Format {
	v := reflect.New(reflect.TypeOf(forma).Elem())
	v.Elem().Set(reflect.ValueOf(forma).Elem())
	return v.Interface().(format.Format)
}

func cloneDesc(desc *description.Session) *description.Session {
	out := &description.Session{
		Title:  desc.Title,
		Medias: make([]*description.Media, len(desc.Medias)),
	}

	for i, media := range desc.Medias {
		formats := make([]format.Format, len(media.Formats))

		for j, forma := range media.Formats {
			formats[j] = cloneFormatShallow(forma)
		}

		out.Medias[i] = &description.Media{
			Type:    media.Type,
			Formats: formats,
		}
	}

	return out
}

// Stream is a media stream.
// It stores tracks, readers and allows to write data to readers, remuxing it when needed.
type Stream struct {
	OrigDesc              *description.Session
	AlwaysAvailable       bool
	HasFallbackSource     bool // enables SubStream swapping without AlwaysAvailable offline machinery
	AlwaysAvailableTracks []conf.AlwaysAvailableTrack
	AlwaysAvailableFile   string
	WriteQueueSize        int
	RTPMaxPayloadSize     int
	ReplaceNTP            bool
	Parent                logger.Writer

	outDescMutex sync.RWMutex
	outDesc      *description.Session

	offlineDesc          *description.Session
	mutex                sync.RWMutex
	subStream            *SubStream
	offlineSubStream     *offlineSubStream
	inboundBytes         atomic.Uint64
	outboundBytes        atomic.Uint64
	medias               map[*description.Media]*streamMedia
	rtspStream           *gortsplib.ServerStream
	rtspsStream          *gortsplib.ServerStream
	readers              map[*Reader]struct{}
	inboundFramesInError *errordumper.Dumper

	timeMutex         sync.Mutex
	firstTimeReceived bool
	lastPTS           time.Duration
	lastSystemTime    time.Time

	hasReaders chan struct{}
}

// Initialize initializes a Stream.
func (s *Stream) Initialize() error {
	if s.AlwaysAvailable {
		if s.OrigDesc != nil {
			panic("should not happen")
		}
		if !s.ReplaceNTP {
			panic("should not happen")
		}

		var err error
		s.offlineDesc, err = buildOfflineDesc(s.AlwaysAvailableTracks, s.AlwaysAvailableFile)
		if err != nil {
			return err
		}

		s.OrigDesc = s.offlineDesc
	}

	s.medias = make(map[*description.Media]*streamMedia)
	s.readers = make(map[*Reader]struct{})
	s.hasReaders = make(chan struct{})

	s.inboundFramesInError = &errordumper.Dumper{
		OnReport: func(val uint64, last error) {
			if val == 1 {
				s.Parent.Log(logger.Warn, "processing error: %v", last)
			} else {
				s.Parent.Log(logger.Warn, "%d processing errors, last was: %v", val, last)
			}
		},
	}
	s.inboundFramesInError.Start()

	s.lastSystemTime = time.Now()

	s.outDesc = &description.Session{
		Title:  s.OrigDesc.Title,
		Medias: make([]*description.Media, len(s.OrigDesc.Medias)),
	}

	for i, origMedia := range s.OrigDesc.Medias {
		sm := &streamMedia{
			origMedia:            origMedia,
			alwaysAvailable:      s.AlwaysAvailable,
			rtpMaxPayloadSize:    s.RTPMaxPayloadSize,
			replaceNTP:           s.ReplaceNTP,
			inboundBytes:         &s.inboundBytes,
			outboundBytes:        &s.outboundBytes,
			updateLastTime:       s.updateLastTime,
			writeRTSP:            s.writeRTSP,
			updateOutDesc:        s.updateOutDesc,
			inboundFramesInError: s.inboundFramesInError,
			parent:               s.Parent,
		}
		err := sm.initialize()
		if err != nil {
			return err
		}

		s.medias[origMedia] = sm
		s.outDesc.Medias[i] = sm.outMedia
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

	s.inboundFramesInError.Stop()

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

// InboundBytes returns received bytes.
func (s *Stream) InboundBytes() uint64 {
	return s.inboundBytes.Load()
}

// OutboundBytes returns sent bytes.
func (s *Stream) OutboundBytes() uint64 {
	outboundBytes := s.outboundBytes.Load()

	s.mutex.RLock()
	defer s.mutex.RUnlock()

	if s.rtspStream != nil {
		stats := s.rtspStream.Stats()
		outboundBytes += stats.OutboundBytes
	}
	if s.rtspsStream != nil {
		stats := s.rtspsStream.Stats()
		outboundBytes += stats.OutboundBytes
	}

	return outboundBytes
}

// InboundFramesInError returns the number of frames received with processing errors.
func (s *Stream) InboundFramesInError() uint64 {
	return s.inboundFramesInError.Get()
}

// RTSPStream returns the RTSP stream.
func (s *Stream) RTSPStream(server *gortsplib.Server) (*gortsplib.ServerStream, error) {
	s.mutex.Lock()
	defer s.mutex.Unlock()

	if s.rtspStream == nil {
		strm := &gortsplib.ServerStream{
			Server: server,
			Desc:   s.outDesc,
		}
		err := strm.Initialize()
		if err != nil {
			return nil, err
		}

		s.rtspStream = strm
	}

	return s.rtspStream, nil
}

// RTSPSStream returns the RTSPS stream.
func (s *Stream) RTSPSStream(server *gortsplib.Server) (*gortsplib.ServerStream, error) {
	s.mutex.Lock()
	defer s.mutex.Unlock()

	if s.rtspsStream == nil {
		strm := &gortsplib.ServerStream{
			Server: server,
			Desc:   s.outDesc,
		}
		err := strm.Initialize()
		if err != nil {
			return nil, err
		}

		s.rtspsStream = strm
	}

	return s.rtspsStream, nil
}

// AddReader adds a reader.
// Used by all protocols except RTSP.
func (s *Stream) AddReader(r *Reader) {
	s.mutex.Lock()
	defer s.mutex.Unlock()

	s.readers[r] = struct{}{}

	for origMedia, origFormats := range r.onDatas {
		sm := s.medias[origMedia]

		for origFormat, onData := range origFormats {
			sf := sm.formats[origFormat]
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

// OutDescCopy returns a copy of the output description.
func (s *Stream) OutDescCopy() *description.Session {
	s.outDescMutex.RLock()
	defer s.outDescMutex.RUnlock()

	return cloneDesc(s.outDesc)
}

func (s *Stream) updateOutDesc(update func()) {
	s.outDescMutex.Lock()
	defer s.outDescMutex.Unlock()

	update()

	if s.rtspStream != nil {
		s.rtspStream.ReloadDesc()
	}

	if s.rtspsStream != nil {
		s.rtspsStream.ReloadDesc()
	}
}

func (s *Stream) updateLastTime(pts time.Duration) {
	s.timeMutex.Lock()
	defer s.timeMutex.Unlock()

	s.firstTimeReceived = true

	if pts > s.lastPTS {
		s.lastPTS = pts
	}

	s.lastSystemTime = time.Now()
}

func (s *Stream) writeRTSP(outMedia *description.Media, pkts []*rtp.Packet, ntp time.Time) {
	if s.rtspStream != nil {
		for _, pkt := range pkts {
			s.rtspStream.WritePacketRTPWithNTP(outMedia, pkt, ntp) //nolint:errcheck
		}
	}

	if s.rtspsStream != nil {
		for _, pkt := range pkts {
			s.rtspsStream.WritePacketRTPWithNTP(outMedia, pkt, ntp) //nolint:errcheck
		}
	}
}
