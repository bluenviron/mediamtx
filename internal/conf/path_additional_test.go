package conf

import (
	"regexp"
	"testing"

	"github.com/bluenviron/mediamtx/internal/logger"
	"github.com/stretchr/testify/require"
)

type _localNilLogger struct{}

func (_localNilLogger) Log(_ logger.Level, _ string, _ ...any) {}

var _testNilLogger logger.Writer = &_localNilLogger{}

func TestRedirectSourceValidation(t *testing.T) {
	pconf := &Path{
		Source:         "redirect",
		SourceRedirect: "/other",
		Name:           "redirect_test",
		RecordPath:     "/tmp/%path/%s",
	}

	err := pconf.validate(&Conf{}, "redirect_test", false, _testNilLogger)
	require.NoError(t, err)

	pconfEmpty := &Path{
		Source:     "redirect",
		Name:       "redirect_empty",
		RecordPath: "/tmp/%path/%s",
	}
	err = pconfEmpty.validate(&Conf{}, "redirect_empty", false, _testNilLogger)
	require.Error(t, err)
}

func TestPublisherSRTPassphraseValidation(t *testing.T) {
	// too short
	pconf := &Path{
		Source:               "publisher",
		SRTPublishPassphrase: "short",
		Name:                 "srt_test",
		RecordPath:           "/tmp/%path/%s",
	}
	err := pconf.validate(&Conf{}, "srt_test", false, _testNilLogger)
	require.Error(t, err)

	// acceptable length
	okPass := "0123456789"
	pconf.SRTPublishPassphrase = okPass
	err = pconf.validate(&Conf{}, "srt_test", false, _testNilLogger)
	require.NoError(t, err)
}

func TestFindPathConfStaticAndRegexp(t *testing.T) {
	static := &Path{Name: "static"}
	re := regexpMustCompile("^cam([0-9]+)$")
	regexpPath := &Path{Regexp: re, Name: "cam_re"}

	mp := map[string]*Path{
		"camera":         static,
		"~^cam([0-9]+)$": regexpPath,
	}

	p, _, err := FindPathConf(mp, "camera")
	require.NoError(t, err)
	require.Equal(t, static, p)

	p2, matches, err := FindPathConf(mp, "cam12")
	require.NoError(t, err)
	require.Equal(t, regexpPath, p2)
	require.NotNil(t, matches)
	require.Equal(t, "12", matches[1])
}

func TestPathHelpers(t *testing.T) {
	p1 := &Path{Source: "publisher"}
	p2 := &Path{Source: "static"}
	require.False(t, p1.HasStaticSource())
	require.True(t, p2.HasStaticSource())

	p1.RunOnDemand = "run"
	require.True(t, p1.HasOnDemandPublisher())

	p2.SourceOnDemand = true
	require.True(t, p2.HasOnDemandStaticSource())

	// Equal
	p3 := p2.Clone()
	require.True(t, p2.Equal(p3))
}

// helper to compile regex like in production
func regexpMustCompile(s string) *regexp.Regexp {
	r, err := regexp.Compile(s)
	if err != nil {
		panic(err)
	}
	return r
}
