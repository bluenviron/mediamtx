package hls

import (
	"bytes"
	"io"
	"strconv"
	"time"

	"github.com/abema/go-mp4"
	"github.com/aler9/gortsplib"
	"github.com/aler9/gortsplib/pkg/h264"
)

type fmp4PartVideoEntry struct {
	pts  time.Duration
	avcc []byte
}

type fmp4PartAudioEntry struct {
	pts time.Duration
	au  []byte
}

func mp4PartGenerateVideoTraf(
	w *mp4Writer,
	trackID int,
	videoEntries []fmp4PartVideoEntry,
	startDTS time.Duration,
	sampleDuration time.Duration,
) (*mp4.Trun, int, error) {
	/*
		traf
		- tfhd
		- tfdt
		- trun
	*/

	_, err := w.writeBoxStart(&mp4.Traf{}) // <traf>
	if err != nil {
		return nil, 0, err
	}

	_, err = w.writeBox(&mp4.Tfhd{ // <tfhd/>
		FullBox: mp4.FullBox{
			Flags: [3]byte{2, 0, 56},
		},
		TrackID:               uint32(trackID),
		DefaultSampleDuration: uint32(sampleDuration * fmp4VideoTimescale / time.Second),
	})
	if err != nil {
		return nil, 0, err
	}

	_, err = w.writeBox(&mp4.Tfdt{ // <tfdt/>
		FullBox: mp4.FullBox{
			Version: 1,
		},
		// sum of decode durations of all earlier samples
		BaseMediaDecodeTimeV1: uint64(startDTS * fmp4VideoTimescale / time.Second),
	})
	if err != nil {
		return nil, 0, err
	}

	trun := &mp4.Trun{ // <trun/>
		FullBox: mp4.FullBox{
			Version: 1,
			Flags:   [3]byte{0, 10, 5},
		},
		SampleCount: uint32(len(videoEntries)),
	}

	dts := startDTS

	for _, e := range videoEntries {
		off := int32((e.pts - dts) * fmp4VideoTimescale / time.Second)

		trun.Entries = append(trun.Entries, mp4.TrunEntry{
			SampleSize:                    uint32(len(e.avcc)),
			SampleCompositionTimeOffsetV1: off,
		})

		dts += sampleDuration
	}

	trunOffset, err := w.writeBox(trun)
	if err != nil {
		return nil, 0, err
	}

	err = w.writeBoxEnd() // </traf>
	if err != nil {
		return nil, 0, err
	}

	return trun, trunOffset, nil
}

func mp4PartGenerateAudioTraf(
	w *mp4Writer,
	trackID int,
	audioTrack *gortsplib.TrackAAC,
	audioEntries []fmp4PartAudioEntry,
) (*mp4.Trun, int, error) {
	/*
		traf
		- tfhd
		- tfdt
		- trun
	*/

	if len(audioEntries) == 0 {
		return nil, 0, nil
	}

	_, err := w.writeBoxStart(&mp4.Traf{}) // <traf>
	if err != nil {
		return nil, 0, err
	}

	_, err = w.writeBox(&mp4.Tfhd{ // <tfhd/>
		FullBox: mp4.FullBox{
			Flags: [3]byte{2, 0, 56},
		},
		TrackID: uint32(trackID),
		// in AAC, an AU always contains 1024 samples
		DefaultSampleDuration: 1024,
	})
	if err != nil {
		return nil, 0, err
	}

	_, err = w.writeBox(&mp4.Tfdt{ // <tfdt/>
		FullBox: mp4.FullBox{
			Version: 1,
		},
		// sum of decode durations of all earlier samples
		BaseMediaDecodeTimeV1: uint64(audioEntries[0].pts * time.Duration(audioTrack.ClockRate()) / time.Second),
	})
	if err != nil {
		return nil, 0, err
	}

	trun := &mp4.Trun{ // <trun/>
		FullBox: mp4.FullBox{
			Version: 0,
			Flags:   [3]byte{0, 0x02, 0x01},
		},
		SampleCount: uint32(len(audioEntries)),
	}

	for _, e := range audioEntries {
		trun.Entries = append(trun.Entries, mp4.TrunEntry{
			SampleSize: uint32(len(e.au)),
		})
	}

	trunOffset, err := w.writeBox(trun)
	if err != nil {
		return nil, 0, err
	}

	err = w.writeBoxEnd() // </traf>
	if err != nil {
		return nil, 0, err
	}

	return trun, trunOffset, nil
}

func mp4PartGenerate(
	videoTrack *gortsplib.TrackH264,
	audioTrack *gortsplib.TrackAAC,
	videoEntries []fmp4PartVideoEntry,
	audioEntries []fmp4PartAudioEntry,
	startDTS time.Duration,
	sampleDuration time.Duration,
) ([]byte, error) {
	/*
		moof
		- mfhd
		- traf (video)
		- traf (audio)
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

	trackID := 1

	var videoTrun *mp4.Trun
	var videoTrunOffset int
	if videoTrack != nil {
		var err error
		videoTrun, videoTrunOffset, err = mp4PartGenerateVideoTraf(w, trackID, videoEntries, startDTS, sampleDuration)
		if err != nil {
			return nil, err
		}

		trackID++
	}

	var audioTrun *mp4.Trun
	var audioTrunOffset int
	if audioTrack != nil {
		var err error
		audioTrun, audioTrunOffset, err = mp4PartGenerateAudioTraf(w, trackID, audioTrack, audioEntries)
		if err != nil {
			return nil, err
		}
	}

	err = w.writeBoxEnd() // </moof>
	if err != nil {
		return nil, err
	}

	mdat := &mp4.Mdat{} // <mdat/>

	dataSize := 0
	videoDataSize := 0

	if videoTrack != nil {
		for _, e := range videoEntries {
			dataSize += len(e.avcc)
		}
		videoDataSize = dataSize
	}

	if audioTrack != nil {
		for _, e := range audioEntries {
			dataSize += len(e.au)
		}
	}

	mdat.Data = make([]byte, dataSize)
	pos := 0

	if videoTrack != nil {
		for _, e := range videoEntries {
			pos += copy(mdat.Data[pos:], e.avcc)
		}
	}

	if audioTrack != nil {
		for _, e := range audioEntries {
			pos += copy(mdat.Data[pos:], e.au)
		}
	}

	mdatOffset, err := w.writeBox(mdat)
	if err != nil {
		return nil, err
	}

	if videoTrack != nil {
		videoTrun.DataOffset = int32(mdatOffset - moofOffset + 8)
		err = w.rewriteBox(videoTrunOffset, videoTrun)
		if err != nil {
			return nil, err
		}
	}

	if audioTrack != nil && audioTrun != nil {
		audioTrun.DataOffset = int32(videoDataSize + mdatOffset - moofOffset + 8)
		err = w.rewriteBox(audioTrunOffset, audioTrun)
		if err != nil {
			return nil, err
		}
	}

	return w.bytes(), nil
}

func fmp4PartName(id uint64) string {
	return "part" + strconv.FormatUint(id, 10)
}

type muxerVariantFMP4Part struct {
	videoTrack     *gortsplib.TrackH264
	audioTrack     *gortsplib.TrackAAC
	id             uint64
	startDTS       time.Duration
	sampleDuration time.Duration

	videoEntries []fmp4PartVideoEntry
	audioEntries []fmp4PartAudioEntry
	rendered     []byte
	duration     time.Duration
}

func newMuxerVariantFMP4Part(
	videoTrack *gortsplib.TrackH264,
	audioTrack *gortsplib.TrackAAC,
	id uint64,
	startDTS time.Duration,
	sampleDuration time.Duration,
) *muxerVariantFMP4Part {
	return &muxerVariantFMP4Part{
		videoTrack:     videoTrack,
		audioTrack:     audioTrack,
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
	p.rendered, err = mp4PartGenerate(
		p.videoTrack,
		p.audioTrack,
		p.videoEntries,
		p.audioEntries,
		p.startDTS,
		p.sampleDuration)
	if err != nil {
		return err
	}

	if p.videoTrack != nil {
		p.duration = time.Duration(len(p.videoEntries)) * p.sampleDuration
	} else {
		p.duration = time.Duration(len(p.audioEntries)) * 1024 * time.Second / time.Duration(p.audioTrack.ClockRate())
	}

	p.videoEntries = nil
	p.audioEntries = nil

	return nil
}

func (p *muxerVariantFMP4Part) writeH264(pts time.Duration, nalus [][]byte) error {
	avcc, err := h264.AVCCEncode(nalus)
	if err != nil {
		return err
	}

	p.videoEntries = append(p.videoEntries, fmp4PartVideoEntry{
		pts:  pts,
		avcc: avcc,
	})

	return nil
}

func (p *muxerVariantFMP4Part) writeAAC(pts time.Duration, aus [][]byte) error {
	for _, au := range aus {
		p.audioEntries = append(p.audioEntries, fmp4PartAudioEntry{
			pts: pts,
			au:  au,
		})
	}

	return nil
}
