//go:build !linux

package recorder

import "os"

func fadviseDropCache(_ *os.File) {}
