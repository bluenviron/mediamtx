package subgroup

import (
	"fmt"
	"io"
)

// SubGroup is a subgroup stream.
type SubGroup struct {
	Header  Header
	Objects []Object
}

// Read reads a SubGroup.
func (s *SubGroup) Read(r io.Reader) error {
	if err := s.Header.read(r); err != nil {
		return err
	}

	// for the moment, support reading a single object only.

	var o1 Object
	err := o1.read(r, &s.Header)
	if err != nil {
		return err
	}
	s.Objects = []Object{o1}

	var o2 Object
	err = o2.read(r, &s.Header)
	if err != nil {
		return err
	}
	if len(o2.Payload) != 0 {
		return fmt.Errorf("unexpected second object")
	}

	return nil
}

func (s SubGroup) marshalSize() int {
	n := s.Header.marshalSize()

	for _, o := range s.Objects {
		n += o.marshalSize(&s.Header)
	}

	n += Object{Payload: nil}.marshalSize(&s.Header)

	return n
}

func (s SubGroup) marshalTo(buf []byte) int {
	n := s.Header.marshalTo(buf)

	for _, o := range s.Objects {
		n += o.marshalTo(buf[n:], &s.Header)
	}

	n += Object{Payload: nil}.marshalTo(buf[n:], &s.Header)

	return n
}

// Marshal encodes a SubGroup.
func (s *SubGroup) Marshal() []byte {
	buf := make([]byte, s.marshalSize())
	s.marshalTo(buf)
	return buf
}
