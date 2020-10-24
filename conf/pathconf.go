package conf

import (
	"encoding/json"
	"fmt"
	"net/url"
	"regexp"
	"strings"

	"github.com/aler9/gortsplib"
)

type PathConf struct {
	Regexp               *regexp.Regexp           `yaml:"-" json:"-"`
	Source               string                   `yaml:"source"`
	SourceProtocol       string                   `yaml:"sourceProtocol"`
	SourceProtocolParsed gortsplib.StreamProtocol `yaml:"-" json:"-"`
	SourceOnDemand       bool                     `yaml:"sourceOnDemand"`
	RunOnInit            string                   `yaml:"runOnInit"`
	RunOnDemand          string                   `yaml:"runOnDemand"`
	RunOnPublish         string                   `yaml:"runOnPublish"`
	RunOnRead            string                   `yaml:"runOnRead"`
	PublishUser          string                   `yaml:"publishUser"`
	PublishPass          string                   `yaml:"publishPass"`
	PublishIps           []string                 `yaml:"publishIps"`
	PublishIpsParsed     []interface{}            `yaml:"-" json:"-"`
	ReadUser             string                   `yaml:"readUser"`
	ReadPass             string                   `yaml:"readPass"`
	ReadIps              []string                 `yaml:"readIps"`
	ReadIpsParsed        []interface{}            `yaml:"-" json:"-"`
}

func (pconf *PathConf) fillAndCheck(name string) error {
	if name == "" {
		return fmt.Errorf("path name can not be empty")
	}

	// normal path
	if name[0] != '~' {
		err := CheckPathName(name)
		if err != nil {
			return fmt.Errorf("invalid path name: %s (%s)", err, name)
		}

		// regular expression path
	} else {
		pathRegexp, err := regexp.Compile(name[1:])
		if err != nil {
			return fmt.Errorf("invalid regular expression: %s", name[1:])
		}
		pconf.Regexp = pathRegexp
	}

	if pconf.Source == "" {
		pconf.Source = "record"
	}

	if strings.HasPrefix(pconf.Source, "rtsp://") {
		if pconf.Regexp != nil {
			return fmt.Errorf("a path with a regular expression (or path 'all') cannot have a RTSP source; use another path")
		}

		u, err := url.Parse(pconf.Source)
		if err != nil {
			return fmt.Errorf("'%s' is not a valid url", pconf.Source)
		}
		if u.User != nil {
			pass, _ := u.User.Password()
			user := u.User.Username()
			if user != "" && pass == "" ||
				user == "" && pass != "" {
				fmt.Errorf("username and password must be both provided")
			}
		}

		if pconf.SourceProtocol == "" {
			pconf.SourceProtocol = "udp"
		}
		switch pconf.SourceProtocol {
		case "udp":
			pconf.SourceProtocolParsed = gortsplib.StreamProtocolUDP

		case "tcp":
			pconf.SourceProtocolParsed = gortsplib.StreamProtocolTCP

		default:
			return fmt.Errorf("unsupported protocol '%s'", pconf.SourceProtocol)
		}

	} else if strings.HasPrefix(pconf.Source, "rtmp://") {
		if pconf.Regexp != nil {
			return fmt.Errorf("a path with a regular expression (or path 'all') cannot have a RTMP source; use another path")
		}

		u, err := url.Parse(pconf.Source)
		if err != nil {
			return fmt.Errorf("'%s' is not a valid url", pconf.Source)
		}
		if u.User != nil {
			pass, _ := u.User.Password()
			user := u.User.Username()
			if user != "" && pass == "" ||
				user == "" && pass != "" {
				fmt.Errorf("username and password must be both provided")
			}
		}

	} else if pconf.Source == "record" {

	} else {
		return fmt.Errorf("unsupported source: '%s'", pconf.Source)
	}

	if pconf.PublishUser != "" {
		if !regexp.MustCompile("^[a-zA-Z0-9]+$").MatchString(pconf.PublishUser) {
			return fmt.Errorf("publish username must be alphanumeric")
		}
	}
	if pconf.PublishPass != "" {
		if !regexp.MustCompile("^[a-zA-Z0-9]+$").MatchString(pconf.PublishPass) {
			return fmt.Errorf("publish password must be alphanumeric")
		}
	}

	if len(pconf.PublishIps) > 0 {
		var err error
		pconf.PublishIpsParsed, err = parseIpCidrList(pconf.PublishIps)
		if err != nil {
			return err
		}
	} else {
		// the configuration file doesn't use nil dicts - avoid test fails by using nil
		pconf.PublishIps = nil
	}

	if pconf.ReadUser != "" && pconf.ReadPass == "" || pconf.ReadUser == "" && pconf.ReadPass != "" {
		return fmt.Errorf("read username and password must be both filled")
	}
	if pconf.ReadUser != "" {
		if !regexp.MustCompile("^[a-zA-Z0-9]+$").MatchString(pconf.ReadUser) {
			return fmt.Errorf("read username must be alphanumeric")
		}
	}
	if pconf.ReadPass != "" {
		if !regexp.MustCompile("^[a-zA-Z0-9]+$").MatchString(pconf.ReadPass) {
			return fmt.Errorf("read password must be alphanumeric")
		}
	}
	if pconf.ReadUser != "" && pconf.ReadPass == "" || pconf.ReadUser == "" && pconf.ReadPass != "" {
		return fmt.Errorf("read username and password must be both filled")
	}

	if len(pconf.ReadIps) > 0 {
		var err error
		pconf.ReadIpsParsed, err = parseIpCidrList(pconf.ReadIps)
		if err != nil {
			return err
		}
	} else {
		// the configuration file doesn't use nil dicts - avoid test fails by using nil
		pconf.ReadIps = nil
	}

	if pconf.Regexp != nil && pconf.RunOnInit != "" {
		return fmt.Errorf("a path with a regular expression does not support option 'runOnInit'; use another path")
	}

	return nil
}

func (pconf *PathConf) Equal(other *PathConf) bool {
	a, _ := json.Marshal(pconf)
	b, _ := json.Marshal(pconf)
	return string(a) == string(b)
}
