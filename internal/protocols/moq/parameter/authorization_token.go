package parameter

import (
	"fmt"

	"github.com/bluenviron/mediamtx/internal/protocols/moq/varint"
)

const typeAuthorizationToken = 0x03

// AuthorizationTokenAliasType is a value of Alias Type.
// spec: draft-18, section 10.2.2
type AuthorizationTokenAliasType uint64

// spec: draft-18, section 10.2.2
const (
	AuthorizationTokenAliasTypeUseValue AuthorizationTokenAliasType = 0x03
)

// AuthorizationToken is the AUTHORIZATION_TOKEN parameter.
// spec: draft-18, section 10.2.2
type AuthorizationToken struct {
	AliasType  AuthorizationTokenAliasType
	TokenType  uint64
	TokenValue []byte
}

func (*AuthorizationToken) isParameter() {}

func (*AuthorizationToken) paramType() uint64 {
	return typeAuthorizationToken
}

func (t *AuthorizationToken) unmarshal(buf []byte) (int, error) {
	var le varint.Varint
	n1, err := le.Unmarshal(buf)
	if err != nil {
		return 0, err
	}
	buf = buf[n1:]

	if uint64(len(buf)) < uint64(le) {
		return 0, fmt.Errorf("not enough bytes for parameter value")
	}

	buf = buf[:le]

	var aliasType varint.Varint
	n, err := aliasType.Unmarshal(buf)
	if err != nil {
		return 0, err
	}
	buf = buf[n:]

	t.AliasType = AuthorizationTokenAliasType(aliasType)

	if t.AliasType != AuthorizationTokenAliasTypeUseValue {
		return 0, fmt.Errorf("unsupported token alias type: %d", aliasType)
	}

	var tokenType varint.Varint
	n, err = tokenType.Unmarshal(buf)
	if err != nil {
		return 0, err
	}

	t.TokenType = uint64(tokenType)
	t.TokenValue = buf[n:]

	return n1 + int(le), nil
}

func (t AuthorizationToken) marshalSize() int {
	n := varint.Varint(t.AliasType).MarshalSize() +
		varint.Varint(t.TokenType).MarshalSize() +
		len(t.TokenValue)

	return varint.Varint(n).MarshalSize() + n
}

func (t AuthorizationToken) marshalTo(buf []byte) int {
	innerSize := varint.Varint(t.AliasType).MarshalSize() +
		varint.Varint(t.TokenType).MarshalSize() +
		len(t.TokenValue)
	n := varint.Varint(innerSize).MarshalTo(buf)
	n += varint.Varint(t.AliasType).MarshalTo(buf[n:])
	n += varint.Varint(t.TokenType).MarshalTo(buf[n:])
	n += copy(buf[n:], t.TokenValue)
	return n
}
