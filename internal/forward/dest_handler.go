package forward

import (
	"context"
	"encoding/hex"
	"errors"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"

	"github.com/bluenviron/mediamtx/internal/conf"
	"github.com/bluenviron/mediamtx/internal/defs"
	forwardmoq "github.com/bluenviron/mediamtx/internal/forward/moq"
	forwardrtmp "github.com/bluenviron/mediamtx/internal/forward/rtmp"
	forwardrtsp "github.com/bluenviron/mediamtx/internal/forward/rtsp"
	forwardsrt "github.com/bluenviron/mediamtx/internal/forward/srt"
	forwardwebrtc "github.com/bluenviron/mediamtx/internal/forward/webrtc"
	"github.com/bluenviron/mediamtx/internal/logger"
	"github.com/bluenviron/mediamtx/internal/stream"
)

const retryPause = 5 * time.Second

var errTerminated = errors.New("terminated")

func sanitizeDestURL(dest string) string {
	u, err := url.Parse(dest)
	if err != nil {
		return dest
	}
	u.User = nil
	u.Fragment = ""
	return u.String()
}

func resolveDest(dest string, pathName string, matches []string) string {
	out := strings.ReplaceAll(dest, "$MTX_PATH", pathName)

	for i := len(matches) - 1; i >= 1; i-- {
		out = strings.ReplaceAll(out, "$G"+strconv.FormatInt(int64(i), 10), matches[i])
	}

	return out
}

func destType(dest string) defs.APIForwardDestType {
	switch {
	case strings.HasPrefix(dest, "rtmp://"), strings.HasPrefix(dest, "rtmps://"):
		return defs.APIForwardDestTypeRTMP

	case strings.HasPrefix(dest, "rtsp://"), strings.HasPrefix(dest, "rtsps://"):
		return defs.APIForwardDestTypeRTSP

	case strings.HasPrefix(dest, "srt://"):
		return defs.APIForwardDestTypeSRT

	case strings.HasPrefix(dest, "moqt://"):
		return defs.APIForwardDestTypeMoQ

	case strings.HasPrefix(dest, "whip://"), strings.HasPrefix(dest, "whips://"):
		return defs.APIForwardDestTypeWebRTC

	default:
		panic("should not happen")
	}
}

func destProtocol(dest string) defs.APIForwardDestProtocol { //nolint:staticcheck
	switch {
	case strings.HasPrefix(dest, "rtmp://"):
		return defs.APIForwardDestProtocolRTMP

	case strings.HasPrefix(dest, "rtmps://"):
		return defs.APIForwardDestProtocolRTMPS

	case strings.HasPrefix(dest, "rtsp://"):
		return defs.APIForwardDestProtocolRTSP

	case strings.HasPrefix(dest, "rtsps://"):
		return defs.APIForwardDestProtocolRTSPS

	case strings.HasPrefix(dest, "srt://"):
		return defs.APIForwardDestProtocolSRT

	case strings.HasPrefix(dest, "moqt://"):
		return defs.APIForwardDestProtocolMoQ

	case strings.HasPrefix(dest, "whip://"):
		return defs.APIForwardDestProtocolWHIP

	case strings.HasPrefix(dest, "whips://"):
		return defs.APIForwardDestProtocolWHIPS

	default:
		panic("should not happen")
	}
}

// DestHandler manages a forward destination.
type DestHandler struct {
	Pos               int
	Conf              conf.ForwardDest
	ReadTimeout       conf.Duration
	WriteTimeout      conf.Duration
	UDPMaxPayloadSize int
	PathName          string
	Matches           []string
	Parent            logger.Writer

	ctx       context.Context
	ctxCancel func()

	uuid       uuid.UUID
	created    time.Time
	typ        defs.APIForwardDestType
	protocol   defs.APIForwardDestProtocol //nolint:staticcheck
	mutex      sync.RWMutex
	state      defs.APIForwardDestState
	lastError  string
	activeDest Dest

	done chan struct{}
}

func (h *DestHandler) initialize() {
	h.uuid = uuid.New()
	h.created = time.Now()
	h.typ = destType(h.Conf.Dest)
	h.protocol = destProtocol(h.Conf.Dest)
	h.state = defs.APIForwardDestStateIdle
}

func (h *DestHandler) start(strm *stream.Stream) {
	h.Log(logger.Debug, "starting")
	h.ctx, h.ctxCancel = context.WithCancel(context.Background())
	h.done = make(chan struct{})
	go h.run(strm)
}

func (h *DestHandler) stop() {
	h.Log(logger.Debug, "stopping")
	h.ctxCancel()
	<-h.done
}

// ID returns the ID.
func (h *DestHandler) ID() uuid.UUID {
	return h.uuid
}

// Log implements logger.Writer.
func (h *DestHandler) Log(level logger.Level, format string, args ...any) {
	id := hex.EncodeToString(h.uuid[:4])
	h.Parent.Log(level, "[%s dest %d %s] "+format,
		append([]any{strings.ToUpper(string(h.protocol)), h.Pos, id}, args...)...)
}

func (h *DestHandler) run(strm *stream.Stream) {
	defer close(h.done)

	defer func() {
		h.mutex.Lock()
		h.state = defs.APIForwardDestStateIdle
		h.mutex.Unlock()
	}()

	for {
		h.mutex.Lock()
		h.state = defs.APIForwardDestStateForwarding
		h.lastError = ""
		h.mutex.Unlock()

		err := h.runOnce(strm)
		if errors.Is(err, errTerminated) {
			return
		}

		h.mutex.Lock()
		h.state = defs.APIForwardDestStateError
		h.lastError = err.Error()
		h.mutex.Unlock()

		h.Log(logger.Error, err.Error())

		timer := time.NewTimer(retryPause)
		select {
		case <-timer.C:
		case <-h.ctx.Done():
			timer.Stop()
			return
		}
	}
}

func (h *DestHandler) runOnce(strm *stream.Stream) error {
	resolvedDest := resolveDest(h.Conf.Dest, h.PathName, h.Matches)

	var dest Dest

	switch h.typ {
	case defs.APIForwardDestTypeRTMP:
		dest = &forwardrtmp.Dest{
			Stream:          strm,
			Dest:            resolvedDest,
			DestFingerprint: h.Conf.DestFingerprint,
			WriteTimeout:    h.WriteTimeout,
			Parent:          h,
		}

	case defs.APIForwardDestTypeRTSP:
		dest = &forwardrtsp.Dest{
			Stream:          strm,
			Dest:            resolvedDest,
			DestFingerprint: h.Conf.DestFingerprint,
			ReadTimeout:     h.ReadTimeout,
			WriteTimeout:    h.WriteTimeout,
			Parent:          h,
		}

	case defs.APIForwardDestTypeSRT:
		dest = &forwardsrt.Dest{
			Stream:            strm,
			Dest:              resolvedDest,
			WriteTimeout:      h.WriteTimeout,
			UDPMaxPayloadSize: h.UDPMaxPayloadSize,
			Parent:            h,
		}

	case defs.APIForwardDestTypeMoQ:
		dest = &forwardmoq.Dest{
			Stream:          strm,
			Dest:            resolvedDest,
			DestFingerprint: h.Conf.DestFingerprint,
			Transport:       h.Conf.MoQTransport,
			Parent:          h,
		}

	case defs.APIForwardDestTypeWebRTC:
		dest = &forwardwebrtc.Dest{
			Stream:          strm,
			Dest:            resolvedDest,
			DestFingerprint: h.Conf.DestFingerprint,
			ReadTimeout:     h.ReadTimeout,
			BearerToken:     h.Conf.WHIPBearerToken,
			Parent:          h,
		}

	default:
		panic("should not happen")
	}

	h.Log(logger.Info, "forwarding to '%s'", sanitizeDestURL(resolvedDest))

	h.mutex.Lock()
	h.activeDest = dest
	h.mutex.Unlock()

	defer func() {
		h.mutex.Lock()
		h.activeDest = nil
		h.mutex.Unlock()
	}()

	destCtx, destCtxCancel := context.WithCancel(context.Background())

	errChan := make(chan error)
	go func() {
		errChan <- dest.Run(destCtx)
	}()

	select {
	case err := <-errChan:
		destCtxCancel()
		return err

	case <-h.ctx.Done():
		destCtxCancel()
		<-errChan
		return errTerminated
	}
}

// APIItem returns an API item.
func (h *DestHandler) APIItem() defs.APIForwardDest {
	h.mutex.RLock()
	defer h.mutex.RUnlock()

	info := defs.ForwardDestInfo{}
	if h.activeDest != nil {
		info = h.activeDest.Info()
	}

	return defs.APIForwardDest{
		ID:            h.uuid,
		Pos:           h.Pos,
		Created:       h.created,
		Conf:          h.Conf,
		Type:          h.typ,
		State:         h.state,
		LastError:     h.lastError,
		OutboundBytes: info.OutboundBytes,
		TypeSpecific:  info.TypeSpecific,
		Protocol:      h.protocol,
	}
}
