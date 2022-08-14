package fmp4

import (
	"io"

	gomp4 "github.com/abema/go-mp4"
	"github.com/orcaman/writerseeker"
)

// mp4Writer is a MP4 writer.
type mp4Writer struct {
	buf *writerseeker.WriterSeeker
	w   *gomp4.Writer
}

// newMP4Writer allocates a mp4Writer.
func newMP4Writer() *mp4Writer {
	w := &mp4Writer{
		buf: &writerseeker.WriterSeeker{},
	}

	w.w = gomp4.NewWriter(w.buf)

	return w
}

// WriteBoxStart writes a box start.
func (w *mp4Writer) WriteBoxStart(box gomp4.IImmutableBox) (int, error) {
	bi := &gomp4.BoxInfo{
		Type: box.GetType(),
	}
	var err error
	bi, err = w.w.StartBox(bi)
	if err != nil {
		return 0, err
	}

	_, err = gomp4.Marshal(w.w, box, gomp4.Context{})
	if err != nil {
		return 0, err
	}

	return int(bi.Offset), nil
}

// WriteBoxEnd writes a box end.
func (w *mp4Writer) WriteBoxEnd() error {
	_, err := w.w.EndBox()
	return err
}

// WriteBox writes a self-closing box.
func (w *mp4Writer) WriteBox(box gomp4.IImmutableBox) (int, error) {
	off, err := w.WriteBoxStart(box)
	if err != nil {
		return 0, err
	}

	err = w.WriteBoxEnd()
	if err != nil {
		return 0, err
	}

	return off, nil
}

// RewriteBox rewrites a box.
func (w *mp4Writer) RewriteBox(off int, box gomp4.IImmutableBox) error {
	prevOff, err := w.w.Seek(0, io.SeekCurrent)
	if err != nil {
		return err
	}

	_, err = w.w.Seek(int64(off), io.SeekStart)
	if err != nil {
		return err
	}

	_, err = w.WriteBoxStart(box)
	if err != nil {
		return err
	}

	err = w.WriteBoxEnd()
	if err != nil {
		return err
	}

	_, err = w.w.Seek(prevOff, io.SeekStart)
	if err != nil {
		return err
	}

	return nil
}

// Bytes returns the MP4 content.
func (w *mp4Writer) Bytes() []byte {
	return w.buf.Bytes()
}
