// Package moq contains the MoQ static source.
package moq

import (
	"encoding/json"
	"fmt"
	"net/url"

	"github.com/bluenviron/gortsplib/v5/pkg/description"

	"github.com/bluenviron/mediamtx/internal/conf"
	"github.com/bluenviron/mediamtx/internal/defs"
	"github.com/bluenviron/mediamtx/internal/logger"
	protomoq "github.com/bluenviron/mediamtx/internal/protocols/moq"
	"github.com/bluenviron/mediamtx/internal/protocols/moq/catalog"
	"github.com/bluenviron/mediamtx/internal/protocols/moq/reorderer"
	"github.com/bluenviron/mediamtx/internal/protocols/moq/subgroup"
	ptls "github.com/bluenviron/mediamtx/internal/protocols/tls"
	"github.com/bluenviron/mediamtx/internal/stream"
)

const maxReorderedSubGroups = 50

type parent interface {
	logger.Writer
	SetReady(req defs.PathSourceStaticSetReadyReq) defs.PathSourceStaticSetReadyRes
	SetNotReady(req defs.PathSourceStaticSetNotReadyReq)
}

type inboundTrack struct {
	onSubGroup func(sg *subgroup.SubGroup) error
	parent     logger.Writer

	reorderer *reorderer.Reorderer
}

func (t *inboundTrack) initialize() {
	t.reorderer = &reorderer.Reorderer{
		MaxReordered: maxReorderedSubGroups,
		Parent:       t.parent,
	}
	t.reorderer.Initialize()
}

func (t *inboundTrack) push(sg *subgroup.SubGroup) error {
	sgs, err := t.reorderer.Push(sg)
	if err != nil {
		return err
	}

	for _, sg := range sgs {
		err = t.onSubGroup(sg)
		if err != nil {
			return err
		}
	}

	return nil
}

// Source is a MoQ static source.
type Source struct {
	ReadTimeout conf.Duration
	Parent      parent
}

// Log implements logger.Writer.
func (s *Source) Log(level logger.Level, format string, args ...any) {
	s.Parent.Log(level, "[MoQ source] "+format, args...)
}

// Run implements StaticSource.
func (s *Source) Run(params defs.StaticSourceRunParams) error {
	u, err := url.Parse(params.ResolvedSource)
	if err != nil {
		return err
	}

	client := &protomoq.Client{
		URL:       u,
		Transport: params.Conf.MoQTransport,
		TLSConfig: ptls.MakeConfig(params.Conf.SourceFingerprint),
		Log:       s,
	}
	if err = client.Initialize(params.Context); err != nil {
		return err
	}
	defer client.Close() //nolint:errcheck

	catalogSg := make(chan *subgroup.SubGroup, 1)
	err = client.Subscribe(params.Context, ".catalog", func(sg *subgroup.SubGroup) error {
		select {
		case catalogSg <- sg:
			return nil
		case <-params.Context.Done():
			return params.Context.Err()
		}
	})
	if err != nil {
		return err
	}

	var sg *subgroup.SubGroup
	select {
	case sg = <-catalogSg:
	case err = <-client.Errors():
		return err
	case <-params.Context.Done():
		return nil
	}

	if len(sg.Objects) == 0 {
		return fmt.Errorf("received empty catalog")
	}

	var cat catalog.Catalog
	err = json.Unmarshal(sg.Objects[0].Payload, &cat)
	if err != nil {
		return fmt.Errorf("failed to parse catalog JSON: %w", err)
	}

	var subStream *stream.SubStream
	medias, writeFuncs, err := protomoq.ToStream(&cat, &subStream)
	if err != nil {
		return err
	}

	res := s.Parent.SetReady(defs.PathSourceStaticSetReadyReq{
		Desc:          &description.Session{Medias: medias},
		UseRTPPackets: false,
		ReplaceNTP:    !params.Conf.UseAbsoluteTimestamp,
	})
	if res.Err != nil {
		return res.Err
	}
	defer s.Parent.SetNotReady(defs.PathSourceStaticSetNotReadyReq{})

	subStream = res.SubStream

	for i, track := range cat.Tracks {
		writer := writeFuncs[uint64(i+1)]
		if writer == nil {
			return fmt.Errorf("missing writer for track %s", track.Name)
		}

		tr := &inboundTrack{onSubGroup: writer, parent: s}
		tr.initialize()

		err = client.Subscribe(params.Context, track.Name, tr.push)
		if err != nil {
			return err
		}
	}

	select {
	case err = <-client.Errors():
		return err
	case <-params.ReloadConf:
		return nil
	case <-params.Context.Done():
		return nil
	}
}

// APISourceDescribe implements StaticSource.
func (*Source) APISourceDescribe() *defs.APIPathSource {
	return &defs.APIPathSource{
		Type: defs.APIPathSourceTypeMoQSource,
		ID:   "",
	}
}
