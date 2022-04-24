package hls

import (
	"io"
	"time"
)

type muxerVariant interface {
	close()
	writeH264(pts time.Duration, nalus [][]byte) error
	writeAAC(pts time.Duration, aus [][]byte) error
	playlistReader() io.Reader
	segmentReader(string) io.Reader
}
