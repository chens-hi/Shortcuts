//go:build windows

package repository

// syncDir 在 Windows 上是空操作：打开目录句柄再 Sync 会直接 Access is denied，
// 该平台不支持目录 fsync。产品只跑 Linux（fnOS），这里仅为本地开发/测试可编译。
func syncDir(dirPath string) error {
	return nil
}
