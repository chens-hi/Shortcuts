//go:build !linux

package agent

func systemRunUID() int {
	return -1
}

func chownSocket(path string, uid, gid int) error {
	return nil
}
