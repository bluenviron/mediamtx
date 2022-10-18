package hls

import (
	"context"
	"crypto/sha256"
	"crypto/tls"
	"encoding/hex"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/aler9/gortsplib"
	"github.com/grafov/m3u8"

	"github.com/aler9/rtsp-simple-server/internal/logger"
)

func segmentsLen(segments []*m3u8.MediaSegment) int {
	for i, seg := range segments {
		if seg == nil {
			return i
		}
	}
	return 0
}

func findStartingSegment(segments []*m3u8.MediaSegment) *m3u8.MediaSegment {
	pos := len(segments) - clientLiveStartingPoint
	if pos < 0 {
		return nil
	}

	return segments[pos]
}

func findSegmentWithID(seqNo uint64, segments []*m3u8.MediaSegment, id uint64) *m3u8.MediaSegment {
	pos := int(int64(id) - int64(seqNo))
	if (pos) >= len(segments) {
		return nil
	}

	return segments[pos]
}

type clientDownloader struct {
	primaryPlaylistURL *url.URL
	segmentQueue       *clientSegmentQueue
	logger             ClientLogger
	onTracks           func(*gortsplib.TrackH264, *gortsplib.TrackMPEG4Audio) error
	onVideoData        func(time.Duration, [][]byte)
	onAudioData        func(time.Duration, []byte)
	rp                 *clientRoutinePool

	streamPlaylistURL *url.URL
	httpClient        *http.Client
	curSegmentID      *uint64
}

func newClientDownloader(
	primaryPlaylistURL *url.URL,
	fingerprint string,
	segmentQueue *clientSegmentQueue,
	logger ClientLogger,
	onTracks func(*gortsplib.TrackH264, *gortsplib.TrackMPEG4Audio) error,
	onVideoData func(time.Duration, [][]byte),
	onAudioData func(time.Duration, []byte),
	rp *clientRoutinePool,
) *clientDownloader {
	var tlsConfig *tls.Config
	if fingerprint != "" {
		tlsConfig = &tls.Config{
			InsecureSkipVerify: true,
			VerifyConnection: func(cs tls.ConnectionState) error {
				h := sha256.New()
				h.Write(cs.PeerCertificates[0].Raw)
				hstr := hex.EncodeToString(h.Sum(nil))
				fingerprintLower := strings.ToLower(fingerprint)

				if hstr != fingerprintLower {
					return fmt.Errorf("server fingerprint do not match: expected %s, got %s",
						fingerprintLower, hstr)
				}

				return nil
			},
		}
	}

	return &clientDownloader{
		primaryPlaylistURL: primaryPlaylistURL,
		segmentQueue:       segmentQueue,
		logger:             logger,
		onTracks:           onTracks,
		onVideoData:        onVideoData,
		onAudioData:        onAudioData,
		rp:                 rp,
		httpClient: &http.Client{
			Transport: &http.Transport{
				TLSClientConfig: tlsConfig,
			},
		},
	}
}

func (d *clientDownloader) run(ctx context.Context) error {
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

func (d *clientDownloader) fillSegmentQueue(ctx context.Context) error {
	var pl *m3u8.MediaPlaylist

	if d.streamPlaylistURL == nil {
		var err error
		pl, err = d.downloadPrimaryPlaylist(ctx)
		if err != nil {
			return err
		}

		if pl.Map != nil && pl.Map.URI != "" {
			byts, err := d.downloadSegment(ctx, pl.Map.URI)
			if err != nil {
				return err
			}

			proc, err := newClientProcessorFMP4(
				byts,
				d.segmentQueue,
				d.logger,
				d.rp,
				d.onTracks,
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
				d.onTracks,
				d.onVideoData,
				d.onAudioData,
			)
			d.rp.add(proc)
		}
	} else {
		var err error
		pl, err = d.downloadStreamPlaylist(ctx)
		if err != nil {
			return err
		}
	}

	pl.Segments = pl.Segments[:segmentsLen(pl.Segments)]
	var seg *m3u8.MediaSegment

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

func (d *clientDownloader) downloadPrimaryPlaylist(ctx context.Context) (*m3u8.MediaPlaylist, error) {
	d.logger.Log(logger.Debug, "downloading primary playlist %s", d.primaryPlaylistURL)

	pl, err := d.downloadPlaylist(ctx, d.primaryPlaylistURL)
	if err != nil {
		return nil, err
	}

	switch plt := pl.(type) {
	case *m3u8.MediaPlaylist:
		d.logger.Log(logger.Debug, "primary playlist is a stream playlist")
		d.streamPlaylistURL = d.primaryPlaylistURL
		return plt, nil

	case *m3u8.MasterPlaylist:
		// choose the variant with the highest bandwidth
		var chosenVariant *m3u8.Variant
		for _, v := range plt.Variants {
			if chosenVariant == nil ||
				v.VariantParams.Bandwidth > chosenVariant.VariantParams.Bandwidth {
				chosenVariant = v
			}
		}

		if chosenVariant == nil {
			return nil, fmt.Errorf("no variants found")
		}

		u, err := clientURLAbsolute(d.primaryPlaylistURL, chosenVariant.URI)
		if err != nil {
			return nil, err
		}

		d.streamPlaylistURL = u

		return d.downloadStreamPlaylist(ctx)

	default:
		return nil, fmt.Errorf("invalid playlist")
	}
}

func (d *clientDownloader) downloadStreamPlaylist(ctx context.Context) (*m3u8.MediaPlaylist, error) {
	d.logger.Log(logger.Debug, "downloading stream playlist %s", d.streamPlaylistURL.String())

	pl, err := d.downloadPlaylist(ctx, d.streamPlaylistURL)
	if err != nil {
		return nil, err
	}

	plt, ok := pl.(*m3u8.MediaPlaylist)
	if !ok {
		return nil, fmt.Errorf("invalid playlist")
	}

	return plt, nil
}

func (d *clientDownloader) downloadPlaylist(ctx context.Context, ur *url.URL) (m3u8.Playlist, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, ur.String(), nil)
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

	pl, _, err := m3u8.DecodeFrom(res.Body, true)
	if err != nil {
		return nil, err
	}

	return pl, nil
}

func (d *clientDownloader) downloadSegment(ctx context.Context, segmentURI string) ([]byte, error) {
	u, err := clientURLAbsolute(d.streamPlaylistURL, segmentURI)
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
