package conf

import (
	"fmt"
	"reflect"
	"regexp"
	"strings"
)

const redactedCredential = "<redacted>"

var reSRTSecretParam = regexp.MustCompile(`([?&])(passphrase|streamid)=([^&#]*)`)

func checkNotRedacted(field string, value string) error {
	if value == redactedCredential {
		return fmt.Errorf("'%s' cannot be %q, "+
			"this word is reserved to avoid confusion with redacted values returned by the API",
			field, redactedCredential)
	}
	return nil
}

func checkURLNotRedacted(field string, value string) error {
	redacted := false

	if strings.HasPrefix(value, "srt://") {
		for _, parts := range reSRTSecretParam.FindAllStringSubmatch(value, -1) {
			redacted = redacted || parts[3] == redactedCredential
		}
	} else {
		u := splitURLUserinfo(value)
		redacted = u.hasValue && (u.user == redactedCredential || u.pass == redactedCredential)
	}

	if redacted {
		return fmt.Errorf("'%s' cannot contain %q, "+
			"this word is reserved to avoid confusion with redacted values returned by the API",
			field, redactedCredential)
	}
	return nil
}

func redactSecretString(s string) string {
	if s == "" {
		return s
	}
	return redactedCredential
}

func redactSecretCredential(c Credential) Credential {
	if c == "" {
		return c
	}
	return Credential(redactedCredential)
}

func restoreSecret(s string, old string) string {
	if s == redactedCredential {
		return old
	}
	return s
}

func splitURLUserinfo(s string) urlUserinfo {
	schemeEnd := strings.Index(s, "://")
	if schemeEnd < 0 {
		return urlUserinfo{}
	}
	authorityStart := schemeEnd + len("://")

	authorityLen := strings.IndexAny(s[authorityStart:], "/?#")
	if authorityLen < 0 {
		authorityLen = len(s) - authorityStart
	}
	authority := s[authorityStart : authorityStart+authorityLen]

	at := strings.LastIndex(authority, "@")
	if at < 0 {
		return urlUserinfo{}
	}

	userinfo := authority[:at]
	user, pass, hasPass := strings.Cut(userinfo, ":")

	return urlUserinfo{
		head:     s[:authorityStart],
		user:     user,
		pass:     pass,
		tail:     s[authorityStart+at:],
		hasPass:  hasPass,
		hasValue: true,
	}
}

func redactLegacyICEServer(server string) string {
	parts := strings.SplitN(server, ":", 4)
	if len(parts) != 4 {
		return server
	}
	parts[1] = redactSecretString(parts[1])
	parts[2] = redactSecretString(parts[2])
	return strings.Join(parts, ":")
}

func restoreLegacyICEServer(server string, old string) string {
	parts := strings.SplitN(server, ":", 4)
	oldParts := strings.SplitN(old, ":", 4)
	if len(parts) != 4 || len(oldParts) != 4 {
		return server
	}
	parts[1] = restoreSecret(parts[1], oldParts[1])
	parts[2] = restoreSecret(parts[2], oldParts[2])
	return strings.Join(parts, ":")
}

func redactURL(s string) string {
	if strings.HasPrefix(s, "srt://") {
		return reSRTSecretParam.ReplaceAllStringFunc(s, func(match string) string {
			parts := reSRTSecretParam.FindStringSubmatch(match)
			if parts[3] == "" {
				return match
			}
			return parts[1] + parts[2] + "=" + redactedCredential
		})
	}

	u := splitURLUserinfo(s)
	if !u.hasValue {
		return s
	}
	return u.join(redactSecretString(u.user), redactSecretString(u.pass))
}

func restoreURL(s string, old string) string {
	if strings.HasPrefix(s, "srt://") {
		return reSRTSecretParam.ReplaceAllStringFunc(s, func(match string) string {
			parts := reSRTSecretParam.FindStringSubmatch(match)
			if parts[3] != redactedCredential {
				return match
			}
			oldValue, ok := srtURLParam(old, parts[2])
			if !ok {
				return match
			}
			return parts[1] + parts[2] + "=" + oldValue
		})
	}

	u := splitURLUserinfo(s)
	oldU := splitURLUserinfo(old)
	if !u.hasValue || !oldU.hasValue {
		return s
	}
	return u.join(restoreSecret(u.user, oldU.user), restoreSecret(u.pass, oldU.pass))
}

func srtURLParam(s string, name string) (string, bool) {
	for _, parts := range reSRTSecretParam.FindAllStringSubmatch(s, -1) {
		if parts[2] == name {
			return parts[3], true
		}
	}
	return "", false
}

func redactPath(p *Path) {
	p.Source = redactURL(p.Source)

	p.SRTReadPassphrase = redactSecretString(p.SRTReadPassphrase)
	p.SRTPublishPassphrase = redactSecretString(p.SRTPublishPassphrase)
	p.WHEPBearerToken = redactSecretString(p.WHEPBearerToken)

	for i := range p.Forward {
		p.Forward[i].Dest = redactURL(p.Forward[i].Dest)
		p.Forward[i].WHIPBearerToken = redactSecretString(p.Forward[i].WHIPBearerToken)
		for key, value := range p.Forward[i].DestHeaders {
			p.Forward[i].DestHeaders[key] = redactSecretString(value)
		}
	}

	if p.PublishPass != nil {
		*p.PublishPass = redactSecretCredential(*p.PublishPass)
	}
	if p.ReadPass != nil {
		*p.ReadPass = redactSecretCredential(*p.ReadPass)
	}
}

func restoreURLField(values reflect.Value, name string, old string) {
	field := values.FieldByName(name)
	if !field.IsValid() || field.IsNil() {
		return
	}
	field.Elem().SetString(restoreURL(field.Elem().String(), old))
}

func restoreStringField(values reflect.Value, name string, old string) {
	field := values.FieldByName(name)
	if !field.IsValid() || field.IsNil() {
		return
	}
	field.Elem().SetString(restoreSecret(field.Elem().String(), old))
}

// restoreRedactedPathValues replaces redacted values, as returned by the API,
// with the corresponding values of the old path.
func restoreRedactedPathValues(values any, old *Path) {
	rv := reflect.ValueOf(values).Elem()

	source := rv.FieldByName("Source")
	if source.IsValid() && !source.IsNil() {
		source.Elem().SetString(restoreURL(source.Elem().String(), old.Source))
	}

	restoreStringField(rv, "SRTReadPassphrase", old.SRTReadPassphrase)
	restoreStringField(rv, "SRTPublishPassphrase", old.SRTPublishPassphrase)
	restoreStringField(rv, "WHEPBearerToken", old.WHEPBearerToken)

	forward := rv.FieldByName("Forward")
	if forward.IsValid() && !forward.IsNil() {
		dests := *forward.Interface().(*Forward)
		for i := range dests {
			if i >= len(old.Forward) {
				break
			}
			dests[i].Dest = restoreURL(dests[i].Dest, old.Forward[i].Dest)
			dests[i].WHIPBearerToken = restoreSecret(dests[i].WHIPBearerToken, old.Forward[i].WHIPBearerToken)
			for key, value := range dests[i].DestHeaders {
				dests[i].DestHeaders[key] = restoreSecret(value, old.Forward[i].DestHeaders[key])
			}
		}
	}
}

// restoreRedactedGlobalValues replaces redacted values, as returned by the API,
// with the corresponding values of the old configuration.
func restoreRedactedGlobalValues(values any, old *Conf) {
	rv := reflect.ValueOf(values).Elem()

	restoreStringField(rv, "HLSCDNSecret", old.HLSCDNSecret)
	restoreURLField(rv, "AuthHTTPAddress", old.AuthHTTPAddress)
	restoreURLField(rv, "AuthJWTJWKS", old.AuthJWTJWKS)
	restoreURLField(rv, "ExternalAuthenticationURL", old.AuthHTTPAddress)

	legacyICEServers := rv.FieldByName("WebRTCICEServers")
	if !legacyICEServers.IsNil() && old.WebRTCICEServers != nil {
		servers := *legacyICEServers.Interface().(*[]string)
		for i := range servers {
			if i >= len(*old.WebRTCICEServers) {
				break
			}
			servers[i] = restoreLegacyICEServer(servers[i], (*old.WebRTCICEServers)[i])
		}
	}

	iceServers := rv.FieldByName("WebRTCICEServers2")
	if !iceServers.IsNil() {
		servers := *iceServers.Interface().(*[]WebRTCICEServer)
		for i := range servers {
			if i >= len(old.WebRTCICEServers2) {
				break
			}
			servers[i].Username = restoreSecret(servers[i].Username, old.WebRTCICEServers2[i].Username)
			servers[i].Password = restoreSecret(servers[i].Password, old.WebRTCICEServers2[i].Password)
		}
	}
}

// Redact clones a configuration and redacts credentials from it.
func Redact(c *Conf) *Conf {
	c = c.Clone()

	for i := range c.AuthInternalUsers {
		c.AuthInternalUsers[i].Pass = redactSecretCredential(c.AuthInternalUsers[i].Pass)
	}

	c.HLSCDNSecret = redactSecretString(c.HLSCDNSecret)

	if c.WebRTCICEServers != nil {
		for i, server := range *c.WebRTCICEServers {
			(*c.WebRTCICEServers)[i] = redactLegacyICEServer(server)
		}
	}
	c.AuthHTTPAddress = redactURL(c.AuthHTTPAddress)
	c.AuthJWTJWKS = redactURL(c.AuthJWTJWKS)

	if c.ExternalAuthenticationURL != nil {
		redacted := redactURL(*c.ExternalAuthenticationURL)
		c.ExternalAuthenticationURL = &redacted
	}

	for i := range c.WebRTCICEServers2 {
		c.WebRTCICEServers2[i].Username = redactSecretString(c.WebRTCICEServers2[i].Username)
		c.WebRTCICEServers2[i].Password = redactSecretString(c.WebRTCICEServers2[i].Password)
	}

	redactPath(&c.PathDefaults)

	for _, pathConf := range c.Paths {
		redactPath(pathConf)
	}

	return c
}

type urlUserinfo struct {
	head     string
	user     string
	pass     string
	tail     string
	hasPass  bool
	hasValue bool
}

func (u urlUserinfo) join(user string, pass string) string {
	userinfo := user
	if u.hasPass {
		userinfo += ":" + pass
	}
	return u.head + userinfo + u.tail
}
