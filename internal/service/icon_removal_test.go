package service

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/chens-hi/shortcuts/internal/config"
	"github.com/chens-hi/shortcuts/internal/deployer"
	"github.com/chens-hi/shortcuts/internal/icons"
	"github.com/chens-hi/shortcuts/internal/model"
	"github.com/chens-hi/shortcuts/internal/provider"
	"github.com/chens-hi/shortcuts/internal/repository"
)

type fakeAppCenter struct{}

func (fakeAppCenter) List() (map[string]deployer.InstalledApp, error) {
	return map[string]deployer.InstalledApp{}, nil
}
func (fakeAppCenter) InstallLocal(dir string) deployer.CommandResult {
	return deployer.CommandResult{}
}
func (fakeAppCenter) Start(appName string) deployer.CommandResult { return deployer.CommandResult{} }
func (fakeAppCenter) Stop(appName string) deployer.CommandResult  { return deployer.CommandResult{} }
func (fakeAppCenter) Uninstall(appName string) deployer.CommandResult {
	return deployer.CommandResult{}
}
func (fakeAppCenter) Ping() (bool, deployer.CommandResult) { return true, deployer.CommandResult{} }
func (fakeAppCenter) StatDir(dir string) deployer.CommandResult { return deployer.CommandResult{OK: true} }

const seededEntryJSON = `{
  "version": 1,
  "entries": [
    {
      "type": "internal_port",
      "appname": "testapp",
      "displayName": "Test",
      "displayIcon": "/icon/testapp",
      "description": "",
      "desiredState": "running",
      "version": "1.0.0",
      "config": {"port": "8080", "protocol": "http", "path": "/", "openMode": "iframe"}
    }
  ]
}`

func newIconTestService(t *testing.T) *Service {
	t.Helper()
	dir := t.TempDir()
	repoPath := filepath.Join(dir, "shortcuts.json")
	if err := os.WriteFile(repoPath, []byte(seededEntryJSON), 0644); err != nil {
		t.Fatal(err)
	}
	iconDir := filepath.Join(dir, "icons")
	if err := os.MkdirAll(iconDir, 0755); err != nil {
		t.Fatal(err)
	}
	registry := provider.NewRegistry()
	registry.Register(provider.InternalPortProvider{})
	registry.Register(provider.ExternalURLProvider{})
	return &Service{
		Config:    config.Config{MainAppname: "shortcuts", IconDir: iconDir},
		Repo:      repository.NewJSONRepository(repoPath),
		Providers: registry,
		Icons:     &icons.Service{Dir: iconDir},
		Deployer:  fakeAppCenter{},
		childBusy: map[string]bool{},
	}
}

func seedIconFiles(t *testing.T, svc *Service) {
	t.Helper()
	for _, name := range []string{"testapp-64.png", "testapp-256.png"} {
		if err := os.WriteFile(filepath.Join(svc.Icons.Dir, name), []byte("png"), 0644); err != nil {
			t.Fatal(err)
		}
	}
}

func iconFileExists(svc *Service, name string) bool {
	_, err := os.Stat(filepath.Join(svc.Icons.Dir, name))
	return err == nil
}

func updateInput(removeIcon bool) EntryInput {
	return EntryInput{
		AppName:     "testapp",
		DisplayName: "Test",
		Config:      json.RawMessage(`{"port": "8080", "protocol": "http", "path": "/", "openMode": "iframe"}`),
		RemoveIcon:  removeIcon,
	}
}

func TestUpdateEntryRemoveIconClearsDisplayIconAndFiles(t *testing.T) {
	svc := newIconTestService(t)
	seedIconFiles(t, svc)

	result, err := svc.UpdateEntry("testapp", updateInput(true))
	if err != nil {
		t.Fatalf("UpdateEntry failed: %v", err)
	}
	if result.Entry.DisplayIcon != "" {
		t.Fatalf("DisplayIcon = %q, want empty after removal", result.Entry.DisplayIcon)
	}
	if result.Entry.Version != model.BumpVersion("1.0.0") {
		t.Fatalf("Version = %q, want %q after removal", result.Entry.Version, model.BumpVersion("1.0.0"))
	}
	if iconFileExists(svc, "testapp-64.png") || iconFileExists(svc, "testapp-256.png") {
		t.Fatal("icon files still exist after removal")
	}
	entries, err := svc.Repo.Load()
	if err != nil {
		t.Fatal(err)
	}
	if entries[0].DisplayIcon != "" {
		t.Fatalf("persisted DisplayIcon = %q, want empty", entries[0].DisplayIcon)
	}
}

func TestUpdateEntryRemoveIconIsIdempotentWithoutFiles(t *testing.T) {
	svc := newIconTestService(t)

	result, err := svc.UpdateEntry("testapp", updateInput(true))
	if err != nil {
		t.Fatalf("UpdateEntry failed without icon files: %v", err)
	}
	if result.Entry.DisplayIcon != "" {
		t.Fatalf("DisplayIcon = %q, want empty", result.Entry.DisplayIcon)
	}
}

func TestUpdateEntryWithoutIconFieldsKeepsSavedIcon(t *testing.T) {
	svc := newIconTestService(t)
	seedIconFiles(t, svc)

	result, err := svc.UpdateEntry("testapp", updateInput(false))
	if err != nil {
		t.Fatalf("UpdateEntry failed: %v", err)
	}
	if result.Entry.DisplayIcon != "/icon/testapp" {
		t.Fatalf("DisplayIcon = %q, want /icon/testapp", result.Entry.DisplayIcon)
	}
	if result.Entry.Version != "1.0.0" {
		t.Fatalf("Version = %q, want unchanged 1.0.0", result.Entry.Version)
	}
	if !iconFileExists(svc, "testapp-64.png") || !iconFileExists(svc, "testapp-256.png") {
		t.Fatal("icon files were removed by a save without icon fields")
	}
}
