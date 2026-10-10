package conf

// RedactedCredential is the value that replaces credentials in redacted configurations.
const RedactedCredential = "<redacted>"

// Redact clones a configuration and redacts credentials from it.
func Redact(c *Conf) *Conf {
	c = c.Clone()

	for i := range c.AuthInternalUsers {
		if c.AuthInternalUsers[i].Pass != "" {
			c.AuthInternalUsers[i].Pass = Credential(RedactedCredential)
		}
	}

	if c.PathDefaults.PublishPass != nil && *c.PathDefaults.PublishPass != "" {
		*c.PathDefaults.PublishPass = Credential(RedactedCredential)
	}
	if c.PathDefaults.ReadPass != nil && *c.PathDefaults.ReadPass != "" {
		*c.PathDefaults.ReadPass = Credential(RedactedCredential)
	}

	for _, pathConf := range c.Paths {
		if pathConf.PublishPass != nil && *pathConf.PublishPass != "" {
			*pathConf.PublishPass = Credential(RedactedCredential)
		}
		if pathConf.ReadPass != nil && *pathConf.ReadPass != "" {
			*pathConf.ReadPass = Credential(RedactedCredential)
		}
	}

	return c
}
