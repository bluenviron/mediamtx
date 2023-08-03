package whip

import (
	"fmt"
	"strconv"

	"github.com/pion/sdp/v3"
	"github.com/pion/webrtc/v3"
)

// ICEFragmentUnmarshal decodes an ICE fragment.
func ICEFragmentUnmarshal(buf []byte) ([]*webrtc.ICECandidateInit, error) {
	buf = append([]byte("v=0\r\no=- 0 0 IN IP4 0.0.0.0\r\ns=-\r\nt=0 0\r\n"), buf...)

	var sdp sdp.SessionDescription
	err := sdp.Unmarshal(buf)
	if err != nil {
		return nil, err
	}

	var ret []*webrtc.ICECandidateInit

	for _, media := range sdp.MediaDescriptions {
		mid, ok := media.Attribute("mid")
		if !ok {
			return nil, fmt.Errorf("mid attribute is missing")
		}

		tmp, err := strconv.ParseUint(mid, 10, 16)
		if err != nil {
			return nil, fmt.Errorf("invalid mid attribute")
		}
		midNum := uint16(tmp)

		for _, attr := range media.Attributes {
			if attr.Key == "candidate" {
				ret = append(ret, &webrtc.ICECandidateInit{
					Candidate:     attr.Value,
					SDPMid:        &mid,
					SDPMLineIndex: &midNum,
				})
			}
		}
	}

	return ret, nil
}

// ICEFragmentMarshal encodes an ICE fragment.
func ICEFragmentMarshal(offer string, candidates []*webrtc.ICECandidateInit) ([]byte, error) {
	var sdp sdp.SessionDescription
	err := sdp.Unmarshal([]byte(offer))
	if err != nil || len(sdp.MediaDescriptions) == 0 {
		return nil, err
	}

	firstMedia := sdp.MediaDescriptions[0]
	iceUfrag, _ := firstMedia.Attribute("ice-ufrag")
	icePwd, _ := firstMedia.Attribute("ice-pwd")

	candidatesByMedia := make(map[uint16][]*webrtc.ICECandidateInit)
	for _, candidate := range candidates {
		mid := *candidate.SDPMLineIndex
		candidatesByMedia[mid] = append(candidatesByMedia[mid], candidate)
	}

	frag := "a=ice-ufrag:" + iceUfrag + "\r\n" +
		"a=ice-pwd:" + icePwd + "\r\n"

	for mid, media := range sdp.MediaDescriptions {
		cbm, ok := candidatesByMedia[uint16(mid)]
		if ok {
			frag += "m=" + media.MediaName.String() + "\r\n" +
				"a=mid:" + strconv.FormatUint(uint64(mid), 10) + "\r\n"

			for _, candidate := range cbm {
				frag += "a=candidate:" + candidate.Candidate + "\r\n"
			}
		}
	}

	return []byte(frag), nil
}
