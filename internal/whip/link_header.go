package whip

import (
	"encoding/json"
	"fmt"
	"regexp"

	"github.com/pion/webrtc/v3"
)

func quoteCredential(v string) string {
	b, _ := json.Marshal(v)
	s := string(b)
	return s[1 : len(s)-1]
}

func unquoteCredential(v string) string {
	var s string
	json.Unmarshal([]byte("\""+v+"\""), &s) //nolint:errcheck
	return s
}

// LinkHeaderMarshal encodes a link header.
func LinkHeaderMarshal(iceServers []webrtc.ICEServer) []string {
	ret := make([]string, len(iceServers))

	for i, server := range iceServers {
		link := "<" + server.URLs[0] + ">; rel=\"ice-server\""
		if server.Username != "" {
			link += "; username=\"" + quoteCredential(server.Username) + "\"" +
				"; credential=\"" + quoteCredential(server.Credential.(string)) + "\"; credential-type=\"password\""
		}
		ret[i] = link
	}

	return ret
}

var reLink = regexp.MustCompile(`^<(.+?)>; rel="ice-server"(; username="(.+?)"` +
	`; credential="(.+?)"; credential-type="password")?`)

// LinkHeaderUnmarshal decodes a link header.
func LinkHeaderUnmarshal(link []string) ([]webrtc.ICEServer, error) {
	ret := make([]webrtc.ICEServer, len(link))

	for i, li := range link {
		m := reLink.FindStringSubmatch(li)
		if m == nil {
			return nil, fmt.Errorf("invalid link header: '%s'", li)
		}

		s := webrtc.ICEServer{
			URLs: []string{m[1]},
		}

		if m[3] != "" {
			s.Username = unquoteCredential(m[3])
			s.Credential = unquoteCredential(m[4])
			s.CredentialType = webrtc.ICECredentialTypePassword
		}

		ret[i] = s
	}

	return ret, nil
}
