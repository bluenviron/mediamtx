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

type clientDownloader struct {
	primaryPlaylistURL *url.URL
	segmentQueue       *clientSegmentQueue
	logger             ClientLogger
	onTracks           func(*gortsplib.TrackH264, *gortsplib.TrackMPEG4Audio) error
	onVideoData        func(time.Duration, [][]byte)
	onAudioData        func(time.Duration, []byte)
	rp                 *clientRoutinePool

	streamPlaylistURL     *url.URL
	downloadedSegmentURIs []string
	httpClient            *http.Client
	lastDownloadTime      time.Time
	firstPlaylistReceived bool
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
		ok := d.segmentQueue.waitUntilSizeIsBelow(ctx, clientMinSegmentsBeforeDownloading)
		if !ok {
			return fmt.Errorf("terminated")
		}

		_, err := d.fillSegmentQueue(ctx)
		if err != nil {
			return err
		}
	}
}

func (d *clientDownloader) fillSegmentQueue(ctx context.Context) (bool, error) {
	minTime := d.lastDownloadTime.Add(clientMinDownloadPause)
	now := time.Now()
	if now.Before(minTime) {
		select {
		case <-time.After(minTime.Sub(now)):
		case <-ctx.Done():
			return false, fmt.Errorf("terminated")
		}
	}

	d.lastDownloadTime = now

	pl, err := func() (*m3u8.MediaPlaylist, error) {
		if d.streamPlaylistURL == nil {
			return d.downloadPrimaryPlaylist(ctx)
		}
		return d.downloadStreamPlaylist(ctx)
	}()
	if err != nil {
		return false, err
	}

	if !d.firstPlaylistReceived {
		d.firstPlaylistReceived = true

		if pl.Map != nil && pl.Map.URI != "" {
			return false, fmt.Errorf("fMP4 streams are not supported yet")
		}

		proc := newClientProcessorMPEGTS(
			d.segmentQueue,
			d.logger,
			d.rp,
			d.onTracks,
			d.onVideoData,
			d.onAudioData,
		)
		d.rp.add(proc.run)
	}

	added := false

	for _, seg := range pl.Segments {
		if seg == nil {
			break
		}

		if !d.segmentWasDownloaded(seg.URI) {
			d.downloadedSegmentURIs = append(d.downloadedSegmentURIs, seg.URI)
			byts, err := d.downloadSegment(ctx, seg.URI)
			if err != nil {
				return false, err
			}

			d.segmentQueue.push(byts)
			added = true
		}
	}

	return added, nil
}

func (d *clientDownloader) segmentWasDownloaded(ur string) bool {
	for _, q := range d.downloadedSegmentURIs {
		if q == ur {
			return true
		}
	}
	return false
}

func (d *clientDownloader) downloadPrimaryPlaylist(ctx context.Context) (*m3u8.MediaPlaylist, error) {
	d.logger.Log(logger.Debug, "downloading primary playlist %s", d.primaryPlaylistURL)

	pl, err := d.downloadPlaylist(ctx, d.primaryPlaylistURL)
	if err != nil {
		return nil, err
	}

	switch plt := pl.(type) {
	case *m3u8.MediaPlaylist:
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
