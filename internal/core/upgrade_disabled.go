//go:build !enableUpgrade

package core

import "fmt"

func upgrade() error {
	return fmt.Errorf("upgrade command is not available")
}
