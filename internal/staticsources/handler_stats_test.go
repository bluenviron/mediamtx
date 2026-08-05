package staticsources

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/bluenviron/mediamtx/internal/defs"
	"github.com/bluenviron/mediamtx/internal/logger"
)

// fakeStatsSource is a staticSource that also provides stats.
type fakeStatsSource struct {
	stats defs.StaticSourceStats
}

func (*fakeStatsSource) Log(logger.Level, string, ...any) {}

func (*fakeStatsSource) Run(defs.StaticSourceRunParams) error { return nil }

func (*fakeStatsSource) APISourceDescribe() *defs.APIPathSource {
	return &defs.APIPathSource{}
}

func (s *fakeStatsSource) SourceStats() defs.StaticSourceStats { return s.stats }

var _ defs.StaticSourceStatsProvider = (*fakeStatsSource)(nil)

// fakeNoStatsSource is a staticSource that does NOT provide stats.
type fakeNoStatsSource struct{}

func (*fakeNoStatsSource) Log(logger.Level, string, ...any) {}

func (*fakeNoStatsSource) Run(defs.StaticSourceRunParams) error { return nil }

func (*fakeNoStatsSource) APISourceDescribe() *defs.APIPathSource {
	return &defs.APIPathSource{}
}

func TestHandlerSourceStatsProvider(t *testing.T) {
	jitter := 0.25
	sentinel := &defs.RTSPSourceStats{
		BaseSourceStats: defs.BaseSourceStats{
			PacketsReceived: 123,
			PacketsLost:     4,
			Jitter:          &jitter,
		},
		PacketsInError: 1,
	}

	h := &Handler{instance: &fakeStatsSource{stats: sentinel}}

	got := h.SourceStats()
	require.Same(t, sentinel, got)
}

func TestHandlerSourceStatsProviderNil(t *testing.T) {
	// a provider may legitimately return nil (e.g. not yet connected)
	h := &Handler{instance: &fakeStatsSource{stats: nil}}
	require.Nil(t, h.SourceStats())
}

func TestHandlerSourceStatsNotProvided(t *testing.T) {
	// sources that do not implement the capability must yield nil
	h := &Handler{instance: &fakeNoStatsSource{}}
	require.Nil(t, h.SourceStats())
}
