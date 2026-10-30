package repository

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/chens-hi/shortcuts/internal/model"
)

func TestJSONRepositorySaveLoadRoundTrip(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "shortcuts.json")
	repo := NewJSONRepository(path)
	entries := []model.Entry{model.NewEntry(model.EntryTypeInternalPort, "app-a", "App A", mustConfig(t, `{"protocol":"http","port":"8080","path":"/","openMode":"iframe"}`))}

	if err := repo.Save(entries); err != nil {
		t.Fatalf("save entries: %v", err)
	}
	loaded, err := repo.Load()
	if err != nil {
		t.Fatalf("load entries: %v", err)
	}
	if len(loaded) != 1 || loaded[0].AppName != "app-a" {
		t.Fatalf("unexpected loaded entries: %#v", loaded)
	}
}

func TestJSONRepositoryLoadLegacyOrBrokenShapes(t *testing.T) {
	tests := []struct {
		name    string
		content string
		want    int
		wantErr bool
	}{
		{name: "bare array is no longer accepted", content: "[]", wantErr: true},
		{name: "empty object", content: "{}", want: 0},
		{name: "invalid json", content: "{bad json", wantErr: true},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "shortcuts.json")
			if err := os.WriteFile(path, []byte(tc.content), 0644); err != nil {
				t.Fatalf("write fixture: %v", err)
			}
			loaded, err := NewJSONRepository(path).Load()
			if tc.wantErr {
				if err == nil {
					t.Fatalf("expected error, got entries %#v", loaded)
				}
				return
			}
			if err != nil {
				t.Fatalf("load legacy data: %v", err)
			}
			if len(loaded) != tc.want {
				t.Fatalf("want %d entries, got %#v", tc.want, loaded)
			}
		})
	}
}

func TestJSONRepositoryLoadRejectsSchemaMismatchInsteadOfSilentClear(t *testing.T) {
	tests := []struct {
		name    string
		content string
	}{
		{name: "entries is a string", content: `{"version":1,"entries":"moviepilot"}`},
		{name: "version is a string", content: `{"version":"one","entries":[]}`},
		{name: "entries entry has wrong type", content: `{"version":1,"entries":[123]}`},
		{name: "bare number", content: `123`},
		{name: "bare string", content: `"shortcuts"`},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "shortcuts.json")
			if err := os.WriteFile(path, []byte(tc.content), 0644); err != nil {
				t.Fatalf("write fixture: %v", err)
			}
			if _, err := NewJSONRepository(path).Load(); err == nil {
				t.Fatal("expected error on schema mismatch, got nil")
			}
		})
	}
}

func TestJSONRepositoryRejectsLegacyArrayFormat(t *testing.T) {
	// 旧版裸数组格式不再兼容（已明确不需要兼容历史数据）：加载必须报错，
	// 而不是把它当成空列表——那样下一次 Save 会把用户数据整片覆盖。
	path := filepath.Join(t.TempDir(), "shortcuts.json")
	content := `[{"type":"internal_port","appname":"testapp","displayName":"Test","version":"1.0.0","config":{"port":"8080","protocol":"http","path":"/","openMode":"iframe"}}]`
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		t.Fatalf("write fixture: %v", err)
	}
	if _, err := NewJSONRepository(path).Load(); err == nil {
		t.Fatal("expected error for bare-array file, got nil")
	}
}

func TestJSONRepositoryRejectsDirtyAppname(t *testing.T) {
	// appname 会被拼进 build/<child> 等路径，脏名字必须在加载时就拦下。
	path := filepath.Join(t.TempDir(), "shortcuts.json")
	content := `{"version":1,"entries":[{"type":"internal_port","appname":"../../etc/evil","displayName":"Evil","version":"1.0.0","config":{"port":"8080","protocol":"http","path":"/","openMode":"iframe"}}]}`
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		t.Fatalf("write fixture: %v", err)
	}
	if _, err := NewJSONRepository(path).Load(); err == nil {
		t.Fatal("expected error for appname with path separators, got nil")
	}
}

func mustConfig(t *testing.T, raw string) []byte {
	t.Helper()
	return []byte(raw)
}
