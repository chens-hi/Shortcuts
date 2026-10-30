package provider

import (
	"encoding/json"
	"os"
	"testing"

	"github.com/chens-hi/shortcuts/internal/model"
)

func validFolderEntry(path string) model.Entry {
	raw, err := json.Marshal(model.FolderConfig{Path: path})
	if err != nil {
		panic(err)
	}
	return model.NewEntry(model.EntryTypeFolder, "photos", "照片", raw)
}

// realProviderFolder 创建并返回一个真实存在、通过 IsValidFolderPath 格式检查的目录。
// /tmp 前缀在 Windows 下解析为当前盘符的 \tmp，Linux 下即 /tmp，两边都能真实创建。
func realProviderFolder(t *testing.T, name string) string {
	t.Helper()
	p := "/tmp/shortcuts-provider-" + name
	if err := os.MkdirAll(p, 0o755); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.RemoveAll(p) })
	return p
}

func TestFolderProviderValidate(t *testing.T) {
	provider := FolderProvider{}
	dir := realProviderFolder(t, "photos")
	sub := dir + "/照片 备份"
	if err := os.MkdirAll(sub, 0o755); err != nil {
		t.Fatal(err)
	}

	// 只测格式校验：存在性由 service 保存链路经 root agent 判定
	// （provider 对 /vol1 共享目录无读权限，本地 stat 会误报，做不了存在性），
	// 见 service 层的 ValidateFolderPath 测试。
	for _, tc := range []struct {
		path    string
		wantErr bool
	}{
		{path: dir, wantErr: false},
		{path: sub, wantErr: false},
		{path: "/", wantErr: true},                          // 根目录是宿主默认位置，拒绝
		{path: "vol1/1000", wantErr: true},                  // 相对路径
		{path: "", wantErr: true},
		{path: "   ", wantErr: true},
		{path: "/vol1/\tx", wantErr: true},                  // 控制字符
	} {
		entry := validFolderEntry(tc.path)
		err := provider.Validate(entry)
		if tc.wantErr && err == nil {
			t.Errorf("Validate(path=%q) = nil, want error", tc.path)
		}
		if !tc.wantErr && err != nil {
			t.Errorf("Validate(path=%q) = %v, want nil", tc.path, err)
		}
	}
}

func TestFolderProviderSpecIsRejected(t *testing.T) {
	// folder 条目不再打包子应用；任何调用 Spec 的路径都必须立刻失败。
	if _, err := (FolderProvider{}).Spec(validFolderEntry("/vol1/1000/照片"), "shortcuts"); err == nil {
		t.Fatal("Spec must reject folder entries: desktop items are created on the client")
	}
}

func TestFolderProviderChanged(t *testing.T) {
	provider := FolderProvider{}
	old := validFolderEntry("/vol1/1000/照片")

	next := old
	if provider.Changed(old, next) {
		t.Error("identical entries reported as changed")
	}

	next = validFolderEntry("/vol1/1000/电影")
	if !provider.Changed(old, next) {
		t.Error("path change not reported")
	}

	renamed := validFolderEntry("/vol1/1000/照片")
	renamed.DisplayName = "新名字"
	if !provider.Changed(old, renamed) {
		t.Error("displayName change not reported")
	}

	stopped := validFolderEntry("/vol1/1000/照片")
	stopped.DesiredState = model.StateStopped
	if provider.Changed(old, stopped) {
		t.Error("desiredState change must not count as config change")
	}
}
