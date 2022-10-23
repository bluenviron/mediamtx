package hls

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"time"

	"github.com/aler9/gortsplib"
	gm3u8 "github.com/grafov/m3u8"

	"github.com/aler9/rtsp-simple-server/internal/hls/m3u8"
	"github.com/aler9/rtsp-simple-server/internal/logger"
)

func segmentsLen(segments []*gm3u8.MediaSegment) int {
	for i, seg := range segments {
		if seg == nil {
			return i
		}
	}
	return 0
}

func findSegmentWithInvPosition(segments []*gm3u8.MediaSegment, pos int) *gm3u8.MediaSegment {
	index := len(segments) - pos
	if index < 0 {
		return nil
	}

	return segments[index]
}

func findSegmentWithID(seqNo uint64, segments []*gm3u8.MediaSegment, id uint64) (*gm3u8.MediaSegment, int) {
	index := int(int64(id) - int64(seqNo))
	if (index) >= len(segments) {
		return nil, 0
	}

	return segments[index], len(segments) - index
}

type clientDownloaderStream struct {
	isLeading            bool
	httpClient           *http.Client
	playlistURL          *url.URL
	initialPlaylist      *m3u8.MediaPlaylist
	logger               ClientLogger
	rp                   *clientRoutinePool
	onStreamTracks       func(context.Context, []gortsplib.Track) bool
	onSetLeadingTimeSync func(clientTimeSync)
	onGetLeadingTimeSync func(context.Context) (clientTimeSync, bool)
	onVideoData          func(time.Duration, [][]byte)
	onAudioData          func(time.Duration, []byte)

	curSegmentID *uint64
}

func newClientDownloaderStream(
	isLeading bool,
	httpClient *http.Client,
	playlistURL *url.URL,
	initialPlaylist *m3u8.MediaPlaylist,
	logger ClientLogger,
	rp *clientRoutinePool,
	onStreamTracks func(context.Context, []gortsplib.Track) bool,
	onSetLeadingTimeSync func(clientTimeSync),
	onGetLeadingTimeSync func(context.Context) (clientTimeSync, bool),
	onVideoData func(time.Duration, [][]byte),
	onAudioData func(time.Duration, []byte),
) *clientDownloaderStream {
	return &clientDownloaderStream{
		isLeading:            isLeading,
		httpClient:           httpClient,
		playlistURL:          playlistURL,
		initialPlaylist:      initialPlaylist,
		logger:               logger,
		rp:                   rp,
		onStreamTracks:       onStreamTracks,
		onSetLeadingTimeSync: onSetLeadingTimeSync,
		onGetLeadingTimeSync: onGetLeadingTimeSync,
		onVideoData:          onVideoData,
		onAudioData:          onAudioData,
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

	segmentQueue := newClientSegmentQueue()

	if initialPlaylist.Map != nil && initialPlaylist.Map.URI != "" {
		byts, err := d.downloadSegment(ctx, initialPlaylist.Map.URI, initialPlaylist.Map.Offset, initialPlaylist.Map.Limit)
		if err != nil {
			return err
		}

		proc, err := newClientProcessorFMP4(
			ctx,
			d.isLeading,
			byts,
			segmentQueue,
			d.logger,
			d.rp,
			d.onStreamTracks,
			d.onSetLeadingTimeSync,
			d.onGetLeadingTimeSync,
			d.onVideoData,
			d.onAudioData,
		)
		if err != nil {
			return err
		}

		d.rp.add(proc)
	} else {
		proc := newClientProcessorMPEGTS(
			d.isLeading,
			segmentQueue,
			d.logger,
			d.rp,
			d.onStreamTracks,
			d.onSetLeadingTimeSync,
			d.onGetLeadingTimeSync,
			d.onVideoData,
			d.onAudioData,
		)
		d.rp.add(proc)
	}

	for {
		ok := segmentQueue.waitUntilSizeIsBelow(ctx, 1)
		if !ok {
			return fmt.Errorf("terminated")
		}

		err := d.fillSegmentQueue(ctx, segmentQueue)
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

func (d *clientDownloaderStream) downloadSegment(ctx context.Context,
	uri string, offset int64, limit int64,
) ([]byte, error) {
	u, err := clientAbsoluteURL(d.playlistURL, uri)
	if err != nil {
		return nil, err
	}

	d.logger.Log(logger.Debug, "downloading segment %s", u)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u.String(), nil)
	if err != nil {
		return nil, err
	}

	if limit != 0 {
		req.Header.Add("Range", "bytes="+strconv.FormatInt(offset, 10)+"-"+strconv.FormatInt(offset+limit-1, 10))
	}

	res, err := d.httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer res.Body.Close()

	if res.StatusCode != http.StatusOK && res.StatusCode != http.StatusPartialContent {
		return nil, fmt.Errorf("bad status code: %d", res.StatusCode)
	}

	byts, err := io.ReadAll(res.Body)
	if err != nil {
		return nil, err
	}

	return byts, nil
}

func (d *clientDownloaderStream) fillSegmentQueue(ctx context.Context, segmentQueue *clientSegmentQueue) error {
	pl, err := d.downloadPlaylist(ctx)
	if err != nil {
		return err
	}

	pl.Segments = pl.Segments[:segmentsLen(pl.Segments)]
	var seg *gm3u8.MediaSegment

	if d.curSegmentID == nil {
		if !pl.Closed { // live stream: start from clientLiveStartingInvPosition
			seg = findSegmentWithInvPosition(pl.Segments, clientLiveStartingInvPosition)
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
		var invPos int
		seg, invPos = findSegmentWithID(pl.SeqNo, pl.Segments, *d.curSegmentID+1)
		if seg == nil {
			return fmt.Errorf("following segment not found or not ready yet")
		}

		d.logger.Log(logger.Debug, "segment inverse position: %d", invPos)

		if !pl.Closed && invPos > clientLiveMaxInvPosition {
			return fmt.Errorf("playback is too late")
		}
	}

	v := seg.SeqId
	d.curSegmentID = &v

	byts, err := d.downloadSegment(ctx, seg.URI, seg.Offset, seg.Limit)
	if err != nil {
		return err
	}

	segmentQueue.push(byts)

	if pl.Closed && pl.Segments[len(pl.Segments)-1] == seg {
		<-ctx.Done()
		return fmt.Errorf("stream has ended")
	}

	return nil
}
