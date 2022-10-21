// Package m3u8 contains a M3U8 parser.
package m3u8

import (
	"bytes"
	"errors"
	"regexp"
	"strings"

	gm3u8 "github.com/grafov/m3u8"
)

var reKeyValue = regexp.MustCompile(`([a-zA-Z0-9_-]+)=("[^"]+"|[^",]+)`)

func decodeParamsLine(line string) map[string]string {
	out := make(map[string]string)
	for _, kv := range reKeyValue.FindAllStringSubmatch(line, -1) {
		k, v := kv[1], kv[2]
		out[k] = strings.Trim(v, ` "`)
	}
	return out
}

// MasterPlaylist is a master playlist.
type MasterPlaylist struct {
	gm3u8.MasterPlaylist
	Alternatives []*gm3u8.Alternative
}

func (MasterPlaylist) isPlaylist() {}

func newMasterPlaylist(byts []byte, mpl *gm3u8.MasterPlaylist) (*MasterPlaylist, error) {
	var alternatives []*gm3u8.Alternative

	// https://github.com/grafov/m3u8/blob/036100c52a87e26c62be56df85450e9c703201a6/reader.go#L301
	for _, line := range strings.Split(string(byts), "\n") {
		if strings.HasPrefix(line, "#EXT-X-MEDIA:") {
			var alt gm3u8.Alternative
			for k, v := range decodeParamsLine(line[13:]) {
				switch k {
				case "TYPE":
					alt.Type = v
				case "GROUP-ID":
					alt.GroupId = v
				case "LANGUAGE":
					alt.Language = v
				case "NAME":
					alt.Name = v
				case "DEFAULT":
					switch {
					case strings.ToUpper(v) == "YES":
						alt.Default = true
					case strings.ToUpper(v) == "NO":
						alt.Default = false
					default:
						return nil, errors.New("value must be YES or NO")
					}
				case "AUTOSELECT":
					alt.Autoselect = v
				case "FORCED":
					alt.Forced = v
				case "CHARACTERISTICS":
					alt.Characteristics = v
				case "SUBTITLES":
					alt.Subtitles = v
				case "URI":
					alt.URI = v
				}
			}
			alternatives = append(alternatives, &alt)
		}
	}

	return &MasterPlaylist{
		MasterPlaylist: *mpl,
		Alternatives:   alternatives,
	}, nil
}

// MediaPlaylist is a media playlist.
type MediaPlaylist gm3u8.MediaPlaylist

func (MediaPlaylist) isPlaylist() {}

// Playlist is a M3U8 playlist.
type Playlist interface {
	isPlaylist()
}

// Unmarshal decodes a M3U8 Playlist.
func Unmarshal(byts []byte) (Playlist, error) {
	pl, _, err := gm3u8.Decode(*(bytes.NewBuffer(byts)), true)
	if err != nil {
		return nil, err
	}

	switch tpl := pl.(type) {
	case *gm3u8.MasterPlaylist:
		return newMasterPlaylist(byts, tpl)

	case *gm3u8.MediaPlaylist:
		return (*MediaPlaylist)(tpl), nil
	}

	panic("unexpected playlist type")
}
