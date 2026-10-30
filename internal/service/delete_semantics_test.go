package service

import (
	"encoding/json"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/chens-hi/shortcuts/internal/config"
	"github.com/chens-hi/shortcuts/internal/deployer"
	"github.com/chens-hi/shortcuts/internal/icons"
	"github.com/chens-hi/shortcuts/internal/logger"
	"github.com/chens-hi/shortcuts/internal/model"
	"github.com/chens-hi/shortcuts/internal/provider"
	"github.com/chens-hi/shortcuts/internal/repository"
)

// recordingAppCenter 记录调用顺序，支持注入失败结果、阻塞卸载，
// 用来验证 DeleteEntry 的落盘与卸载顺序，以及卸载失败时的兜底行为。
type recordingAppCenter struct {
	mu             sync.Mutex
	installed      map[string]deployer.InstalledApp
	calls          []string
	stop           deployer.CommandResult
	uninstall      deployer.CommandResult
	blockUninstall chan struct{} // 非 nil 时 Uninstall 阻塞到关闭
}

func newRecordingAppCenter(children ...string) *recordingAppCenter {
	installed := map[string]deployer.InstalledApp{}
	for _, child := range children {
		installed[child] = deployer.InstalledApp{AppName: child}
	}
	return &recordingAppCenter{
		installed: installed,
		stop:      deployer.CommandResult{OK: true},
		uninstall: deployer.CommandResult{OK: true},
	}
}

func (c *recordingAppCenter) call(name string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.calls = append(c.calls, name)
}

func (c *recordingAppCenter) Calls() []string {
	c.mu.Lock()
	defer c.mu.Unlock()
	return append([]string{}, c.calls...)
}

func (c *recordingAppCenter) List() (map[string]deployer.InstalledApp, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	out := map[string]deployer.InstalledApp{}
	for k, v := range c.installed {
		out[k] = v
	}
	return out, nil
}

func (c *recordingAppCenter) Start(appName string) deployer.CommandResult {
	c.call("start:" + appName)
	return deployer.CommandResult{OK: true}
}

func (c *recordingAppCenter) Stop(appName string) deployer.CommandResult {
	c.call("stop:" + appName)
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.stop
}

func (c *recordingAppCenter) Uninstall(appName string) deployer.CommandResult {
	c.call("uninstall:" + appName)
	if c.blockUninstall != nil {
		<-c.blockUninstall
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	if !c.uninstall.OK {
		return c.uninstall
	}
	delete(c.installed, appName)
	return c.uninstall
}

func (c *recordingAppCenter) InstallLocal(dir string) deployer.CommandResult {
	c.call("install:" + filepath.Base(dir))
	return deployer.CommandResult{}
}
func (c *recordingAppCenter) Ping() (bool, deployer.CommandResult) {
	return true, deployer.CommandResult{}
}
func (c *recordingAppCenter) StatDir(dir string) deployer.CommandResult {
	c.call("stat-dir:" + dir)
	// 模拟 root agent 的真实 stat：路径存在且是目录才算通过。现有用例不调用
	// StatDir（folder 保存校验是新增行为），所以改成真实 stat 不会影响它们。
	info, err := os.Stat(dir)
	if err != nil || !info.IsDir() {
		return deployer.CommandResult{OK: false, Error: "path does not exist or is not a directory"}
	}
	return deployer.CommandResult{OK: true, Stdout: dir}
}

func containsCall(calls []string, want string) bool {
	for _, c := range calls {
		if c == want {
			return true
		}
	}
	return false
}

func indexOfCall(calls []string, want string) int {
	for i, c := range calls {
		if c == want {
			return i
		}
	}
	return -1
}

func entrySeed(appname, port string) map[string]any {
	return map[string]any{
		"type":         "internal_port",
		"appname":      appname,
		"displayName":  appname,
		"desiredState": "running",
		"version":      "1.0.0",
		"config": map[string]string{
			"port":     port,
			"protocol": "http",
			"path":     "/",
			"openMode": "iframe",
		},
	}
}

func newDeleteTestService(t *testing.T, app *recordingAppCenter, seed []map[string]any) (*Service, string) {
	t.Helper()
	dir := t.TempDir()
	repoPath := filepath.Join(dir, "shortcuts.json")
	raw, err := json.MarshalIndent(map[string]any{"version": 1, "entries": seed}, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(repoPath, raw, 0644); err != nil {
		t.Fatal(err)
	}
	iconDir := filepath.Join(dir, "icons")
	if err := os.MkdirAll(iconDir, 0755); err != nil {
		t.Fatal(err)
	}
	registry := provider.NewRegistry()
	registry.Register(provider.InternalPortProvider{})
	registry.Register(provider.ExternalURLProvider{})
	registry.Register(provider.FolderProvider{})
	return &Service{
		Config: config.Config{
			MainAppname: "shortcuts",
			IconDir:     iconDir,
			BuildDir:    filepath.Join(dir, "build"),
		},
		Repo:              repository.NewJSONRepository(repoPath),
		Providers:         registry,
		Icons:             &icons.Service{Dir: iconDir},
		Deployer:          app,
		Log:               logger.New(),
		PollDelay:         time.Millisecond,
		InstallAttempts:   2,
		UninstallAttempts: 3,
		childBusy:         map[string]bool{},
	}, repoPath
}

func loadAppNames(t *testing.T, svc *Service) []string {
	t.Helper()
	entries, err := svc.Repo.Load()
	if err != nil {
		t.Fatal(err)
	}
	names := make([]string, 0, len(entries))
	for _, e := range entries {
		names = append(names, e.AppName)
	}
	return names
}

func TestDeleteEntryRemovesEntryFromDataFile(t *testing.T) {
	app := newRecordingAppCenter("shortcuts-bbb")
	svc, _ := newDeleteTestService(t, app, []map[string]any{
		entrySeed("aaa", "8001"), entrySeed("bbb", "8002"), entrySeed("ccc", "8003"),
	})

	result, err := svc.DeleteEntry("bbb")
	if err != nil {
		t.Fatalf("DeleteEntry failed: %v", err)
	}
	if result.Child != "shortcuts-bbb" {
		t.Fatalf("Child = %q, want shortcuts-bbb", result.Child)
	}
	if result.StillInstalled {
		t.Fatal("StillInstalled = true, want false")
	}
	if result.UninstallDetail != "" {
		t.Fatalf("UninstallDetail = %q, want empty", result.UninstallDetail)
	}
	got := make([]string, 0, len(result.Entries))
	for _, e := range result.Entries {
		got = append(got, e.AppName)
	}
	if len(got) != 2 || got[0] != "aaa" || got[1] != "ccc" {
		t.Fatalf("remaining = %v, want [aaa ccc]", got)
	}
	if persisted := loadAppNames(t, svc); len(persisted) != 2 || persisted[0] != "aaa" || persisted[1] != "ccc" {
		t.Fatalf("persisted = %v, want [aaa ccc]", persisted)
	}
	if !containsCall(app.Calls(), "uninstall:shortcuts-bbb") {
		t.Fatalf("child app was not uninstalled; calls=%v", app.Calls())
	}
}

func TestDeleteEntrySavesBeforeUninstall(t *testing.T) {
	app := newRecordingAppCenter("shortcuts-bbb")
	app.blockUninstall = make(chan struct{})
	svc, repoPath := newDeleteTestService(t, app, []map[string]any{
		entrySeed("aaa", "8001"), entrySeed("bbb", "8002"),
	})

	done := make(chan error, 1)
	go func() {
		_, err := svc.DeleteEntry("bbb")
		done <- err
	}()

	deadline := time.Now().Add(5 * time.Second)
	for !containsCall(app.Calls(), "uninstall:shortcuts-bbb") {
		if time.Now().After(deadline) {
			t.Fatalf("Uninstall was never called; calls=%v", app.Calls())
		}
		time.Sleep(5 * time.Millisecond)
	}

	// 卸载正被阻塞：配置此时必须已经落盘，不能被卸载进度拖住。
	if persisted := loadAppNames(t, svc); containsName(persisted, "bbb") {
		t.Fatalf("bbb still in data file while uninstall is blocked; entries=%v", persisted)
	}
	if raw, err := os.ReadFile(repoPath); err != nil {
		t.Fatal(err)
	} else if len(raw) == 0 {
		t.Fatal("data file is empty while uninstall is blocked")
	}

	close(app.blockUninstall)
	if err := <-done; err != nil {
		t.Fatalf("DeleteEntry failed after uninstall unblocked: %v", err)
	}
}

func TestDeleteEntrySucceedsWhenChildUninstallFails(t *testing.T) {
	app := newRecordingAppCenter("shortcuts-bbb")
	app.uninstall = deployer.CommandResult{OK: false, Stdout: "uninstall: boom"}
	svc, _ := newDeleteTestService(t, app, []map[string]any{
		entrySeed("aaa", "8001"), entrySeed("bbb", "8002"),
	})

	result, err := svc.DeleteEntry("bbb")
	if err != nil {
		t.Fatalf("DeleteEntry must not fail when uninstall fails, got %v", err)
	}
	if !result.StillInstalled {
		t.Fatal("StillInstalled = false, want true")
	}
	if result.UninstallDetail == "" {
		t.Fatal("UninstallDetail is empty, want uninstall output")
	}
	if persisted := loadAppNames(t, svc); containsName(persisted, "bbb") {
		t.Fatalf("bbb survived in data file; entries=%v", persisted)
	}
}

func TestDeleteEntryStillAttemptsUninstallWhenStopFails(t *testing.T) {
	app := newRecordingAppCenter("shortcuts-bbb")
	app.stop = deployer.CommandResult{OK: false, Stdout: "stop: refused"}
	svc, _ := newDeleteTestService(t, app, []map[string]any{
		entrySeed("aaa", "8001"), entrySeed("bbb", "8002"),
	})

	result, err := svc.DeleteEntry("bbb")
	if err != nil {
		t.Fatalf("DeleteEntry failed: %v", err)
	}
	if result.StillInstalled {
		t.Fatal("StillInstalled = true, want false")
	}
	calls := app.Calls()
	stopIdx := indexOfCall(calls, "stop:shortcuts-bbb")
	uninstallIdx := indexOfCall(calls, "uninstall:shortcuts-bbb")
	if stopIdx < 0 || uninstallIdx < 0 {
		t.Fatalf("expected stop and uninstall to be attempted; calls=%v", calls)
	}
	if stopIdx > uninstallIdx {
		t.Fatalf("stop (%d) must precede uninstall (%d); calls=%v", stopIdx, uninstallIdx, calls)
	}
}

func TestDeleteEntryUnknownAppNameKeepsDataFile(t *testing.T) {
	app := newRecordingAppCenter()
	svc, repoPath := newDeleteTestService(t, app, []map[string]any{
		entrySeed("aaa", "8001"), entrySeed("bbb", "8002"),
	})
	before, err := os.ReadFile(repoPath)
	if err != nil {
		t.Fatal(err)
	}

	_, err = svc.DeleteEntry("missing")
	if err == nil {
		t.Fatal("DeleteEntry succeeded for an unknown appname")
	}
	after, err := os.ReadFile(repoPath)
	if err != nil {
		t.Fatal(err)
	}
	if string(before) != string(after) {
		t.Fatal("data file changed for an unknown appname")
	}
	if len(app.Calls()) != 0 {
		t.Fatalf("deployer was called for an unknown appname; calls=%v", app.Calls())
	}
	if persisted := loadAppNames(t, svc); len(persisted) != 2 {
		t.Fatalf("persisted = %v, want 2 entries", persisted)
	}
}

func TestConcurrentDeleteAndRuntimeUpdateDoNotLoseData(t *testing.T) {
	names := []string{"a1", "a2", "a3", "a4", "a5"}
	seed := []map[string]any{}
	for i, name := range names {
		seed = append(seed, entrySeed(name, "800"+string(rune('1'+i))))
	}
	// 没有任何已安装子应用，删除不会触发卸载命令。
	svc, _ := newDeleteTestService(t, newRecordingAppCenter(), seed)

	var wg sync.WaitGroup
	for i := 0; i < 50; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			name := names[i%len(names)]
			if i%2 == 0 {
				_, _ = svc.DeleteEntry(name)
			} else {
				_, _ = svc.SetRuntime(name, "stop")
			}
		}(i)
	}
	wg.Wait()

	entries, err := svc.Repo.Load()
	if err != nil {
		t.Fatalf("data file is unreadable after concurrent writes: %v", err)
	}
	known := map[string]bool{}
	for _, name := range names {
		known[name] = true
	}
	seen := map[string]bool{}
	for _, e := range entries {
		if !known[e.AppName] {
			t.Fatalf("unexpected entry %q appeared", e.AppName)
		}
		if seen[e.AppName] {
			t.Fatalf("entry %q is duplicated", e.AppName)
		}
		seen[e.AppName] = true
		if e.DesiredState != model.StateRunning && e.DesiredState != model.StateStopped {
			t.Fatalf("entry %q has DesiredState %q", e.AppName, e.DesiredState)
		}
	}
}

func containsName(names []string, want string) bool {
	for _, name := range names {
		if name == want {
			return true
		}
	}
	return false
}
