package service

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/chens-hi/shortcuts/internal/config"
	"github.com/chens-hi/shortcuts/internal/icons"
	"github.com/chens-hi/shortcuts/internal/model"
	"github.com/chens-hi/shortcuts/internal/provider"
	"github.com/chens-hi/shortcuts/internal/repository"
)

const internalPortSeed = `{
  "version": 1,
  "entries": [
    {
      "type": "internal_port",
      "appname": "testapp",
      "displayName": "Test",
      "description": "旧描述",
      "desiredState": "stopped",
      "version": "1.0.0",
      "config": {"port": "8080", "protocol": "http", "path": "/", "openMode": "iframe"}
    }
  ]
}`

const externalURLSeed = `{
  "version": 1,
  "entries": [
    {
      "type": "external_url",
      "appname": "baidu",
      "displayName": "Baidu",
      "description": "desc",
      "desiredState": "running",
      "version": "1.0.0",
      "config": {"url": "https://www.baidu.com", "openMode": "url"}
    }
  ]
}`

const samePortConfig = `{"port": "8080", "protocol": "http", "path": "/", "openMode": "iframe"}`

// newMergeTestService 构造一个只含单条条目的 Service，
// 用于验证更新时的字段合并语义。返回数据文件路径以便断言未被改写。
func newMergeTestService(t *testing.T, seed string) (*Service, string) {
	t.Helper()
	dir := t.TempDir()
	repoPath := filepath.Join(dir, "shortcuts.json")
	if err := os.WriteFile(repoPath, []byte(seed), 0644); err != nil {
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
		Config:    config.Config{MainAppname: "shortcuts", IconDir: iconDir},
		Repo:      repository.NewJSONRepository(repoPath),
		Providers: registry,
		Icons:     &icons.Service{Dir: iconDir},
		Deployer:  fakeAppCenter{},
		childBusy: map[string]bool{},
	}, repoPath
}

// 未提供 description 与 desiredState 的部分更新必须保留旧值：
// 不允许"只改端口"这类操作顺带把描述清空、把已停止的条目改回运行中。
func TestUpdateEntryKeepsOmittedFields(t *testing.T) {
	svc, _ := newMergeTestService(t, internalPortSeed)

	result, err := svc.UpdateEntry("testapp", EntryInput{
		AppName:     "testapp",
		DisplayName: "Test",
		Config:      json.RawMessage(samePortConfig),
	})
	if err != nil {
		t.Fatalf("UpdateEntry failed: %v", err)
	}
	if result.Entry.Description != "旧描述" {
		t.Fatalf("Description = %q, want 旧描述 preserved", result.Entry.Description)
	}
	if result.Entry.DesiredState != model.StateStopped {
		t.Fatalf("DesiredState = %q, want stopped preserved", result.Entry.DesiredState)
	}
	// 没有任何变更：版本号不应被白白提升。
	if result.Changed || result.Entry.Version != "1.0.0" {
		t.Fatalf("Changed=%v Version=%q, want unchanged 1.0.0", result.Changed, result.Entry.Version)
	}
}

// 显式传空的 description 表示"清空"，必须生效并触发版本号提升。
func TestUpdateEntryClearsDescriptionWhenExplicitlyEmpty(t *testing.T) {
	svc, _ := newMergeTestService(t, internalPortSeed)

	result, err := svc.UpdateEntry("testapp", EntryInput{
		AppName:     "testapp",
		DisplayName: "Test",
		Description: "",
		Provided:    map[string]bool{"description": true},
		Config:      json.RawMessage(samePortConfig),
	})
	if err != nil {
		t.Fatalf("UpdateEntry failed: %v", err)
	}
	if result.Entry.Description != "" {
		t.Fatalf("Description = %q, want empty", result.Entry.Description)
	}
	if result.Entry.Version != model.BumpVersion("1.0.0") {
		t.Fatalf("Version = %q, want bumped after description change", result.Entry.Version)
	}
}

// 显式提供的 desiredState 必须被采用（而不是被当成空值丢弃）。
func TestUpdateEntryHonorsExplicitDesiredState(t *testing.T) {
	svc, _ := newMergeTestService(t, internalPortSeed)

	result, err := svc.UpdateEntry("testapp", EntryInput{
		AppName:      "testapp",
		DisplayName:  "Test",
		DesiredState: model.StateRunning,
		Provided:     map[string]bool{"desiredState": true},
		Config:       json.RawMessage(samePortConfig),
	})
	if err != nil {
		t.Fatalf("UpdateEntry failed: %v", err)
	}
	if result.Entry.DesiredState != model.StateRunning {
		t.Fatalf("DesiredState = %q, want running", result.Entry.DesiredState)
	}
}

// 只改端口（不带完整 config JSON）时，本次改动必须生效，
// 同时未提供的 protocol/path/openMode 必须保留，不能被重建时丢掉。
func TestUpdateEntryPartialPortMergesIntoExistingConfig(t *testing.T) {
	svc, _ := newMergeTestService(t, internalPortSeed)

	result, err := svc.UpdateEntry("testapp", EntryInput{
		AppName:     "testapp",
		DisplayName: "Test",
		Description: "旧描述",
		Port:        "9090",
		Provided:    map[string]bool{"description": true, "port": true},
	})
	if err != nil {
		t.Fatalf("UpdateEntry failed: %v", err)
	}
	cfg := model.InternalPortConfig{}
	if err := json.Unmarshal(result.Entry.Config, &cfg); err != nil {
		t.Fatalf("unmarshal config: %v", err)
	}
	if cfg.Port != "9090" {
		t.Fatalf("Port = %q, want 9090 applied", cfg.Port)
	}
	if cfg.Protocol != "http" || cfg.Path != "/" || cfg.OpenMode != "iframe" {
		t.Fatalf("unprovided config fields were lost: %#v", cfg)
	}
	if result.Entry.Version != model.BumpVersion("1.0.0") {
		t.Fatalf("Version = %q, want bumped after config change", result.Entry.Version)
	}
}

// 外部链接只改 openMode 时，url 必须保留。
func TestUpdateExternalURLPartialOpenModeKeepsURL(t *testing.T) {
	svc, _ := newMergeTestService(t, externalURLSeed)

	result, err := svc.UpdateEntry("baidu", EntryInput{
		AppName:     "baidu",
		DisplayName: "Baidu",
		Description: "desc",
		OpenMode:    "iframe",
		Provided:    map[string]bool{"description": true, "openMode": true},
	})
	if err != nil {
		t.Fatalf("UpdateEntry failed: %v", err)
	}
	cfg := model.ExternalURLConfig{}
	if err := json.Unmarshal(result.Entry.Config, &cfg); err != nil {
		t.Fatalf("unmarshal config: %v", err)
	}
	if cfg.URL != "https://www.baidu.com" {
		t.Fatalf("URL = %q, want https://www.baidu.com preserved", cfg.URL)
	}
	if cfg.OpenMode != "iframe" {
		t.Fatalf("OpenMode = %q, want iframe applied", cfg.OpenMode)
	}
}

// 数据文件结构不匹配时，UpdateEntry 必须在写盘之前中止，
// 用户数据不能因为一次"空列表"结果被覆盖成空。
func TestUpdateEntryAbortsAndKeepsDataOnSchemaMismatch(t *testing.T) {
	corrupt := `{"version":1,"entries":"not-a-list"}`
	svc, repoPath := newMergeTestService(t, corrupt)

	if _, err := svc.UpdateEntry("testapp", EntryInput{AppName: "testapp", DisplayName: "Test"}); err == nil {
		t.Fatal("expected error on schema mismatch, got nil")
	}
	got, err := os.ReadFile(repoPath)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != corrupt {
		t.Fatalf("data file was overwritten: %s", got)
	}
}

func folderSeedJSONWithPath(path string) string {
	return `{
  "version": 1,
  "entries": [
    {
      "type": "folder",
      "appname": "photos",
      "displayName": "照片",
      "description": "相册目录",
      "desiredState": "running",
      "version": "1.0.0",
      "config": {"path": "` + path + `"}
    }
  ]
}`
}

// folder 条目只改 path 的部分更新：新路径生效、旧字段保留、版本号提升。
func TestUpdateEntryFolderPathMergesIntoExistingConfig(t *testing.T) {
	oldPath := realFolderPath(t, "photos")
	newPath := realFolderPath(t, "photos-movies")
	svc, _ := newMergeTestService(t, folderSeedJSONWithPath(oldPath))

	result, err := svc.UpdateEntry("photos", EntryInput{
		AppName:     "photos",
		DisplayName: "照片",
		Path:        newPath,
		Provided:    map[string]bool{"path": true},
	})
	if err != nil {
		t.Fatalf("UpdateEntry failed: %v", err)
	}
	cfg := model.FolderConfig{}
	if err := json.Unmarshal(result.Entry.Config, &cfg); err != nil {
		t.Fatalf("unmarshal config: %v", err)
	}
	if cfg.Path != newPath {
		t.Fatalf("Path = %q, want %q", cfg.Path, newPath)
	}
	if result.Entry.Description != "相册目录" {
		t.Fatalf("Description = %q, want preserved", result.Entry.Description)
	}
	if !result.Changed || result.Entry.Version != "1.0.1" {
		t.Fatalf("Changed=%v Version=%q, want bumped to 1.0.1", result.Changed, result.Entry.Version)
	}
}

// folder 条目不提供任何配置字段时不得误清配置，版本号保持不变。
func TestUpdateEntryFolderKeepsConfigWhenNothingProvided(t *testing.T) {
	path := realFolderPath(t, "photos")
	svc, _ := newMergeTestService(t, folderSeedJSONWithPath(path))

	result, err := svc.UpdateEntry("photos", EntryInput{
		AppName:     "photos",
		DisplayName: "照片",
	})
	if err != nil {
		t.Fatalf("UpdateEntry failed: %v", err)
	}
	cfg := model.FolderConfig{}
	if err := json.Unmarshal(result.Entry.Config, &cfg); err != nil {
		t.Fatalf("unmarshal config: %v", err)
	}
	if cfg.Path != path {
		t.Fatalf("Path = %q, want %q", cfg.Path, path)
	}
	if result.Changed || result.Entry.Version != "1.0.0" {
		t.Fatalf("Changed=%v Version=%q, want unchanged 1.0.0", result.Changed, result.Entry.Version)
	}
}
