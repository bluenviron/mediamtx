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

	"github.com/aler9/gortsplib/v2/pkg/format"
	gm3u8 "github.com/grafov/m3u8"

	"github.com/aler9/rtsp-simple-server/internal/hls/m3u8"
	"github.com/aler9/rtsp-simple-server/internal/logger"
)

func clientDownloadPlaylist(ctx context.Context, httpClient *http.Client, ur *url.URL) (m3u8.Playlist, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, ur.String(), nil)
	if err != nil {
		return nil, err
	}

	res, err := httpClient.Do(req)
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

	return m3u8.Unmarshal(byts)
}

func pickLeadingPlaylist(variants []*gm3u8.Variant) *gm3u8.Variant {
	var candidates []*gm3u8.Variant //nolint:prealloc
	for _, v := range variants {
		if v.Codecs != "" && !codecParametersAreSupported(v.Codecs) {
			continue
		}
		candidates = append(candidates, v)
	}
	if candidates == nil {
		return nil
	}

	// pick the variant with the greatest bandwidth
	var leadingPlaylist *gm3u8.Variant
	for _, v := range candidates {
		if leadingPlaylist == nil ||
			v.VariantParams.Bandwidth > leadingPlaylist.VariantParams.Bandwidth {
			leadingPlaylist = v
		}
	}
	return leadingPlaylist
}

func pickAudioPlaylist(alternatives []*gm3u8.Alternative, groupID string) *gm3u8.Alternative {
	candidates := func() []*gm3u8.Alternative {
		var ret []*gm3u8.Alternative
		for _, alt := range alternatives {
			if alt.GroupId == groupID {
				ret = append(ret, alt)
			}
		}
		return ret
	}()
	if candidates == nil {
		return nil
	}

	// pick the default audio playlist
	for _, alt := range candidates {
		if alt.Default {
			return alt
		}
	}

	// alternatively, pick the first one
	return candidates[0]
}

type clientTimeSync interface{}

type clientDownloaderPrimary struct {
	primaryPlaylistURL *url.URL
	logger             ClientLogger
	onTracks           func([]format.Format) error
	onData             map[format.Format]func(time.Duration, interface{})
	rp                 *clientRoutinePool

	httpClient      *http.Client
	leadingTimeSync clientTimeSync

	// in
	streamTracks chan []format.Format

	// out
	startStreaming       chan struct{}
	leadingTimeSyncReady chan struct{}
}

func newClientDownloaderPrimary(
	primaryPlaylistURL *url.URL,
	fingerprint string,
	logger ClientLogger,
	rp *clientRoutinePool,
	onTracks func([]format.Format) error,
	onData map[format.Format]func(time.Duration, interface{}),
) *clientDownloaderPrimary {
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

	return &clientDownloaderPrimary{
		primaryPlaylistURL: primaryPlaylistURL,
		logger:             logger,
		onTracks:           onTracks,
		onData:             onData,
		rp:                 rp,
		httpClient: &http.Client{
			Transport: &http.Transport{
				TLSClientConfig: tlsConfig,
			},
		},
		streamTracks:         make(chan []format.Format),
		startStreaming:       make(chan struct{}),
		leadingTimeSyncReady: make(chan struct{}),
	}
}

func (d *clientDownloaderPrimary) run(ctx context.Context) error {
	d.logger.Log(logger.Debug, "downloading primary playlist %s", d.primaryPlaylistURL)

	pl, err := clientDownloadPlaylist(ctx, d.httpClient, d.primaryPlaylistURL)
	if err != nil {
		return err
	}

	streamCount := 0

	switch plt := pl.(type) {
	case *m3u8.MediaPlaylist:
		d.logger.Log(logger.Debug, "primary playlist is a stream playlist")
		ds := newClientDownloaderStream(
			true,
			d.httpClient,
			d.primaryPlaylistURL,
			plt,
			d.logger,
			d.rp,
			d.onStreamTracks,
			d.onSetLeadingTimeSync,
			d.onGetLeadingTimeSync,
			d.onData,
		)
		d.rp.add(ds)
		streamCount++

	case *m3u8.MasterPlaylist:
		leadingPlaylist := pickLeadingPlaylist(plt.Variants)
		if leadingPlaylist == nil {
			return fmt.Errorf("no variants with supported codecs found")
		}

		u, err := clientAbsoluteURL(d.primaryPlaylistURL, leadingPlaylist.URI)
		if err != nil {
			return err
		}

		ds := newClientDownloaderStream(
			true,
			d.httpClient,
			u,
			nil,
			d.logger,
			d.rp,
			d.onStreamTracks,
			d.onSetLeadingTimeSync,
			d.onGetLeadingTimeSync,
			d.onData,
		)
		d.rp.add(ds)
		streamCount++

		if leadingPlaylist.Audio != "" {
			audioPlaylist := pickAudioPlaylist(plt.Alternatives, leadingPlaylist.Audio)
			if audioPlaylist == nil {
				return fmt.Errorf("audio playlist with id \"%s\" not found", leadingPlaylist.Audio)
			}

			u, err := clientAbsoluteURL(d.primaryPlaylistURL, audioPlaylist.URI)
			if err != nil {
				return err
			}

			ds := newClientDownloaderStream(
				false,
				d.httpClient,
				u,
				nil,
				d.logger,
				d.rp,
				d.onStreamTracks,
				d.onSetLeadingTimeSync,
				d.onGetLeadingTimeSync,
				d.onData,
			)
			d.rp.add(ds)
			streamCount++
		}

	default:
		return fmt.Errorf("invalid playlist")
	}

	var tracks []format.Format

	for i := 0; i < streamCount; i++ {
		select {
		case streamTracks := <-d.streamTracks:
			tracks = append(tracks, streamTracks...)
		case <-ctx.Done():
			return fmt.Errorf("terminated")
		}
	}

	if len(tracks) == 0 {
		return fmt.Errorf("no supported tracks found")
	}

	err = d.onTracks(tracks)
	if err != nil {
		return err
	}

	close(d.startStreaming)

	return nil
}

func (d *clientDownloaderPrimary) onStreamTracks(ctx context.Context, tracks []format.Format) bool {
	select {
	case d.streamTracks <- tracks:
	case <-ctx.Done():
		return false
	}

	select {
	case <-d.startStreaming:
	case <-ctx.Done():
		return false
	}

	return true
}

func (d *clientDownloaderPrimary) onSetLeadingTimeSync(ts clientTimeSync) {
	d.leadingTimeSync = ts
	close(d.leadingTimeSyncReady)
}

func (d *clientDownloaderPrimary) onGetLeadingTimeSync(ctx context.Context) (clientTimeSync, bool) {
	select {
	case <-d.leadingTimeSyncReady:
	case <-ctx.Done():
		return nil, false
	}
	return d.leadingTimeSync, true
}
