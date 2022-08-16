package core

import (
	"context"
	"crypto/sha256"
	"crypto/tls"
	"encoding/hex"
	"fmt"
	"net"
	"net/url"
	"strings"
	"time"

	"github.com/aler9/gortsplib"
	"github.com/aler9/gortsplib/pkg/h264"
	"github.com/notedit/rtmp/format/flv/flvio"

	"github.com/aler9/rtsp-simple-server/internal/conf"
	"github.com/aler9/rtsp-simple-server/internal/logger"
	"github.com/aler9/rtsp-simple-server/internal/rtmp"
	"github.com/aler9/rtsp-simple-server/internal/rtmp/message"
)

type rtmpSourceParent interface {
	log(logger.Level, string, ...interface{})
	sourceStaticImplSetReady(req pathSourceStaticSetReadyReq) pathSourceStaticSetReadyRes
	sourceStaticImplSetNotReady(req pathSourceStaticSetNotReadyReq)
}

type rtmpSource struct {
	ur           string
	fingerprint  string
	readTimeout  conf.StringDuration
	writeTimeout conf.StringDuration
	parent       rtmpSourceParent
}

func newRTMPSource(
	ur string,
	fingerprint string,
	readTimeout conf.StringDuration,
	writeTimeout conf.StringDuration,
	parent rtmpSourceParent,
) *rtmpSource {
	return &rtmpSource{
		ur:           ur,
		fingerprint:  fingerprint,
		readTimeout:  readTimeout,
		writeTimeout: writeTimeout,
		parent:       parent,
	}
}

func (s *rtmpSource) Log(level logger.Level, format string, args ...interface{}) {
	s.parent.log(level, "[rtmp source] "+format, args...)
}

// run implements sourceStaticImpl.
func (s *rtmpSource) run(ctx context.Context) error {
	s.Log(logger.Debug, "connecting")

	u, err := url.Parse(s.ur)
	if err != nil {
		return err
	}

	// add default port
	_, _, err = net.SplitHostPort(u.Host)
	if err != nil {
		u.Host = net.JoinHostPort(u.Host, "1935")
	}

	ctx2, cancel2 := context.WithTimeout(ctx, time.Duration(s.readTimeout))
	defer cancel2()

	nconn, err := func() (net.Conn, error) {
		if u.Scheme == "rtmp" {
			return (&net.Dialer{}).DialContext(ctx2, "tcp", u.Host)
		}

		tlsConfig := &tls.Config{
			InsecureSkipVerify: true,
			VerifyConnection: func(cs tls.ConnectionState) error {
				h := sha256.New()
				h.Write(cs.PeerCertificates[0].Raw)
				hstr := hex.EncodeToString(h.Sum(nil))
				fingerprintLower := strings.ToLower(s.fingerprint)

				if hstr != fingerprintLower {
					return fmt.Errorf("server fingerprint do not match: expected %s, got %s",
						fingerprintLower, hstr)
				}

				return nil
			},
		}

		return (&tls.Dialer{Config: tlsConfig}).DialContext(ctx2, "tcp", u.Host)
	}()
	if err != nil {
		return err
	}

	conn := rtmp.NewConn(nconn)

	readDone := make(chan error)
	go func() {
		readDone <- func() error {
			nconn.SetReadDeadline(time.Now().Add(time.Duration(s.readTimeout)))
			nconn.SetWriteDeadline(time.Now().Add(time.Duration(s.writeTimeout)))
			err = conn.InitializeClient(u, false)
			if err != nil {
				return err
			}

			nconn.SetWriteDeadline(time.Time{})
			nconn.SetReadDeadline(time.Now().Add(time.Duration(s.readTimeout)))
			videoTrack, audioTrack, err := conn.ReadTracks()
			if err != nil {
				return err
			}

			var tracks gortsplib.Tracks
			videoTrackID := -1
			audioTrackID := -1

			if videoTrack != nil {
				videoTrackID = len(tracks)
				tracks = append(tracks, videoTrack)
			}

			if audioTrack != nil {
				audioTrackID = len(tracks)
				tracks = append(tracks, audioTrack)
			}

			res := s.parent.sourceStaticImplSetReady(pathSourceStaticSetReadyReq{
				tracks:             tracks,
				generateRTPPackets: true,
			})
			if res.err != nil {
				return res.err
			}

			s.Log(logger.Info, "ready: %s", sourceTrackInfo(tracks))

			defer func() {
				s.parent.sourceStaticImplSetNotReady(pathSourceStaticSetNotReadyReq{})
			}()

			// disable write deadline to allow outgoing acknowledges
			nconn.SetWriteDeadline(time.Time{})

			for {
				nconn.SetReadDeadline(time.Now().Add(time.Duration(s.readTimeout)))
				msg, err := conn.ReadMessage()
				if err != nil {
					return err
				}

				switch tmsg := msg.(type) {
				case *message.MsgVideo:
					if tmsg.H264Type == flvio.AVC_NALU {
						if videoTrack == nil {
							return fmt.Errorf("received an H264 packet, but track is not set up")
						}

						nalus, err := h264.AVCCUnmarshal(tmsg.Payload)
						if err != nil {
							return fmt.Errorf("unable to decode AVCC: %v", err)
						}

						res.stream.writeData(&data{
							trackID:      videoTrackID,
							ptsEqualsDTS: h264.IDRPresent(nalus),
							pts:          tmsg.DTS + tmsg.PTSDelta,
							h264NALUs:    nalus,
						})
					}

				case *message.MsgAudio:
					if tmsg.AACType == flvio.AAC_RAW {
						if audioTrack == nil {
							return fmt.Errorf("received an AAC packet, but track is not set up")
						}

						res.stream.writeData(&data{
							trackID:      audioTrackID,
							ptsEqualsDTS: true,
							pts:          tmsg.DTS,
							mpeg4AudioAU: tmsg.Payload,
						})
					}
				}
			}
		}()
	}()

	select {
	case err := <-readDone:
		nconn.Close()
		return err

	case <-ctx.Done():
		nconn.Close()
		<-readDone
		return nil
	}
}

// apiSourceDescribe implements sourceStaticImpl.
func (*rtmpSource) apiSourceDescribe() interface{} {
	return struct {
		Type string `json:"type"`
	}{"rtmpSource"}
}
