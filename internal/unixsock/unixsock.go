// Package unixsock 处理 unix socket 监听文件的公共准备逻辑。
// 两个进程（非特权的 server 与 root agent）各自绑一个 socket，
// 启动前都要清理上一次留下的文件，规则必须一致。
package unixsock

import (
	"fmt"
	"net"
	"os"
)

// RemoveStale 在监听前清理上一个进程留下的 socket 文件。
//
// 只删除真正的 socket：SOCKET_PATH 之类配错成普通文件时，无条件 os.Remove
// 会把无关文件删掉。能连上说明还有进程在监听，返回错误拒绝启动，
// 而不是把对方的 socket 文件抢走（那会留下一个活着但谁也连不上的进程）。
func RemoveStale(path string) error {
	fi, err := os.Lstat(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}
	if fi.Mode()&os.ModeSocket == 0 {
		return fmt.Errorf("%s 已存在但不是 socket，拒绝启动以免删掉无关文件", path)
	}
	if conn, err := net.Dial("unix", path); err == nil {
		_ = conn.Close()
		return fmt.Errorf("已有进程在 %s 上监听，拒绝启动", path)
	}
	return os.Remove(path)
}
