package dvr

import (
	"context"
	"fmt"
	"io"
	"math"
	"net/http"
	"net/url"
	"os"
	"path"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/bluenviron/mediamtx/internal/hlss3uploader"
	"github.com/bluenviron/mediamtx/internal/logger"
	"github.com/bluenviron/mediamtx/internal/models"
)

const (
	Window                       = 2 * time.Hour
	defaultStoragePrefix         = "live-hls"
	defaultTargetDurationSec     = 1
	defaultSegmentDurationSec    = 2.0
	serveCountFlushInterval      = 5 * time.Second
	serveOutcomeLocal            = "local"
	serveOutcomeRedirect         = "redirect"
	serveOutcomeProxy            = "proxy"
	serveOutcomeMiss             = "miss"
	cacheControlNoCache          = "no-cache"
	cacheControlImmutableSegment = "public, max-age=31536000, immutable"
	mediaTypePlaylistM3U8        = ".m3u8"
	mediaTypeInitKeyword         = "init"

	contentTypeMP4     = "video/mp4"
	contentTypeM4S     = "video/iso.segment"
	contentTypeTS      = "video/mp2t"
	contentTypeDefault = "application/octet-stream"
)

type Service struct {
	Config       hlss3uploader.StorageConfig
	Repository   models.LiveHLSSegmentRepository
	Parent       logger.Writer
	SegmentCount int

	provider hlss3uploader.StorageProvider
	mu       sync.Mutex
	sessions map[string]string
	active   map[string]bool

	counts      map[serveCountKey]serveCounts
	flushMu     sync.Mutex
	stopFlush   chan struct{}
	flushDone   chan struct{}
	stopFlushIO context.CancelFunc
	closeDone   chan struct{}
	closed      bool
	serving     int
	serveDone   *sync.Cond
}

type serveCountKey struct{ streamID, segmentName string }
type serveCounts struct{ local, redirect, proxy, miss int64 }

func (s *Service) Initialize() {
	s.sessions = make(map[string]string)
	s.active = make(map[string]bool)
	s.startServeCountFlush(serveCountFlushInterval)
	if s.Config.Prefix == "" {
		s.Config.Prefix = defaultStoragePrefix
	}
	provider, err := hlss3uploader.NewProviderSelector().Select(context.Background(), s.Config)
	if err != nil {
		s.log(logger.Info, "DVR external storage disabled: %v", err)
		return
	}
	s.provider = provider
	s.log(logger.Info, "DVR storage initialized (provider: %s, bucket: %s, endpoint: %s)", provider.Name(), s.Config.Bucket, s.Config.Endpoint)
}

func (s *Service) Close() {
	s.mu.Lock()
	if s.closed {
		done := s.closeDone
		s.mu.Unlock()
		<-done
		return
	}
	s.closed = true
	s.closeDone = make(chan struct{})
	if s.serveDone == nil {
		s.serveDone = sync.NewCond(&s.mu)
	}
	for s.serving != 0 {
		s.serveDone.Wait()
	}
	stop, done, stopIO := s.stopFlush, s.flushDone, s.stopFlushIO
	s.mu.Unlock()
	defer close(s.closeDone)
	if stopIO != nil {
		stopIO()
	}
	if stop != nil {
		close(stop)
		<-done
	}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	s.flushServeCountsWithContext(ctx, "")
	s.mu.Lock()
	retry := len(s.counts) != 0
	s.mu.Unlock()
	if retry {
		s.flushServeCountsWithContext(ctx, "") // retry once within the same shutdown budget
	}
	s.mu.Lock()
	remaining := len(s.counts)
	s.mu.Unlock()
	if remaining != 0 {
		// No durable queue exists; shutdown cannot preserve counts after a prolonged outage.
		s.log(logger.Warn, "DVR shutdown with %d unflushed segment count keys", remaining)
	}
	if s.provider != nil {
		_ = s.provider.Close()
	}
}

func (s *Service) StartSession(streamID string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.sessions == nil {
		s.sessions = make(map[string]string)
	}
	s.sessions[streamID] = fmt.Sprintf("%s-%d", strings.ReplaceAll(streamID, "/", "_"), time.Now().UnixNano())
	if s.active == nil {
		s.active = make(map[string]bool)
	}
	s.active[streamID] = true
}

func (s *Service) EndSession(streamID string) {
	s.mu.Lock()
	if s.active == nil {
		s.active = make(map[string]bool)
	}
	s.active[streamID] = false
	s.mu.Unlock()
	s.flushServeCounts(streamID)
}

func (s *Service) IsActive(streamID string) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.active[streamID]
}

func contentTypeForExt(localPath string) string {
	switch strings.ToLower(filepath.Ext(localPath)) {
	case ".mp4":
		return contentTypeMP4
	case ".m4s":
		return contentTypeM4S
	case ".ts":
		return contentTypeTS
	default:
		return contentTypeDefault
	}
}

func isFMP4Segment(name string) bool {
	ext := strings.ToLower(filepath.Ext(name))
	return ext == ".mp4" || ext == ".m4s" || ext == ".mp"
}

func isInitSegment(name string) bool {
	lower := strings.ToLower(name)
	return strings.Contains(lower, mediaTypeInitKeyword)
}

func (s *Service) RenderPlaylist(streamID string, now time.Time) ([]byte, bool, error) {
	if s == nil || s.Repository == nil {
		return nil, false, nil
	}
	if playlist, ok, err := s.renderFMP4Playlist(streamID, now); err != nil || ok {
		return playlist, ok, err
	}
	segments, err := s.Repository.ListWindow(streamID, s.currentSessionID(streamID), now.Add(-Window))
	if err != nil || len(segments) == 0 {
		return nil, false, err
	}
	sort.SliceStable(segments, func(i, j int) bool {
		if segments[i].Sequence == nil || segments[j].Sequence == nil {
			return segments[i].StartedAt.Before(segments[j].StartedAt)
		}
		return *segments[i].Sequence < *segments[j].Sequence
	})

	var mediaSegments []models.LiveHLSSegment

	for _, seg := range segments {
		if !isInitSegment(seg.SegmentName) {
			mediaSegments = append(mediaSegments, seg)
		}
	}

	if len(mediaSegments) == 0 {
		return nil, false, nil
	}

	if s.SegmentCount > 0 && len(mediaSegments) > s.SegmentCount {
		mediaSegments = mediaSegments[len(mediaSegments)-s.SegmentCount:]
	}

	mediaSeq := int64(0)
	if mediaSegments[0].Sequence != nil {
		mediaSeq = *mediaSegments[0].Sequence
	}
	target := 1
	for _, seg := range mediaSegments {
		if v := int(math.Ceil(float64(seg.DurationMS) / 1000)); v > target {
			target = v
		}
	}

	var b strings.Builder
	b.WriteString("#EXTM3U\n#EXT-X-VERSION:7\n#EXT-X-INDEPENDENT-SEGMENTS\n")
	b.WriteString(fmt.Sprintf("#EXT-X-TARGETDURATION:%d\n", target))
	b.WriteString(fmt.Sprintf("#EXT-X-MEDIA-SEQUENCE:%d\n", mediaSeq))

	for _, seg := range mediaSegments {
		if s.SegmentCount == 0 && !seg.StartedAt.IsZero() {
			b.WriteString("#EXT-X-PROGRAM-DATE-TIME:")
			b.WriteString(seg.StartedAt.UTC().Format(time.RFC3339Nano))
			b.WriteByte('\n')
		}
		dur := float64(seg.DurationMS) / 1000
		if dur <= 0 {
			dur = 2.0
		}
		b.WriteString(fmt.Sprintf("#EXTINF:%.3f,\n", dur))
		b.WriteString("/media/")
		b.WriteString(escapePath(streamID))
		b.WriteByte('/')
		b.WriteString(url.PathEscape(seg.SegmentName))
		b.WriteByte('\n')
	}
	if !s.IsActive(streamID) {
		b.WriteString("#EXT-X-ENDLIST\n")
	}
	return []byte(b.String()), true, nil
}

func (s *Service) renderFMP4Playlist(streamID string, now time.Time) ([]byte, bool, error) {
	segments, err := s.Repository.ListFMP4Window(streamID, now.Add(-Window))
	if err != nil || len(segments) == 0 {
		return nil, false, err
	}
	var mediaSegments []models.LiveHLSSegment
	for _, seg := range segments {
		if isFMP4Segment(seg.SegmentName) && !isInitSegment(seg.SegmentName) && seg.InitSegmentName != "" {
			mediaSegments = append(mediaSegments, seg)
		}
	}
	if len(mediaSegments) == 0 {
		return nil, false, nil
	}

	if s.SegmentCount > 0 && len(mediaSegments) > s.SegmentCount {
		mediaSegments = mediaSegments[len(mediaSegments)-s.SegmentCount:]
	}

	mediaSeq := int64(0)
	if mediaSegments[0].MediaSequence != nil {
		mediaSeq = *mediaSegments[0].MediaSequence
	} else if mediaSegments[0].Sequence != nil {
		mediaSeq = *mediaSegments[0].Sequence
	}
	target := 1
	for _, seg := range mediaSegments {
		if v := int(math.Ceil(float64(seg.DurationMS) / 1000)); v > target {
			target = v
		}
	}

	var b strings.Builder
	b.WriteString("#EXTM3U\n#EXT-X-VERSION:10\n#EXT-X-INDEPENDENT-SEGMENTS\n")
	b.WriteString(fmt.Sprintf("#EXT-X-TARGETDURATION:%d\n", target))
	b.WriteString(fmt.Sprintf("#EXT-X-MEDIA-SEQUENCE:%d\n", mediaSeq))

	lastInit := ""
	lastMuxSession := ""
	for i, seg := range mediaSegments {
		initChanged := seg.InitSegmentName != "" && seg.InitSegmentName != lastInit
		muxChanged := i > 0 && seg.MuxSessionID != "" && seg.MuxSessionID != lastMuxSession
		if i > 0 && (initChanged || muxChanged) {
			b.WriteString("#EXT-X-DISCONTINUITY\n")
		}
		if initChanged {
			b.WriteString(fmt.Sprintf("#EXT-X-MAP:URI=\"/media/%s/%s\"\n", escapePath(streamID), url.PathEscape(seg.InitSegmentName)))
			lastInit = seg.InitSegmentName
		}
		if seg.MuxSessionID != "" {
			lastMuxSession = seg.MuxSessionID
		}
		if s.SegmentCount == 0 && !seg.StartedAt.IsZero() {
			b.WriteString("#EXT-X-PROGRAM-DATE-TIME:")
			b.WriteString(seg.StartedAt.UTC().Format(time.RFC3339Nano))
			b.WriteByte('\n')
		}
		dur := float64(seg.DurationMS) / 1000
		if dur <= 0 {
			dur = 2.0
		}
		b.WriteString(fmt.Sprintf("#EXTINF:%.3f,\n", dur))
		b.WriteString("/media/")
		b.WriteString(escapePath(streamID))
		b.WriteByte('/')
		b.WriteString(url.PathEscape(seg.SegmentName))
		b.WriteByte('\n')
	}
	return []byte(b.String()), true, nil
}

func (s *Service) ServeMedia(w http.ResponseWriter, r *http.Request, streamID, segmentName string) bool {
	if s == nil {
		return false
	}
	s.startServeCountFlush(serveCountFlushInterval) // zero-value services also flush periodically
	s.mu.Lock()
	if s.closed {
		s.mu.Unlock()
		return false
	}
	s.serving++
	s.mu.Unlock()
	defer func() {
		s.mu.Lock()
		s.serving--
		if s.serving == 0 && s.serveDone != nil {
			s.serveDone.Broadcast()
		}
		s.mu.Unlock()
	}()

	var segment *models.LiveHLSSegment
	if s.Repository != nil {
		seg, err := s.Repository.GetByStreamAndSegment(streamID, segmentName)
		if err == nil && seg != nil {
			segment = seg
		}
	}

	// 1. Try local file if available on disk
	localPath := ""
	if segment != nil && segment.LocalPath != "" {
		localPath = segment.LocalPath
	} else {
		localPath = filepath.Join("./input-live", streamID, segmentName)
	}

	if localPath != "" {
		if _, err := os.Stat(localPath); err == nil {
			ct := ""
			if segment != nil && segment.ContentType != "" {
				ct = segment.ContentType
			} else {
				ct = contentTypeForExt(localPath)
			}
			w.Header().Set("Content-Type", ct)
			http.ServeFile(w, r, localPath)
			s.incServeCount(segment, streamID, segmentName, serveOutcomeLocal)
			return true
		}
	}

	// 2. Fallback to remote storage (Redirect Link or Proxy)
	storageKey := ""
	if segment != nil && segment.StorageKey != "" {
		storageKey = segment.StorageKey
	} else {
		prefix := strings.Trim(s.Config.Prefix, "/")
		if prefix == "" {
			prefix = defaultStoragePrefix
		}
		storageKey = path.Join(prefix, streamID, segmentName)
	}

	if storageKey != "" && s.provider != nil {
		// If segment has StorageETag (file UUID or CDN format), ensure provider mapping is primed
		if segment != nil && segment.StorageETag != "" {
			if depinProv, ok := s.provider.(*hlss3uploader.DePINStorageProvider); ok {
				depinProv.RegisterKeyUUID(storageKey, segment.StorageETag)
			}
			if cdnProv, ok := s.provider.(*hlss3uploader.CDNStorageProvider); ok {
				cdnProv.RegisterKeyETag(storageKey, segment.StorageETag)
			}
		}

		// 1. Prioritize Direct Download / Presigned Redirect Link (HTTP 302)
		if link, err := s.presign(r.Context(), storageKey); err == nil && link != "" {
			http.Redirect(w, r, link, http.StatusFound)
			s.incServeCount(segment, streamID, segmentName, serveOutcomeRedirect)
			return true
		}

		// 2. Fallback to proxy streaming if link generation is unavailable
		if s.proxyStorageObject(r.Context(), w, storageKey) {
			s.incServeCount(segment, streamID, segmentName, serveOutcomeProxy)
			return true
		}
	}

	s.incServeCount(segment, streamID, segmentName, serveOutcomeMiss)
	return false
}

func (s *Service) incServeCount(segment *models.LiveHLSSegment, streamID, segmentName, outcome string) {
	if s == nil || s.Repository == nil || segment == nil {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.counts == nil {
		// Pending counts are lost on process crash; distinct keys can grow during a DB outage.
		s.counts = make(map[serveCountKey]serveCounts)
	}
	key := serveCountKey{streamID, segmentName}
	count := s.counts[key]
	switch outcome {
	case serveOutcomeLocal:
		count.local++
	case serveOutcomeRedirect:
		count.redirect++
	case serveOutcomeProxy:
		count.proxy++
	case serveOutcomeMiss:
		count.miss++
	}
	s.counts[key] = count
}

// Exported spellings of the serve outcomes, for a caller that serves a segment
// itself and reports how it was delivered through RecordServe.
const (
	ServeOutcomeLocal    = serveOutcomeLocal
	ServeOutcomeRedirect = serveOutcomeRedirect
	ServeOutcomeProxy    = serveOutcomeProxy
	ServeOutcomeMiss     = serveOutcomeMiss
)

// RecordServe counts one request for a live segment under how it was
// delivered. ServeMedia does this for the DVR route; RecordServe is for the
// HLS muxer's own segment route, which serves the same segments whether or not
// DVR is enabled.
//
// A segment without a row is skipped, as in ServeMedia: its counts could never
// be written, and would be retried for as long as the process lives.
func (s *Service) RecordServe(streamID, segmentName, outcome string) {
	if s == nil || s.Repository == nil {
		return
	}
	s.mu.Lock()
	closed := s.closed
	s.mu.Unlock()
	if closed {
		return
	}
	s.startServeCountFlush(serveCountFlushInterval) // zero-value services also flush periodically

	segment, err := s.Repository.GetByStreamAndSegment(streamID, segmentName)
	if err != nil || segment == nil {
		return
	}
	s.incServeCount(segment, streamID, segmentName, outcome)
}

func (s *Service) startServeCountFlush(interval time.Duration) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.stopFlush != nil || s.closed {
		return
	}
	s.stopFlush = make(chan struct{})
	s.flushDone = make(chan struct{})
	flushCtx, stopIO := context.WithCancel(context.Background())
	s.stopFlushIO = stopIO
	go func() {
		defer stopIO()
		ticker := time.NewTicker(interval)
		defer ticker.Stop()
		defer close(s.flushDone)
		for {
			select {
			case <-ticker.C:
				ctx, cancel := context.WithTimeout(flushCtx, 2*time.Second)
				s.flushServeCountsWithContext(ctx, "")
				cancel()
			case <-s.stopFlush:
				return
			}
		}
	}()
}

func (s *Service) flushServeCounts(streamID string) {
	s.flushServeCountsWithContext(context.Background(), streamID)
}

func (s *Service) flushServeCountsWithContext(ctx context.Context, streamID string) {
	s.flushMu.Lock()
	defer s.flushMu.Unlock()
	batch, ok := s.Repository.(models.LiveHLSSegmentServeCounter)
	if !ok {
		return
	}
	s.mu.Lock()
	pending := make(map[serveCountKey]serveCounts)
	for key, count := range s.counts {
		if streamID == "" || streamID == key.streamID {
			pending[key] = count
			delete(s.counts, key)
		}
	}
	s.mu.Unlock()
	for key, count := range pending {
		if ctx.Err() != nil {
			s.mu.Lock()
			current := s.counts[key]
			current.local += count.local
			current.redirect += count.redirect
			current.proxy += count.proxy
			current.miss += count.miss
			s.counts[key] = current
			s.mu.Unlock()
			continue
		}
		callCtx, cancel := context.WithTimeout(ctx, 2*time.Second)
		err := batch.AddServeCounts(callCtx, key.streamID, key.segmentName, count.local, count.redirect, count.proxy, count.miss)
		cancel()
		if err != nil {
			s.mu.Lock()
			current := s.counts[key]
			current.local += count.local
			current.redirect += count.redirect
			current.proxy += count.proxy
			current.miss += count.miss
			s.counts[key] = current
			s.mu.Unlock()
			s.log(logger.Warn, "failed to flush serve counts for %s/%s: %v", key.streamID, key.segmentName, err)
		}
	}
}

func (s *Service) currentSessionID(streamID string) string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.sessions[streamID]
}

func (s *Service) presign(ctx context.Context, remoteKey string) (string, error) {
	if linkProv, ok := s.provider.(hlss3uploader.LinkStorageProvider); ok {
		return linkProv.GetLink(ctx, remoteKey)
	}
	return "", fmt.Errorf("provider does not support presign or link generation")
}

func (s *Service) proxyStorageObject(ctx context.Context, w http.ResponseWriter, key string) bool {
	readable, ok := s.provider.(hlss3uploader.ReadableStorageProvider)
	if !ok {
		return false
	}
	if ctx == nil {
		ctx = context.Background()
	}
	body, contentType, contentLength, err := readable.GetObject(ctx, key)
	if err != nil {
		return false
	}
	defer body.Close()

	if contentType != "" {
		w.Header().Set("Content-Type", contentType)
	} else {
		w.Header().Set("Content-Type", contentTypeForExt(key))
	}
	if contentLength >= 0 {
		w.Header().Set("Content-Length", strconv.FormatInt(contentLength, 10))
	}

	if strings.HasSuffix(key, mediaTypePlaylistM3U8) {
		w.Header().Set("Cache-Control", cacheControlNoCache)
	} else {
		w.Header().Set("Cache-Control", cacheControlImmutableSegment)
	}

	w.WriteHeader(http.StatusOK)
	_, _ = io.Copy(w, body)
	return true
}

func (s *Service) log(level logger.Level, format string, args ...interface{}) {
	if s.Parent != nil {
		s.Parent.Log(level, "[DVR] "+format, args...)
	}
}

func escapePath(v string) string {
	parts := strings.Split(v, "/")
	for i := range parts {
		parts[i] = url.PathEscape(parts[i])
	}
	return strings.Join(parts, "/")
}
