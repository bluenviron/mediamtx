// Package stream contains the Stream object.
package stream

import (
	"sync"
	"sync/atomic"
	"time"

	"github.com/bluenviron/gortsplib/v4"
	"github.com/bluenviron/gortsplib/v4/pkg/description"
	"github.com/bluenviron/gortsplib/v4/pkg/format"
	"github.com/pion/rtp"

	"github.com/bluenviron/mediamtx/internal/counterdumper"
	"github.com/bluenviron/mediamtx/internal/logger"
	"github.com/bluenviron/mediamtx/internal/unit"
)

// Reader is a stream reader.
type Reader interface {
	logger.Writer
}

// ReadFunc is the callback passed to AddReader().
type ReadFunc func(unit.Unit) error

// Stream is a media stream.
// It stores tracks, readers and allows to write data to readers, converting it when needed.
type Stream struct {
	WriteQueueSize     int
	RTPMaxPayloadSize  int
	Desc               *description.Session
	GenerateRTPPackets bool
	Parent             logger.Writer

	bytesReceived    *uint64
	bytesSent        *uint64
	streamMedias     map[*description.Media]*streamMedia
	mutex            sync.RWMutex
	rtspStream       *gortsplib.ServerStream
	rtspsStream      *gortsplib.ServerStream
	streamReaders    map[Reader]*streamReader
	processingErrors *counterdumper.CounterDumper

	readerRunning chan struct{}
}

// Initialize initializes a Stream.
func (s *Stream) Initialize() error {
	s.bytesReceived = new(uint64)
	s.bytesSent = new(uint64)
	s.streamMedias = make(map[*description.Media]*streamMedia)
	s.streamReaders = make(map[Reader]*streamReader)
	s.readerRunning = make(chan struct{})

	s.processingErrors = &counterdumper.CounterDumper{
		OnReport: func(val uint64) {
			s.Parent.Log(logger.Warn, "%d processing %s",
				val,
				func() string {
					if val == 1 {
						return "error"
					}
					return "errors"
				}())
		},
	}
	s.processingErrors.Start()

	for _, media := range s.Desc.Medias {
		s.streamMedias[media] = &streamMedia{
			rtpMaxPayloadSize:  s.RTPMaxPayloadSize,
			media:              media,
			generateRTPPackets: s.GenerateRTPPackets,
			processingErrors:   s.processingErrors,
			parent:             s.Parent,
		}
		err := s.streamMedias[media].initialize()
		if err != nil {
			return err
		}
	}

	return nil
}

// Close closes all resources of the stream.
func (s *Stream) Close() {
	s.processingErrors.Stop()

	if s.rtspStream != nil {
		s.rtspStream.Close()
	}
	if s.rtspsStream != nil {
		s.rtspsStream.Close()
	}
}

// BytesReceived returns received bytes.
func (s *Stream) BytesReceived() uint64 {
	return atomic.LoadUint64(s.bytesReceived)
}

// BytesSent returns sent bytes.
func (s *Stream) BytesSent() uint64 {
	s.mutex.RLock()
	defer s.mutex.RUnlock()

	bytesSent := atomic.LoadUint64(s.bytesSent)
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
func (s *Stream) AddReader(reader Reader, medi *description.Media, forma format.Format, cb ReadFunc) {
	s.mutex.Lock()
	defer s.mutex.Unlock()

	sr, ok := s.streamReaders[reader]
	if !ok {
		sr = &streamReader{
			queueSize: s.WriteQueueSize,
			parent:    reader,
		}
		sr.initialize()

		s.streamReaders[reader] = sr
	}

	sm := s.streamMedias[medi]
	sf := sm.formats[forma]
	sf.addReader(sr, cb)
}

// RemoveReader removes a reader.
// Used by all protocols except RTSP.
func (s *Stream) RemoveReader(reader Reader) {
	s.mutex.Lock()
	defer s.mutex.Unlock()

	sr := s.streamReaders[reader]

	for _, sm := range s.streamMedias {
		for _, sf := range sm.formats {
			sf.removeReader(sr)
		}
	}

	delete(s.streamReaders, reader)

	sr.stop()
}

// StartReader starts a reader.
// Used by all protocols except RTSP.
func (s *Stream) StartReader(reader Reader) {
	s.mutex.Lock()
	defer s.mutex.Unlock()

	sr := s.streamReaders[reader]

	sr.start()

	for _, sm := range s.streamMedias {
		for _, sf := range sm.formats {
			sf.startReader(sr)
		}
	}

	select {
	case <-s.readerRunning:
	default:
		close(s.readerRunning)
	}
}

// ReaderError returns whenever there's an error.
func (s *Stream) ReaderError(reader Reader) chan error {
	sr := s.streamReaders[reader]
	return sr.error()
}

// ReaderFormats returns all formats that a reader is reading.
func (s *Stream) ReaderFormats(reader Reader) []format.Format {
	s.mutex.RLock()
	defer s.mutex.RUnlock()

	sr := s.streamReaders[reader]
	var formats []format.Format

	for _, sm := range s.streamMedias {
		for forma, sf := range sm.formats {
			if _, ok := sf.pausedReaders[sr]; ok {
				formats = append(formats, forma)
			} else if _, ok := sf.runningReaders[sr]; ok {
				formats = append(formats, forma)
			}
		}
	}

	return formats
}

// WaitRunningReader waits for a running reader.
func (s *Stream) WaitRunningReader() {
	<-s.readerRunning
}

// WriteUnit writes a Unit.
func (s *Stream) WriteUnit(medi *description.Media, forma format.Format, u unit.Unit) {
	sm := s.streamMedias[medi]
	sf := sm.formats[forma]

	s.mutex.RLock()
	defer s.mutex.RUnlock()

	sf.writeUnit(s, medi, u)
}

// WriteRTPPacket writes a RTP packet.
func (s *Stream) WriteRTPPacket(
	medi *description.Media,
	forma format.Format,
	pkt *rtp.Packet,
	ntp time.Time,
	pts int64,
) {
	sm := s.streamMedias[medi]
	sf := sm.formats[forma]

	s.mutex.RLock()
	defer s.mutex.RUnlock()

	sf.writeRTPPacket(s, medi, pkt, ntp, pts)
}
