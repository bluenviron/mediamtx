package hls

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"path"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/bluenviron/gohlslib/v2"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"

	"github.com/bluenviron/mediamtx/internal/conf"
	"github.com/bluenviron/mediamtx/internal/defs"
	"github.com/bluenviron/mediamtx/internal/hlss3uploader"
	"github.com/bluenviron/mediamtx/internal/logger"
	"github.com/bluenviron/mediamtx/internal/models"
	"github.com/bluenviron/mediamtx/internal/protocols/hls"
	"github.com/bluenviron/mediamtx/internal/stream"
	"github.com/bluenviron/mediamtx/internal/watchusage"
)

const (
	sessionCookieName     = "hlsSession"
	sessionQueryParamName = "session"
	sessionCloseAfter     = 30 * time.Second
	sessionCleanupPeriod  = sessionCloseAfter / 3
)

// this prevents directory traversal.
// functionally it's useless since there's already conf.IsValidPathName, but it's needed by CodeQL.
func absolutePathInside(base string, candidate string) (string, error) {
	baseAbs, err := filepath.Abs(filepath.Clean(base))
	if err != nil {
		return "", err
	}

	candidateAbs, err := filepath.Abs(filepath.Clean(candidate))
	if err != nil {
		return "", err
	}

	if !strings.HasPrefix(candidateAbs, baseAbs) {
		return "", fmt.Errorf("path escapes base directory")
	}

	return candidateAbs, nil
}

// removeMuxerLeftovers deletes the init files, playlists and segment files
// gohlslib leaves in the muxer directory after Close. Each muxer uses a new
// random prefix, so without this they pile up on every reconnect and the
// directory is never empty for os.Remove. Only regular files directly in dir
// are removed; subdirectories are never entered.
func removeMuxerLeftovers(dir string) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return
	}
	for _, e := range entries {
		if !e.Type().IsRegular() {
			continue
		}
		name := strings.ToLower(e.Name())
		switch {
		case strings.HasSuffix(name, "_init.mp4"),
			strings.HasSuffix(name, ".m3u8"),
			strings.HasSuffix(name, ".m4s"),
			strings.HasSuffix(name, ".ts"),
			strings.HasSuffix(name, ".mp4"):
			os.Remove(filepath.Join(dir, e.Name()))
		}
	}
}

// removeMuxerDirectory deletes the muxer's leftover files and its directory,
// then every parent directory that is now empty, up to but excluding root
// (hlsDirectory). An ABR stream keeps renditions in <root>/<id>/<rendition>;
// the base muxer in <root>/<id> usually closes first, while rendition muxers
// are still flushing uploads, so its own os.Remove fails on a non-empty
// directory. The last muxer to finish removes <root>/<id>. os.Remove never
// deletes a non-empty directory, so a parent still in use is left alone.
func removeMuxerDirectory(root string, dir string) {
	removeMuxerLeftovers(dir)

	rootAbs, err := filepath.Abs(root)
	if err != nil {
		return
	}
	cur, err := filepath.Abs(dir)
	if err != nil {
		return
	}

	for {
		rel, relErr := filepath.Rel(rootAbs, cur)
		if relErr != nil || rel == "." || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
			return
		}
		if os.Remove(cur) != nil {
			return
		}
		cur = filepath.Dir(cur)
	}
}

type instanceParent interface {
	logger.Writer
	closeInstance(*muxerInstance, error)
}

type muxerInstance struct {
	variant                  conf.HLSVariant
	segmentCount             int
	segmentDuration          conf.Duration
	partDuration             conf.Duration
	segmentMaxSize           conf.StringSize
	directory                string
	uploadConfig             *MuxerUploadConfig
	pathConf                 *conf.Path
	hlsTranscodingRenditions []conf.HLSTranscodingRendition
	pathName                 string
	streamKey                string
	bytesSent                *atomic.Uint64
	wg                       *sync.WaitGroup
	stream                   *stream.Stream
	server                   logger.Writer
	parent                   instanceParent

	ctx       context.Context
	ctxCancel func()
	hmuxer    *gohlslib.Muxer
	reader    *stream.Reader
	uploader  *hlss3uploader.HLSS3Uploader

	watchCounter WatchCounter
	segmentRepo  models.LiveHLSSegmentRepository
}

func isOriginalPath(pathName string) bool {
	return strings.HasSuffix(pathName, "/original") ||
		strings.HasSuffix(pathName, "/video/original") ||
		strings.HasSuffix(pathName, "/audio/original")
}

// effectiveVariant is the variant of this muxer.
//
// A transcoded stream plays through its ABR master playlist, so the source and
// every rendition are variants of one playlist and share one variant. That is
// never low-latency: a low-latency muxer opens every playlist with EXT-X-GAP
// segments (gap.mp4, which iOS LL-HLS needs), and video.js does not support
// them - it requests gap.mp4, gets 404 and stops.
func (mi *muxerInstance) effectiveVariant() conf.HLSVariant {
	if mi.pathConf != nil && mi.pathConf.HLSTranscoding {
		if mi.variant == conf.HLSVariant(gohlslib.MuxerVariantMPEGTS) {
			return conf.HLSVariant(gohlslib.MuxerVariantMPEGTS)
		}
		return conf.HLSVariant(gohlslib.MuxerVariantFMP4)
	}
	return mi.variant
}

func (mi *muxerInstance) shouldUploadToProvider() bool {
	if mi.pathConf == nil || !mi.pathConf.HLSTranscoding {
		return true
	}
	if !isABROutputPath(mi.pathName) {
		return false
	}
	return !isOriginalPath(mi.pathName)
}

func (mi *muxerInstance) initialize() error {
	mi.variant = mi.effectiveVariant()
	mi.Log(logger.Debug, "instance created")

	var muxerDirectory string

	if mi.directory != "" {
		var err error
		muxerDirectory, err = absolutePathInside(mi.directory, filepath.Join(mi.directory, mi.pathName))
		if err != nil {
			return err
		}

		err = os.MkdirAll(muxerDirectory, 0o755)
		if err != nil {
			return err
		}
	}

	if muxerDirectory != "" && mi.uploadConfig != nil && mi.shouldUploadToProvider() {
		mi.uploader = mi.uploadConfig.NewUploader(muxerDirectory, mi.pathName, mi.streamKey, mi)
		if err := mi.uploader.Initialize(); err != nil {
			mi.Log(logger.Warn, "failed to initialize HLS uploader: %v", err)
			mi.uploader = nil
		}
	}

	mi.hmuxer = &gohlslib.Muxer{
		Variant:            gohlslib.MuxerVariant(mi.variant),
		SegmentCount:       mi.segmentCount,
		SegmentMinDuration: time.Duration(mi.segmentDuration),
		PartMinDuration:    time.Duration(mi.partDuration),
		SegmentMaxSize:     uint64(mi.segmentMaxSize),
		Directory:          muxerDirectory,
		OnEncodeError: func(err error) {
			mi.Log(logger.Warn, err.Error())
		},
	}

	mi.reader = &stream.Reader{
		SkipOutboundBytes: true,
		Parent:            mi,
	}

	err := hls.FromStream(
		mi.stream.OrigDesc,
		mi.stream.OutDescCopy(),
		mi.reader,
		mi.hmuxer)
	if err != nil {
		return err
	}

	err = mi.hmuxer.Start()
	if err != nil {
		return err
	}

	mi.Log(logger.Info, "is converting into HLS, %s",
		defs.FormatsInfo(mi.reader.Formats()))

	mi.stream.AddReader(mi.reader)

	mi.ctx, mi.ctxCancel = context.WithCancel(context.Background())

	mi.wg.Add(1)
	go mi.run()

	return nil
}

// Log implements logger.Writer.
func (mi *muxerInstance) Log(level logger.Level, format string, args ...any) {
	mi.parent.Log(level, format, args...)
}

func (mi *muxerInstance) close() {
	mi.ctxCancel()
}

func (mi *muxerInstance) run() {
	defer mi.wg.Done()

	err := mi.runInner()

	mi.ctxCancel()

	mi.stream.RemoveReader(mi.reader)

	mi.hmuxer.Close()

	if mi.uploader != nil {
		mi.uploader.FlushAndClose()
		mi.uploader = nil
	}

	if mi.hmuxer.Directory != "" {
		removeMuxerDirectory(mi.directory, mi.hmuxer.Directory)
	}

	mi.Log(logger.Debug, "instance destroyed: %v", err)

	mi.parent.closeInstance(mi, err)
}

func (mi *muxerInstance) runInner() error {
	for {
		select {
		case <-mi.ctx.Done():
			return fmt.Errorf("terminated")

		case err := <-mi.reader.Error():
			return err
		}
	}
}

func (mi *muxerInstance) localSegmentAvailable(fileName string) bool {
	if mi.directory == "" || mi.variant == conf.HLSVariant(gohlslib.MuxerVariantLowLatency) {
		return true
	}
	localPath := filepath.Join(mi.directory, mi.pathName, fileName)
	_, err := os.Stat(localPath)
	return !os.IsNotExist(err)
}

func isHLSSegment(fileName string) bool {
	return strings.HasSuffix(fileName, ".mp4") ||
		strings.HasSuffix(fileName, ".ts") ||
		strings.HasSuffix(fileName, ".m4s") ||
		strings.HasSuffix(fileName, ".mp")
}

func (mi *muxerInstance) hasAudio() bool {
	if mi.stream != nil {
		for _, media := range mi.stream.OutDescCopy().Medias {
			for _, forma := range media.Formats {
				switch forma.Codec() {
				case "MPEG-4 Audio", "Opus", "LPCM", "Vorbis":
					return true
				}
			}
		}
	}
	if mi.hmuxer != nil {
		for _, track := range mi.hmuxer.Tracks {
			if !track.Codec.IsVideo() {
				return true
			}
		}
	}
	return false
}

func (mi *muxerInstance) primaryVideoPlaylist() string {
	if mi.directory != "" {
		muxerDir := filepath.Join(mi.directory, mi.pathName)
		matches, _ := filepath.Glob(filepath.Join(muxerDir, "*video*_stream.m3u8"))
		if len(matches) > 0 {
			return filepath.Base(matches[0])
		}
		matches, _ = filepath.Glob(filepath.Join(muxerDir, "*_stream.m3u8"))
		for _, m := range matches {
			base := filepath.Base(m)
			if strings.Contains(base, "video") || strings.Contains(base, "main") {
				return base
			}
		}
		if len(matches) > 0 {
			return filepath.Base(matches[0])
		}
	}
	if mi.variant == conf.HLSVariant(gohlslib.MuxerVariantMPEGTS) {
		return "main_stream.m3u8"
	}
	return "video1_stream.m3u8"
}

func (mi *muxerInstance) isMediaPlaylistReady() bool {
	if mi.directory == "" || mi.variant == conf.HLSVariant(gohlslib.MuxerVariantLowLatency) {
		return true
	}
	muxerDir := filepath.Join(mi.directory, mi.pathName)
	matches, _ := filepath.Glob(filepath.Join(muxerDir, "*_stream.m3u8"))
	return len(matches) > 0
}

func (mi *muxerInstance) handleRequest(ctx *gin.Context, isCDN bool) {
	if mi.variant == conf.HLSVariant(gohlslib.MuxerVariantLowLatency) {
		// Counted like the other variants: the API bills from bytesSent.
		mi.hmuxer.Handle(&responseWriterCounter{
			ResponseWriter: ctx.Writer,
			bytesSent:      mi.bytesSent,
		}, ctx.Request)
		return
	}

	fileName := path.Base(ctx.Request.URL.Path)

	if strings.HasSuffix(fileName, ".m3u8") || strings.HasSuffix(fileName, "_init.mp4") {
		mi.hmuxer.Handle(ctx.Writer, ctx.Request)
		return
	}

	w := ctx.Writer

	if !isCDN {
		w = &responseWriterNoCache{ResponseWriter: w}
	}

	w = &responseWriterCounter{
		ResponseWriter: w,
		bytesSent:      mi.bytesSent,
	}

	if !isHLSSegment(fileName) || mi.localSegmentAvailable(fileName) {
		mi.hmuxer.Handle(w, ctx.Request)
		mi.countWatch(fileName)
		return
	}

	if mi.uploader == nil {
		ctx.Status(http.StatusNotFound)
		return
	}

	remoteKey := mi.uploader.BuildRemoteKey(mi.pathName, fileName)
	if !mi.uploader.IsUploaded(remoteKey) {
		ctx.Status(http.StatusNotFound)
		return
	}

	mi.Log(logger.Info, "[HLS Resolver] Local miss for %s. Proxying from remote.", fileName)
	if mi.uploader.ProxyObject(ctx.Request.Context(), ctx.Writer, remoteKey) {
		mi.countWatch(fileName)
		return
	}

	url, err := mi.uploader.Presign(remoteKey)
	if err != nil {
		ctx.Status(http.StatusBadGateway)
		return
	}
	mi.Log(logger.Info, "[HLS Resolver] Local miss for %s. S3 fallback successful, redirecting to remote.", fileName)
	ctx.Redirect(http.StatusFound, url)
	mi.countWatch(fileName)
}

type WatchCounter interface {
	Add(mediaID uuid.UUID, rendition string, ms int64)
}

// countWatch records the watched duration of a served billable video segment.
func (mi *muxerInstance) countWatch(fileName string) {
	if mi.watchCounter == nil || !isHLSSegment(fileName) ||
		strings.Contains(fileName, "_init") ||
		strings.Contains(fileName, "_part") ||
		strings.Contains(fileName, "_audio") {
		return
	}
	mediaID, rendition, ok := watchusage.RenditionFromPath(mi.pathName)
	if !ok || !watchusage.Billable(rendition) {
		return
	}
	var ms int64
	if mi.segmentRepo != nil {
		if seg, err := mi.segmentRepo.GetByStreamAndSegment(mi.pathName, fileName); err == nil && seg != nil {
			ms = seg.DurationMS
		}
	}
	if ms <= 0 {
		ms = time.Duration(mi.segmentDuration).Milliseconds()
	}
	mi.watchCounter.Add(mediaID, rendition, ms)
}
