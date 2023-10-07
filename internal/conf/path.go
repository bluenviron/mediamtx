package conf

import (
	"encoding/json"
	"fmt"
	"net"
	gourl "net/url"
	"reflect"
	"regexp"
	"strings"
	"time"

	"github.com/bluenviron/gortsplib/v4/pkg/headers"
	"github.com/bluenviron/gortsplib/v4/pkg/url"
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

func srtCheckPassphrase(passphrase string) error {
	switch {
	case len(passphrase) < 10 || len(passphrase) > 79:
		return fmt.Errorf("must be between 10 and 79 characters")

	default:
		return nil
	}
}

// Path is a path configuration.
type Path struct {
	Regexp *regexp.Regexp `json:"-"`

	// General
	Source                     string         `json:"source"`
	SourceFingerprint          string         `json:"sourceFingerprint"`
	SourceOnDemand             bool           `json:"sourceOnDemand"`
	SourceOnDemandStartTimeout StringDuration `json:"sourceOnDemandStartTimeout"`
	SourceOnDemandCloseAfter   StringDuration `json:"sourceOnDemandCloseAfter"`
	MaxReaders                 int            `json:"maxReaders"`
	SRTReadPassphrase          string         `json:"srtReadPassphrase"`
	Record                     bool           `json:"record"`

	// Authentication
	PublishUser Credential `json:"publishUser"`
	PublishPass Credential `json:"publishPass"`
	PublishIPs  IPsOrCIDRs `json:"publishIPs"`
	ReadUser    Credential `json:"readUser"`
	ReadPass    Credential `json:"readPass"`
	ReadIPs     IPsOrCIDRs `json:"readIPs"`

	// Publisher
	OverridePublisher        bool   `json:"overridePublisher"`
	DisablePublisherOverride *bool  `json:"disablePublisherOverride,omitempty"` // deprecated
	Fallback                 string `json:"fallback"`
	SRTPublishPassphrase     string `json:"srtPublishPassphrase"`

	// RTSP
	SourceProtocol      SourceProtocol `json:"sourceProtocol"`
	SourceAnyPortEnable bool           `json:"sourceAnyPortEnable"`
	RTSPRangeType       RTSPRangeType  `json:"rtspRangeType"`
	RTSPRangeStart      string         `json:"rtspRangeStart"`

	// Redirect
	SourceRedirect string `json:"sourceRedirect"`

	// Raspberry Pi Camera
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

	// Hooks
	RunOnInit                  string         `json:"runOnInit"`
	RunOnInitRestart           bool           `json:"runOnInitRestart"`
	RunOnDemand                string         `json:"runOnDemand"`
	RunOnDemandRestart         bool           `json:"runOnDemandRestart"`
	RunOnDemandStartTimeout    StringDuration `json:"runOnDemandStartTimeout"`
	RunOnDemandCloseAfter      StringDuration `json:"runOnDemandCloseAfter"`
	RunOnReady                 string         `json:"runOnReady"`
	RunOnReadyRestart          bool           `json:"runOnReadyRestart"`
	RunOnNotReady              string         `json:"runOnNotReady"`
	RunOnRead                  string         `json:"runOnRead"`
	RunOnReadRestart           bool           `json:"runOnReadRestart"`
	RunOnUnread                string         `json:"runOnUnread"`
	RunOnRecordSegmentComplete string         `json:"runOnRecordSegmentComplete"`
}

func (pconf *Path) setDefaults() {
	// General
	pconf.Source = "publisher"
	pconf.SourceOnDemandStartTimeout = 10 * StringDuration(time.Second)
	pconf.SourceOnDemandCloseAfter = 10 * StringDuration(time.Second)
	pconf.Record = true

	// Publisher
	pconf.OverridePublisher = true

	// Raspberry Pi Camera
	pconf.RPICameraWidth = 1920
	pconf.RPICameraHeight = 1080
	pconf.RPICameraContrast = 1
	pconf.RPICameraSaturation = 1
	pconf.RPICameraSharpness = 1
	pconf.RPICameraExposure = "normal"
	pconf.RPICameraAWB = "auto"
	pconf.RPICameraDenoise = "off"
	pconf.RPICameraMetering = "centre"
	pconf.RPICameraFPS = 30
	pconf.RPICameraIDRPeriod = 60
	pconf.RPICameraBitrate = 1000000
	pconf.RPICameraProfile = "main"
	pconf.RPICameraLevel = "4.1"
	pconf.RPICameraAfMode = "auto"
	pconf.RPICameraAfRange = "normal"
	pconf.RPICameraAfSpeed = "normal"
	pconf.RPICameraTextOverlay = "%Y-%m-%d %H:%M:%S - MediaMTX"

	// Hooks
	pconf.RunOnDemandStartTimeout = 10 * StringDuration(time.Second)
	pconf.RunOnDemandCloseAfter = 10 * StringDuration(time.Second)
}

func newPath(defaults *Path, partial *OptionalPath) *Path {
	pconf := &Path{}
	copyStructFields(pconf, defaults)
	copyStructFields(pconf, partial.Values)
	return pconf
}

// Clone clones the configuration.
func (pconf Path) Clone() *Path {
	enc, err := json.Marshal(pconf)
	if err != nil {
		panic(err)
	}

	var dest Path
	err = json.Unmarshal(enc, &dest)
	if err != nil {
		panic(err)
	}

	dest.Regexp = pconf.Regexp

	return &dest
}

func (pconf *Path) check(conf *Conf, name string) error {
	switch {
	case name == "all":
		pconf.Regexp = regexp.MustCompile("^.*$")

	case name == "" || name[0] != '~': // normal path
		err := IsValidPathName(name)
		if err != nil {
			return fmt.Errorf("invalid path name '%s': %s", name, err)
		}

	default: // regular expression-based path
		regexp, err := regexp.Compile(name[1:])
		if err != nil {
			return fmt.Errorf("invalid regular expression: %s", name[1:])
		}
		pconf.Regexp = regexp
	}

	// General

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

		switch pconf.RPICameraExposure {
		case "normal", "short", "long", "custom":
		default:
			return fmt.Errorf("invalid 'rpiCameraExposure' value")
		}

		switch pconf.RPICameraAWB {
		case "auto", "incandescent", "tungsten", "fluorescent", "indoor", "daylight", "cloudy", "custom":
		default:
			return fmt.Errorf("invalid 'rpiCameraAWB' value")
		}

		switch pconf.RPICameraDenoise {
		case "off", "cdn_off", "cdn_fast", "cdn_hq":
		default:
			return fmt.Errorf("invalid 'rpiCameraDenoise' value")
		}

		switch pconf.RPICameraMetering {
		case "centre", "spot", "matrix", "custom":
		default:
			return fmt.Errorf("invalid 'rpiCameraMetering' value")
		}

		switch pconf.RPICameraAfMode {
		case "auto", "manual", "continuous":
		default:
			return fmt.Errorf("invalid 'rpiCameraAfMode' value")
		}

		switch pconf.RPICameraAfRange {
		case "normal", "macro", "full":
		default:
			return fmt.Errorf("invalid 'rpiCameraAfRange' value")
		}

		switch pconf.RPICameraAfSpeed {
		case "normal", "fast":
		default:
			return fmt.Errorf("invalid 'rpiCameraAfSpeed' value")
		}

	default:
		return fmt.Errorf("invalid source: '%s'", pconf.Source)
	}

	if pconf.SourceOnDemand {
		if pconf.Source == "publisher" {
			return fmt.Errorf("'sourceOnDemand' is useless when source is 'publisher'")
		}
	}
	if pconf.SRTReadPassphrase != "" {
		err := srtCheckPassphrase(pconf.SRTReadPassphrase)
		if err != nil {
			return fmt.Errorf("invalid 'readRTPassphrase': %v", err)
		}
	}

	// Publisher

	if pconf.DisablePublisherOverride != nil {
		pconf.OverridePublisher = !*pconf.DisablePublisherOverride
	}
	if pconf.Fallback != "" {
		if pconf.Source != "publisher" {
			return fmt.Errorf("'fallback' can only be used when source is 'publisher'")
		}

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
	if pconf.SRTPublishPassphrase != "" {
		if pconf.Source != "publisher" {
			return fmt.Errorf("'srtPublishPassphase' can only be used when source is 'publisher'")
		}

		err := srtCheckPassphrase(pconf.SRTPublishPassphrase)
		if err != nil {
			return fmt.Errorf("invalid 'srtPublishPassphrase': %v", err)
		}
	}

	// Authentication

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

	// Hooks

	if pconf.RunOnInit != "" && pconf.Regexp != nil {
		return fmt.Errorf("a path with a regular expression (or path 'all')" +
			" does not support option 'runOnInit'; use another path")
	}
	if pconf.RunOnDemand != "" && pconf.Source != "publisher" {
		return fmt.Errorf("'runOnDemand' can be used only when source is 'publisher'")
	}

	return nil
}

// Equal checks whether two Paths are equal.
func (pconf *Path) Equal(other *Path) bool {
	return reflect.DeepEqual(pconf, other)
}

// HasStaticSource checks whether the path has a static source.
func (pconf Path) HasStaticSource() bool {
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
func (pconf Path) HasOnDemandStaticSource() bool {
	return pconf.HasStaticSource() && pconf.SourceOnDemand
}

// HasOnDemandPublisher checks whether the path has a on-demand publisher.
func (pconf Path) HasOnDemandPublisher() bool {
	return pconf.RunOnDemand != ""
}
