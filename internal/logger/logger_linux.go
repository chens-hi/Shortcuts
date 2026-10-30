//go:build linux

package logger

import "os"

func SystemUID() int {
	return os.Getuid()
}
