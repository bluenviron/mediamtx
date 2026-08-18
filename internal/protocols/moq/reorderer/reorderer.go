// Package reorderer contains a subgroup reorderer.
package reorderer

import (
	"slices"
	"sync"

	"github.com/bluenviron/mediamtx/internal/logger"
	"github.com/bluenviron/mediamtx/internal/protocols/moq/subgroup"
)

// Reorderer is a subgroup reorderer.
type Reorderer struct {
	MaxReordered    int
	MaxPendingBytes int
	Parent          logger.Writer

	initialized  bool
	mu           sync.Mutex
	curGroupID   uint64
	pending      map[uint64]*subgroup.SubGroup
	pendingBytes int
}

func subGroupPayloadSize(sg *subgroup.SubGroup) int {
	n := 0
	for _, obj := range sg.Objects {
		n += len(obj.Payload)
	}
	return n
}

// Initialize initializes the reorderer.
func (r *Reorderer) Initialize() {
	r.pending = make(map[uint64]*subgroup.SubGroup)
}

// Push pushes a subgroup and returns a list of subgroups that are now in order.
func (r *Reorderer) Push(sg *subgroup.SubGroup) ([]*subgroup.SubGroup, error) {
	r.mu.Lock()
	defer r.mu.Unlock()

	if !r.initialized {
		r.initialized = true
		r.curGroupID = sg.Header.GroupID
		return []*subgroup.SubGroup{sg}, nil
	}

	switch {
	case sg.Header.GroupID <= r.curGroupID:
		r.Parent.Log(logger.Warn, "skipping out-of-order subgroup")

	case sg.Header.GroupID == r.curGroupID+1 && len(r.pending) == 0:
		r.curGroupID = sg.Header.GroupID
		return []*subgroup.SubGroup{sg}, nil

	default:
		if prev, ok := r.pending[sg.Header.GroupID]; ok {
			r.pendingBytes -= subGroupPayloadSize(prev)
		}
		r.pending[sg.Header.GroupID] = sg
		r.pendingBytes += subGroupPayloadSize(sg)

		diff := sg.Header.GroupID - r.curGroupID

		var countInRange uint64
		for id := range r.pending {
			if id <= sg.Header.GroupID {
				countInRange++
			}
		}

		switch {
		case countInRange == diff:
			return r.flushUpTo(sg.Header.GroupID), nil

		case len(r.pending) > r.MaxReordered:
			r.Parent.Log(logger.Warn, "too many reordered subgroups, flushing")
			return r.flushUpTo(sg.Header.GroupID), nil

		case r.pendingBytes > r.MaxPendingBytes:
			r.Parent.Log(logger.Warn, "too many reordered bytes, flushing")
			return r.flushUpTo(sg.Header.GroupID), nil
		}
	}

	return nil, nil
}

func (r *Reorderer) flushUpTo(maxGroupID uint64) []*subgroup.SubGroup {
	ids := make([]uint64, 0, len(r.pending))
	for id := range r.pending {
		if id > r.curGroupID && id <= maxGroupID {
			ids = append(ids, id)
		}
	}
	slices.Sort(ids)

	out := make([]*subgroup.SubGroup, 0, len(ids))
	for _, id := range ids {
		out = append(out, r.pending[id])
		r.pendingBytes -= subGroupPayloadSize(r.pending[id])
		delete(r.pending, id)
	}

	r.curGroupID = maxGroupID

	for {
		next, ok := r.pending[r.curGroupID+1]
		if !ok {
			break
		}
		out = append(out, next)
		r.pendingBytes -= subGroupPayloadSize(next)
		delete(r.pending, r.curGroupID+1)
		r.curGroupID++
	}

	return out
}
