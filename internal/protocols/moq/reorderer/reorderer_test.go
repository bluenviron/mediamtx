package reorderer

import (
	"testing"

	"github.com/bluenviron/mediamtx/internal/logger"
	"github.com/bluenviron/mediamtx/internal/protocols/moq/subgroup"
	"github.com/stretchr/testify/require"
)

type nopLogger struct{}

func (nopLogger) Log(logger.Level, string, ...any) {}

func makeSG(groupID uint64) *subgroup.SubGroup {
	return &subgroup.SubGroup{
		Header: subgroup.Header{GroupID: groupID},
	}
}

func TestReordererFirstPush(t *testing.T) {
	r := &Reorderer{MaxReordered: 5, Parent: nopLogger{}}
	r.Initialize()

	sg0 := makeSG(0)
	out, err := r.Push(sg0)
	require.NoError(t, err)
	require.Equal(t, []*subgroup.SubGroup{sg0}, out)
}

func TestReordererInOrder(t *testing.T) {
	r := &Reorderer{MaxReordered: 5, Parent: nopLogger{}}
	r.Initialize()

	sg0 := makeSG(0)
	out, err := r.Push(sg0)
	require.NoError(t, err)
	require.Equal(t, []*subgroup.SubGroup{sg0}, out)

	sg1 := makeSG(1)
	out, err = r.Push(sg1)
	require.NoError(t, err)
	require.Equal(t, []*subgroup.SubGroup{sg1}, out)

	sg2 := makeSG(2)
	out, err = r.Push(sg2)
	require.NoError(t, err)
	require.Equal(t, []*subgroup.SubGroup{sg2}, out)
}

func TestReordererStale(t *testing.T) {
	r := &Reorderer{MaxReordered: 5, Parent: nopLogger{}}
	r.Initialize()

	sg5 := makeSG(5)
	out, err := r.Push(sg5)
	require.NoError(t, err)
	require.Equal(t, []*subgroup.SubGroup{sg5}, out)

	// same GroupID again
	out, err = r.Push(makeSG(5))
	require.NoError(t, err)
	require.Nil(t, out)

	// older GroupID
	out, err = r.Push(makeSG(3))
	require.NoError(t, err)
	require.Nil(t, out)
}

func TestReordererOutOfOrderPending(t *testing.T) {
	r := &Reorderer{MaxReordered: 5, Parent: nopLogger{}}
	r.Initialize()

	sg0 := makeSG(0)
	out, err := r.Push(sg0)
	require.NoError(t, err)
	require.Equal(t, []*subgroup.SubGroup{sg0}, out)

	// sg2 arrives before sg1; held in pending
	out, err = r.Push(makeSG(2))
	require.NoError(t, err)
	require.Nil(t, out)
}

func TestReordererOutOfOrderFilled(t *testing.T) {
	r := &Reorderer{MaxReordered: 5, Parent: nopLogger{}}
	r.Initialize()

	sg0 := makeSG(0)
	out, err := r.Push(sg0)
	require.NoError(t, err)
	require.Equal(t, []*subgroup.SubGroup{sg0}, out)

	// sg2 arrives out of order, held in pending
	sg2 := makeSG(2)
	out, err = r.Push(sg2)
	require.NoError(t, err)
	require.Nil(t, out)

	// sg1 (the missing piece) arrives; gap is now filled; sg1 and the
	// already-pending sg2 are both returned together
	sg1 := makeSG(1)
	out, err = r.Push(sg1)
	require.NoError(t, err)
	require.Equal(t, []*subgroup.SubGroup{sg1, sg2}, out)
}

func TestReordererDrainAfterFill(t *testing.T) { //nolint:dupl
	r := &Reorderer{MaxReordered: 5, Parent: nopLogger{}}
	r.Initialize()

	sg0 := makeSG(0)
	out, err := r.Push(sg0)
	require.NoError(t, err)
	require.Equal(t, []*subgroup.SubGroup{sg0}, out)

	// sg2 and sg3 arrive before sg1
	sg2 := makeSG(2)
	out, err = r.Push(sg2)
	require.NoError(t, err)
	require.Nil(t, out)

	sg3 := makeSG(3)
	out, err = r.Push(sg3)
	require.NoError(t, err)
	require.Nil(t, out)

	// sg1 arrives; fills the gap; sg1, sg2, and sg3 are all returned together
	sg1 := makeSG(1)
	out, err = r.Push(sg1)
	require.NoError(t, err)
	require.Equal(t, []*subgroup.SubGroup{sg1, sg2, sg3}, out)
}

func TestReordererMaxReordered(t *testing.T) { //nolint:dupl
	r := &Reorderer{MaxReordered: 2, Parent: nopLogger{}}
	r.Initialize()

	sg0 := makeSG(0)
	out, err := r.Push(sg0)
	require.NoError(t, err)
	require.Equal(t, []*subgroup.SubGroup{sg0}, out)

	// first out-of-order item (pending=1, not over limit)
	sg2 := makeSG(2)
	out, err = r.Push(sg2)
	require.NoError(t, err)
	require.Nil(t, out)

	// second out-of-order item (pending=2, not over limit)
	sg4 := makeSG(4)
	out, err = r.Push(sg4)
	require.NoError(t, err)
	require.Nil(t, out)

	// third out-of-order item pushes pending to 3, which exceeds MaxReordered=2;
	// all available items are flushed in order, skipping the missing ones (1, 3)
	sg5 := makeSG(5)
	out, err = r.Push(sg5)
	require.NoError(t, err)
	require.Equal(t, []*subgroup.SubGroup{sg2, sg4, sg5}, out)
}
