package mpegts

import (
	"bytes"
	"context"
	"fmt"

	"github.com/aler9/gortsplib"
	"github.com/aler9/gortsplib/pkg/mpeg4audio"
	"github.com/asticode/go-astits"
)

func findMPEG4AudioConfig(dem *astits.Demuxer, pid uint16) (*mpeg4audio.Config, error) {
	for {
		data, err := dem.NextData()
		if err != nil {
			return nil, err
		}

		if data.PES == nil || data.PID != pid {
			continue
		}

		var adtsPkts mpeg4audio.ADTSPackets
		err = adtsPkts.Unmarshal(data.PES.Data)
		if err != nil {
			return nil, fmt.Errorf("unable to decode ADTS: %s", err)
		}

		pkt := adtsPkts[0]
		return &mpeg4audio.Config{
			Type:         pkt.Type,
			SampleRate:   pkt.SampleRate,
			ChannelCount: pkt.ChannelCount,
		}, nil
	}
}

// Track is a MPEG-TS track.
type Track struct {
	ES    *astits.PMTElementaryStream
	Track gortsplib.Track
}

// FindTracks finds the tracks in a MPEG-TS stream.
func FindTracks(byts []byte) ([]*Track, error) {
	var tracks []*Track
	dem := astits.NewDemuxer(context.Background(), bytes.NewReader(byts))

	for {
		data, err := dem.NextData()
		if err != nil {
			return nil, err
		}

		if data.PMT != nil {
			for _, es := range data.PMT.ElementaryStreams {
				switch es.StreamType {
				case astits.StreamTypeH264Video,
					astits.StreamTypeAACAudio:
				default:
					return nil, fmt.Errorf("track type %d not supported (yet)", es.StreamType)
				}

				tracks = append(tracks, &Track{
					ES: es,
				})
			}
			break
		}
	}

	if tracks == nil {
		return nil, fmt.Errorf("no tracks found")
	}

	for _, t := range tracks {
		switch t.ES.StreamType {
		case astits.StreamTypeH264Video:
			t.Track = &gortsplib.TrackH264{
				PayloadType:       96,
				PacketizationMode: 1,
			}

		case astits.StreamTypeAACAudio:
			conf, err := findMPEG4AudioConfig(dem, t.ES.ElementaryPID)
			if err != nil {
				return nil, err
			}

			t.Track = &gortsplib.TrackMPEG4Audio{
				PayloadType:      96,
				Config:           conf,
				SizeLength:       13,
				IndexLength:      3,
				IndexDeltaLength: 3,
			}
		}
	}

	return tracks, nil
}
