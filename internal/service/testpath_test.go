package service

import (
	"os"
	"testing"
)

// realFolderPath 返回一个真实存在、且通过 IsValidFolderPath 格式检查的目录。
// 统一用 /tmp 前缀：Windows 下 Go 把它解析为当前盘符的 \tmp（已验证），Linux 下即 /tmp，
// 两边的测试机都能真实创建，避免服务端「目录必须存在」校验把假路径判成非法。
func realFolderPath(t *testing.T, name string) string {
	t.Helper()
	p := "/tmp/shortcuts-test-" + name
	if err := os.MkdirAll(p, 0o755); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.RemoveAll(p) })
	return p
}
