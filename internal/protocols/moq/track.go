package moq

import (
	"github.com/bluenviron/gortsplib/v5/pkg/description"
	"github.com/bluenviron/gortsplib/v5/pkg/format"
	"github.com/bluenviron/mediamtx/internal/unit"
)

// Track is a Media-over-QUIC track.
type Track struct {
	Codec      string
	Samplerate int
	Channels   int
	InitData   string
	Media      *description.Media
	Format     format.Format
	OnData     func(u *unit.Unit, wrapped func([]byte, int64) error) error
}
