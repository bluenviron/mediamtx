package mp4

import (
	"io"

	"github.com/abema/go-mp4"
)

type mp4Writer struct {
	w *mp4.Writer
}

func newMP4Writer(w io.WriteSeeker) *mp4Writer {
	return &mp4Writer{
		w: mp4.NewWriter(w),
	}
}

func (w *mp4Writer) writeBoxStart(box mp4.IImmutableBox) (int, error) {
	bi := &mp4.BoxInfo{
		Type: box.GetType(),
	}
	var err error
	bi, err = w.w.StartBox(bi)
	if err != nil {
		return 0, err
	}

	_, err = mp4.Marshal(w.w, box, mp4.Context{})
	if err != nil {
		return 0, err
	}

	return int(bi.Offset), nil
}

func (w *mp4Writer) writeBoxEnd() error {
	_, err := w.w.EndBox()
	return err
}

func (w *mp4Writer) writeBox(box mp4.IImmutableBox) (int, error) {
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

func (w *mp4Writer) rewriteBox(off int, box mp4.IImmutableBox) error {
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
