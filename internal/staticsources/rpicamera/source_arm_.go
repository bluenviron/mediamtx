//go:build (linux && arm) || (linux && arm64)

package rpicamera

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/bluenviron/gortsplib/v5/pkg/description"
	"github.com/bluenviron/gortsplib/v5/pkg/format"
	"github.com/bluenviron/gortsplib/v5/pkg/format/rtph264"
	"github.com/bluenviron/gortsplib/v5/pkg/format/rtpmjpeg"
	"github.com/bluenviron/mediacommon/v2/pkg/codecs/h264"
	"github.com/bluenviron/mediamtx/internal/defs"
	"github.com/bluenviron/mediamtx/internal/logger"
	"github.com/bluenviron/mediamtx/internal/stream"
	"github.com/bluenviron/mediamtx/internal/unit"
	"github.com/pion/rtp"
)

const (
	pauseBetweenErrors = 1 * time.Second
)

type secondaryReader struct {
	ctx       context.Context
	ctxCancel func()
}

// Close implements reader.
func (r *secondaryReader) Close() {
	r.ctxCancel()
}

// APIReaderDescribe implements reader.
func (*secondaryReader) APIReaderDescribe() *defs.APIPathReader {
	return &defs.APIPathReader{
		Type: defs.APIPathReaderTypeHidden,
		ID:   "",
	}
}

// Run implements StaticSource.
func (s *Source) Run(params defs.StaticSourceRunParams) error {
	if !params.Conf.RPICameraSecondary {
		return s.runPrimary(params)
	}
	return s.runSecondary(params)
}

func (s *Source) runPrimary(params defs.StaticSourceRunParams) error {
	var p cameraParams
	p.fromConf(s.LogLevel, params.Conf)

	var forma format.Format

	if p.Codec == "hardwareH264" || p.Codec == "softwareH264" {
		forma = &format.H264{
			PayloadTyp:        96,
			PacketizationMode: 1,
		}
	} else {
		forma = &format.MJPEG{}
	}

	var medias []*description.Media

	media := &description.Media{
		Type:    description.MediaTypeVideo,
		Formats: []format.Format{forma},
	}
	medias = append(medias, media)

	var mediaSecondary *description.Media

	if params.Conf.RPICameraSecondaryWidth != 0 {
		mediaSecondary = &description.Media{
			Type: description.MediaTypeApplication,
			Formats: []format.Format{&format.Generic{
				PayloadTyp: 96,
				RTPMa:      "rpicamera_secondary/90000",
				ClockRat:   90000,
			}},
		}
		medias = append(medias, mediaSecondary)
	}

	var encode func(au []byte) ([]*rtp.Packet, error)

	if p.Codec == "hardwareH264" || p.Codec == "softwareH264" {
		encH264 := &rtph264.Encoder{
			PayloadType:       96,
			PayloadMaxSize:    s.RTPMaxPayloadSize,
			PacketizationMode: 1,
		}
		err := encH264.Init()
		if err != nil {
			return err
		}

		encode = func(au []byte) ([]*rtp.Packet, error) {
			var nalus h264.AnnexB
			err = nalus.Unmarshal(au)
			if err != nil {
				return nil, err
			}

			return encH264.Encode(nalus)
		}
	} else {
		encMJPEG := &rtpmjpeg.Encoder{
			PayloadMaxSize: s.RTPMaxPayloadSize,
		}
		err := encMJPEG.Init()
		if err != nil {
			return err
		}

		encode = func(au []byte) ([]*rtp.Packet, error) {
			return encMJPEG.Encode(au)
		}
	}

	var subStream *stream.SubStream

	initializeSubStream := func() {
		if subStream == nil {
			res := s.Parent.SetReady(defs.PathSourceStaticSetReadyReq{
				Desc:          &description.Session{Medias: medias},
				UseRTPPackets: true,
				ReplaceNTP:    false,
			})
			if res.Err != nil {
				panic("should not happen")
			}

			subStream = res.SubStream
		}
	}

	onData := func(pts int64, ntp time.Time, au []byte) {
		initializeSubStream()

		pkts, err2 := encode(au)
		if err2 != nil {
			s.Log(logger.Error, err2.Error())
			return
		}

		for _, pkt := range pkts {
			pkt.Timestamp = uint32(pts)
			subStream.WriteUnit(media, media.Formats[0], &unit.Unit{
				PTS:        pts,
				NTP:        ntp,
				RTPPackets: []*rtp.Packet{pkt},
			})
		}
	}

	var onDataSecondary func(pts int64, ntp time.Time, au []byte)

	if params.Conf.RPICameraSecondaryWidth != 0 {
		var encodeSecondary func(au []byte) ([]*rtp.Packet, error)

		if p.SecondaryCodec == "hardwareH264" || p.SecondaryCodec == "softwareH264" {
			secondaryEncH264 := &rtph264.Encoder{
				PayloadType:       96,
				PayloadMaxSize:    s.RTPMaxPayloadSize,
				PacketizationMode: 1,
			}
			err := secondaryEncH264.Init()
			if err != nil {
				panic(err)
			}

			encodeSecondary = func(au []byte) ([]*rtp.Packet, error) {
				var nalus h264.AnnexB
				err = nalus.Unmarshal(au)
				if err != nil {
					return nil, err
				}

				return secondaryEncH264.Encode(nalus)
			}
		} else {
			secondaryEncMJPEG := &rtpmjpeg.Encoder{
				PayloadMaxSize: s.RTPMaxPayloadSize,
			}
			err := secondaryEncMJPEG.Init()
			if err != nil {
				panic(err)
			}

			encodeSecondary = func(au []byte) ([]*rtp.Packet, error) {
				return secondaryEncMJPEG.Encode(au)
			}
		}

		onDataSecondary = func(pts int64, ntp time.Time, au []byte) {
			initializeSubStream()

			pkts, err2 := encodeSecondary(au)
			if err2 != nil {
				s.Log(logger.Error, err2.Error())
				return
			}

			for _, pkt := range pkts {
				pkt.Timestamp = uint32(pts)
				subStream.WriteUnit(mediaSecondary, mediaSecondary.Formats[0], &unit.Unit{
					PTS:        pts,
					NTP:        ntp,
					RTPPackets: []*rtp.Packet{pkt},
				})
			}
		}
	}

	defer func() {
		if subStream != nil {
			s.Parent.SetNotReady(defs.PathSourceStaticSetNotReadyReq{})
		}
	}()

	cam := &camera{
		params:          p,
		onData:          onData,
		onDataSecondary: onDataSecondary,
	}
	err := cam.initialize() //nolint:staticcheck
	if err != nil {         //nolint:staticcheck
		return err
	}
	defer cam.close()

	cameraErr := make(chan error)
	go func() {
		cameraErr <- cam.wait()
	}()

	for {
		select {
		case err = <-cameraErr:
			return err

		case cnf := <-params.ReloadConf:
			var p cameraParams
			p.fromConf(s.LogLevel, cnf)
			cam.reloadParams(p)

		case <-params.Context.Done():
			return nil
		}
	}
}

func (s *Source) runSecondary(params defs.StaticSourceRunParams) error {
	var p cameraParams
	p.fromConf(s.LogLevel, params.Conf)

	r := &secondaryReader{}
	r.ctx, r.ctxCancel = context.WithCancel(context.Background())
	defer r.ctxCancel()

	path, primaryStream, err := s.waitForPrimary(r, params)
	if err != nil {
		return err
	}

	defer path.RemoveReader(defs.PathRemoveReaderReq{Author: r})

	var forma format.Format
	if p.Codec == "hardwareH264" || p.Codec == "softwareH264" {
		forma = &format.H264{
			PayloadTyp:        96,
			PacketizationMode: 1,
		}
	} else {
		forma = &format.MJPEG{}
	}

	media := &description.Media{
		Type:    description.MediaTypeVideo,
		Formats: []format.Format{forma},
	}

	res := s.Parent.SetReady(defs.PathSourceStaticSetReadyReq{
		Desc:          &description.Session{Medias: []*description.Media{media}},
		UseRTPPackets: true,
	})
	if res.Err != nil {
		return res.Err
	}

	rdr := &stream.Reader{Parent: s}

	rdr.OnData(
		primaryStream.OrigDesc.Medias[1],
		primaryStream.OrigDesc.Medias[1].Formats[0],
		func(u *unit.Unit) error {
			clone := *u.RTPPackets[0]
			if p.Codec != "hardwareH264" && p.Codec != "softwareH264" {
				clone.PayloadType = 26
			}

			res.SubStream.WriteUnit(media, media.Formats[0], &unit.Unit{
				PTS:        u.PTS,
				NTP:        u.NTP,
				RTPPackets: []*rtp.Packet{&clone},
			})
			return nil
		})

	primaryStream.AddReader(rdr)
	defer primaryStream.RemoveReader(rdr)

	select {
	case err = <-rdr.Error():
		return err

	case <-r.ctx.Done():
		return fmt.Errorf("primary stream closed")

	case <-params.Context.Done():
		return fmt.Errorf("terminated")
	}
}

func (s *Source) waitForPrimary(
	r *secondaryReader,
	params defs.StaticSourceRunParams,
) (defs.Path, *stream.Stream, error) {
	for {
		res, err := s.Parent.AddReader(defs.PathAddReaderReq{
			Author: r,
			AccessRequest: defs.PathAccessRequest{
				Name:     params.Conf.RPICameraPrimaryName,
				SkipAuth: true,
			},
		})
		if err != nil {
			if _, ok := errors.AsType[*defs.PathNoStreamAvailableError](err); ok {
				select {
				case <-time.After(pauseBetweenErrors):
				case <-params.Context.Done():
					return nil, nil, fmt.Errorf("terminated")
				}
				continue
			}

			return nil, nil, err
		}

		return res.Path, res.Stream, nil
	}
}
