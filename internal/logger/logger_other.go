//go:build !linux

package logger

func SystemUID() int {
	return -1
}
