package playback

import (
	"errors"
	"fmt"
	"net/http"

	"github.com/bluenviron/mediacommon/v2/pkg/formats/fmp4"
	"github.com/bluenviron/mediacommon/v2/pkg/formats/fmp4/seekablebuffer"
	"github.com/bluenviron/mediamtx/internal/conf"
	"github.com/bluenviron/mediamtx/internal/recordstore"
	"github.com/gin-gonic/gin"
)

func (s *Server) onHLSInit(ctx *gin.Context) {
	pathName := ctx.Query("path")

	if !s.doAuth(ctx, pathName) {
		return
	}

	pathConf, err := s.safeFindPathConf(pathName)
	if err != nil {
		s.writeError(ctx, http.StatusBadRequest, err)
		return
	}

	if pathConf.RecordFormat != conf.RecordFormatFMP4 {
		s.writeError(ctx, http.StatusBadRequest, fmt.Errorf("HLS requires fMP4 recording format"))
		return
	}

	segments, err := recordstore.FindSegments(pathConf, pathName, nil, nil)
	if err != nil {
		if errors.Is(err, recordstore.ErrNoSegmentsFound) {
			s.writeError(ctx, http.StatusNotFound, err)
		} else {
			s.writeError(ctx, http.StatusBadRequest, err)
		}
		return
	}

	parsed, err := parseSegments(segments)
	if err != nil {
		s.writeError(ctx, http.StatusInternalServerError, err)
		return
	}

	if len(parsed) == 0 {
		s.writeError(ctx, http.StatusNotFound, recordstore.ErrNoSegmentsFound)
		return
	}

	// Marshal the init segment (ftyp+moov) from the first recording
	init := &fmp4.Init{Tracks: parsed[0].init.Tracks}
	var buf seekablebuffer.Buffer
	err = init.Marshal(&buf)
	if err != nil {
		s.writeError(ctx, http.StatusInternalServerError, fmt.Errorf("failed to marshal init segment: %w", err))
		return
	}

	ctx.Header("Content-Type", "video/mp4")
	ctx.Data(http.StatusOK, "video/mp4", buf.Bytes())
}
