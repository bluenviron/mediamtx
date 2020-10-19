package conf

import (
	"fmt"
	"net"
	"net/url"
	"os"
	"regexp"
	"strings"
	"time"

	"github.com/aler9/gortsplib"
	"github.com/aler9/gortsplib/headers"
	"gopkg.in/yaml.v2"

	"github.com/aler9/rtsp-simple-server/confenv"
	"github.com/aler9/rtsp-simple-server/loghandler"
)

func parseIpCidrList(in []string) ([]interface{}, error) {
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

var rePathName = regexp.MustCompile("^[0-9a-zA-Z_\\-/]+$")

func checkPathName(name string) error {
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
		return fmt.Errorf("can contain only alfanumeric characters, underscore, minus or slash")
	}

	return nil
}

type PathConf struct {
	Regexp               *regexp.Regexp           `yaml:"-"`
	Source               string                   `yaml:"source"`
	SourceUrl            *url.URL                 `yaml:"-"`
	SourceProtocol       string                   `yaml:"sourceProtocol"`
	SourceProtocolParsed gortsplib.StreamProtocol `yaml:"-"`
	SourceOnDemand       bool                     `yaml:"sourceOnDemand"`
	RunOnInit            string                   `yaml:"runOnInit"`
	RunOnDemand          string                   `yaml:"runOnDemand"`
	RunOnPublish         string                   `yaml:"runOnPublish"`
	RunOnRead            string                   `yaml:"runOnRead"`
	PublishUser          string                   `yaml:"publishUser"`
	PublishPass          string                   `yaml:"publishPass"`
	PublishIps           []string                 `yaml:"publishIps"`
	PublishIpsParsed     []interface{}            `yaml:"-"`
	ReadUser             string                   `yaml:"readUser"`
	ReadPass             string                   `yaml:"readPass"`
	ReadIps              []string                 `yaml:"readIps"`
	ReadIpsParsed        []interface{}            `yaml:"-"`
}

type Conf struct {
	Protocols             []string                              `yaml:"protocols"`
	ProtocolsParsed       map[gortsplib.StreamProtocol]struct{} `yaml:"-"`
	RtspPort              int                                   `yaml:"rtspPort"`
	RtpPort               int                                   `yaml:"rtpPort"`
	RtcpPort              int                                   `yaml:"rtcpPort"`
	RunOnConnect          string                                `yaml:"runOnConnect"`
	ReadTimeout           time.Duration                         `yaml:"readTimeout"`
	WriteTimeout          time.Duration                         `yaml:"writeTimeout"`
	AuthMethods           []string                              `yaml:"authMethods"`
	AuthMethodsParsed     []headers.AuthMethod                  `yaml:"-"`
	Metrics               bool                                  `yaml:"metrics"`
	Pprof                 bool                                  `yaml:"pprof"`
	LogDestinations       []string                              `yaml:"logDestinations"`
	LogDestinationsParsed map[loghandler.Destination]struct{}   `yaml:"-"`
	LogFile               string                                `yaml:"logFile"`
	Paths                 map[string]*PathConf                  `yaml:"paths"`
}

func Load(fpath string) (*Conf, error) {
	conf := &Conf{}

	// read from file
	err := func() error {
		// rtsp-simple-server.yml is optional
		if fpath == "rtsp-simple-server.yml" {
			if _, err := os.Stat(fpath); err != nil {
				return nil
			}
		}

		f, err := os.Open(fpath)
		if err != nil {
			return err
		}
		defer f.Close()

		err = yaml.NewDecoder(f).Decode(conf)
		if err != nil {
			return err
		}

		return nil
	}()
	if err != nil {
		return nil, err
	}

	// read from environment
	err = confenv.Process("RTSP", conf)
	if err != nil {
		return nil, err
	}

	if len(conf.Protocols) == 0 {
		conf.Protocols = []string{"udp", "tcp"}
	}
	conf.ProtocolsParsed = make(map[gortsplib.StreamProtocol]struct{})
	for _, proto := range conf.Protocols {
		switch proto {
		case "udp":
			conf.ProtocolsParsed[gortsplib.StreamProtocolUDP] = struct{}{}

		case "tcp":
			conf.ProtocolsParsed[gortsplib.StreamProtocolTCP] = struct{}{}

		default:
			return nil, fmt.Errorf("unsupported protocol: %s", proto)
		}
	}
	if len(conf.ProtocolsParsed) == 0 {
		return nil, fmt.Errorf("no protocols provided")
	}

	if conf.RtspPort == 0 {
		conf.RtspPort = 8554
	}
	if conf.RtpPort == 0 {
		conf.RtpPort = 8000
	}
	if (conf.RtpPort % 2) != 0 {
		return nil, fmt.Errorf("rtp port must be even")
	}
	if conf.RtcpPort == 0 {
		conf.RtcpPort = 8001
	}
	if conf.RtcpPort != (conf.RtpPort + 1) {
		return nil, fmt.Errorf("rtcp and rtp ports must be consecutive")
	}

	if conf.ReadTimeout == 0 {
		conf.ReadTimeout = 10 * time.Second
	}
	if conf.WriteTimeout == 0 {
		conf.WriteTimeout = 5 * time.Second
	}

	if len(conf.AuthMethods) == 0 {
		conf.AuthMethods = []string{"basic", "digest"}
	}
	for _, method := range conf.AuthMethods {
		switch method {
		case "basic":
			conf.AuthMethodsParsed = append(conf.AuthMethodsParsed, headers.AuthBasic)

		case "digest":
			conf.AuthMethodsParsed = append(conf.AuthMethodsParsed, headers.AuthDigest)

		default:
			return nil, fmt.Errorf("unsupported authentication method: %s", method)
		}
	}

	if len(conf.LogDestinations) == 0 {
		conf.LogDestinations = []string{"stdout"}
	}
	conf.LogDestinationsParsed = make(map[loghandler.Destination]struct{})
	for _, dest := range conf.LogDestinations {
		switch dest {
		case "stdout":
			conf.LogDestinationsParsed[loghandler.DestinationStdout] = struct{}{}

		case "file":
			conf.LogDestinationsParsed[loghandler.DestinationFile] = struct{}{}

		case "syslog":
			conf.LogDestinationsParsed[loghandler.DestinationSyslog] = struct{}{}

		default:
			return nil, fmt.Errorf("unsupported log destination: %s", dest)
		}
	}
	if conf.LogFile == "" {
		conf.LogFile = "rtsp-simple-server.log"
	}

	if len(conf.Paths) == 0 {
		conf.Paths = map[string]*PathConf{
			"all": {},
		}
	}

	// "all" is an alias for "~^.*$"
	if _, ok := conf.Paths["all"]; ok {
		conf.Paths["~^.*$"] = conf.Paths["all"]
		delete(conf.Paths, "all")
	}

	for name, pconf := range conf.Paths {
		if pconf == nil {
			conf.Paths[name] = &PathConf{}
			pconf = conf.Paths[name]
		}

		if name == "" {
			return nil, fmt.Errorf("path name can not be empty")
		}

		// normal path
		if name[0] != '~' {
			err := checkPathName(name)
			if err != nil {
				return nil, fmt.Errorf("invalid path name: %s (%s)", err, name)
			}

			// regular expression path
		} else {
			pathRegexp, err := regexp.Compile(name[1:])
			if err != nil {
				return nil, fmt.Errorf("invalid regular expression: %s", name[1:])
			}
			pconf.Regexp = pathRegexp
		}

		if pconf.Source == "" {
			pconf.Source = "record"
		}

		if strings.HasPrefix(pconf.Source, "rtsp://") {
			if pconf.Regexp != nil {
				return nil, fmt.Errorf("a path with a regular expression (or path 'all') cannot have a RTSP source; use another path")
			}

			pconf.SourceUrl, err = url.Parse(pconf.Source)
			if err != nil {
				return nil, fmt.Errorf("'%s' is not a valid url", pconf.Source)
			}
			if pconf.SourceUrl.Port() == "" {
				pconf.SourceUrl.Host += ":554"
			}
			if pconf.SourceUrl.User != nil {
				pass, _ := pconf.SourceUrl.User.Password()
				user := pconf.SourceUrl.User.Username()
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
				return nil, fmt.Errorf("unsupported protocol '%s'", pconf.SourceProtocol)
			}

		} else if strings.HasPrefix(pconf.Source, "rtmp://") {
			if pconf.Regexp != nil {
				return nil, fmt.Errorf("a path with a regular expression (or path 'all') cannot have a RTMP source; use another path")
			}

			pconf.SourceUrl, err = url.Parse(pconf.Source)
			if err != nil {
				return nil, fmt.Errorf("'%s' is not a valid url", pconf.Source)
			}
			if pconf.SourceUrl.Port() == "" {
				pconf.SourceUrl.Host += ":1935"
			}

		} else if pconf.Source == "record" {

		} else {
			return nil, fmt.Errorf("unsupported source: '%s'", pconf.Source)
		}

		if pconf.PublishUser != "" {
			if !regexp.MustCompile("^[a-zA-Z0-9]+$").MatchString(pconf.PublishUser) {
				return nil, fmt.Errorf("publish username must be alphanumeric")
			}
		}
		if pconf.PublishPass != "" {
			if !regexp.MustCompile("^[a-zA-Z0-9]+$").MatchString(pconf.PublishPass) {
				return nil, fmt.Errorf("publish password must be alphanumeric")
			}
		}

		if len(pconf.PublishIps) > 0 {
			pconf.PublishIpsParsed, err = parseIpCidrList(pconf.PublishIps)
			if err != nil {
				return nil, err
			}
		} else {
			// the configuration file doesn't use nil dicts - avoid test fails by using nil
			pconf.PublishIps = nil
		}

		if pconf.ReadUser != "" && pconf.ReadPass == "" || pconf.ReadUser == "" && pconf.ReadPass != "" {
			return nil, fmt.Errorf("read username and password must be both filled")
		}
		if pconf.ReadUser != "" {
			if !regexp.MustCompile("^[a-zA-Z0-9]+$").MatchString(pconf.ReadUser) {
				return nil, fmt.Errorf("read username must be alphanumeric")
			}
		}
		if pconf.ReadPass != "" {
			if !regexp.MustCompile("^[a-zA-Z0-9]+$").MatchString(pconf.ReadPass) {
				return nil, fmt.Errorf("read password must be alphanumeric")
			}
		}
		if pconf.ReadUser != "" && pconf.ReadPass == "" || pconf.ReadUser == "" && pconf.ReadPass != "" {
			return nil, fmt.Errorf("read username and password must be both filled")
		}

		if len(pconf.ReadIps) > 0 {
			pconf.ReadIpsParsed, err = parseIpCidrList(pconf.ReadIps)
			if err != nil {
				return nil, err
			}
		} else {
			// the configuration file doesn't use nil dicts - avoid test fails by using nil
			pconf.ReadIps = nil
		}

		if pconf.Regexp != nil && pconf.RunOnInit != "" {
			return nil, fmt.Errorf("a path with a regular expression does not support option 'runOnInit'; use another path")
		}
	}

	return conf, nil
}

func (conf *Conf) CheckPathNameAndFindConf(name string) (*PathConf, error) {
	err := checkPathName(name)
	if err != nil {
		return nil, fmt.Errorf("invalid path name: %s (%s)", err, name)
	}

	// normal path
	if pconf, ok := conf.Paths[name]; ok {
		return pconf, nil
	}

	// regular expression path
	for _, pconf := range conf.Paths {
		if pconf.Regexp != nil && pconf.Regexp.MatchString(name) {
			return pconf, nil
		}
	}

	return nil, fmt.Errorf("unable to find a valid configuration for path '%s'", name)
}
