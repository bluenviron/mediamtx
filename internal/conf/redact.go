package conf

import (
	"fmt"
	"reflect"
)

const redactedCredential = "<redacted>"

func checkNotRedacted(field string, value string) error {
	if value == redactedCredential {
		return fmt.Errorf("'%s' cannot be %q, "+
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

func redactPath(p *Path) {
	p.SRTReadPassphrase = redactSecretString(p.SRTReadPassphrase)
	p.SRTPublishPassphrase = redactSecretString(p.SRTPublishPassphrase)
	p.WHEPBearerToken = redactSecretString(p.WHEPBearerToken)

	for i := range p.Forward {
		p.Forward[i].WHIPBearerToken = redactSecretString(p.Forward[i].WHIPBearerToken)
	}

	if p.PublishPass != nil {
		*p.PublishPass = redactSecretCredential(*p.PublishPass)
	}
	if p.ReadPass != nil {
		*p.ReadPass = redactSecretCredential(*p.ReadPass)
	}
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
			dests[i].WHIPBearerToken = restoreSecret(dests[i].WHIPBearerToken, old.Forward[i].WHIPBearerToken)
		}
	}
}

// restoreRedactedGlobalValues replaces redacted values, as returned by the API,
// with the corresponding values of the old configuration.
func restoreRedactedGlobalValues(values any, old *Conf) {
	rv := reflect.ValueOf(values).Elem()

	restoreStringField(rv, "HLSCDNSecret", old.HLSCDNSecret)

	iceServers := rv.FieldByName("WebRTCICEServers2")
	if !iceServers.IsNil() {
		servers := *iceServers.Interface().(*[]WebRTCICEServer)
		for i := range servers {
			if i >= len(old.WebRTCICEServers2) {
				break
			}
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

	for i := range c.WebRTCICEServers2 {
		c.WebRTCICEServers2[i].Password = redactSecretString(c.WebRTCICEServers2[i].Password)
	}

	redactPath(&c.PathDefaults)

	for _, pathConf := range c.Paths {
		redactPath(pathConf)
	}

	return c
}
