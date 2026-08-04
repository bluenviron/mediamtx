package conf

import (
	"fmt"
)

// Forward is a list of ForwardDest.
type Forward []ForwardDest

// Validate validates the configuration.
func (p Forward) Validate() error {
	for i, entry := range p {
		err := entry.Validate()
		if err != nil {
			return fmt.Errorf("entry %d: %w", i, err)
		}
	}

	return nil
}
