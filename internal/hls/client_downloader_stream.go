package hls

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"time"

	"github.com/aler9/gortsplib"
	gm3u8 "github.com/grafov/m3u8"

	"github.com/aler9/rtsp-simple-server/internal/hls/m3u8"
	"github.com/aler9/rtsp-simple-server/internal/logger"
)

type clientDownloaderStream struct {
	httpClient      *http.Client
	playlistURL     *url.URL
	initialPlaylist *m3u8.MediaPlaylist
	logger          ClientLogger
	rp              *clientRoutinePool
	onStreamTracks  func(context.Context, []gortsplib.Track)
	onVideoData     func(time.Duration, [][]byte)
	onAudioData     func(time.Duration, []byte)

	segmentQueue *clientSegmentQueue
	curSegmentID *uint64
}

func newClientDownloaderStream(
	httpClient *http.Client,
	playlistURL *url.URL,
	initialPlaylist *m3u8.MediaPlaylist,
	logger ClientLogger,
	rp *clientRoutinePool,
	onStreamTracks func(context.Context, []gortsplib.Track),
	onVideoData func(time.Duration, [][]byte),
	onAudioData func(time.Duration, []byte),
) *clientDownloaderStream {
	return &clientDownloaderStream{
		httpClient:      httpClient,
		playlistURL:     playlistURL,
		initialPlaylist: initialPlaylist,
		logger:          logger,
		rp:              rp,
		segmentQueue:    newClientSegmentQueue(),
		onStreamTracks:  onStreamTracks,
		onVideoData:     onVideoData,
		onAudioData:     onAudioData,
	}
}

func (d *clientDownloaderStream) run(ctx context.Context) error {
	initialPlaylist := d.initialPlaylist
	d.initialPlaylist = nil
	if initialPlaylist == nil {
		var err error
		initialPlaylist, err = d.downloadPlaylist(ctx)
		if err != nil {
			return err
		}
	}

	if initialPlaylist.Map != nil && initialPlaylist.Map.URI != "" {
		byts, err := d.downloadSegment(ctx, initialPlaylist.Map.URI)
		if err != nil {
			return err
		}

		proc, err := newClientProcessorFMP4(
			ctx,
			byts,
			d.segmentQueue,
			d.logger,
			d.rp,
			d.onStreamTracks,
			d.onVideoData,
			d.onAudioData,
		)
		if err != nil {
			return err
		}

		d.rp.add(proc)
	} else {
		proc := newClientProcessorMPEGTS(
			d.segmentQueue,
			d.logger,
			d.rp,
			d.onStreamTracks,
			d.onVideoData,
			d.onAudioData,
		)
		d.rp.add(proc)
	}

	for {
		ok := d.segmentQueue.waitUntilSizeIsBelow(ctx, 1)
		if !ok {
			return fmt.Errorf("terminated")
		}

		err := d.fillSegmentQueue(ctx)
		if err != nil {
			return err
		}
	}
}

func (d *clientDownloaderStream) downloadPlaylist(ctx context.Context) (*m3u8.MediaPlaylist, error) {
	d.logger.Log(logger.Debug, "downloading stream playlist %s", d.playlistURL.String())

	pl, err := clientDownloadPlaylist(ctx, d.httpClient, d.playlistURL)
	if err != nil {
		return nil, err
	}

	plt, ok := pl.(*m3u8.MediaPlaylist)
	if !ok {
		return nil, fmt.Errorf("invalid playlist")
	}

	return plt, nil
}

func (d *clientDownloaderStream) downloadSegment(ctx context.Context, segmentURI string) ([]byte, error) {
	u, err := clientAbsoluteURL(d.playlistURL, segmentURI)
	if err != nil {
		return nil, err
	}

	d.logger.Log(logger.Debug, "downloading segment %s", u)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u.String(), nil)
	if err != nil {
		return nil, err
	}

	res, err := d.httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer res.Body.Close()

	if res.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("bad status code: %d", res.StatusCode)
	}

	byts, err := io.ReadAll(res.Body)
	if err != nil {
		return nil, err
	}

	return byts, nil
}

func (d *clientDownloaderStream) fillSegmentQueue(ctx context.Context) error {
	pl, err := d.downloadPlaylist(ctx)
	if err != nil {
		return err
	}

	pl.Segments = pl.Segments[:segmentsLen(pl.Segments)]
	var seg *gm3u8.MediaSegment

	if d.curSegmentID == nil {
		if !pl.Closed { // live stream: start from clientLiveStartingPoint
			seg = findStartingSegment(pl.Segments)
			if seg == nil {
				return fmt.Errorf("there aren't enough segments to fill the buffer")
			}
		} else { // VOD stream: start from beginning
			if len(pl.Segments) == 0 {
				return fmt.Errorf("no segments found")
			}
			seg = pl.Segments[0]
		}
	} else {
		seg = findSegmentWithID(pl.SeqNo, pl.Segments, *d.curSegmentID+1)
		if seg == nil {
			if !pl.Closed { // live stream
				d.logger.Log(logger.Warn, "resetting segment ID")
				seg = findStartingSegment(pl.Segments)
				if seg == nil {
					return fmt.Errorf("there aren't enough segments to fill the buffer")
				}
			} else { // VOD stream
				return fmt.Errorf("following segment not found")
			}
		}
	}

	v := seg.SeqId
	d.curSegmentID = &v

	byts, err := d.downloadSegment(ctx, seg.URI)
	if err != nil {
		return err
	}

	d.segmentQueue.push(byts)

	if pl.Closed && pl.Segments[len(pl.Segments)-1] == seg {
		<-ctx.Done()
		return fmt.Errorf("stream has ended")
	}

	return nil
}
