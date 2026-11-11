package mpegts

import (
	"strings"

	"golang.org/x/text/language"
)

// ISO 639-2 bibliographic codes that differ from the terminological ones.
// MPEG-TS streams can use either.
var bibliographicToTerminological = map[string]string{
	"alb": "sqi", "arm": "hye", "baq": "eus", "bur": "mya", "chi": "zho",
	"cze": "ces", "dut": "nld", "fre": "fra", "geo": "kat", "ger": "deu",
	"gre": "ell", "ice": "isl", "mac": "mkd", "mao": "mri", "may": "msa",
	"per": "fas", "rum": "ron", "slo": "slk", "tib": "bod", "wel": "cym",
}

func isThreeLetters(v string) bool {
	if len(v) != 3 {
		return false
	}
	for _, c := range v {
		if c < 'a' || c > 'z' {
			return false
		}
	}
	return true
}

// languageFromMPEGTS converts an ISO 639-2 language code (used by MPEG-TS)
// into a language tag (RFC 5646, used by the SDP lang attribute).
// RFC 5646 requires the shortest ISO 639 code, therefore "eng" becomes "en".
// Codes without a two-letter equivalent are returned unchanged.
// An empty string is returned if the code is not valid.
func languageFromMPEGTS(v string) string {
	v = strings.ToLower(v)

	if t, ok := bibliographicToTerminological[v]; ok {
		v = t
	}

	if base, err := language.ParseBase(v); err == nil {
		return base.String()
	}

	// unknown to the library but still well-formed (e.g. private use codes)
	if isThreeLetters(v) {
		return v
	}

	return ""
}

// languageToMPEGTS converts a language tag (RFC 5646, used by the SDP lang attribute)
// into an ISO 639-2 language code (used by MPEG-TS).
// Only the primary language subtag is kept, since MPEG-TS cannot represent
// region or script, therefore "en-US" becomes "eng".
// An empty string is returned if the tag cannot be represented.
func languageToMPEGTS(v string) string {
	primary, _, _ := strings.Cut(strings.ToLower(v), "-")

	if t, ok := bibliographicToTerminological[primary]; ok {
		primary = t
	}

	if base, err := language.ParseBase(primary); err == nil {
		if iso3 := base.ISO3(); iso3 != "" {
			return iso3
		}
	}

	if isThreeLetters(primary) {
		return primary
	}

	return ""
}
