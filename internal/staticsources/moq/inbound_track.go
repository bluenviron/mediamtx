package moq

import (
	"github.com/bluenviron/mediamtx/internal/protocols/moq/reorderer"
	"github.com/bluenviron/mediamtx/internal/protocols/moq/subgroup"
)

type inboundTrack struct {
	onSubGroup      func(sg *subgroup.SubGroup) error
	orchestrator    *reorderer.Orchestrator
	addInboundBytes func(objects []subgroup.Object)

	reorderer *reorderer.Reorderer
}

func (t *inboundTrack) initialize() {
	t.reorderer = &reorderer.Reorderer{
		Parent: t.orchestrator,
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
