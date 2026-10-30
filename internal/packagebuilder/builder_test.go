package packagebuilder_test

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/chens-hi/shortcuts/internal/icons"
	"github.com/chens-hi/shortcuts/internal/model"
	"github.com/chens-hi/shortcuts/internal/packagebuilder"
	"github.com/chens-hi/shortcuts/internal/provider"
)

// The template tree is part of the fnOS package source, so the test reads it
// from the repo layout rather than fabricating a fixture.
const repoTemplateDir = "../../shortcuts/app/templates"

func TestBuildProducesInstallableChildPackage(t *testing.T) {
	entry := model.NewEntry(model.EntryTypeExternalURL, "moviepilot", "电影天堂",
		[]byte(`{"url":"http://127.0.0.1:13000","openMode":"iframe"}`))
	entry.Description = "line one\nline two"

	spec, err := provider.ExternalURLProvider{}.Spec(entry, "shortcuts")
	if err != nil {
		t.Fatalf("Spec: %v", err)
	}

	b := &packagebuilder.Builder{
		BuildDir:    t.TempDir(),
		TemplateDir: repoTemplateDir,
		Icons:       &icons.Service{Dir: t.TempDir()},
		MainAppname: "shortcuts",
	}
	result, err := b.Build(entry, spec)
	if err != nil {
		t.Fatalf("Build: %v", err)
	}

	if result.AppName != "shortcuts-moviepilot" {
		t.Fatalf("Result.AppName = %q, want shortcuts-moviepilot", result.AppName)
	}
	if result.Dir == "" {
		t.Fatal("Result.Dir is empty")
	}

	m := manifest(t, result.Dir)
	for key, want := range map[string]string{
		"appname":                    "shortcuts-moviepilot",
		"version":                    "1.0.0",
		"display_name":               "电影天堂",
		"desc":                       "line one line two",
		"platform":                   "all",
		"source":                     "thirdparty",
		"maintainer":                 "chens-hi",
		"maintainer_url":             "https://github.com/chens-hi",
		"distributor":                "shortcuts",
		"distributor_url":            "https://github.com/chens-hi/shortcuts",
		"desktop_uidir":              "ui",
		"os_min_version":             "1.2.0",
		"desktop_applaunchname":      "shortcuts-moviepilot",
		"ctl_stop":                   "true",
		"checkport":                  "false",
		"disable_authorization_path": "true",
		"install_type":               "root",
	} {
		if got := m[key]; got != want {
			t.Errorf("manifest[%q] = %q, want %q", key, got, want)
		}
	}
	if len(m) != 17 {
		t.Errorf("manifest has %d fields, want 17: %v", len(m), m)
	}
	// A multi-line description must not break the manifest into two lines.
	if !strings.Contains(m["desc"], "line one line two") {
		t.Errorf("manifest desc = %q, want the description flattened", m["desc"])
	}

	uiConfig, err := os.ReadFile(filepath.Join(result.Dir, "app", "ui", "config"))
	if err != nil {
		t.Fatalf("read app/ui/config: %v", err)
	}
	var parsed map[string]any
	if err := json.Unmarshal(uiConfig, &parsed); err != nil {
		t.Fatalf("app/ui/config is not valid JSON: %v\n%s", err, uiConfig)
	}
	urlBlock, ok := parsed[".url"].(map[string]any)
	if !ok {
		t.Fatalf("app/ui/config has no .url block:\n%s", uiConfig)
	}
	item, ok := urlBlock["shortcuts-moviepilot"].(map[string]any)
	if !ok {
		t.Fatalf(".url is not keyed by the child appname:\n%s", uiConfig)
	}
	if item["type"] != "iframe" {
		t.Errorf("ui config type = %v, want iframe", item["type"])
	}
	if item["url"] != "/cgi/ThirdParty/shortcuts-moviepilot/index.cgi" {
		t.Errorf("ui config url = %v, want the matching CGI path", item["url"])
	}

	cgi, err := os.ReadFile(filepath.Join(result.Dir, "app", "ui", "index.cgi"))
	if err != nil {
		t.Fatalf("read index.cgi: %v", err)
	}
	for _, want := range []string{"Status: 302 Found", "Location:", "http://127.0.0.1:13000"} {
		if !strings.Contains(string(cgi), want) {
			t.Errorf("index.cgi missing %q", want)
		}
	}

	for _, name := range []string{"install_callback", "config_callback", "upgrade_callback"} {
		callback, err := os.ReadFile(filepath.Join(result.Dir, "cmd", name))
		if err != nil {
			t.Fatalf("read cmd/%s: %v", name, err)
		}
		for _, want := range []string{"${TRIM_APPDEST}/app/ui/config", "JSON", "${TRIM_APPDEST}/app/ui/index.cgi"} {
			if !strings.Contains(string(callback), want) {
				t.Errorf("cmd/%s missing %q", name, want)
			}
		}
	}

	for _, rel := range []string{
		"ICON.PNG",
		"ICON_256.PNG",
		filepath.Join("app", "ui", "images", "icon_64.png"),
		filepath.Join("app", "ui", "images", "icon_256.png"),
	} {
		if !fileExists(filepath.Join(result.Dir, rel)) {
			t.Errorf("expected packaged file %s", rel)
		}
	}
}

func TestBuildRemovesStaleOutput(t *testing.T) {
	entry := model.NewEntry(model.EntryTypeInternalPort, "baidu", "百度",
		[]byte(`{"port":"13000","protocol":"http","path":"/","openMode":"iframe"}`))
	spec, err := provider.InternalPortProvider{}.Spec(entry, "shortcuts")
	if err != nil {
		t.Fatalf("Spec: %v", err)
	}

	buildDir := t.TempDir()
	b := &packagebuilder.Builder{
		BuildDir:    buildDir,
		TemplateDir: repoTemplateDir,
		Icons:       &icons.Service{Dir: t.TempDir()},
		MainAppname: "shortcuts",
	}
	first, err := b.Build(entry, spec)
	if err != nil {
		t.Fatalf("first Build: %v", err)
	}
	if err := os.WriteFile(filepath.Join(first.Dir, "stale.txt"), []byte("stale"), 0644); err != nil {
		t.Fatalf("seed stale file: %v", err)
	}

	second, err := b.Build(entry, spec)
	if err != nil {
		t.Fatalf("second Build: %v", err)
	}
	if second.Dir != first.Dir {
		t.Fatalf("Build returned %q, want the same dir %q", second.Dir, first.Dir)
	}
	if fileExists(filepath.Join(first.Dir, "stale.txt")) {
		t.Error("Build did not remove the previous output")
	}
}

func TestBuildFailsWithoutIcons(t *testing.T) {
	entry := model.NewEntry(model.EntryTypeInternalPort, "baidu", "百度",
		[]byte(`{"port":"13000","protocol":"http","path":"/","openMode":"iframe"}`))
	spec, err := provider.InternalPortProvider{}.Spec(entry, "shortcuts")
	if err != nil {
		t.Fatalf("Spec: %v", err)
	}
	b := &packagebuilder.Builder{
		BuildDir:    t.TempDir(),
		TemplateDir: repoTemplateDir,
		Icons:       nil,
		MainAppname: "shortcuts",
	}
	if _, err := b.Build(entry, spec); err == nil {
		t.Error("Build = nil error with a nil icons service, want an error")
	}
}

// 两种子应用类型共用一个 manifest.tmpl：随条目变化的字段之外必须完全一致。
func TestBothEntryTypesRenderSameManifestFields(t *testing.T) {
	build := func(t *testing.T, entry model.Entry, spec provider.ApplicationSpec) map[string]string {
		t.Helper()
		b := &packagebuilder.Builder{
			BuildDir:    t.TempDir(),
			TemplateDir: repoTemplateDir,
			Icons:       &icons.Service{Dir: t.TempDir()},
			MainAppname: "shortcuts",
		}
		result, err := b.Build(entry, spec)
		if err != nil {
			t.Fatalf("Build: %v", err)
		}
		return manifest(t, result.Dir)
	}

	urlEntry := model.NewEntry(model.EntryTypeExternalURL, "moviepilot", "电影天堂",
		[]byte(`{"url":"http://127.0.0.1:13000","openMode":"iframe"}`))
	urlSpec, err := provider.ExternalURLProvider{}.Spec(urlEntry, "shortcuts")
	if err != nil {
		t.Fatalf("Spec external_url: %v", err)
	}
	portEntry := model.NewEntry(model.EntryTypeInternalPort, "baidu", "百度",
		[]byte(`{"port":"13000","protocol":"http","path":"/","openMode":"iframe"}`))
	portSpec, err := provider.InternalPortProvider{}.Spec(portEntry, "shortcuts")
	if err != nil {
		t.Fatalf("Spec internal_port: %v", err)
	}

	urlManifest := build(t, urlEntry, urlSpec)
	portManifest := build(t, portEntry, portSpec)

	developerFields := map[string]string{
		"platform":        "all",
		"maintainer":      "chens-hi",
		"maintainer_url":  "https://github.com/chens-hi",
		"distributor":     "shortcuts",
		"distributor_url": "https://github.com/chens-hi/shortcuts",
	}
	for key, want := range developerFields {
		if got := urlManifest[key]; got != want {
			t.Errorf("external_url manifest[%q] = %q, want %q", key, got, want)
		}
		if got := portManifest[key]; got != want {
			t.Errorf("internal_port manifest[%q] = %q, want %q", key, got, want)
		}
	}

	// 只有随条目本身变化的字段允许不同。
	perEntry := map[string]bool{
		"appname": true, "version": true, "display_name": true,
		"desc": true, "desktop_applaunchname": true,
	}
	for key, urlValue := range urlManifest {
		if perEntry[key] {
			continue
		}
		if urlValue != portManifest[key] {
			t.Errorf("manifest[%q] differs between types: %q vs %q", key, urlValue, portManifest[key])
		}
	}
}

func TestBuildRejectsEscapingExtraFilePaths(t *testing.T) {
	entry := model.NewEntry(model.EntryTypeExternalURL, "moviepilot", "电影天堂",
		[]byte(`{"url":"http://127.0.0.1:13000","openMode":"iframe"}`))
	spec, err := provider.ExternalURLProvider{}.Spec(entry, "shortcuts")
	if err != nil {
		t.Fatalf("Spec: %v", err)
	}
	spec.ExtraFiles = map[string]string{"../escape.txt": "nope"}

	b := &packagebuilder.Builder{
		BuildDir:    t.TempDir(),
		TemplateDir: repoTemplateDir,
		Icons:       &icons.Service{Dir: t.TempDir()},
		MainAppname: "shortcuts",
	}
	if _, err := b.Build(entry, spec); err == nil {
		t.Fatal("Build = nil error for an escaping extra file path, want an error")
	}
}

func TestBuildFailsWhenManifestTemplateMissing(t *testing.T) {
	templateDir := t.TempDir()
	// copyTemplates 只要能遍历到类型目录即可通过，这里故意不放 manifest.tmpl。
	if err := os.MkdirAll(filepath.Join(templateDir, "external-url"), 0755); err != nil {
		t.Fatalf("setup template dir: %v", err)
	}

	entry := model.NewEntry(model.EntryTypeExternalURL, "moviepilot", "电影天堂",
		[]byte(`{"url":"http://127.0.0.1:13000","openMode":"iframe"}`))
	spec, err := provider.ExternalURLProvider{}.Spec(entry, "shortcuts")
	if err != nil {
		t.Fatalf("Spec: %v", err)
	}
	b := &packagebuilder.Builder{
		BuildDir:    t.TempDir(),
		TemplateDir: templateDir,
		Icons:       &icons.Service{Dir: t.TempDir()},
		MainAppname: "shortcuts",
	}
	if _, err := b.Build(entry, spec); err == nil {
		t.Fatal("Build = nil error without manifest.tmpl, want an error")
	} else if !strings.Contains(err.Error(), "manifest.tmpl") {
		t.Errorf("error = %v, want it to mention manifest.tmpl", err)
	}
}

func manifest(t *testing.T, dir string) map[string]string {
	t.Helper()
	raw, err := os.ReadFile(filepath.Join(dir, "manifest"))
	if err != nil {
		t.Fatalf("read manifest: %v", err)
	}
	out := map[string]string{}
	for _, line := range strings.Split(string(raw), "\n") {
		line = strings.TrimSpace(line)
		if !strings.Contains(line, "=") {
			continue
		}
		parts := strings.SplitN(line, "=", 2)
		out[strings.TrimSpace(parts[0])] = strings.TrimSpace(parts[1])
	}
	return out
}

func fileExists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}
