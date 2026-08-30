package conf

const (
	redactedCredential = "<redacted>"
)

// Redact clones a configuration and redacts credentials from it.
func Redact(c *Conf) *Conf {
	c = c.Clone()

	for i := range c.AuthInternalUsers {
		if c.AuthInternalUsers[i].Pass != "" {
			c.AuthInternalUsers[i].Pass = Credential(redactedCredential)
		}
	}

	if c.PathDefaults.PublishPass != nil && *c.PathDefaults.PublishPass != "" {
		*c.PathDefaults.PublishPass = Credential(redactedCredential)
	}
	if c.PathDefaults.ReadPass != nil && *c.PathDefaults.ReadPass != "" {
		*c.PathDefaults.ReadPass = Credential(redactedCredential)
	}

	for _, pathConf := range c.Paths {
		if pathConf.PublishPass != nil && *pathConf.PublishPass != "" {
			*pathConf.PublishPass = Credential(redactedCredential)
		}
		if pathConf.ReadPass != nil && *pathConf.ReadPass != "" {
			*pathConf.ReadPass = Credential(redactedCredential)
		}
	}

	return c
}
