//go:build linux

package agent

import "os"

func systemRunUID() int {
	return os.Getuid()
}

func chownSocket(path string, uid, gid int) error {
	if uid < 0 || gid < 0 {
		return nil
	}
	return os.Chown(path, uid, gid)
}
