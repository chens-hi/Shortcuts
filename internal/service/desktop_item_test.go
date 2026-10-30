package service

import (
	"errors"
	"strings"
	"testing"

	"github.com/chens-hi/shortcuts/internal/deployer"
	"github.com/chens-hi/shortcuts/internal/model"
)

func folderSeed(appname, path string) map[string]any {
	return map[string]any{
		"type":         "folder",
		"appname":      appname,
		"displayName":  appname,
		"desiredState": "running",
		"version":      "1.0.0",
		"config": map[string]string{
			"path": path,
		},
	}
}

func TestDecorateFolderUsesDesktopEntryKey(t *testing.T) {
	apps := map[string]deployer.InstalledApp{}

	undeployed := decorate(model.Entry{Type: model.EntryTypeFolder, AppName: "photos"}, apps, "shortcuts")
	if undeployed.Status != model.StatusUndeployed {
		t.Fatalf("status = %q, want undeployed", undeployed.Status)
	}
	if undeployed.Runtime != model.RuntimeUnknown {
		t.Fatalf("runtime = %q, want unknown", undeployed.Runtime)
	}

	// 桌面项存在时就是已创建，即使 appcenter 里没有任何子应用。
	deployed := decorate(model.Entry{
		Type:            model.EntryTypeFolder,
		AppName:         "photos",
		DesktopEntryKey: "folder:57",
	}, apps, "shortcuts")
	if deployed.Status != model.StatusDeployed {
		t.Fatalf("status = %q, want deployed", deployed.Status)
	}
	if deployed.Runtime != model.RuntimeUnknown {
		t.Fatalf("runtime = %q, want unknown", deployed.Runtime)
	}
}

func TestSetDesktopEntryKey(t *testing.T) {
	svc, _ := newDeleteTestService(t, newRecordingAppCenter(), []map[string]any{
		folderSeed("photos", "/vol1/照片"),
	})

	entry, err := svc.SetDesktopEntryKey("photos", "folder:57")
	if err != nil {
		t.Fatalf("SetDesktopEntryKey: %v", err)
	}
	if entry.DesktopEntryKey != "folder:57" {
		t.Fatalf("DesktopEntryKey = %q, want folder:57", entry.DesktopEntryKey)
	}
	if entry.Status != model.StatusDeployed {
		t.Fatalf("status = %q, want deployed", entry.Status)
	}

	entries, err := svc.Repo.Load()
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 1 || entries[0].DesktopEntryKey != "folder:57" {
		t.Fatalf("persisted = %+v, want desktopEntryKey folder:57", entries)
	}

	cleared, err := svc.SetDesktopEntryKey("photos", "")
	if err != nil {
		t.Fatalf("clear: %v", err)
	}
	if cleared.DesktopEntryKey != "" || cleared.Status != model.StatusUndeployed {
		t.Fatalf("cleared = %+v", cleared)
	}

	for _, bad := range []string{"folder:", "folder:abc", "folder:1\n", "entry:57", "x", "folder:123456789012345678901"} {
		if _, err := svc.SetDesktopEntryKey("photos", bad); err == nil {
			t.Errorf("invalid key %q was accepted", bad)
		}
	}

	if _, err := svc.SetDesktopEntryKey("missing", "folder:1"); err == nil {
		t.Error("unknown entry was accepted")
	}

	portSvc, _ := newDeleteTestService(t, newRecordingAppCenter(), []map[string]any{
		entrySeed("aaa", "8001"),
	})
	if _, err := portSvc.SetDesktopEntryKey("aaa", "folder:1"); err == nil {
		t.Error("non-folder entry was accepted")
	}
}

func TestDeployEntryRejectsFolder(t *testing.T) {
	// 预置同名旧子应用，且条目记录的部署版本与当前版本一致：没有守卫时
	// DeployEntry 会先命中「已是最新版本」返回 DEPLOY_SKIPPED，而不是报错。
	app := newRecordingAppCenter("shortcuts-photos")
	seed := folderSeed("photos", "/vol1/照片")
	seed["deployedVersion"] = "1.0.0"
	svc, _ := newDeleteTestService(t, app, []map[string]any{seed})

	_, err := svc.DeployEntry("photos")
	var apiErr *model.APIError
	if !errors.As(err, &apiErr) || apiErr.Code != model.ErrValidation {
		t.Fatalf("DeployEntry err = %v, want %s", err, model.ErrValidation)
	}
	for _, call := range app.Calls() {
		if strings.HasPrefix(call, "install:") {
			t.Fatalf("folder deploy must not install anything; calls=%v", app.Calls())
		}
	}
}

func TestDeleteEntrySkipsUninstallForNewFolderEntry(t *testing.T) {
	// 预置同名旧子应用：若缺少「folder 且没有部署版本」的条件，删除会把它卸掉。
	// 未预置时 bestEffortUninstall 会在「未安装」处早退，测不出条件本身。
	app := newRecordingAppCenter("shortcuts-photos")
	svc, _ := newDeleteTestService(t, app, []map[string]any{
		folderSeed("photos", "/vol1/照片"),
	})
	result, err := svc.DeleteEntry("photos")
	if err != nil {
		t.Fatalf("DeleteEntry: %v", err)
	}
	for _, call := range app.Calls() {
		if call == "stop:shortcuts-photos" || call == "uninstall:shortcuts-photos" {
			t.Fatalf("new folder entry must not touch a child app; calls=%v", app.Calls())
		}
	}
	if result.Child != "" {
		t.Fatalf("Child = %q, want empty for a new folder entry", result.Child)
	}
}

func TestDeleteEntryUninstallsLegacyFolderChild(t *testing.T) {
	app := newRecordingAppCenter("shortcuts-photos")
	seed := folderSeed("photos", "/vol1/照片")
	seed["deployedVersion"] = "1.0.0"
	svc, _ := newDeleteTestService(t, app, []map[string]any{seed})
	if _, err := svc.DeleteEntry("photos"); err != nil {
		t.Fatalf("DeleteEntry: %v", err)
	}
	if !containsCall(app.Calls(), "uninstall:shortcuts-photos") {
		t.Fatalf("legacy child was not uninstalled; calls=%v", app.Calls())
	}
}

func TestUpdateEntryKeepsDesktopEntryKey(t *testing.T) {
	dir := realFolderPath(t, "photos")
	svc, _ := newDeleteTestService(t, newRecordingAppCenter(), []map[string]any{
		folderSeed("photos", dir),
	})
	if _, err := svc.SetDesktopEntryKey("photos", "folder:57"); err != nil {
		t.Fatalf("seed key: %v", err)
	}

	saveInput := EntryInput{
		AppName:     "photos",
		DisplayName: "照片库",
		Path:        dir,
		Config:      []byte(`{"path":"` + dir + `"}`),
	}
	saveInput.Provided = map[string]bool{"displayName": true, "path": true, "config": true}
	if _, err := svc.UpdateEntry("photos", saveInput); err != nil {
		t.Fatalf("UpdateEntry: %v", err)
	}

	entries, err := svc.Repo.Load()
	if err != nil {
		t.Fatal(err)
	}
	if entries[0].DesktopEntryKey != "folder:57" {
		t.Fatalf("DesktopEntryKey lost after update: %+v", entries[0])
	}
}
