package parameter

import (
	"fmt"

	"github.com/bluenviron/mediamtx/internal/protocols/moq/version"
)

const typeIncludeProperties = 0x35

// IncludeProperties controls whether Track Properties appear in the response.
type IncludeProperties struct {
	Value bool
}

func (*IncludeProperties) isParameter() {}

func (*IncludeProperties) paramType() uint64 { return typeIncludeProperties }

func (p *IncludeProperties) unmarshal(buf []byte, _ version.Version) (int, error) {
	if len(buf) == 0 || buf[0] > 1 {
		return 0, fmt.Errorf("invalid INCLUDE_PROPERTIES value")
	}
	p.Value = buf[0] == 1
	return 1, nil
}

func (p IncludeProperties) marshalSize(_ version.Version) int { return 1 }

func (p IncludeProperties) marshalTo(buf []byte, _ version.Version) int {
	if p.Value {
		buf[0] = 1
	} else {
		buf[0] = 0
	}
	return 1
}
