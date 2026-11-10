package mpegts

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestLanguageFromMPEGTS(t *testing.T) {
	for _, ca := range []struct{ in, out string }{
		{"eng", "en"},
		{"ITA", "it"},
		{"deu", "de"},
		{"ger", "de"}, // bibliographic
		{"fre", "fr"}, // bibliographic
		{"fra", "fr"},
		{"und", "und"},
		{"qaa", "qaa"}, // no two-letter equivalent
		{"", ""},
		{"en", "en"},
		{"1234", ""},
	} {
		t.Run(ca.in, func(t *testing.T) {
			require.Equal(t, ca.out, languageFromMPEGTS(ca.in))
		})
	}
}

func TestLanguageToMPEGTS(t *testing.T) {
	for _, ca := range []struct{ in, out string }{
		{"en", "eng"},
		{"it", "ita"},
		{"en-US", "eng"},
		{"zh-Hant-TW", "zho"},
		{"de", "deu"},
		{"EN", "eng"},
		{"eng", "eng"},
		{"qaa", "qaa"},
		{"", ""},
		{"x-private", ""},
		{"!!", ""},
	} {
		t.Run(ca.in, func(t *testing.T) {
			require.Equal(t, ca.out, languageToMPEGTS(ca.in))
		})
	}
}
