package whip

import (
	"fmt"
	"strings"

	"github.com/pion/webrtc/v4"
)

var linkHeaderCredentialReplacer = strings.NewReplacer(
	`\`, `\\`,
	`"`, `\"`,
)

func quoteCredential(v string) string {
	return linkHeaderCredentialReplacer.Replace(v)
}

func readQuotedCredential(v string) (string, string, bool) {
	if len(v) == 0 || v[0] != '"' {
		return "", "", false
	}

	var ret strings.Builder
	ret.Grow(len(v) - 1)

	escaped := false

	for i := 1; i < len(v); i++ {
		switch v[i] {
		case '\\':
			if escaped {
				ret.WriteByte('\\')
				escaped = false
			} else {
				escaped = true
			}

		case '"':
			if escaped {
				ret.WriteByte('"')
				escaped = false
			} else {
				return ret.String(), v[i+1:], true
			}

		default:
			if escaped {
				return "", "", false
			}
			ret.WriteByte(v[i])
		}
	}

	return "", "", false
}

// LinkHeaderMarshal encodes a link header.
func LinkHeaderMarshal(iceServers []webrtc.ICEServer) []string {
	ret := make([]string, len(iceServers))

	for i, server := range iceServers {
		var link strings.Builder

		link.WriteByte('<')
		link.WriteString(server.URLs[0])
		link.WriteString(`>; rel="ice-server"`)

		if server.Username != "" {
			link.WriteString(`; username="`)
			link.WriteString(quoteCredential(server.Username))
			link.WriteString(`"; credential="`)
			link.WriteString(quoteCredential(server.Credential.(string)))
			link.WriteString(`"; credential-type="password"`)
		}

		ret[i] = link.String()
	}

	return ret
}

// LinkHeaderUnmarshal decodes a link header.
func LinkHeaderUnmarshal(link []string) ([]webrtc.ICEServer, error) {
	ret := make([]webrtc.ICEServer, len(link))

	for i, li := range link {
		var ok bool
		li, ok = strings.CutPrefix(li, "<")
		if !ok {
			return nil, fmt.Errorf("invalid link header: '%s'", li)
		}

		var url string
		url, li, ok = strings.Cut(li, `>; rel="ice-server"`)
		if !ok {
			return nil, fmt.Errorf("invalid link header: '<%s'", li)
		}

		s := webrtc.ICEServer{
			URLs: []string{url},
		}

		if li != "" {
			li, ok = strings.CutPrefix(li, `; username=`)
			if !ok {
				return nil, fmt.Errorf("invalid link header: '<%s'", li)
			}

			s.Username, li, ok = readQuotedCredential(li)
			if !ok || s.Username == "" {
				return nil, fmt.Errorf("invalid link header: '<%s'", li)
			}

			li, ok = strings.CutPrefix(li, `; credential=`)
			if !ok {
				return nil, fmt.Errorf("invalid link header: '<%s'", li)
			}

			s.Credential, li, ok = readQuotedCredential(li)
			if !ok {
				return nil, fmt.Errorf("invalid link header: '<%s'", li)
			}

			li, ok = strings.CutPrefix(li, `; credential-type="password"`)
			if !ok || li != "" {
				return nil, fmt.Errorf("invalid link header: '<%s'", li)
			}

			s.CredentialType = webrtc.ICECredentialTypePassword
		}

		ret[i] = s
	}

	return ret, nil
}
