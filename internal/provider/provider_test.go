package provider

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/chens-hi/shortcuts/internal/model"
)

func validURLEntry(url, openMode string) model.Entry {
	return model.NewEntry(model.EntryTypeExternalURL, "moviepilot", "电影天堂",
		[]byte(`{"url":"`+url+`","openMode":"`+openMode+`"}`))
}

func validPortEntry(port, protocol, path, openMode string) model.Entry {
	return model.NewEntry(model.EntryTypeInternalPort, "moviepilot", "电影天堂",
		[]byte(`{"port":"`+port+`","protocol":"`+protocol+`","path":"`+path+`","openMode":"`+openMode+`"}`))
}

func TestExternalURLProviderValidate(t *testing.T) {
	provider := ExternalURLProvider{}

	for _, tc := range []struct {
		url      string
		openMode string
		wantErr  bool
	}{
		{url: "http://127.0.0.1:13000", openMode: model.OpenModeIframe, wantErr: false},
		{url: "https://www.baidu.com", openMode: model.OpenModeURL, wantErr: false},
		{url: "https://www.baidu.com", openMode: "", wantErr: false}, // defaults to url
		{url: "ftp://example.com", openMode: model.OpenModeURL, wantErr: true},
		{url: "not a url", openMode: model.OpenModeURL, wantErr: true},
		{url: "http://example.com", openMode: "bogus", wantErr: true},
	} {
		entry := validURLEntry(tc.url, tc.openMode)
		err := provider.Validate(entry)
		if tc.wantErr && err == nil {
			t.Errorf("Validate(url=%q openMode=%q) = nil, want error", tc.url, tc.openMode)
		}
		if !tc.wantErr && err != nil {
			t.Errorf("Validate(url=%q openMode=%q) = %v, want nil", tc.url, tc.openMode, err)
		}
	}
}

func TestExternalURLProviderRejectsInvalidEntry(t *testing.T) {
	provider := ExternalURLProvider{}
	entry := validURLEntry("http://127.0.0.1:13000", model.OpenModeURL)
	entry.DisplayName = ""

	if err := provider.Validate(entry); err == nil {
		t.Fatal("Validate = nil for an empty displayName, want error")
	}
}

func TestInternalPortProviderValidate(t *testing.T) {
	provider := InternalPortProvider{}

	for _, tc := range []struct {
		port     string
		protocol string
		path     string
		openMode string
		wantErr  bool
	}{
		{port: "13000", protocol: "http", path: "/app", openMode: model.OpenModeIframe, wantErr: false},
		{port: "1", protocol: "https", path: "/", openMode: model.OpenModeURL, wantErr: false},
		{port: "65535", protocol: "http", path: "/a/b/c", openMode: model.OpenModeIframe, wantErr: false},
		{port: "13000", protocol: "http", path: "?k=abc", openMode: model.OpenModeURL, wantErr: false},
		{port: "0", protocol: "http", path: "/", openMode: model.OpenModeURL, wantErr: true},
		{port: "65536", protocol: "http", path: "/", openMode: model.OpenModeURL, wantErr: true},
		{port: "13000", protocol: "ftp", path: "/", openMode: model.OpenModeURL, wantErr: true},
		{port: "13000", protocol: "http", path: "no-slash", openMode: model.OpenModeURL, wantErr: true},
		{port: "13000", protocol: "http", path: "/", openMode: "bogus", wantErr: true},
	} {
		err := provider.Validate(validPortEntry(tc.port, tc.protocol, tc.path, tc.openMode))
		if tc.wantErr && err == nil {
			t.Errorf("Validate(port=%s protocol=%s path=%s openMode=%s) = nil, want error", tc.port, tc.protocol, tc.path, tc.openMode)
		}
		if !tc.wantErr && err != nil {
			t.Errorf("Validate(port=%s protocol=%s path=%s openMode=%s) = %v, want nil", tc.port, tc.protocol, tc.path, tc.openMode, err)
		}
	}
}

func TestExternalURLProviderChanged(t *testing.T) {
	provider := ExternalURLProvider{}
	base := validURLEntry("http://127.0.0.1:13000", model.OpenModeIframe)

	if provider.Changed(base, base) {
		t.Error("Changed = true for an identical entry")
	}
	same := base
	same.Description = "  "
	if !provider.Changed(base, same) {
		t.Error("Changed = false for a different description")
	}
	same = base
	same.DisplayIcon = "a64.png"
	if !provider.Changed(base, same) {
		t.Error("Changed = false for a different displayIcon")
	}
	same = base
	same.DesiredState = model.StateStopped
	// 期望状态不算配置变化：子应用包内容与它无关，SetRuntime 改同一个字段
	// 也不 bump 版本。若算成变化，改一次启停会让下次部署整包重装。
	if provider.Changed(base, same) {
		t.Error("Changed = true for a different desiredState (should not bump)")
	}
	// 配置解不出来时按"有变化"处理，避免静默跳过重建。
	if !provider.Changed(base, model.Entry{Type: base.Type, AppName: base.AppName, DisplayName: base.DisplayName, Config: []byte("{not-json")}) {
		t.Error("Changed = false for an undecodable config")
	}
}

func TestSpecUIConfigKeyMatchesChildAppname(t *testing.T) {
	provider := ExternalURLProvider{}
	entry := validURLEntry("https://www.baidu.com", model.OpenModeURL)
	spec, err := provider.Spec(entry, "shortcuts")
	if err != nil {
		t.Fatalf("Spec: %v", err)
	}
	raw, err := json.Marshal(spec.UIConfig)
	if err != nil {
		t.Fatalf("marshal UIConfig: %v", err)
	}
	got := string(raw)
	if !strings.Contains(got, `"shortcuts-moviepilot"`) || !strings.Contains(got, `"/cgi/ThirdParty/shortcuts-moviepilot/index.cgi"`) {
		t.Fatalf("UIConfig does not key off the child appname and the matching CGI path:\n%s", got)
	}
}

func TestExternalURLSpecNeutralisesShellInjection(t *testing.T) {
	// index.cgi 是 /bin/bash 脚本，URL 直接进赋值语句。用 %q 生成的双引号字符串
	// 在 bash 里仍然会展开 $(...) 和反引号，等于把用户填的 URL 当命令执行。
	cases := []struct {
		url  string
		want string
	}{
		{"http://a/$(id)", `TARGET_URL='http://a/$(id)'`},
		{"http://a/b`id`", "TARGET_URL='http://a/b`id`'"},
		{"http://a/it's", `TARGET_URL='http://a/it'\''s'`},
	}
	for _, tc := range cases {
		spec, err := ExternalURLProvider{}.Spec(validURLEntry(tc.url, model.OpenModeURL), "shortcuts")
		if err != nil {
			t.Fatalf("Spec(%q): %v", tc.url, err)
		}
		if !strings.Contains(spec.CGIScript, tc.want) {
			t.Fatalf("CGIScript 未安全引用 %q，期望包含 %s，实际:\n%s", tc.url, tc.want, spec.CGIScript)
		}
		if strings.Contains(spec.CGIScript, `TARGET_URL="`) {
			t.Fatalf("CGIScript 仍在用双引号赋值，注入未修复:\n%s", spec.CGIScript)
		}
	}
}

func TestSpecUIConfigMarksEntryReadOnly(t *testing.T) {
	// 子应用必须对所有人可见（allUsers=true）但只读（accessPerm=readonly），
	// 这样非管理员可以点开快捷方式但无法在 fnOS 应用设置里修改配置。
	cases := []struct {
		name     string
		provider EntryProvider
		entry    model.Entry
	}{
		{name: "external_url", provider: ExternalURLProvider{}, entry: validURLEntry("https://www.baidu.com", model.OpenModeURL)},
		{name: "internal_port", provider: InternalPortProvider{}, entry: validPortEntry("13000", "http", "/app", model.OpenModeIframe)},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			spec, err := tc.provider.Spec(tc.entry, "shortcuts")
			if err != nil {
				t.Fatalf("Spec: %v", err)
			}
			raw, err := json.Marshal(spec.UIConfig)
			if err != nil {
				t.Fatalf("marshal UIConfig: %v", err)
			}
			got := string(raw)
			for _, want := range []string{`"allUsers":true`, `"control":{"accessPerm":"readonly"}`} {
				if !strings.Contains(got, want) {
					t.Errorf("UIConfig missing %s:\n%s", want, got)
				}
			}
		})
	}
}

func TestInternalPortSpecCarriesPortAndPath(t *testing.T) {
	provider := InternalPortProvider{}
	entry := validPortEntry("13000", "http", "/app", model.OpenModeIframe)
	spec, err := provider.Spec(entry, "shortcuts")
	if err != nil {
		t.Fatalf("Spec: %v", err)
	}
	raw, err := json.Marshal(spec.UIConfig)
	if err != nil {
		t.Fatalf("marshal UIConfig: %v", err)
	}
	got := string(raw)
	for _, want := range []string{`"port":"13000"`, `"protocol":"http"`, `"url":"/app"`, `"type":"iframe"`} {
		if !strings.Contains(got, want) {
			t.Fatalf("UIConfig missing %s:\n%s", want, got)
		}
	}
}

func TestInternalPortSpecPassesQueryPathThrough(t *testing.T) {
	// 纯查询串不补斜杠，原样进 .url 块；最终 URL 由 fnOS 拼接、浏览器把
	// 空路径按 / 处理，host:port?k=… 与 host:port/?k=… 等价。
	provider := InternalPortProvider{}
	entry := validPortEntry("13430", "http", "?k=abc", model.OpenModeURL)
	spec, err := provider.Spec(entry, "shortcuts")
	if err != nil {
		t.Fatalf("Spec: %v", err)
	}
	raw, err := json.Marshal(spec.UIConfig)
	if err != nil {
		t.Fatalf("marshal UIConfig: %v", err)
	}
	if got := string(raw); !strings.Contains(got, `"url":"?k=abc"`) {
		t.Fatalf("UIConfig 应原样透传查询串 path，实际:\n%s", got)
	}
}
