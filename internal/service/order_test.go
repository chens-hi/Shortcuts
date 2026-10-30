package service

import (
	"encoding/json"
	"errors"
	"os"
	"reflect"
	"testing"

	"github.com/chens-hi/shortcuts/internal/model"
)

func appNamesOf(entries []model.Entry) []string {
	names := make([]string, 0, len(entries))
	for _, entry := range entries {
		names = append(names, entry.AppName)
	}
	return names
}

func TestReorderEntriesAppliesOrderWithoutSideEffects(t *testing.T) {
	app := newRecordingAppCenter()
	svc, repoPath := newDeleteTestService(t, app, []map[string]any{
		entrySeed("aaa", "8001"),
		entrySeed("bbb", "8002"),
		entrySeed("ccc", "8003"),
	})

	before, err := svc.Repo.Load()
	if err != nil {
		t.Fatal(err)
	}

	want := []string{"ccc", "aaa", "bbb"}
	ordered, err := svc.ReorderEntries(want)
	if err != nil {
		t.Fatalf("ReorderEntries: %v", err)
	}
	if got := appNamesOf(ordered); !reflect.DeepEqual(got, want) {
		t.Fatalf("returned order = %v, want %v", got, want)
	}
	// decorate 是查询期的装饰，返回值里有快捷子应用名，但它不能被写盘。
	if ordered[0].ShortcutAppname != "shortcuts-ccc" {
		t.Fatalf("decorated shortcut appname = %q, want shortcuts-ccc", ordered[0].ShortcutAppname)
	}
	if got := loadAppNames(t, svc); !reflect.DeepEqual(got, want) {
		t.Fatalf("persisted order = %v, want %v", got, want)
	}

	after, err := svc.Repo.Load()
	if err != nil {
		t.Fatal(err)
	}
	byName := make(map[string]model.Entry, len(after))
	for _, entry := range after {
		byName[entry.AppName] = entry
	}
	for _, old := range before {
		cur := byName[old.AppName]
		if cur.Version != old.Version ||
			cur.DeployedVersion != old.DeployedVersion ||
			cur.DesiredState != old.DesiredState ||
			cur.DisplayName != old.DisplayName ||
			string(cur.Config) != string(old.Config) ||
			cur.DesktopEntryKey != old.DesktopEntryKey {
			t.Fatalf("entry %s changed beyond order: before=%+v after=%+v", old.AppName, old, cur)
		}
	}

	if calls := app.Calls(); len(calls) != 0 {
		t.Fatalf("reorder must not touch the deployer, got calls %v", calls)
	}

	raw, err := os.ReadFile(repoPath)
	if err != nil {
		t.Fatal(err)
	}
	var file struct {
		Version int           `json:"version"`
		Entries []model.Entry `json:"entries"`
	}
	if err := json.Unmarshal(raw, &file); err != nil {
		t.Fatalf("unmarshal data file: %v", err)
	}
	if file.Version != 1 {
		t.Fatalf("file version = %d, want 1", file.Version)
	}
	if got := appNamesOf(file.Entries); !reflect.DeepEqual(got, want) {
		t.Fatalf("file order = %v, want %v", got, want)
	}
	for _, entry := range file.Entries {
		if entry.ShortcutAppname != "" || entry.InstalledVersion != "" || entry.RawStatus != "" {
			t.Fatalf("decorated fields persisted for %s: %+v", entry.AppName, entry)
		}
	}
}

func TestReorderEntriesRejectsMismatchedSets(t *testing.T) {
	cases := []struct {
		name  string
		order []string
	}{
		{"missing", []string{"aaa"}},
		{"duplicate", []string{"aaa", "aaa", "ccc"}},
		{"unknown", []string{"aaa", "bbb", "zzz"}},
		{"empty", []string{}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			app := newRecordingAppCenter()
			svc, _ := newDeleteTestService(t, app, []map[string]any{
				entrySeed("aaa", "8001"),
				entrySeed("bbb", "8002"),
				entrySeed("ccc", "8003"),
			})

			_, err := svc.ReorderEntries(tc.order)
			var apiErr *model.APIError
			if !errors.As(err, &apiErr) || apiErr.Code != model.ErrValidation {
				t.Fatalf("ReorderEntries(%v) err = %v, want %s", tc.order, err, model.ErrValidation)
			}
			if got := loadAppNames(t, svc); !reflect.DeepEqual(got, []string{"aaa", "bbb", "ccc"}) {
				t.Fatalf("rejected reorder changed the file: %v", got)
			}
			if calls := app.Calls(); len(calls) != 0 {
				t.Fatalf("rejected reorder touched the deployer: %v", calls)
			}
		})
	}
}

func TestReorderEntriesAllowsEmptyListWhenNoEntries(t *testing.T) {
	svc, _ := newDeleteTestService(t, newRecordingAppCenter(), nil)

	ordered, err := svc.ReorderEntries([]string{})
	if err != nil {
		t.Fatalf("ReorderEntries(empty): %v", err)
	}
	if len(ordered) != 0 {
		t.Fatalf("ordered = %v, want empty", ordered)
	}
}
