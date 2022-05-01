package hls

import (
	"bytes"
	"io"
	"strconv"
	"time"

	"github.com/abema/go-mp4"
)

type fmp4PartEntry struct {
	pts  time.Duration
	avcc []byte
}

func mp4PartGenerate(
	entries []fmp4PartEntry,
	startDTS time.Duration,
	sampleDuration time.Duration,
) ([]byte, error) {
	/*
		moof
		- mfhd
		- traf
		  - tfhd
		  - tfdt
		  - trun
		mdat
	*/

	w := newMP4Writer()

	moofOffset, err := w.writeBoxStart(&mp4.Moof{}) // <moof>
	if err != nil {
		return nil, err
	}

	_, err = w.writeBox(&mp4.Mfhd{ // <mfhd/>
		SequenceNumber: 0,
	})
	if err != nil {
		return nil, err
	}

	_, err = w.writeBoxStart(&mp4.Traf{}) // <traf>
	if err != nil {
		return nil, err
	}

	_, err = w.writeBox(&mp4.Tfhd{ // <tfhd/>
		FullBox: mp4.FullBox{
			Flags: [3]byte{2, 0, 56},
		},
		TrackID:               1,
		DefaultSampleDuration: uint32(sampleDuration * fmp4Timescale / time.Second),
	})
	if err != nil {
		return nil, err
	}

	_, err = w.writeBox(&mp4.Tfdt{ // <tfdt/>
		FullBox: mp4.FullBox{
			Version: 1,
		},
		// sum of decode durations of all earlier samples
		BaseMediaDecodeTimeV1: uint64(startDTS * fmp4Timescale / time.Second),
	})
	if err != nil {
		return nil, err
	}

	trun := &mp4.Trun{ // <trun/>
		FullBox: mp4.FullBox{
			Version: 1,
			Flags:   [3]byte{0, 10, 5},
		},
		SampleCount: uint32(len(entries)),
	}

	dts := startDTS

	for _, e := range entries {
		off := int32((e.pts - dts) * fmp4Timescale / time.Second)

		trun.Entries = append(trun.Entries, mp4.TrunEntry{
			SampleSize:                    uint32(len(e.avcc)),
			SampleCompositionTimeOffsetV1: off,
		})

		dts += sampleDuration
	}

	trunOffset, err := w.writeBox(trun)
	if err != nil {
		return nil, err
	}

	err = w.writeBoxEnd() // </traf>
	if err != nil {
		return nil, err
	}

	err = w.writeBoxEnd() // </moof>
	if err != nil {
		return nil, err
	}

	mdat := &mp4.Mdat{} // <mdat/>

	size := 0
	for _, e := range entries {
		size += len(e.avcc)
	}

	data := make([]byte, size)
	pos := 0
	for _, e := range entries {
		pos += copy(data[pos:], e.avcc)
	}
	mdat.Data = data

	mdatOffset, err := w.writeBox(mdat)
	if err != nil {
		return nil, err
	}

	trun.DataOffset = int32(mdatOffset - moofOffset + 8)
	err = w.rewriteBox(trunOffset, trun)
	if err != nil {
		return nil, err
	}

	return w.bytes(), nil //  + sampleDuration
}

func fmp4PartName(id uint64) string {
	return "part" + strconv.FormatUint(id, 10)
}

type muxerVariantFMP4Part struct {
	id             uint64
	startDTS       time.Duration
	sampleDuration time.Duration

	entries  []fmp4PartEntry
	rendered []byte
	duration time.Duration
}

func newMuxerVariantFMP4Part(
	id uint64,
	startDTS time.Duration,
	sampleDuration time.Duration,
) *muxerVariantFMP4Part {
	return &muxerVariantFMP4Part{
		id:             id,
		startDTS:       startDTS,
		sampleDuration: sampleDuration,
	}
}

func (p *muxerVariantFMP4Part) name() string {
	return fmp4PartName(p.id)
}

func (p *muxerVariantFMP4Part) reader() io.Reader {
	return bytes.NewReader(p.rendered)
}

func (p *muxerVariantFMP4Part) finalize() error {
	var err error
	p.rendered, err = mp4PartGenerate(p.entries, p.startDTS, p.sampleDuration)
	if err != nil {
		return err
	}

	p.duration = time.Duration(len(p.entries)) * p.sampleDuration
	p.entries = nil
	return nil
}

func (p *muxerVariantFMP4Part) writeH264(pts time.Duration, avcc []byte) error {
	p.entries = append(p.entries, fmp4PartEntry{
		pts:  pts,
		avcc: avcc,
	})
	return nil
}
