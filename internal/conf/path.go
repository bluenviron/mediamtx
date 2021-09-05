package conf

import (
	"encoding/json"
	"fmt"
	"net"
	"net/url"
	"regexp"
	"strings"
	"time"

	"github.com/aler9/gortsplib"
	"github.com/aler9/gortsplib/pkg/base"
)

const userPassSupportedChars = "A-Z,0-9,!,$,(,),*,+,.,;,<,=,>,[,],^,_,-,{,}"

var reUserPass = regexp.MustCompile(`^[a-zA-Z0-9!\$\(\)\*\+\.;<=>\[\]\^_\-\{\}]+$`)

var rePathName = regexp.MustCompile(`^[0-9a-zA-Z_\-/\.~]+$`)

func parseIPCidrList(in []string) ([]interface{}, error) {
	if len(in) == 0 {
		return nil, nil
	}

	var ret []interface{}
	for _, t := range in {
		_, ipnet, err := net.ParseCIDR(t)
		if err == nil {
			ret = append(ret, ipnet)
			continue
		}

		ip := net.ParseIP(t)
		if ip != nil {
			ret = append(ret, ip)
			continue
		}

		return nil, fmt.Errorf("unable to parse ip/network '%s'", t)
	}
	return ret, nil
}

// CheckPathName checks if a path name is valid.
func CheckPathName(name string) error {
	if name == "" {
		return fmt.Errorf("cannot be empty")
	}

	if name[0] == '/' {
		return fmt.Errorf("can't begin with a slash")
	}

	if name[len(name)-1] == '/' {
		return fmt.Errorf("can't end with a slash")
	}

	if !rePathName.MatchString(name) {
		return fmt.Errorf("can contain only alphanumeric characters, underscore, dot, tilde, minus or slash")
	}

	return nil
}

// PathConf is a path configuration.
type PathConf struct {
	Regexp *regexp.Regexp `yaml:"-" json:"-"`

	// source
	Source                     string                    `yaml:"source" json:"source"`
	SourceProtocol             string                    `yaml:"sourceProtocol" json:"sourceProtocol"`
	SourceProtocolParsed       *gortsplib.ClientProtocol `yaml:"-" json:"-"`
	SourceAnyPortEnable        bool                      `yaml:"sourceAnyPortEnable" json:"sourceAnyPortEnable"`
	SourceFingerprint          string                    `yaml:"sourceFingerprint" json:"sourceFingerprint"`
	SourceOnDemand             bool                      `yaml:"sourceOnDemand" json:"sourceOnDemand"`
	SourceOnDemandStartTimeout time.Duration             `yaml:"sourceOnDemandStartTimeout" json:"sourceOnDemandStartTimeout"`
	SourceOnDemandCloseAfter   time.Duration             `yaml:"sourceOnDemandCloseAfter" json:"sourceOnDemandCloseAfter"`
	SourceRedirect             string                    `yaml:"sourceRedirect" json:"sourceRedirect"`
	DisablePublisherOverride   bool                      `yaml:"disablePublisherOverride" json:"disablePublisherOverride"`
	Fallback                   string                    `yaml:"fallback" json:"fallback"`

	// authentication
	PublishUser      string        `yaml:"publishUser" json:"publishUser"`
	PublishPass      string        `yaml:"publishPass" json:"publishPass"`
	PublishIPs       []string      `yaml:"publishIPs" json:"publishIPs"`
	PublishIPsParsed []interface{} `yaml:"-" json:"-"`
	ReadUser         string        `yaml:"readUser" json:"readUser"`
	ReadPass         string        `yaml:"readPass" json:"readPass"`
	ReadIPs          []string      `yaml:"readIPs" json:"readIPs"`
	ReadIPsParsed    []interface{} `yaml:"-" json:"-"`

	// custom commands
	RunOnInit               string        `yaml:"runOnInit" json:"runOnInit"`
	RunOnInitRestart        bool          `yaml:"runOnInitRestart" json:"runOnInitRestart"`
	RunOnDemand             string        `yaml:"runOnDemand" json:"runOnDemand"`
	RunOnDemandRestart      bool          `yaml:"runOnDemandRestart" json:"runOnDemandRestart"`
	RunOnDemandStartTimeout time.Duration `yaml:"runOnDemandStartTimeout" json:"runOnDemandStartTimeout"`
	RunOnDemandCloseAfter   time.Duration `yaml:"runOnDemandCloseAfter" json:"runOnDemandCloseAfter"`
	RunOnPublish            string        `yaml:"runOnPublish" json:"runOnPublish"`
	RunOnPublishRestart     bool          `yaml:"runOnPublishRestart" json:"runOnPublishRestart"`
	RunOnRead               string        `yaml:"runOnRead" json:"runOnRead"`
	RunOnReadRestart        bool          `yaml:"runOnReadRestart" json:"runOnReadRestart"`
}

func (pconf *PathConf) checkAndFillMissing(name string) error {
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
		pconf.Source = "publisher"
	}

	switch {
	case pconf.Source == "publisher":

	case strings.HasPrefix(pconf.Source, "rtsp://") ||
		strings.HasPrefix(pconf.Source, "rtsps://"):
		if pconf.Regexp != nil {
			return fmt.Errorf("a path with a regular expression (or path 'all') cannot have a RTSP source; use another path")
		}

		_, err := base.ParseURL(pconf.Source)
		if err != nil {
			return fmt.Errorf("'%s' is not a valid RTSP URL", pconf.Source)
		}

		if pconf.SourceProtocol == "" {
			pconf.SourceProtocol = "automatic"
		}

		switch pconf.SourceProtocol {
		case "udp":
			v := gortsplib.ClientProtocolUDP
			pconf.SourceProtocolParsed = &v

		case "multicast":
			v := gortsplib.ClientProtocolMulticast
			pconf.SourceProtocolParsed = &v

		case "tcp":
			v := gortsplib.ClientProtocolTCP
			pconf.SourceProtocolParsed = &v

		case "automatic":

		default:
			return fmt.Errorf("unsupported protocol '%s'", pconf.SourceProtocol)
		}

		if strings.HasPrefix(pconf.Source, "rtsps://") && pconf.SourceFingerprint == "" {
			return fmt.Errorf("sourceFingerprint is required with a RTSPS URL")
		}

	case strings.HasPrefix(pconf.Source, "rtmp://"):
		if pconf.Regexp != nil {
			return fmt.Errorf("a path with a regular expression (or path 'all') cannot have a RTMP source; use another path")
		}

		u, err := url.Parse(pconf.Source)
		if err != nil {
			return fmt.Errorf("'%s' is not a valid RTMP URL", pconf.Source)
		}
		if u.Scheme != "rtmp" {
			return fmt.Errorf("'%s' is not a valid RTMP URL", pconf.Source)
		}

		if u.User != nil {
			pass, _ := u.User.Password()
			user := u.User.Username()
			if user != "" && pass == "" ||
				user == "" && pass != "" {
				return fmt.Errorf("username and password must be both provided")
			}
		}

	case strings.HasPrefix(pconf.Source, "http://") ||
		strings.HasPrefix(pconf.Source, "https://"):
		if pconf.Regexp != nil {
			return fmt.Errorf("a path with a regular expression (or path 'all') cannot have a HLS source; use another path")
		}

		u, err := url.Parse(pconf.Source)
		if err != nil {
			return fmt.Errorf("'%s' is not a valid HLS URL", pconf.Source)
		}
		if u.Scheme != "http" && u.Scheme != "https" {
			return fmt.Errorf("'%s' is not a valid HLS URL", pconf.Source)
		}

		if u.User != nil {
			pass, _ := u.User.Password()
			user := u.User.Username()
			if user != "" && pass == "" ||
				user == "" && pass != "" {
				return fmt.Errorf("username and password must be both provided")
			}
		}

	case pconf.Source == "redirect":
		if pconf.SourceRedirect == "" {
			return fmt.Errorf("source redirect must be filled")
		}

		_, err := base.ParseURL(pconf.SourceRedirect)
		if err != nil {
			return fmt.Errorf("'%s' is not a valid RTSP URL", pconf.SourceRedirect)
		}

	default:
		return fmt.Errorf("invalid source: '%s'", pconf.Source)
	}

	if pconf.SourceOnDemand {
		if pconf.Source == "publisher" {
			return fmt.Errorf("'sourceOnDemand' is useless when source is 'publisher'")
		}
	}

	if pconf.SourceOnDemandStartTimeout == 0 {
		pconf.SourceOnDemandStartTimeout = 10 * time.Second
	}

	if pconf.SourceOnDemandCloseAfter == 0 {
		pconf.SourceOnDemandCloseAfter = 10 * time.Second
	}

	if pconf.Fallback != "" {
		if strings.HasPrefix(pconf.Fallback, "/") {
			err := CheckPathName(pconf.Fallback[1:])
			if err != nil {
				return fmt.Errorf("'%s': %s", pconf.Fallback, err)
			}

		} else {
			_, err := base.ParseURL(pconf.Fallback)
			if err != nil {
				return fmt.Errorf("'%s' is not a valid RTSP URL", pconf.Fallback)
			}
		}
	}

	if (pconf.PublishUser != "" && pconf.PublishPass == "") || (pconf.PublishUser == "" && pconf.PublishPass != "") {
		return fmt.Errorf("read username and password must be both filled")
	}
	if pconf.PublishUser != "" {
		if pconf.Source != "publisher" {
			return fmt.Errorf("'publishUser' is useless when source is not 'publisher'")
		}

		if !strings.HasPrefix(pconf.PublishUser, "sha256:") && !reUserPass.MatchString(pconf.PublishUser) {
			return fmt.Errorf("publish username contains unsupported characters (supported are %s)", userPassSupportedChars)
		}
	}
	if pconf.PublishPass != "" {
		if pconf.Source != "publisher" {
			return fmt.Errorf("'publishPass' is useless when source is not 'publisher', since the stream is not provided by a publisher, but by a fixed source")
		}

		if !strings.HasPrefix(pconf.PublishPass, "sha256:") && !reUserPass.MatchString(pconf.PublishPass) {
			return fmt.Errorf("publish password contains unsupported characters (supported are %s)", userPassSupportedChars)
		}
	}
	if len(pconf.PublishIPs) == 0 {
		pconf.PublishIPs = nil
	}
	var err error
	pconf.PublishIPsParsed, err = func() ([]interface{}, error) {
		if len(pconf.PublishIPs) == 0 {
			return nil, nil
		}

		if pconf.Source != "publisher" {
			return nil, fmt.Errorf("'publishIPs' is useless when source is not 'publisher', since the stream is not provided by a publisher, but by a fixed source")
		}

		return parseIPCidrList(pconf.PublishIPs)
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
	if len(pconf.ReadIPs) == 0 {
		pconf.ReadIPs = nil
	}
	pconf.ReadIPsParsed, err = func() ([]interface{}, error) {
		return parseIPCidrList(pconf.ReadIPs)
	}()
	if err != nil {
		return err
	}

	if pconf.RunOnInit != "" && pconf.Regexp != nil {
		return fmt.Errorf("a path with a regular expression does not support option 'runOnInit'; use another path")
	}

	if pconf.RunOnPublish != "" && pconf.Source != "publisher" {
		return fmt.Errorf("'runOnPublish' is useless when source is not 'publisher', since the stream is not provided by a publisher, but by a fixed source")
	}

	if pconf.RunOnDemand != "" && pconf.Source != "publisher" {
		return fmt.Errorf("'runOnDemand' can be used only when source is 'publisher'")
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
