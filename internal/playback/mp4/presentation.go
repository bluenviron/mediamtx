// Package mp4 contains a MP4 muxer.
package mp4

import (
	"io"
	"time"

	"github.com/abema/go-mp4"
	"github.com/bluenviron/mediacommon/pkg/formats/fmp4/seekablebuffer"
)

const (
	globalTimescale = 1000
)

func durationMp4ToGo(v int64, timeScale uint32) time.Duration {
	timeScale64 := int64(timeScale)
	secs := v / timeScale64
	dec := v % timeScale64
	return time.Duration(secs)*time.Second + time.Duration(dec)*time.Second/time.Duration(timeScale64)
}

// Presentation is timed sequence of video/audio samples.
type Presentation struct {
	Tracks []*Track
}

// Marshal encodes a Presentation.
func (p *Presentation) Marshal(w io.Writer) error {
	/*
		|ftyp|
		|moov|
		|    |mvhd|
		|    |trak|
		|    |trak|
		|    |....|
		|mdat|
	*/

	dataSize, sortedSamples := p.sortSamples()

	err := p.marshalFtypAndMoov(w)
	if err != nil {
		return err
	}

	return p.marshalMdat(w, dataSize, sortedSamples)
}

func (p *Presentation) sortSamples() (uint32, []*Sample) {
	sampleCount := 0
	for _, track := range p.Tracks {
		sampleCount += len(track.Samples)
	}

	processedSamples := make([]int, len(p.Tracks))
	elapsed := make([]int64, len(p.Tracks))
	offset := uint32(0)
	sortedSamples := make([]*Sample, sampleCount)
	pos := 0

	for i, track := range p.Tracks {
		elapsed[i] = int64(track.TimeOffset)
	}

	for {
		bestTrack := -1
		var bestElapsed time.Duration

		for i, track := range p.Tracks {
			if processedSamples[i] < len(track.Samples) {
				elapsedGo := durationMp4ToGo(elapsed[i], track.TimeScale)

				if bestTrack == -1 || elapsedGo < bestElapsed {
					bestTrack = i
					bestElapsed = elapsedGo
				}
			}
		}

		if bestTrack == -1 {
			break
		}

		sample := p.Tracks[bestTrack].Samples[processedSamples[bestTrack]]
		sample.offset = offset

		processedSamples[bestTrack]++
		elapsed[bestTrack] += int64(sample.Duration)
		offset += sample.PayloadSize
		sortedSamples[pos] = sample
		pos++
	}

	return offset, sortedSamples
}

func (p *Presentation) marshalFtypAndMoov(w io.Writer) error {
	var outBuf seekablebuffer.Buffer
	mw := newMP4Writer(&outBuf)

	_, err := mw.writeBox(&mp4.Ftyp{ // <ftyp/>
		MajorBrand:   [4]byte{'i', 's', 'o', 'm'},
		MinorVersion: 1,
		CompatibleBrands: []mp4.CompatibleBrandElem{
			{CompatibleBrand: [4]byte{'i', 's', 'o', 'm'}},
			{CompatibleBrand: [4]byte{'i', 's', 'o', '2'}},
			{CompatibleBrand: [4]byte{'m', 'p', '4', '1'}},
			{CompatibleBrand: [4]byte{'m', 'p', '4', '2'}},
		},
	})
	if err != nil {
		return err
	}

	_, err = mw.writeBoxStart(&mp4.Moov{}) // <moov>
	if err != nil {
		return err
	}

	mvhd := &mp4.Mvhd{ // <mvhd/>
		Timescale:   globalTimescale,
		Rate:        65536,
		Volume:      256,
		Matrix:      [9]int32{0x00010000, 0, 0, 0, 0x00010000, 0, 0, 0, 0x40000000},
		NextTrackID: uint32(len(p.Tracks) + 1),
	}
	mvhdOffset, err := mw.writeBox(mvhd)
	if err != nil {
		return err
	}

	stcos := make([]*mp4.Stco, len(p.Tracks))
	stcosOffsets := make([]int, len(p.Tracks))

	for i, track := range p.Tracks {
		var res *headerTrackMarshalResult
		res, err = track.marshal(mw)
		if err != nil {
			return err
		}

		stcos[i] = res.stco
		stcosOffsets[i] = res.stcoOffset

		if res.presentationDuration > mvhd.DurationV0 {
			mvhd.DurationV0 = res.presentationDuration
		}
	}

	err = mw.rewriteBox(mvhdOffset, mvhd)
	if err != nil {
		return err
	}

	err = mw.writeBoxEnd() // </moov>
	if err != nil {
		return err
	}

	moovEndOffset, err := outBuf.Seek(0, io.SeekCurrent)
	if err != nil {
		return err
	}

	dataOffset := moovEndOffset + 8

	for i := range p.Tracks {
		for j := range stcos[i].ChunkOffset {
			stcos[i].ChunkOffset[j] += uint32(dataOffset)
		}

		err = mw.rewriteBox(stcosOffsets[i], stcos[i])
		if err != nil {
			return err
		}
	}

	_, err = w.Write(outBuf.Bytes())
	return err
}

func (p *Presentation) marshalMdat(w io.Writer, dataSize uint32, sortedSamples []*Sample) error {
	mdatSize := uint32(8) + dataSize

	_, err := w.Write([]byte{byte(mdatSize >> 24), byte(mdatSize >> 16), byte(mdatSize >> 8), byte(mdatSize)})
	if err != nil {
		return err
	}

	_, err = w.Write([]byte{'m', 'd', 'a', 't'})
	if err != nil {
		return err
	}

	for _, sa := range sortedSamples {
		pl, err := sa.GetPayload()
		if err != nil {
			return err
		}

		_, err = w.Write(pl)
		if err != nil {
			return err
		}
	}

	return nil
}
