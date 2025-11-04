// Package stream contains the Stream object.
package stream

import (
	"sync"
	"sync/atomic"
	"time"

	"github.com/bluenviron/gortsplib/v5"
	"github.com/bluenviron/gortsplib/v5/pkg/description"
	"github.com/bluenviron/gortsplib/v5/pkg/format"
	"github.com/pion/rtp"

	"github.com/bluenviron/mediamtx/internal/counterdumper"
	"github.com/bluenviron/mediamtx/internal/logger"
	"github.com/bluenviron/mediamtx/internal/unit"
)

// Stream is a media stream.
// It stores tracks, readers and allows to write data to readers, converting it when needed.
type Stream struct {
	WriteQueueSize     int
	RTPMaxPayloadSize  int
	Desc               *description.Session
	GenerateRTPPackets bool
	FillNTP            bool
	Parent             logger.Writer

	bytesReceived     *uint64
	bytesSent         *uint64
	lastRTPTimestamp  *int64 // Unix timestamp in nanoseconds
	keyFramesCount    map[string]*uint64 // per codec key frame count
	lastKeyFrameTS    map[string]*int64  // per codec last key frame timestamp (Unix nanoseconds)
	prevKeyFrameTS    map[string]*int64  // per codec previous key frame timestamp (Unix nanoseconds)
	lastGOPSize       map[string]*int64   // per codec last GOP size in nanoseconds
	frameTimestamps   map[string][]int64  // per codec frame timestamps for FPS calculation (sliding window)
	fpsMutex          sync.RWMutex        // mutex for frame timestamps
	keyFramesMutex    sync.RWMutex
	medias            map[*description.Media]*streamMedia
	mutex             sync.RWMutex
	rtspStream        *gortsplib.ServerStream
	rtspsStream       *gortsplib.ServerStream
	readers           map[*Reader]struct{}
	processingErrors  *counterdumper.CounterDumper
}

// Initialize initializes a Stream.
func (s *Stream) Initialize() error {
	s.bytesReceived = new(uint64)
	s.bytesSent = new(uint64)
	s.lastRTPTimestamp = new(int64)
	s.keyFramesCount = make(map[string]*uint64)
	s.lastKeyFrameTS = make(map[string]*int64)
	s.prevKeyFrameTS = make(map[string]*int64)
	s.lastGOPSize = make(map[string]*int64)
	s.frameTimestamps = make(map[string][]int64)
	s.medias = make(map[*description.Media]*streamMedia)
	s.readers = make(map[*Reader]struct{})

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
		s.medias[media] = &streamMedia{
			rtpMaxPayloadSize:  s.RTPMaxPayloadSize,
			media:              media,
			generateRTPPackets: s.GenerateRTPPackets,
			fillNTP:            s.FillNTP,
			processingErrors:   s.processingErrors,
			parent:             s.Parent,
		}
		err := s.medias[media].initialize()
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

// LastRTPTimestamp returns the timestamp of the last RTP packet received (Unix timestamp in nanoseconds).
func (s *Stream) LastRTPTimestamp() int64 {
	return atomic.LoadInt64(s.lastRTPTimestamp)
}

// KeyFramesCount returns the count of key frames received for a specific codec.
// If codec is empty, returns total count across all codecs.
func (s *Stream) KeyFramesCount(codec string) uint64 {
	s.keyFramesMutex.RLock()
	defer s.keyFramesMutex.RUnlock()
	
	if codec == "" {
		total := uint64(0)
		for _, count := range s.keyFramesCount {
			total += atomic.LoadUint64(count)
		}
		return total
	}
	
	if count, ok := s.keyFramesCount[codec]; ok {
		return atomic.LoadUint64(count)
	}
	return 0
}

// KeyFramesCountPerCodec returns a map of codec to key frame count.
func (s *Stream) KeyFramesCountPerCodec() map[string]uint64 {
	s.keyFramesMutex.RLock()
	defer s.keyFramesMutex.RUnlock()
	
	result := make(map[string]uint64)
	for codec, count := range s.keyFramesCount {
		result[codec] = atomic.LoadUint64(count)
	}
	return result
}

// LastKeyFrameTimestamp returns the timestamp of the last key frame received for a specific codec.
// If codec is empty, returns the most recent timestamp across all codecs.
func (s *Stream) LastKeyFrameTimestamp(codec string) int64 {
	s.keyFramesMutex.RLock()
	defer s.keyFramesMutex.RUnlock()
	
	if codec == "" {
		var maxTS int64
		for _, ts := range s.lastKeyFrameTS {
			if tsVal := atomic.LoadInt64(ts); tsVal > maxTS {
				maxTS = tsVal
			}
		}
		return maxTS
	}
	
	if ts, ok := s.lastKeyFrameTS[codec]; ok {
		return atomic.LoadInt64(ts)
	}
	return 0
}

// LastKeyFrameTimestampPerCodec returns a map of codec to last key frame timestamp.
func (s *Stream) LastKeyFrameTimestampPerCodec() map[string]int64 {
	s.keyFramesMutex.RLock()
	defer s.keyFramesMutex.RUnlock()
	
	result := make(map[string]int64)
	for codec, ts := range s.lastKeyFrameTS {
		result[codec] = atomic.LoadInt64(ts)
	}
	return result
}

// LastGOPSizePerCodec returns a map of codec to last GOP size in nanoseconds.
func (s *Stream) LastGOPSizePerCodec() map[string]int64 {
	s.keyFramesMutex.RLock()
	defer s.keyFramesMutex.RUnlock()
	
	result := make(map[string]int64)
	for codec, gopSize := range s.lastGOPSize {
		result[codec] = atomic.LoadInt64(gopSize)
	}
	return result
}

// FPSPerCodec returns a map of codec to FPS (frames per second).
// Calculated based on frames received in the last second.
func (s *Stream) FPSPerCodec() map[string]float64 {
	s.fpsMutex.RLock()
	defer s.fpsMutex.RUnlock()
	
	result := make(map[string]float64)
	now := time.Now().UnixNano()
	oneSecondAgo := now - int64(time.Second)
	
	for codec, timestamps := range s.frameTimestamps {
		// Count frames in the last second
		count := 0
		for _, ts := range timestamps {
			if ts >= oneSecondAgo {
				count++
			}
		}
		result[codec] = float64(count)
	}
	return result
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

// WriteUnit writes a Unit.
func (s *Stream) WriteUnit(medi *description.Media, forma format.Format, u *unit.Unit) {
	sm := s.medias[medi]
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
	sm := s.medias[medi]
	sf := sm.formats[forma]

	s.mutex.RLock()
	defer s.mutex.RUnlock()

	sf.writeRTPPacket(s, medi, pkt, ntp, pts)
}
