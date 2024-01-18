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
		for _, kv := range strings.Split(raw[len("#!::"):], ",") {
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

				case "publish":
					s.mode = streamIDModePublish

				default:
					return fmt.Errorf("unsupported mode '%s'", value)
				}

			default:
				return fmt.Errorf("unsupported key '%s'", key)
			}
		}
	} else {
		parts := strings.Split(raw, ":")
		if len(parts) < 2 || len(parts) > 5 {
			return fmt.Errorf("stream ID must be 'action:pathname[:query]' or 'action:pathname:user:pass[:query]', " +
				"where action is either read or publish, pathname is the path name, user and pass are the credentials, " +
				"query is an optional token containing additional information")
		}

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
