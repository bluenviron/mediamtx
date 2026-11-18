package srt

import (
	"fmt"
	"strings"
)

type streamIDMode int

const (
	streamIDModeRead streamIDMode = iota
	streamIDModePublish
)

type streamID struct {
	mode  streamIDMode
	path  string
	query string
	user  string
	pass  string
}

func (s *streamID) unmarshal(raw string) error {
	// standard syntax
	// https://github.com/Haivision/srt/blob/master/docs/features/access-control.md
	if strings.HasPrefix(raw, "#!::") {
		var (
			modeSet bool
			bmdUUID string
			bmdName string
			isBMD   bool
		)

		for kv := range strings.SplitSeq(raw[len("#!::"):], ",") {
			kv2 := strings.SplitN(kv, "=", 2)
			if len(kv2) != 2 {
				return fmt.Errorf("invalid value")
			}

			key, value := kv2[0], kv2[1]

			switch key {
			case "u":
				s.user = value

			case "r":
				s.path = value

			case "h":

			case "s":
				s.pass = value

			case "t":

			case "m":
				switch value {
				case "request":
					s.mode = streamIDModeRead
					modeSet = true

				case "publish":
					s.mode = streamIDModePublish
					modeSet = true

				default:
					return fmt.Errorf("unsupported mode '%s'", value)
				}

			// Blackmagic Design encoders (ATEM Mini, Web Presenter, ...)
			// send non-standard keys and no resource name or mode.
			case "bmd_uuid":
				bmdUUID = value
				isBMD = true

			case "bmd_name":
				bmdName = value
				isBMD = true
			}
		}

		// Only touched when a Blackmagic encoder is detected.
		// Every other stream ID behaves exactly as before.
		if isBMD {
			// Blackmagic encoders only ever publish.
			if !modeSet {
				s.mode = streamIDModePublish
			}

			// Path: an explicit r= wins. Otherwise use the key field (u=),
			// which mirrors how the ATEM RTMP key works as the stream name.
			// Fall back to the device UUID.
			// bmd_name (e.g. "ATEM Mini Extreme ISO") is deliberately not used
			// as it contains spaces, which are not valid in path names.
			_ = bmdName

			if s.path == "" {
				if s.user != "" {
					s.path = s.user
					s.user = "" // used as a name, not a credential
				} else {
					s.path = bmdUUID
				}
			}
		}
	} else {
		parts := strings.Split(raw, ":")
		if len(parts) < 2 || len(parts) > 5 {
			return fmt.Errorf("stream ID must be 'action:pathname[:query]' or 'action:pathname:user:pass[:query]', " +
				"where action is either read or publish, pathname is the path name, user and pass are the credentials, " +
				"query is an optional token containing additional information")
		}

		parts[len(parts)-1] = strings.TrimSuffix(parts[len(parts)-1], "#feedbackplay")

		switch parts[0] {
		case "read":
			s.mode = streamIDModeRead

		case "publish":
			s.mode = streamIDModePublish

		default:
			return fmt.Errorf("stream ID must be 'action:pathname[:query]' or 'action:pathname:user:pass[:query]', " +
				"where action is either read or publish, pathname is the path name, user and pass are the credentials, " +
				"query is an optional token containing additional information")
		}

		s.path = parts[1]

		if len(parts) == 4 || len(parts) == 5 {
			s.user, s.pass = parts[2], parts[3]
		}

		if len(parts) == 3 {
			s.query = parts[2]
		} else if len(parts) == 5 {
			s.query = parts[4]
		}
	}

	return nil
}
