package agent

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/chens-hi/shortcuts/internal/config"
	"github.com/chens-hi/shortcuts/internal/deployer"
	"github.com/chens-hi/shortcuts/internal/logger"
)

// makeServer builds a Server with a temp BuildDir and a known MainAppname
// so tests can exercise the real resolveInstallDir path logic.
func makeServer(t *testing.T) (*Server, string) {
	t.Helper()
	base := t.TempDir()
	cfg := config.Config{
		MainAppname: "shortcuts",
		BuildDir:    base,
	}
	return &Server{cfg: cfg, log: logger.New(), sem: make(chan struct{}, maxConcurrentCLI)}, base
}

func TestResolveInstallDirValid(t *testing.T) {
	s, base := makeServer(t)
	// Create the directory so it exists and EvalSymlinks succeeds.
	dir := filepath.Join(base, "shortcuts-testapp")
	if err := os.MkdirAll(dir, 0755); err != nil {
		t.Fatal(err)
	}

	resolved, ok := s.resolveInstallDir(dir)
	if !ok {
		t.Fatalf("resolveInstallDir(%q) = ok=false", dir)
	}
	if resolved != dir {
		t.Fatalf("resolveInstallDir = %q, want %q", resolved, dir)
	}
}

func TestResolveInstallDirRejectsTraversal(t *testing.T) {
	s, base := makeServer(t)
	// Create the base directory but pass a path that walks out of it.
	if err := os.MkdirAll(base, 0755); err != nil {
		t.Fatal(err)
	}

	dir := filepath.Join(base, "..", "shortcuts-testapp")
	_, ok := s.resolveInstallDir(dir)
	if ok {
		t.Fatalf("resolveInstallDir(%q) accepted a traversal path", dir)
	}
}

func TestResolveInstallDirRejectsNonChildName(t *testing.T) {
	s, base := makeServer(t)
	dir := filepath.Join(base, "otherapp-testapp")
	if err := os.MkdirAll(dir, 0755); err != nil {
		t.Fatal(err)
	}

	_, ok := s.resolveInstallDir(dir)
	if ok {
		t.Fatalf("resolveInstallDir accepted a non-child appname %q", filepath.Base(dir))
	}
}

func TestResolveInstallDirRejectsOutsideDir(t *testing.T) {
	s, _ := makeServer(t)
	dir := "/tmp/shortcuts-testapp"
	_, ok := s.resolveInstallDir(dir)
	if ok {
		t.Fatalf("resolveInstallDir accepted a path outside BuildDir")
	}
}

func TestResolveInstallDirNonExistentIsOK(t *testing.T) {
	s, base := makeServer(t)
	// The directory doesn't exist yet; EvalSymlinks fails with NotExist,
	// which is allowed (the directory gets created by the caller).
	dir := filepath.Join(base, "shortcuts-newapp")
	resolved, ok := s.resolveInstallDir(dir)
	if !ok {
		t.Fatalf("resolveInstallDir(%q) = ok=false, want ok", dir)
	}
	if resolved != dir {
		t.Fatalf("resolveInstallDir = %q, want %q", resolved, dir)
	}
}

func TestResolveInstallDirRejectsSymlink(t *testing.T) {
	s, base := makeServer(t)
	if err := os.MkdirAll(base, 0755); err != nil {
		t.Fatal(err)
	}
	// Create a real directory outside base.
	outside := filepath.Join(t.TempDir(), "shortcuts-testapp")
	if err := os.MkdirAll(outside, 0755); err != nil {
		t.Fatal(err)
	}
	// Put a symlink inside base pointing to it.
	link := filepath.Join(base, "shortcuts-testapp")
	if err := os.Symlink(outside, link); err != nil {
		t.Skip("symlink not supported on this platform")
	}
	_, ok := s.resolveInstallDir(link)
	if ok {
		t.Fatal("resolveInstallDir accepted a symlink pointing outside BuildDir")
	}
}

func TestIsChildAppNameRootBoundary(t *testing.T) {
	tests := []struct {
		name, appName string
		want          bool
	}{
		{"valid", "shortcuts-testapp", true},
		{"valid with hyphens", "shortcuts-my-app", true},
		{"empty", "", false},
		{"no dash", "shortcuts", false},
		{"wrong prefix", "otherapp-testapp", false},
		{"starts with dash", "shortcuts-abc", true},
		{"uppercase", "shortcuts-TestApp", false},
		{"too long", "shortcuts-" + "a", true},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := deployer.IsChildAppName("shortcuts", tc.appName)
			if got != tc.want {
				t.Errorf("IsChildAppName(%q) = %v, want %v", tc.appName, got, tc.want)
			}
		})
	}
}

func TestResolveInstallDirEmptyMainAppname(t *testing.T) {
	s := &Server{cfg: config.Config{MainAppname: "", BuildDir: t.TempDir()}, sem: make(chan struct{}, maxConcurrentCLI)}
	dir := filepath.Join(s.cfg.BuildDir, "shortcuts-testapp")
	if err := os.MkdirAll(dir, 0755); err != nil {
		t.Fatal(err)
	}
	_, ok := s.resolveInstallDir(dir)
	if ok {
		t.Fatal("resolveInstallDir accepted with empty MainAppname")
	}
}

func TestStatDir(t *testing.T) {
	// 用 /tmp 前缀：Windows 下 Go 解析为当前盘符的 \tmp（其他测试已验证），
	// Linux 下即 /tmp，两边都能真实创建且通过 IsValidFolderPath 的 / 开头校验。
	real := "/tmp/shortcuts-agent-stat-" + t.Name()
	if err := os.MkdirAll(real, 0o755); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.RemoveAll(real) })
	for _, tc := range []struct {
		name string
		path string
		want bool
	}{
		{"real dir", real, true},
		{"nonexistent", filepath.Join(real, "不存在"), false},
		{"file is not a dir", filepath.Join(real, "file.txt"), false},
		{"root rejected", "/", false},
		{"relative rejected", "vol1/1000", false},
		{"empty rejected", "", false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if tc.name == "file is not a dir" {
				if err := os.WriteFile(tc.path, []byte("x"), 0644); err != nil {
					t.Fatal(err)
				}
			}
			got := statDir(tc.path)
			if got.OK != tc.want {
				t.Errorf("statDir(%q).OK = %v, want %v (err=%s)", tc.path, got.OK, tc.want, got.Error)
			}
		})
	}
}
