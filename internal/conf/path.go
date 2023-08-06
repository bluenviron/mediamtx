package conf

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net"
	gourl "net/url"
	"reflect"
	"regexp"
	"strings"
	"time"

	"github.com/bluenviron/gortsplib/v3/pkg/headers"
	"github.com/bluenviron/gortsplib/v3/pkg/url"
)

var rePathName = regexp.MustCompile(`^[0-9a-zA-Z_\-/\.~]+$`)

// IsValidPathName checks if a path name is valid.
func IsValidPathName(name string) error {
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
	Regexp *regexp.Regexp `json:"-"`

	// source
	Source string `json:"source"`

	// general
	SourceFingerprint          string         `json:"sourceFingerprint"`
	SourceOnDemand             bool           `json:"sourceOnDemand"`
	SourceOnDemandStartTimeout StringDuration `json:"sourceOnDemandStartTimeout"`
	SourceOnDemandCloseAfter   StringDuration `json:"sourceOnDemandCloseAfter"`
	MaxReaders                 int            `json:"maxReaders"`

	// authentication
	PublishUser Credential `json:"publishUser"`
	PublishPass Credential `json:"publishPass"`
	PublishIPs  IPsOrCIDRs `json:"publishIPs"`
	ReadUser    Credential `json:"readUser"`
	ReadPass    Credential `json:"readPass"`
	ReadIPs     IPsOrCIDRs `json:"readIPs"`

	// publisher
	OverridePublisher        bool   `json:"overridePublisher"`
	DisablePublisherOverride bool   `json:"disablePublisherOverride"` // deprecated
	Fallback                 string `json:"fallback"`

	// rtsp
	SourceProtocol      SourceProtocol `json:"sourceProtocol"`
	SourceAnyPortEnable bool           `json:"sourceAnyPortEnable"`
	RtspRangeType       RtspRangeType  `json:"rtspRangeType"`
	RtspRangeStart      string         `json:"rtspRangeStart"`

	// redirect
	SourceRedirect string `json:"sourceRedirect"`

	// raspberry pi camera
	RPICameraCamID             int     `json:"rpiCameraCamID"`
	RPICameraWidth             int     `json:"rpiCameraWidth"`
	RPICameraHeight            int     `json:"rpiCameraHeight"`
	RPICameraHFlip             bool    `json:"rpiCameraHFlip"`
	RPICameraVFlip             bool    `json:"rpiCameraVFlip"`
	RPICameraBrightness        float64 `json:"rpiCameraBrightness"`
	RPICameraContrast          float64 `json:"rpiCameraContrast"`
	RPICameraSaturation        float64 `json:"rpiCameraSaturation"`
	RPICameraSharpness         float64 `json:"rpiCameraSharpness"`
	RPICameraExposure          string  `json:"rpiCameraExposure"`
	RPICameraAWB               string  `json:"rpiCameraAWB"`
	RPICameraDenoise           string  `json:"rpiCameraDenoise"`
	RPICameraShutter           int     `json:"rpiCameraShutter"`
	RPICameraMetering          string  `json:"rpiCameraMetering"`
	RPICameraGain              float64 `json:"rpiCameraGain"`
	RPICameraEV                float64 `json:"rpiCameraEV"`
	RPICameraROI               string  `json:"rpiCameraROI"`
	RPICameraHDR               bool    `json:"rpiCameraHDR"`
	RPICameraTuningFile        string  `json:"rpiCameraTuningFile"`
	RPICameraMode              string  `json:"rpiCameraMode"`
	RPICameraFPS               float64 `json:"rpiCameraFPS"`
	RPICameraIDRPeriod         int     `json:"rpiCameraIDRPeriod"`
	RPICameraBitrate           int     `json:"rpiCameraBitrate"`
	RPICameraProfile           string  `json:"rpiCameraProfile"`
	RPICameraLevel             string  `json:"rpiCameraLevel"`
	RPICameraAfMode            string  `json:"rpiCameraAfMode"`
	RPICameraAfRange           string  `json:"rpiCameraAfRange"`
	RPICameraAfSpeed           string  `json:"rpiCameraAfSpeed"`
	RPICameraLensPosition      float64 `json:"rpiCameraLensPosition"`
	RPICameraAfWindow          string  `json:"rpiCameraAfWindow"`
	RPICameraTextOverlayEnable bool    `json:"rpiCameraTextOverlayEnable"`
	RPICameraTextOverlay       string  `json:"rpiCameraTextOverlay"`

	// external commands
	RunOnInit               string         `json:"runOnInit"`
	RunOnInitRestart        bool           `json:"runOnInitRestart"`
	RunOnDemand             string         `json:"runOnDemand"`
	RunOnDemandRestart      bool           `json:"runOnDemandRestart"`
	RunOnDemandStartTimeout StringDuration `json:"runOnDemandStartTimeout"`
	RunOnDemandCloseAfter   StringDuration `json:"runOnDemandCloseAfter"`
	RunOnReady              string         `json:"runOnReady"`
	RunOnReadyRestart       bool           `json:"runOnReadyRestart"`
	RunOnRead               string         `json:"runOnRead"`
	RunOnReadRestart        bool           `json:"runOnReadRestart"`
}

func (pconf *PathConf) check(conf *Conf, name string) error {
	switch {
	case name == "all":
		pconf.Regexp = regexp.MustCompile("^.*$")

	case name == "" || name[0] != '~': // normal path
		err := IsValidPathName(name)
		if err != nil {
			return fmt.Errorf("invalid path name '%s': %s", name, err)
		}

	default: // regular expression-based path
		pathRegexp, err := regexp.Compile(name[1:])
		if err != nil {
			return fmt.Errorf("invalid regular expression: %s", name[1:])
		}
		pconf.Regexp = pathRegexp
	}

	switch {
	case pconf.Source == "publisher":

	case strings.HasPrefix(pconf.Source, "rtsp://") ||
		strings.HasPrefix(pconf.Source, "rtsps://"):
		if pconf.Regexp != nil {
			return fmt.Errorf("a path with a regular expression (or path 'all') cannot have a RTSP source. use another path")
		}

		_, err := url.Parse(pconf.Source)
		if err != nil {
			return fmt.Errorf("'%s' is not a valid URL", pconf.Source)
		}

	case strings.HasPrefix(pconf.Source, "rtmp://") ||
		strings.HasPrefix(pconf.Source, "rtmps://"):
		if pconf.Regexp != nil {
			return fmt.Errorf("a path with a regular expression (or path 'all') cannot have a RTMP source. use another path")
		}

		u, err := gourl.Parse(pconf.Source)
		if err != nil {
			return fmt.Errorf("'%s' is not a valid URL", pconf.Source)
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
			return fmt.Errorf("a path with a regular expression (or path 'all') cannot have a HLS source. use another path")
		}

		u, err := gourl.Parse(pconf.Source)
		if err != nil {
			return fmt.Errorf("'%s' is not a valid URL", pconf.Source)
		}
		if u.Scheme != "http" && u.Scheme != "https" {
			return fmt.Errorf("'%s' is not a valid URL", pconf.Source)
		}

		if u.User != nil {
			pass, _ := u.User.Password()
			user := u.User.Username()
			if user != "" && pass == "" ||
				user == "" && pass != "" {
				return fmt.Errorf("username and password must be both provided")
			}
		}

	case strings.HasPrefix(pconf.Source, "udp://"):
		if pconf.Regexp != nil {
			return fmt.Errorf("a path with a regular expression (or path 'all') cannot have a HLS source. use another path")
		}

		_, _, err := net.SplitHostPort(pconf.Source[len("udp://"):])
		if err != nil {
			return fmt.Errorf("'%s' is not a valid UDP URL", pconf.Source)
		}

	case strings.HasPrefix(pconf.Source, "srt://"):
		if pconf.Regexp != nil {
			return fmt.Errorf("a path with a regular expression (or path 'all') cannot have a SRT source. use another path")
		}

		_, err := gourl.Parse(pconf.Source)
		if err != nil {
			return fmt.Errorf("'%s' is not a valid URL", pconf.Source)
		}

	case strings.HasPrefix(pconf.Source, "whep://") ||
		strings.HasPrefix(pconf.Source, "wheps://"):
		if pconf.Regexp != nil {
			return fmt.Errorf("a path with a regular expression (or path 'all') " +
				"cannot have a WebRTC/WHEP source. use another path")
		}

		_, err := gourl.Parse(pconf.Source)
		if err != nil {
			return fmt.Errorf("'%s' is not a valid URL", pconf.Source)
		}

	case pconf.Source == "redirect":
		if pconf.SourceRedirect == "" {
			return fmt.Errorf("source redirect must be filled")
		}

		_, err := url.Parse(pconf.SourceRedirect)
		if err != nil {
			return fmt.Errorf("'%s' is not a valid RTSP URL", pconf.SourceRedirect)
		}

	case pconf.Source == "rpiCamera":
		if pconf.Regexp != nil {
			return fmt.Errorf(
				"a path with a regular expression (or path 'all') cannot have 'rpiCamera' as source. use another path")
		}

		for otherName, otherPath := range conf.Paths {
			if otherPath != pconf && otherPath != nil &&
				otherPath.Source == "rpiCamera" && otherPath.RPICameraCamID == pconf.RPICameraCamID {
				return fmt.Errorf("'rpiCamera' with same camera ID %d is used as source in two paths, '%s' and '%s'",
					pconf.RPICameraCamID, name, otherName)
			}
		}

	default:
		return fmt.Errorf("invalid source: '%s'", pconf.Source)
	}

	if pconf.SourceOnDemand {
		if pconf.Source == "publisher" {
			return fmt.Errorf("'sourceOnDemand' is useless when source is 'publisher'")
		}
	}

	if pconf.DisablePublisherOverride {
		pconf.OverridePublisher = true
	}

	if pconf.Fallback != "" {
		if strings.HasPrefix(pconf.Fallback, "/") {
			err := IsValidPathName(pconf.Fallback[1:])
			if err != nil {
				return fmt.Errorf("'%s': %s", pconf.Fallback, err)
			}
		} else {
			_, err := url.Parse(pconf.Fallback)
			if err != nil {
				return fmt.Errorf("'%s' is not a valid RTSP URL", pconf.Fallback)
			}
		}
	}

	if (pconf.PublishUser != "" && pconf.PublishPass == "") ||
		(pconf.PublishUser == "" && pconf.PublishPass != "") {
		return fmt.Errorf("read username and password must be both filled")
	}

	if pconf.PublishUser != "" && pconf.Source != "publisher" {
		return fmt.Errorf("'publishUser' is useless when source is not 'publisher', since " +
			"the stream is not provided by a publisher, but by a fixed source")
	}

	if len(pconf.PublishIPs) > 0 && pconf.Source != "publisher" {
		return fmt.Errorf("'publishIPs' is useless when source is not 'publisher', since " +
			"the stream is not provided by a publisher, but by a fixed source")
	}

	if (pconf.ReadUser != "" && pconf.ReadPass == "") ||
		(pconf.ReadUser == "" && pconf.ReadPass != "") {
		return fmt.Errorf("read username and password must be both filled")
	}

	if contains(conf.AuthMethods, headers.AuthDigest) {
		if strings.HasPrefix(string(pconf.PublishUser), "sha256:") ||
			strings.HasPrefix(string(pconf.PublishPass), "sha256:") ||
			strings.HasPrefix(string(pconf.ReadUser), "sha256:") ||
			strings.HasPrefix(string(pconf.ReadPass), "sha256:") {
			return fmt.Errorf("hashed credentials can't be used when the digest auth method is available")
		}
	}

	if conf.ExternalAuthenticationURL != "" {
		if pconf.PublishUser != "" ||
			len(pconf.PublishIPs) > 0 ||
			pconf.ReadUser != "" ||
			len(pconf.ReadIPs) > 0 {
			return fmt.Errorf("credentials or IPs can't be used together with 'externalAuthenticationURL'")
		}
	}

	if pconf.RunOnInit != "" && pconf.Regexp != nil {
		return fmt.Errorf("a path with a regular expression does not support option 'runOnInit'; use another path")
	}

	if pconf.RunOnDemand != "" && pconf.Source != "publisher" {
		return fmt.Errorf("'runOnDemand' can be used only when source is 'publisher'")
	}

	return nil
}

// Equal checks whether two PathConfs are equal.
func (pconf *PathConf) Equal(other *PathConf) bool {
	return reflect.DeepEqual(pconf, other)
}

// Clone clones the configuration.
func (pconf PathConf) Clone() *PathConf {
	enc, err := json.Marshal(pconf)
	if err != nil {
		panic(err)
	}

	var dest PathConf
	err = json.Unmarshal(enc, &dest)
	if err != nil {
		panic(err)
	}

	return &dest
}

// HasStaticSource checks whether the path has a static source.
func (pconf PathConf) HasStaticSource() bool {
	return strings.HasPrefix(pconf.Source, "rtsp://") ||
		strings.HasPrefix(pconf.Source, "rtsps://") ||
		strings.HasPrefix(pconf.Source, "rtmp://") ||
		strings.HasPrefix(pconf.Source, "rtmps://") ||
		strings.HasPrefix(pconf.Source, "http://") ||
		strings.HasPrefix(pconf.Source, "https://") ||
		strings.HasPrefix(pconf.Source, "udp://") ||
		strings.HasPrefix(pconf.Source, "srt://") ||
		strings.HasPrefix(pconf.Source, "whep://") ||
		strings.HasPrefix(pconf.Source, "wheps://") ||
		pconf.Source == "rpiCamera"
}

// HasOnDemandStaticSource checks whether the path has a on demand static source.
func (pconf PathConf) HasOnDemandStaticSource() bool {
	return pconf.HasStaticSource() && pconf.SourceOnDemand
}

// HasOnDemandPublisher checks whether the path has a on-demand publisher.
func (pconf PathConf) HasOnDemandPublisher() bool {
	return pconf.RunOnDemand != ""
}

// UnmarshalJSON implements json.Unmarshaler. It is used to set default values.
func (pconf *PathConf) UnmarshalJSON(b []byte) error {
	// source
	pconf.Source = "publisher"

	// general
	pconf.SourceOnDemandStartTimeout = 10 * StringDuration(time.Second)
	pconf.SourceOnDemandCloseAfter = 10 * StringDuration(time.Second)

	// publisher
	pconf.OverridePublisher = true

	// raspberry pi camera
	pconf.RPICameraWidth = 1920
	pconf.RPICameraHeight = 1080
	pconf.RPICameraContrast = 1
	pconf.RPICameraSaturation = 1
	pconf.RPICameraSharpness = 1
	pconf.RPICameraFPS = 30
	pconf.RPICameraIDRPeriod = 60
	pconf.RPICameraBitrate = 1000000
	pconf.RPICameraProfile = "main"
	pconf.RPICameraLevel = "4.1"
	pconf.RPICameraTextOverlay = "%Y-%m-%d %H:%M:%S - MediaMTX"

	// external commands
	pconf.RunOnDemandStartTimeout = 10 * StringDuration(time.Second)
	pconf.RunOnDemandCloseAfter = 10 * StringDuration(time.Second)

	type alias PathConf
	d := json.NewDecoder(bytes.NewReader(b))
	d.DisallowUnknownFields()
	return d.Decode((*alias)(pconf))
}
