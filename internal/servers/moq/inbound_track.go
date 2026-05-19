package moq

import (
	"github.com/bluenviron/mediamtx/internal/logger"
	"github.com/bluenviron/mediamtx/internal/protocols/moq/reorderer"
	"github.com/bluenviron/mediamtx/internal/protocols/moq/subgroup"
)

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

	for _, s := range sgs {
		err = t.onSubGroup(s)
		if err != nil {
			return err
		}
	}

	return nil
}
