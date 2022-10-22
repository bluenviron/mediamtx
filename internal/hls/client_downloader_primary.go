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

func allCodecsAreSupported(codecs string) bool {
	for _, codec := range strings.Split(codecs, ",") {
		if !strings.HasPrefix(codec, "avc1") &&
			!strings.HasPrefix(codec, "mp4a") {
			return false
		}
	}
	return true
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

type clientDownloaderPrimary struct {
	primaryPlaylistURL *url.URL
	logger             ClientLogger
	onTracks           func(*gortsplib.TrackH264, *gortsplib.TrackMPEG4Audio) error
	onVideoData        func(time.Duration, [][]byte)
	onAudioData        func(time.Duration, []byte)
	rp                 *clientRoutinePool

	httpClient *http.Client

	// in
	streamTracks chan []gortsplib.Track

	// out
	startStreaming chan struct{}
}

func newClientDownloaderPrimary(
	primaryPlaylistURL *url.URL,
	fingerprint string,
	logger ClientLogger,
	rp *clientRoutinePool,
	onTracks func(*gortsplib.TrackH264, *gortsplib.TrackMPEG4Audio) error,
	onVideoData func(time.Duration, [][]byte),
	onAudioData func(time.Duration, []byte),
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
		onVideoData:        onVideoData,
		onAudioData:        onAudioData,
		rp:                 rp,
		httpClient: &http.Client{
			Transport: &http.Transport{
				TLSClientConfig: tlsConfig,
			},
		},
		streamTracks:   make(chan []gortsplib.Track),
		startStreaming: make(chan struct{}),
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
			d.httpClient,
			d.primaryPlaylistURL,
			plt,
			d.logger,
			d.rp,
			d.onStreamTracks,
			d.onVideoData,
			d.onAudioData)
		d.rp.add(ds)
		streamCount++

	case *m3u8.MasterPlaylist:
		// gather variants with supported codecs
		var supportedVariants []*gm3u8.Variant
		for _, v := range plt.Variants {
			if !allCodecsAreSupported(v.Codecs) {
				continue
			}
			supportedVariants = append(supportedVariants, v)
		}
		if supportedVariants == nil {
			return fmt.Errorf("no variants with supported codecs found")
		}

		// choose the variant with the greatest bandwidth
		var chosenVariant *gm3u8.Variant
		for _, v := range supportedVariants {
			if chosenVariant == nil ||
				v.VariantParams.Bandwidth > chosenVariant.VariantParams.Bandwidth {
				chosenVariant = v
			}
		}
		if chosenVariant == nil {
			return fmt.Errorf("no variants found")
		}

		u, err := clientAbsoluteURL(d.primaryPlaylistURL, chosenVariant.URI)
		if err != nil {
			return err
		}

		ds := newClientDownloaderStream(
			d.httpClient,
			u,
			nil,
			d.logger,
			d.rp,
			d.onStreamTracks,
			d.onVideoData,
			d.onAudioData)
		d.rp.add(ds)
		streamCount++

		if chosenVariant.Audio != "" {
			audioPlaylist := pickAudioPlaylist(plt.Alternatives, chosenVariant.Audio)
			if audioPlaylist == nil {
				return fmt.Errorf("audio playlist with id \"%s\" not found", chosenVariant.Audio)
			}

			u, err := clientAbsoluteURL(d.primaryPlaylistURL, audioPlaylist.URI)
			if err != nil {
				return err
			}

			ds := newClientDownloaderStream(
				d.httpClient,
				u,
				nil,
				d.logger,
				d.rp,
				d.onStreamTracks,
				d.onVideoData,
				d.onAudioData)
			d.rp.add(ds)
			streamCount++
		}

	default:
		return fmt.Errorf("invalid playlist")
	}

	var tracks []gortsplib.Track

	for i := 0; i < streamCount; i++ {
		select {
		case streamTracks := <-d.streamTracks:
			tracks = append(tracks, streamTracks...)
		case <-ctx.Done():
			return fmt.Errorf("terminated")
		}
	}

	var videoTrack *gortsplib.TrackH264
	var audioTrack *gortsplib.TrackMPEG4Audio

	for _, track := range tracks {
		switch ttrack := track.(type) {
		case *gortsplib.TrackH264:
			if videoTrack != nil {
				return fmt.Errorf("multiple video tracks are not supported")
			}
			videoTrack = ttrack

		case *gortsplib.TrackMPEG4Audio:
			if audioTrack != nil {
				return fmt.Errorf("multiple audio tracks are not supported")
			}
			audioTrack = ttrack
		}
	}

	err = d.onTracks(videoTrack, audioTrack)
	if err != nil {
		return err
	}

	close(d.startStreaming)
	return nil
}

func (d *clientDownloaderPrimary) onStreamTracks(ctx context.Context, tracks []gortsplib.Track) bool {
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
