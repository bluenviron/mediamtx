package fmp4

import (
	"io"

	gomp4 "github.com/abema/go-mp4"
	"github.com/orcaman/writerseeker"
)

type mp4Writer struct {
	buf *writerseeker.WriterSeeker
	w   *gomp4.Writer
}

func newMP4Writer() *mp4Writer {
	w := &mp4Writer{
		buf: &writerseeker.WriterSeeker{},
	}

	w.w = gomp4.NewWriter(w.buf)

	return w
}

func (w *mp4Writer) writeBoxStart(box gomp4.IImmutableBox) (int, error) {
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

func (w *mp4Writer) writeBoxEnd() error {
	_, err := w.w.EndBox()
	return err
}

func (w *mp4Writer) WriteBox(box gomp4.IImmutableBox) (int, error) {
	off, err := w.writeBoxStart(box)
	if err != nil {
		return 0, err
	}

	err = w.writeBoxEnd()
	if err != nil {
		return 0, err
	}

	return off, nil
}

func (w *mp4Writer) rewriteBox(off int, box gomp4.IImmutableBox) error {
	prevOff, err := w.w.Seek(0, io.SeekCurrent)
	if err != nil {
		return err
	}

	_, err = w.w.Seek(int64(off), io.SeekStart)
	if err != nil {
		return err
	}

	_, err = w.writeBoxStart(box)
	if err != nil {
		return err
	}

	err = w.writeBoxEnd()
	if err != nil {
		return err
	}

	_, err = w.w.Seek(prevOff, io.SeekStart)
	if err != nil {
		return err
	}

	return nil
}

func (w *mp4Writer) bytes() []byte {
	return w.buf.Bytes()
}
