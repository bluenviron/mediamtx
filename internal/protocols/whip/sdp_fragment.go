package whip

import (
	"strings"

	"github.com/pion/sdp/v3"
)

// SDPFragment is a SDP fragment.
// It's basically a SDP without most mandatory fields.
type SDPFragment struct {
	Attributes []sdp.Attribute
	Medias     []*sdp.MediaDescription
}

// Unmarshal decodes a SDP fragment.
func (f *SDPFragment) Unmarshal(buf []byte) error {
	buf = append([]byte("v=0\r\no=- 0 0 IN IP4 0.0.0.0\r\ns=-\r\nt=0 0\r\n"), buf...)

	var sdp sdp.SessionDescription
	err := sdp.Unmarshal(buf)
	if err != nil {
		return err
	}

	f.Attributes = sdp.Attributes
	f.Medias = sdp.MediaDescriptions

	return nil
}

// Marshal encodes a SDP fragment.
func (f SDPFragment) Marshal() ([]byte, error) {
	var b strings.Builder

	for _, a := range f.Attributes {
		if a.Value != "" {
			b.WriteString("a=" + a.Key + ":" + a.Value + "\r\n")
		} else {
			b.WriteString("a=" + a.Key + "\r\n")
		}
	}

	for _, m := range f.Medias {
		b.WriteString("m=" + m.MediaName.String() + "\r\n")
		for _, a := range m.Attributes {
			if a.Value != "" {
				b.WriteString("a=" + a.Key + ":" + a.Value + "\r\n")
			} else {
				b.WriteString("a=" + a.Key + "\r\n")
			}
		}
	}

	return []byte(b.String()), nil
}
