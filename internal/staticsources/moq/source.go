// Package moq contains the MoQ static source.
package moq

import (
	"encoding/json"
	"fmt"
	"net/url"
	"sync"

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
	onSubGroup      func(sg *subgroup.SubGroup) error
	parent          logger.Writer
	addInboundBytes func(objects []subgroup.Object)

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
	t.addInboundBytes(sg.Objects)

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

	mutex        sync.RWMutex
	client       *protomoq.Client
	transport    string
	inboundBytes uint64
}

// Log implements logger.Writer.
func (s *Source) Log(level logger.Level, format string, args ...any) {
	s.Parent.Log(level, "[MoQ source] "+format, args...)
}

func (s *Source) addInboundBytes(objects []subgroup.Object) {
	n := 0
	for _, obj := range objects {
		n += len(obj.Payload)
	}

	s.mutex.Lock()
	defer s.mutex.Unlock()

	s.inboundBytes += uint64(n)
}

// Info returns runtime information.
func (s *Source) Info() defs.StaticSourceInfo {
	s.mutex.RLock()
	defer s.mutex.RUnlock()

	if s.transport == "" {
		return defs.StaticSourceInfo{}
	}

	return defs.StaticSourceInfo{
		TypeSpecific: &defs.APIStaticSourceTypeSpecificMoQ{
			RemoteAddr:   s.client.RemoteAddr().String(),
			Transport:    s.transport,
			InboundBytes: s.inboundBytes,
		},
	}
}

// Run implements StaticSource.
func (s *Source) Run(params defs.StaticSourceRunParams) error {
	u, err := url.Parse(params.ResolvedSource)
	if err != nil {
		return err
	}

	transport := params.Conf.MoQTransport
	if transport == "" {
		transport = conf.MoQTransportQUIC
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

	s.mutex.Lock()
	s.client = client
	s.transport = string(transport)
	s.mutex.Unlock()

	defer func() {
		s.mutex.Lock()
		s.client = nil
		s.transport = ""
		s.inboundBytes = 0
		s.mutex.Unlock()
	}()

	catalogSg := make(chan *subgroup.SubGroup, 1)
	err = client.Subscribe(params.Context, ".catalog", func(sg *subgroup.SubGroup) error {
		s.addInboundBytes(sg.Objects)

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

		tr := &inboundTrack{onSubGroup: writer, parent: s, addInboundBytes: s.addInboundBytes}
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
