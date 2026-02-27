//go:build !enable_upgrade

package core

import "fmt"

func upgrade() error {
	return fmt.Errorf("upgrade command is not available")
}
