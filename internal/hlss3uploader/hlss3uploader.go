package hlss3uploader

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"os"
	"path"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/fsnotify/fsnotify"

	"github.com/bluenviron/mediamtx/internal/logger"
	"github.com/bluenviron/mediamtx/internal/models"
)

// StorageProvider is the interface for pluggable storage backends (S3, Local, CDN, MinIO, GCS, etc.).
type StorageProvider interface {
	Name() string
	UploadFile(ctx context.Context, localPath, remoteKey, contentType string) (string, error)
	DeleteFolder(ctx context.Context, prefix string) error
	Close() error
}

type ReadableStorageProvider interface {
	GetObject(ctx context.Context, key string) (io.ReadCloser, string, int64, error)
}

// LinkStorageProvider is the interface for storage backends that can generate direct download/presigned links.
type LinkStorageProvider interface {
	GetLink(ctx context.Context, key string) (string, error)
}

// ReadinessWaiter is the interface for storage backends that can wait until an uploaded file is ready/servable.
type ReadinessWaiter interface {
	WaitReady(ctx context.Context, fileID string) error
}

// HLSS3Uploader watches local HLS directory (e.g. ./input-live),
// and uploads generated init and media segment files to the configured StorageProvider.
// Playlists are not uploaded (nothing reads them from the provider); _stream.m3u8
// playlists are only ingested locally into the segment repository.
// Local files are never deleted by the uploader: the muxer serves them for playback.
type HLSS3Uploader struct {
	Config     StorageConfig
	Parent     logger.Writer
	Repository models.LiveHLSSegmentRepository

	provider StorageProvider

	// cleanupRepo records the remote folder lifecycle; nil without a database.
	cleanupRepo models.LiveHLSCleanupRepository

	ctx       context.Context
	ctxCancel func()
	watcher   *fsnotify.Watcher

	taskChan chan string
	done     chan struct{}
	wg       sync.WaitGroup

	muClose  sync.RWMutex
	isClosed bool

	processingFiles sync.Map
	uploadedFiles   sync.Map

	MaxReadinessPollers int
	ShutdownTimeout     time.Duration
	pollersMu           sync.Mutex
	pollersSem          chan struct{}
	pollersWg           sync.WaitGroup
	pollersClosed       bool
	pollersCtx          context.Context
	pollersCancel       context.CancelFunc
}

const (
	defaultStoragePrefix         = "live-hls"
	defaultStorageDirectory      = "./input-live"
	defaultWorkerCount           = 4
	defaultTaskQueueSize         = 1000
	defaultSegmentDurationMS     = 2000
	defaultUploadTimeout         = 2 * time.Minute
	defaultPresignExpires        = 15 * time.Minute
	defaultMaxReadinessPollers   = 32
	cacheControlNoCache          = "no-cache"
	cacheControlImmutableSegment = "public, max-age=31536000, immutable"

	contentTypeMP4     = "video/mp4"
	contentTypeM4S     = "video/iso.segment"
	contentTypeTS      = "video/mp2t"
	contentTypeM3U8    = "application/x-mpegURL"
	contentTypeDefault = "application/octet-stream"

	extM3U8 = ".m3u8"
	extMP4  = ".mp4"
	extMP   = ".mp"
	extM4S  = ".m4s"
	extTS   = ".ts"
)

// Initialize initializes and starts the HLSS3Uploader background service using supplied Config.
func (u *HLSS3Uploader) Initialize() error {
	u.ctx, u.ctxCancel = context.WithCancel(context.Background())
	u.pollersCtx, u.pollersCancel = context.WithCancel(u.ctx)
	maxPollers := u.MaxReadinessPollers
	if maxPollers <= 0 {
		maxPollers = defaultMaxReadinessPollers
	}
	u.pollersSem = make(chan struct{}, maxPollers)

	u.done = make(chan struct{})
	u.taskChan = make(chan string, defaultTaskQueueSize)
	if u.Config.Workers <= 0 {
		u.Config.Workers = defaultWorkerCount
	}

	selector := NewProviderSelector()
	provider, err := selector.Select(u.ctx, u.Config)
	if err != nil {
		u.Log(logger.Info, "HLS Storage Uploader disabled: %v", err)
		close(u.done)
		return nil
	}
	u.provider = provider

	if u.Config.Directory == "" {
		u.Config.Directory = defaultStorageDirectory
	}
	if u.Config.Prefix == "" {
		u.Config.Prefix = defaultStoragePrefix
	}

	// A reconnect before the delayed cleanup is due keeps the remote objects:
	// the row goes back to active and its delete_after is cleared.
	u.cleanupRepo = defaultCleanupRepository()
	if u.cleanupRepo != nil {
		contractID := ""
		if cp, ok := u.provider.(interface{ ContractID() string }); ok {
			contractID = cp.ContractID()
		}
		if err := u.cleanupRepo.UpsertActive(u.provider.Name(), u.cleanupStreamName(), u.remoteFolderPrefix(), contractID); err != nil {
			u.Log(logger.Warn, "failed to record remote folder as active: %v", err)
		}
	}

	watcher, err := fsnotify.NewWatcher()
	if err != nil {
		u.Log(logger.Error, "failed to create fsnotify watcher: %v", err)
		close(u.done)
		return err
	}
	u.watcher = watcher

	// Watch before the muxer writes its first file so no event is missed.
	_ = os.MkdirAll(u.Config.Directory, 0o755)
	if err := u.watcher.Add(u.Config.Directory); err != nil {
		u.Log(logger.Warn, "failed to watch directory %s: %v", u.Config.Directory, err)
	}

	u.Log(logger.Info, "HLS Storage Uploader initialized (provider: %s, bucket: %s, endpoint: %s, dir: %s)", u.provider.Name(), u.Config.Bucket, u.Config.Endpoint, u.Config.Directory)

	for i := 0; i < u.Config.Workers; i++ {
		u.wg.Add(1)
		go u.workerLoop()
	}

	// Start directory watcher loop
	go u.watchLoop()

	return nil
}

// Close stops the HLSS3Uploader and waits for goroutines to exit.
func (u *HLSS3Uploader) Close() {
	u.FlushAndClose()
}

// FlushAndClose flushes all remaining segment/playlist files from the local directory,
// finishes any pending uploads, and then cleanly closes all workers and providers.
func (u *HLSS3Uploader) FlushAndClose() {
	u.muClose.Lock()
	if u.isClosed {
		u.muClose.Unlock()
		return
	}
	u.isClosed = true
	u.muClose.Unlock()

	// 1. Stop watching for new filesystem events
	if u.watcher != nil {
		if err := u.watcher.Close(); err != nil {
			u.Log(logger.Warn, "failed to close watcher: %v", err)
		}
	}

	// 2. Wait for watchLoop to exit
	if u.done != nil {
		select {
		case <-u.done:
		case <-time.After(3 * time.Second):
			if u.Parent != nil {
				u.Parent.Log(logger.Warn, "[HLS Uploader Close] watchLoop did not exit in time")
			}
		}
	}

	// 3. Scan directory one last time to capture the final segment and updated playlist (with #EXT-X-ENDLIST).
	// FlushAndClose marks the uploader as closed before this point in order to block new fsnotify events;
	// therefore the final scan must bypass the normal isClosed guard used by sendToTaskChan().
	u.scanDirectoryForClose(u.Config.Directory)

	// 4. Safely close task channel so workers know when all work is drained
	u.muClose.Lock()
	if u.taskChan != nil {
		close(u.taskChan)
	}
	u.muClose.Unlock()

	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), u.getShutdownTimeout())
	defer shutdownCancel()

	// 5. Wait for upload workers to finish all remaining queued uploads
	workersDone := make(chan struct{})
	go func() {
		u.wg.Wait()
		close(workersDone)
	}()
	workersTimedOut := false
	select {
	case <-workersDone:
	case <-shutdownCtx.Done():
		workersTimedOut = true
		if u.Parent != nil {
			u.Parent.Log(logger.Warn, "[HLS Uploader Close] workers did not exit in time")
		}
		if u.ctxCancel != nil {
			u.ctxCancel()
		}
	}

	// 6. Stop readiness pollers and wait for them to finish before closing provider
	pollersDone := u.stopPollers()
	pollersTimedOut := false
	select {
	case <-pollersDone:
	case <-shutdownCtx.Done():
		pollersTimedOut = true
		if u.Parent != nil {
			u.Parent.Log(logger.Warn, "[HLS Uploader Close] readiness pollers did not exit in time")
		}
	}

	// 7. Schedule the remote live folder for deletion after the grace period
	// (DB-backed chore, see cleanup.go). A reconnect before it is due cancels it.
	if u.provider != nil {
		u.scheduleRemoteCleanup()
	}

	// 8. Close storage provider: never close while workers or pollers are still running.
	// If either timed out, defer close until they actually finish to prevent use-after-close.
	if u.provider != nil {
		provider := u.provider
		if workersTimedOut || pollersTimedOut {
			u.Log(logger.Warn, "deferring storage provider close until in-flight workers and pollers exit")
			go func() {
				<-workersDone
				<-pollersDone
				if err := provider.Close(); err != nil {
					u.Log(logger.Warn, "failed to close storage provider: %v", err)
				}
			}()
		} else {
			if err := provider.Close(); err != nil {
				u.Log(logger.Warn, "failed to close storage provider: %v", err)
			}
		}
	}
}

func (u *HLSS3Uploader) getShutdownTimeout() time.Duration {
	if u.ShutdownTimeout > 0 {
		return u.ShutdownTimeout
	}
	return 15 * time.Second
}

func (u *HLSS3Uploader) stopPollers() <-chan struct{} {
	u.pollersMu.Lock()
	u.pollersClosed = true
	if u.pollersCancel != nil {
		u.pollersCancel()
	}
	if u.ctxCancel != nil {
		u.ctxCancel()
	}
	u.pollersMu.Unlock()

	pollersDone := make(chan struct{})
	go func() {
		u.pollersWg.Wait()
		close(pollersDone)
	}()
	return pollersDone
}

// Log implements logger.Writer.
func (u *HLSS3Uploader) Log(level logger.Level, format string, args ...interface{}) {
	if u.Parent != nil {
		u.Parent.Log(level, "[HLS Storage Uploader] "+format, args...)
	}
}

// EnqueueSegment is called by the Muxer callback when a segment file is finalized.
// It queues the file for immediate upload to S3 without polling.
func (u *HLSS3Uploader) EnqueueSegment(filePath string, duration time.Duration) {
	u.sendToTaskChan(filePath)
}

func (u *HLSS3Uploader) watchLoop() {
	defer close(u.done)

	// Queue existing segment files (init files and playlists of past sessions are skipped).
	u.scanDirectory(u.Config.Directory)

	for {
		select {
		case <-u.ctx.Done():
			return

		case event, ok := <-u.watcher.Events:
			if !ok {
				return
			}

			ext := strings.ToLower(filepath.Ext(event.Name))

			isSegment := ext == ".m4s" || ext == ".ts" || ext == ".mp4" || ext == ".mp"

			// Skip sub-directory creation events — each subpath has its own muxer and uploader
			if event.Op&fsnotify.Create != 0 {
				info, err := os.Stat(event.Name)
				if err == nil && info.IsDir() {
					continue
				}
			}

			// Segment file: enqueue for waitStable + upload.
			// _stream.m3u8 playlist: enqueue for local ingest only (never uploaded).
			if u.isRelevantFile(event.Name) && event.Op&(fsnotify.Create|fsnotify.Write) != 0 {
				u.sendToTaskChan(event.Name)
			}

			if isSegment && event.Op&fsnotify.Remove != 0 {
				go u.handleLocalRemove(event.Name)
			}

		case err, ok := <-u.watcher.Errors:
			if !ok {
				return
			}
			u.Log(logger.Warn, "watcher error: %v", err)
		}
	}
}

func (u *HLSS3Uploader) scanDirectory(dir string) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return
	}
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		// Init files and playlists left by earlier sessions belong to other
		// muxer prefixes; the current session's files arrive as fsnotify events.
		if isInitSegmentFile(entry.Name()) || strings.HasSuffix(strings.ToLower(entry.Name()), extM3U8) {
			continue
		}
		if u.isRelevantFile(entry.Name()) {
			u.sendToTaskChan(filepath.Join(dir, entry.Name()))
		}
	}
}

func (u *HLSS3Uploader) scanDirectoryForClose(dir string) {
	if u.taskChan == nil {
		return
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		return
	}
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		if u.isRelevantFile(entry.Name()) {
			u.taskChan <- filepath.Join(dir, entry.Name())
		}
	}
}

// isRelevantFile reports whether name must be handled: media segments, init
// files and, when a repository is set, _stream.m3u8 playlists (ingest only).
func (u *HLSS3Uploader) isRelevantFile(name string) bool {
	base := filepath.Base(name)
	switch strings.ToLower(filepath.Ext(base)) {
	case extM4S, extTS, extMP4, extMP:
		return true
	case extM3U8:
		return u.Repository != nil && strings.HasSuffix(base, "_stream.m3u8")
	}
	return false
}

// sendToTaskChan sends filePath to the upload worker queue without blocking.
// When the queue is full the oldest queued path is dropped, so the number of
// goroutines and queued items stays bounded. Dropped playlists are re-sent by
// the next write; a dropped segment is retried by the final scan on close.
func (u *HLSS3Uploader) sendToTaskChan(filePath string) {
	u.muClose.RLock()
	defer u.muClose.RUnlock()
	if u.isClosed {
		return
	}
	for i := 0; i < 2; i++ {
		select {
		case u.taskChan <- filePath:
			return
		default:
		}
		select {
		case dropped := <-u.taskChan:
			u.Log(logger.Warn, "upload queue full, dropping oldest entry %s", filepath.Base(dropped))
		default:
		}
	}
}

func (u *HLSS3Uploader) handleLocalRemove(filePath string) {
	if u.Repository == nil {
		return
	}

	if filepath.Clean(filepath.Dir(filePath)) != filepath.Clean(u.Config.Directory) {
		return
	}

	streamName := strings.Trim(strings.TrimSpace(u.Config.StreamName), "/")
	if streamName == "" {
		streamName = filepath.Base(u.Config.Directory)
	}

	segmentName := filepath.Base(filePath)
	_ = u.Repository.MarkLocalDeleted(streamName, segmentName, time.Now())
}

func (u *HLSS3Uploader) workerLoop() {
	defer u.wg.Done()

	for {
		select {
		case <-u.ctx.Done():
			return
		case filePath, ok := <-u.taskChan:
			if !ok {
				return
			}
			u.processFile(filePath)
		}
	}
}

func (u *HLSS3Uploader) processFile(filePath string) {
	if filepath.Clean(filepath.Dir(filePath)) != filepath.Clean(u.Config.Directory) {
		return
	}

	relPath, err := filepath.Rel(u.Config.Directory, filePath)
	if err != nil {
		relPath = filepath.Base(filePath)
	}
	relPath = filepath.ToSlash(relPath)

	ext := strings.ToLower(filepath.Ext(filePath))
	isSegment := ext == ".m4s" || ext == ".ts" || ext == ".mp4" || ext == ".mp"

	streamName := strings.Trim(strings.TrimSpace(u.Config.StreamName), "/")
	keyRelPath := relPath
	if streamName != "" && !strings.HasPrefix(keyRelPath, streamName+"/") {
		keyRelPath = streamName + "/" + keyRelPath
	}
	remoteKey := fmt.Sprintf("%s/%s", strings.TrimSuffix(u.Config.Prefix, "/"), keyRelPath)

	// Segments: deduplicate — never re-upload a successfully completed segment.
	if isSegment {
		if strings.Contains(filepath.Base(filePath), "_part") {
			// Skip uploading LL-HLS parts. The full segment will be uploaded instead.
			return
		}
		if _, exists := u.uploadedFiles.Load(remoteKey); exists {
			return
		}
		// Init files: consult the DB too so reconnects do not re-upload.
		if isInitSegmentFile(filepath.Base(filePath)) && u.IsUploaded(remoteKey) {
			u.uploadedFiles.Store(remoteKey, true)
			return
		}
	}

	// Prevent concurrent in-flight uploads for the same remote key.
	if _, loaded := u.processingFiles.LoadOrStore(remoteKey, true); loaded {
		return
	}
	defer u.processingFiles.Delete(remoteKey)

	if !u.waitStable(filePath) {
		return
	}

	// Playlists are not uploaded: nothing reads them from the provider, the
	// muxer serves them locally and DVR builds playlists from the DB.
	if ext == extM3U8 {
		if strings.HasSuffix(filepath.Base(filePath), "_stream.m3u8") && u.Repository != nil {
			u.ingestFMP4Playlist(filePath, streamName)
		}
		return
	}

	info, err := os.Stat(filePath)
	if err != nil || info.Size() == 0 {
		return
	}

	durationMS := u.inferDurationMS(filePath)
	if isInitSegmentFile(filepath.Base(filePath)) {
		durationMS = 0
	} else if durationMS <= 0 {
		durationMS = 2000 // default 2s segment duration per spec
	}
	duration := time.Duration(durationMS) * time.Millisecond
	startedAt := info.ModTime().Add(-duration)
	if duration <= 0 {
		startedAt = info.ModTime()
	}

	contentType := getContentType(ext)
	segmentRecord := models.LiveHLSSegment{
		StreamID:       streamName,
		SegmentName:    filepath.Base(filePath),
		LocalPath:      filePath,
		StorageBackend: u.provider.Name(),
		StorageKey:     remoteKey,
		StartedAt:      startedAt,
		DurationMS:     durationMS,
		SizeBytes:      info.Size(),
		ContentType:    contentType,
	}
	if segmentRecord.StreamID == "" {
		segmentRecord.StreamID = filepath.Base(u.Config.Directory)
	}
	populateABRMetadata(&segmentRecord, filepath.Base(filePath))
	if isSegment && u.Repository != nil {
		mergeExistingDVRMetadata(u.Repository, &segmentRecord)
	}
	if isSegment && u.Repository != nil {
		_ = u.Repository.UpsertLocal(segmentRecord)
		_ = u.Repository.UpsertUploading(segmentRecord)
	}

	uploadCtx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	start := time.Now()
	etag, err := u.provider.UploadFile(uploadCtx, filePath, remoteKey, contentType)
	cancel()

	if err != nil {
		if isSegment && u.Repository != nil {
			_ = u.Repository.UpsertUploadFailed(segmentRecord)
		}
		u.Log(logger.Warn, "failed to upload %s via provider %s (key: %s, streamKey: %s): %v", relPath, u.provider.Name(), remoteKey, u.Config.StreamKey, err)
		return
	}

	if isSegment {
		now := time.Now()
		segmentRecord.StorageETag = etag
		segmentRecord.UploadedAt = &now
		if u.Repository != nil {
			_ = u.Repository.UpsertUploaded(segmentRecord)
		}
		u.uploadedFiles.Store(remoteKey, true)

		if waiter, ok := u.provider.(ReadinessWaiter); ok && u.Repository != nil {
			u.startReadinessPoller(waiter, segmentRecord.StreamID, segmentRecord.SegmentName, etag, start)
		}
	}
}

func (u *HLSS3Uploader) startReadinessPoller(waiter ReadinessWaiter, streamID, segmentName, etag string, start time.Time) {
	u.pollersMu.Lock()
	if u.pollersClosed {
		u.pollersMu.Unlock()
		return
	}
	if u.pollersSem == nil {
		maxPollers := u.MaxReadinessPollers
		if maxPollers <= 0 {
			maxPollers = defaultMaxReadinessPollers
		}
		u.pollersSem = make(chan struct{}, maxPollers)
	}
	select {
	case u.pollersSem <- struct{}{}:
	default:
		u.pollersMu.Unlock()
		u.Log(logger.Warn, "readiness poller saturated, dropping measurement for %s/%s", streamID, segmentName)
		return
	}
	if u.pollersCtx == nil {
		baseCtx := u.ctx
		if baseCtx == nil {
			baseCtx = context.Background()
		}
		u.pollersCtx, u.pollersCancel = context.WithCancel(baseCtx)
	}
	pollerCtx := u.pollersCtx
	u.pollersWg.Add(1)
	u.pollersMu.Unlock()

	go func() {
		defer func() {
			<-u.pollersSem
			u.pollersWg.Done()
			if r := recover(); r != nil {
				u.Log(logger.Warn, "readiness poller panic for %s/%s: %v", streamID, segmentName, r)
			}
		}()

		ctx, cancel := context.WithTimeout(pollerCtx, 2*time.Minute)
		defer cancel()

		if err := waiter.WaitReady(ctx, etag); err != nil {
			u.Log(logger.Warn, "readiness poller failed for %s/%s (etag: %s): %v", streamID, segmentName, etag, err)
			return
		}
		elapsedMS := time.Since(start).Milliseconds()
		if err := u.Repository.SetReadyMS(ctx, streamID, segmentName, elapsedMS); err != nil {
			u.Log(logger.Warn, "failed to record ready_ms for %s/%s: %v", streamID, segmentName, err)
		}
	}()
}


func getContentType(ext string) string {
	switch ext {
	case extM4S:
		return contentTypeM4S
	case extMP4, extMP:
		return contentTypeMP4
	case extTS:
		return contentTypeTS
	case extM3U8:
		return contentTypeM3U8
	default:
		return contentTypeDefault
	}
}

var (
	extinfRe        = regexp.MustCompile(`^#EXTINF:([0-9.]+)`)                               // #EXTINF:1.000,
	partRe          = regexp.MustCompile(`^#EXT-X-PART:.*DURATION=([0-9.]+).*URI="([^"]+)"`) // LL-HLS parts
	mapURIRe        = regexp.MustCompile(`^#EXT-X-MAP:.*URI="?([^",]+)"?`)                   // init segment
	mediaSequenceRe = regexp.MustCompile(`^#EXT-X-MEDIA-SEQUENCE:([0-9]+)`)                  // media sequence
	pdtRe           = regexp.MustCompile(`^#EXT-X-PROGRAM-DATE-TIME:(.+)$`)
	fmp4SegmentRe   = regexp.MustCompile(`_seg[0-9]+\.mp4$`)
)

func isInitSegmentFile(name string) bool {
	return strings.HasSuffix(strings.ToLower(name), "_init.mp4")
}

func (u *HLSS3Uploader) ingestFMP4Playlist(playlistPath string, streamName string) {
	entries, initName := parseFMP4Playlist(playlistPath)
	if len(entries) == 0 {
		return
	}
	playlistName := filepath.Base(playlistPath)
	childToken := strings.TrimSuffix(playlistName, "_stream.m3u8")
	if streamName == "" {
		streamName = filepath.Base(u.Config.Directory)
	}
	initStorageKey := ""
	if initName != "" {
		initStorageKey = u.remoteKeyForName(streamName, initName)
	}
	storageBackend := "local"
	if u.provider != nil {
		storageBackend = u.provider.Name()
	}
	for _, entry := range entries {
		segName := filepath.Base(entry.URI)
		localPath := filepath.Join(filepath.Dir(playlistPath), segName)
		size := int64(0)
		startedAt := entry.ProgramDateTime
		if startedAt.IsZero() {
			if existing, err := u.Repository.GetByStreamAndSegment(streamName, segName); err == nil && existing != nil {
				startedAt = existing.StartedAt
			}
		}
		if info, err := os.Stat(localPath); err == nil {
			size = info.Size()
			if startedAt.IsZero() && entry.DurationMS > 0 {
				startedAt = info.ModTime().Add(-time.Duration(entry.DurationMS) * time.Millisecond)
			}
		}
		sequence := entry.MediaSequence
		segment := models.LiveHLSSegment{
			StreamID:        streamName,
			SegmentName:     segName,
			LocalPath:       localPath,
			StorageBackend:  storageBackend,
			StorageKey:      u.remoteKeyForName(streamName, segName),
			StartedAt:       startedAt,
			DurationMS:      entry.DurationMS,
			SizeBytes:       size,
			ContentType:     getContentType(strings.ToLower(filepath.Ext(segName))),
			MediaSequence:   &sequence,
			InitSegmentName: initName,
			InitStorageKey:  initStorageKey,
			PlaylistName:    playlistName,
			MuxSessionID:    deriveMuxSessionID(segName, childToken),
		}
		populateABRMetadata(&segment, segName)
		if existing, err := u.Repository.GetByStreamAndSegment(streamName, segName); err == nil && existing != nil && existing.Status == models.LiveHLSSegmentStatusUploadedS3 {
			segment.StorageETag = existing.StorageETag
			segment.UploadedAt = existing.UploadedAt
			_ = u.Repository.UpsertUploaded(segment)
		} else {
			_ = u.Repository.UpsertLocal(segment)
		}
	}
}

type fmp4PlaylistEntry struct {
	URI             string
	DurationMS      int64
	MediaSequence   int64
	ProgramDateTime time.Time
}

func parseFMP4Playlist(playlistPath string) ([]fmp4PlaylistEntry, string) {
	byts, err := os.ReadFile(playlistPath)
	if err != nil {
		return nil, ""
	}
	var entries []fmp4PlaylistEntry
	var initName string
	mediaSequence := int64(0)
	pendingDuration := int64(0)
	var pendingPDT time.Time
	for _, line := range strings.Split(string(byts), "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		if matches := mediaSequenceRe.FindStringSubmatch(line); matches != nil {
			mediaSequence, _ = strconv.ParseInt(matches[1], 10, 64)
			continue
		}
		if matches := mapURIRe.FindStringSubmatch(line); matches != nil {
			initName = filepath.Base(matches[1])
			continue
		}
		if matches := pdtRe.FindStringSubmatch(line); matches != nil {
			pendingPDT, _ = time.Parse(time.RFC3339Nano, strings.TrimSpace(matches[1]))
			continue
		}
		if matches := extinfRe.FindStringSubmatch(line); matches != nil {
			pendingDuration = secondsToMS(matches[1])
			continue
		}
		if strings.HasPrefix(line, "#") {
			continue
		}
		entries = append(entries, fmp4PlaylistEntry{
			URI:             line,
			DurationMS:      pendingDuration,
			MediaSequence:   mediaSequence + int64(len(entries)),
			ProgramDateTime: pendingPDT,
		})
		pendingDuration = 0
		pendingPDT = time.Time{}
	}
	return entries, initName
}

func (u *HLSS3Uploader) remoteKeyForName(streamName, name string) string {
	keyRelPath := path.Join(strings.Trim(streamName, "/"), name)
	return path.Join(strings.TrimSuffix(u.Config.Prefix, "/"), keyRelPath)
}

// cleanupStreamName is the stream key of the cleanup row (the DePIN contract key).
func (u *HLSS3Uploader) cleanupStreamName() string {
	return depinCacheKey(u.Config.StreamName)
}

func (u *HLSS3Uploader) scheduleRemoteCleanup() {
	if !cleanupEnabled(u.Config) {
		u.Log(logger.Info, "remote HLS cleanup disabled, keeping %s", u.remoteFolderPrefix())
		return
	}
	if u.cleanupRepo == nil {
		u.Log(logger.Warn, "no database: remote HLS folder %s is not scheduled for cleanup", u.remoteFolderPrefix())
		return
	}
	now := time.Now()
	err := u.cleanupRepo.MarkPendingDelete(u.provider.Name(), u.cleanupStreamName(), u.remoteFolderPrefix(),
		now, now.Add(cleanupGracePeriod(u.Config)))
	if err != nil {
		u.Log(logger.Warn, "failed to schedule remote HLS cleanup: %v", err)
	}
}

func (u *HLSS3Uploader) remoteFolderPrefix() string {
	prefix := strings.TrimSuffix(u.Config.Prefix, "/")
	if prefix == "" {
		prefix = defaultStoragePrefix
	}
	streamName := strings.Trim(u.Config.StreamName, "/")
	if streamName == "" {
		streamName = filepath.Base(u.Config.Directory)
	}
	return path.Join(prefix, streamName)
}

func populateABRMetadata(segment *models.LiveHLSSegment, fileName string) {
	base, track, rendition := deriveABRPathMetadata(segment.StreamID)
	// A rendition's directory holds the video and the audio files of the same
	// muxer (<mux>_video1_seg0.mp4, <mux>_audio2_seg0.mp4). The path names only
	// the rendition, so the track comes from the file: otherwise every audio
	// segment is stored as video and doubles the duration and segment count.
	if track == "video" && isAudioSegmentFile(fileName) {
		track = "audio"
	}
	segment.BaseStreamID = base
	segment.TrackType = track
	segment.Rendition = rendition
	if segment.MuxSessionID == "" {
		childToken := strings.TrimSuffix(segment.PlaylistName, "_stream.m3u8")
		segment.MuxSessionID = deriveMuxSessionID(fileName, childToken)
	}
}

func mergeExistingDVRMetadata(repo models.LiveHLSSegmentRepository, segment *models.LiveHLSSegment) {
	if repo == nil || segment == nil {
		return
	}
	existing, err := repo.GetByStreamAndSegment(segment.StreamID, segment.SegmentName)
	if err != nil || existing == nil {
		return
	}
	if segment.BaseStreamID == "" {
		segment.BaseStreamID = existing.BaseStreamID
	}
	if segment.TrackType == "" || segment.TrackType == "unknown" {
		segment.TrackType = existing.TrackType
	}
	if segment.Rendition == "" {
		segment.Rendition = existing.Rendition
	}
	if segment.MuxSessionID == "" || strings.Contains(segment.MuxSessionID, "_seg") {
		segment.MuxSessionID = existing.MuxSessionID
	}
	if segment.InitSegmentName == "" {
		segment.InitSegmentName = existing.InitSegmentName
	}
	if segment.InitStorageKey == "" {
		segment.InitStorageKey = existing.InitStorageKey
	}
	if segment.PlaylistName == "" {
		segment.PlaylistName = existing.PlaylistName
	}
	if segment.MediaSequence == nil {
		segment.MediaSequence = existing.MediaSequence
	}
	if segment.StartedAt.IsZero() {
		segment.StartedAt = existing.StartedAt
	}
	if segment.DurationMS <= 0 {
		segment.DurationMS = existing.DurationMS
	}
}

// audioFileRe matches the muxer's audio files: <mux>_audio<N>_init.mp4 and
// <mux>_audio<N>_seg<N>.mp4.
var audioFileRe = regexp.MustCompile(`(^|_)audio[0-9]*_`)

func isAudioSegmentFile(fileName string) bool {
	return audioFileRe.MatchString(filepath.Base(fileName))
}

func deriveABRPathMetadata(streamID string) (string, string, string) {
	if idx := strings.LastIndex(streamID, "/video/"); idx >= 0 {
		return streamID[:idx], "video", streamID[idx+len("/video/"):]
	}
	if idx := strings.LastIndex(streamID, "/audio/"); idx >= 0 {
		return streamID[:idx], "audio", streamID[idx+len("/audio/"):]
	}
	if idx := strings.LastIndex(streamID, "/"); idx >= 0 {
		rendition := streamID[idx+1:]
		if rendition == "original" {
			return streamID[:idx], "video", rendition
		}
		if _, err := strconv.Atoi(strings.TrimSuffix(rendition, "p")); err == nil {
			return streamID[:idx], "video", rendition
		}
	}
	return streamID, "unknown", ""
}

func deriveMuxSessionID(fileName, childToken string) string {
	base := strings.TrimSuffix(fileName, filepath.Ext(fileName))
	if childToken != "" {
		for _, suffix := range []string{"_" + childToken + "_init", "_" + childToken} {
			if strings.HasSuffix(base, suffix) {
				return strings.TrimSuffix(base, suffix)
			}
		}
		segSuffix := regexp.MustCompile(`_` + regexp.QuoteMeta(childToken) + `_seg[0-9]+$`)
		if loc := segSuffix.FindStringIndex(base); loc != nil && loc[1] == len(base) {
			return base[:loc[0]]
		}
	}
	if loc := fmp4SegmentRe.FindStringIndex(fileName); loc != nil && loc[1] == len(fileName) {
		return fileName[:loc[0]]
	}
	return base
}

func (u *HLSS3Uploader) inferDurationMS(filePath string) int64 {
	playlistPath := filepath.Join(filepath.Dir(filePath), "index.m3u8")
	if strings.HasSuffix(filepath.Base(filePath), ".mp4") {
		matches, _ := filepath.Glob(filepath.Join(filepath.Dir(filePath), "*_stream.m3u8"))
		for _, candidate := range matches {
			if duration := inferDurationFromPlaylist(candidate, filepath.Base(filePath)); duration > 0 {
				return duration
			}
		}
	}
	return inferDurationFromPlaylist(playlistPath, filepath.Base(filePath))
}

func inferDurationFromPlaylist(playlistPath string, name string) int64 {
	byts, err := os.ReadFile(playlistPath)
	if err != nil {
		return 0
	}

	lines := strings.Split(string(byts), "\n")
	var pendingDuration int64
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}

		if matches := partRe.FindStringSubmatch(line); matches != nil {
			if matches[2] == name {
				return secondsToMS(matches[1])
			}
			continue
		}

		if matches := extinfRe.FindStringSubmatch(line); matches != nil {
			pendingDuration = secondsToMS(matches[1])
			continue
		}

		if line == name || filepath.Base(line) == name {
			return pendingDuration
		}
	}

	return 0
}

func secondsToMS(raw string) int64 {
	seconds, err := strconv.ParseFloat(strings.TrimRight(raw, ","), 64)
	if err != nil {
		return 0
	}
	return int64(seconds * 1000)
}

// waitStable polls the file until its size stops changing, ensuring it is fully written.
// A bounded timeout of 5s guarantees workers will not deadlock on stalled or empty files.
func (u *HLSS3Uploader) waitStable(filePath string) bool {
	var lastSize int64 = -1
	ticker := time.NewTicker(200 * time.Millisecond)
	defer ticker.Stop()
	timeout := time.After(5 * time.Second)

	var ctxDone <-chan struct{}
	if u.ctx != nil {
		ctxDone = u.ctx.Done()
	}

	for {
		select {
		case <-ctxDone:
			return false
		case <-timeout:
			// Ensure worker never deadlocks on 0-byte or stalled files
			info, err := os.Stat(filePath)
			if err == nil && info.Size() > 0 {
				return true
			}
			return false
		case <-ticker.C:
			info, err := os.Stat(filePath)
			if err != nil {
				if os.IsNotExist(err) {
					return false
				}
				continue
			}

			if info.Size() == lastSize && lastSize > 0 {
				return true
			}
			lastSize = info.Size()
		}
	}
}

// BuildRemoteKey constructs the S3 storage key for a given stream path and file name
// using the configured prefix, ensuring consistency between upload and lookup.
func (u *HLSS3Uploader) BuildRemoteKey(streamPath, fileName string) string {
	prefix := strings.TrimSuffix(u.Config.Prefix, "/")
	if prefix == "" {
		prefix = "live-hls"
	}
	return path.Join(prefix, streamPath, fileName)
}

func (u *HLSS3Uploader) IsUploaded(remoteKey string) bool {
	if _, ok := u.uploadedFiles.Load(remoteKey); ok {
		return true
	}
	if u.Repository == nil {
		return false
	}
	segment, err := u.Repository.GetByStorageKey(remoteKey)
	if err != nil {
		return false
	}
	return segment.Status == models.LiveHLSSegmentStatusUploadedS3
}

func (u *HLSS3Uploader) Presign(remoteKey string) (string, error) {
	if linkProv, ok := u.provider.(LinkStorageProvider); ok {
		return linkProv.GetLink(context.Background(), remoteKey)
	}
	return "", fmt.Errorf("provider does not support presign or link generation")
}

func contentTypeForRemoteKey(remoteKey string) string {
	switch strings.ToLower(path.Ext(remoteKey)) {
	case extM3U8:
		return contentTypeM3U8
	case extMP4, extMP:
		return contentTypeMP4
	case extM4S:
		return contentTypeM4S
	case extTS:
		return contentTypeTS
	default:
		return contentTypeDefault
	}
}

func (u *HLSS3Uploader) ProxyObject(ctx context.Context, w http.ResponseWriter, remoteKey string) bool {
	readable, ok := u.provider.(ReadableStorageProvider)
	if !ok {
		return false
	}
	if ctx == nil {
		ctx = context.Background()
	}

	body, contentType, contentLength, err := readable.GetObject(ctx, remoteKey)
	if err != nil {
		return false
	}
	defer body.Close()

	if contentType != "" {
		w.Header().Set("Content-Type", contentType)
	} else {
		w.Header().Set("Content-Type", contentTypeForRemoteKey(remoteKey))
	}
	if contentLength >= 0 {
		w.Header().Set("Content-Length", strconv.FormatInt(contentLength, 10))
	}

	if strings.HasSuffix(remoteKey, extM3U8) {
		w.Header().Set("Cache-Control", cacheControlNoCache)
	} else {
		w.Header().Set("Cache-Control", cacheControlImmutableSegment)
	}

	w.WriteHeader(http.StatusOK)
	_, _ = io.Copy(w, body)
	return true
}
