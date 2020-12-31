package conf

import (
	"encoding/json"
	"fmt"
	"net/url"
	"regexp"
	"strings"
	"time"

	"github.com/aler9/gortsplib"
	"github.com/aler9/gortsplib/pkg/base"
)

var reUserPass = regexp.MustCompile(`^[a-zA-Z0-9!\$\(\)\*\+\.;<=>\[\]\^_\-\{\}]+$`)

const userPassSupportedChars = "A-Z,0-9,!,$,(,),*,+,.,;,<,=,>,[,],^,_,-,{,}"

// PathConf is a path configuration.
type PathConf struct {
	Regexp                     *regexp.Regexp            `yaml:"-" json:"-"`
	Source                     string                    `yaml:"source"`
	SourceProtocol             string                    `yaml:"sourceProtocol"`
	SourceProtocolParsed       *gortsplib.StreamProtocol `yaml:"-" json:"-"`
	SourceOnDemand             bool                      `yaml:"sourceOnDemand"`
	SourceOnDemandStartTimeout time.Duration             `yaml:"sourceOnDemandStartTimeout"`
	SourceOnDemandCloseAfter   time.Duration             `yaml:"sourceOnDemandCloseAfter"`
	SourceRedirect             string                    `yaml:"sourceRedirect"`
	Fallback                   string                    `yaml:"fallback"`
	RunOnInit                  string                    `yaml:"runOnInit"`
	RunOnInitRestart           bool                      `yaml:"runOnInitRestart"`
	RunOnDemand                string                    `yaml:"runOnDemand"`
	RunOnDemandRestart         bool                      `yaml:"runOnDemandRestart"`
	RunOnDemandStartTimeout    time.Duration             `yaml:"runOnDemandStartTimeout"`
	RunOnDemandCloseAfter      time.Duration             `yaml:"runOnDemandCloseAfter"`
	RunOnPublish               string                    `yaml:"runOnPublish"`
	RunOnPublishRestart        bool                      `yaml:"runOnPublishRestart"`
	RunOnRead                  string                    `yaml:"runOnRead"`
	RunOnReadRestart           bool                      `yaml:"runOnReadRestart"`
	PublishUser                string                    `yaml:"publishUser"`
	PublishPass                string                    `yaml:"publishPass"`
	PublishIps                 []string                  `yaml:"publishIps"`
	PublishIpsParsed           []interface{}             `yaml:"-" json:"-"`
	ReadUser                   string                    `yaml:"readUser"`
	ReadPass                   string                    `yaml:"readPass"`
	ReadIps                    []string                  `yaml:"readIps"`
	ReadIpsParsed              []interface{}             `yaml:"-" json:"-"`
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

	if pconf.Source == "record" {

	} else if strings.HasPrefix(pconf.Source, "rtsp://") ||
		strings.HasPrefix(pconf.Source, "rtsps://") {
		if pconf.Regexp != nil {
			return fmt.Errorf("a path with a regular expression (or path 'all') cannot have a RTSP source; use another path")
		}

		u, err := base.ParseURL(pconf.Source)
		if err != nil {
			return fmt.Errorf("'%s' is not a valid RTSP url", pconf.Source)
		}

		if u.User != nil {
			pass, _ := u.User.Password()
			user := u.User.Username()
			if user != "" && pass == "" ||
				user == "" && pass != "" {
				return fmt.Errorf("username and password must be both provided")
			}
		}

		if pconf.SourceProtocol == "" {
			pconf.SourceProtocol = "automatic"
		}

		switch pconf.SourceProtocol {
		case "udp":
			v := gortsplib.StreamProtocolUDP
			pconf.SourceProtocolParsed = &v

		case "tcp":
			v := gortsplib.StreamProtocolTCP
			pconf.SourceProtocolParsed = &v

		case "automatic":

		default:
			return fmt.Errorf("unsupported protocol '%s'", pconf.SourceProtocol)
		}

	} else if strings.HasPrefix(pconf.Source, "rtmp://") {
		if pconf.Regexp != nil {
			return fmt.Errorf("a path with a regular expression (or path 'all') cannot have a RTMP source; use another path")
		}

		u, err := url.Parse(pconf.Source)
		if err != nil {
			return fmt.Errorf("'%s' is not a valid RTMP url", pconf.Source)
		}
		if u.Scheme != "rtmp" {
			return fmt.Errorf("'%s' is not a valid RTMP url", pconf.Source)
		}

		if u.User != nil {
			pass, _ := u.User.Password()
			user := u.User.Username()
			if user != "" && pass == "" ||
				user == "" && pass != "" {
				return fmt.Errorf("username and password must be both provided")
			}
		}

	} else if pconf.Source == "redirect" {
		if pconf.SourceRedirect == "" {
			return fmt.Errorf("source redirect must be filled")
		}

		_, err := base.ParseURL(pconf.SourceRedirect)
		if err != nil {
			return fmt.Errorf("'%s' is not a valid RTSP url", pconf.SourceRedirect)
		}

	} else {
		return fmt.Errorf("invalid source: '%s'", pconf.Source)
	}

	if pconf.SourceOnDemandStartTimeout == 0 {
		pconf.SourceOnDemandStartTimeout = 10 * time.Second
	}

	if pconf.SourceOnDemandCloseAfter == 0 {
		pconf.SourceOnDemandCloseAfter = 10 * time.Second
	}

	if pconf.Fallback != "" {
		_, err := base.ParseURL(pconf.Fallback)
		if err != nil {
			return fmt.Errorf("'%s' is not a valid RTSP url", pconf.Fallback)
		}
	}

	if (pconf.PublishUser != "" && pconf.PublishPass == "") || (pconf.PublishUser == "" && pconf.PublishPass != "") {
		return fmt.Errorf("read username and password must be both filled")
	}
	if pconf.PublishUser != "" {
		if pconf.Source != "record" {
			return fmt.Errorf("'publishUser' is useless when source is not 'record', since the stream is not provided by a publisher, but by a fixed source")
		}

		if !strings.HasPrefix(pconf.PublishUser, "sha256:") && !reUserPass.MatchString(pconf.PublishUser) {
			return fmt.Errorf("publish username contains unsupported characters (supported are %s)", userPassSupportedChars)
		}
	}
	if pconf.PublishPass != "" {
		if pconf.Source != "record" {
			return fmt.Errorf("'publishPass' is useless when source is not 'record', since the stream is not provided by a publisher, but by a fixed source")
		}

		if !strings.HasPrefix(pconf.PublishPass, "sha256:") && !reUserPass.MatchString(pconf.PublishPass) {
			return fmt.Errorf("publish password contains unsupported characters (supported are %s)", userPassSupportedChars)
		}
	}
	if len(pconf.PublishIps) == 0 {
		pconf.PublishIps = nil
	}
	var err error
	pconf.PublishIpsParsed, err = func() ([]interface{}, error) {
		if len(pconf.PublishIps) == 0 {
			return nil, nil
		}

		if pconf.Source != "record" {
			return nil, fmt.Errorf("'publishIps' is useless when source is not 'record', since the stream is not provided by a publisher, but by a fixed source")
		}

		return parseIPCidrList(pconf.PublishIps)
	}()
	if err != nil {
		return err
	}

	if (pconf.ReadUser != "" && pconf.ReadPass == "") || (pconf.ReadUser == "" && pconf.ReadPass != "") {
		return fmt.Errorf("read username and password must be both filled")
	}
	if pconf.ReadUser != "" {
		if !strings.HasPrefix(pconf.ReadUser, "sha256:") && !reUserPass.MatchString(pconf.ReadUser) {
			return fmt.Errorf("read username contains unsupported characters (supported are %s)", userPassSupportedChars)
		}
	}
	if pconf.ReadPass != "" {
		if !strings.HasPrefix(pconf.ReadPass, "sha256:") && !reUserPass.MatchString(pconf.ReadPass) {
			return fmt.Errorf("read password contains unsupported characters (supported are %s)", userPassSupportedChars)
		}
	}
	if len(pconf.ReadIps) == 0 {
		pconf.ReadIps = nil
	}
	pconf.ReadIpsParsed, err = func() ([]interface{}, error) {
		return parseIPCidrList(pconf.ReadIps)
	}()
	if err != nil {
		return err
	}

	if pconf.RunOnInit != "" && pconf.Regexp != nil {
		return fmt.Errorf("a path with a regular expression does not support option 'runOnInit'; use another path")
	}

	if pconf.RunOnPublish != "" && pconf.Source != "record" {
		return fmt.Errorf("'runOnPublish' is useless when source is not 'record', since the stream is not provided by a publisher, but by a fixed source")
	}

	if pconf.RunOnDemandStartTimeout == 0 {
		pconf.RunOnDemandStartTimeout = 10 * time.Second
	}

	if pconf.RunOnDemandCloseAfter == 0 {
		pconf.RunOnDemandCloseAfter = 10 * time.Second
	}

	return nil
}

// Equal checks whether two PathConfs are equal.
func (pconf *PathConf) Equal(other *PathConf) bool {
	a, _ := json.Marshal(pconf)
	b, _ := json.Marshal(other)
	return string(a) == string(b)
}
