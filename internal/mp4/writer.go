package mp4

import (
	"io"

	gomp4 "github.com/abema/go-mp4"
	"github.com/orcaman/writerseeker"
)

// Writer is a MP4 writer.
type Writer struct {
	buf *writerseeker.WriterSeeker
	w   *gomp4.Writer
}

// NewWriter allocates a Writer.
func NewWriter() *Writer {
	w := &Writer{
		buf: &writerseeker.WriterSeeker{},
	}

	w.w = gomp4.NewWriter(w.buf)

	return w
}

// WriteBoxStart writes a box start.
func (w *Writer) WriteBoxStart(box gomp4.IImmutableBox) (int, error) {
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
func (w *Writer) WriteBoxEnd() error {
	_, err := w.w.EndBox()
	return err
}

// WriteBox writes a self-closing box.
func (w *Writer) WriteBox(box gomp4.IImmutableBox) (int, error) {
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
func (w *Writer) RewriteBox(off int, box gomp4.IImmutableBox) error {
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
func (w *Writer) Bytes() []byte {
	return w.buf.Bytes()
}
