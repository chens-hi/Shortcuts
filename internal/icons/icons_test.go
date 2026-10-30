package icons

import (
	"os"
	"path/filepath"
	"testing"
)

func TestRemoveDeletesIconFiles(t *testing.T) {
	dir := t.TempDir()
	s := &Service{Dir: dir}
	for _, name := range []string{"testapp-64.png", "testapp-256.png"} {
		if err := os.WriteFile(filepath.Join(dir, name), []byte("png"), 0644); err != nil {
			t.Fatal(err)
		}
	}

	removed, err := s.Remove("testapp")
	if err != nil {
		t.Fatalf("Remove returned error: %v", err)
	}
	if !removed {
		t.Fatal("Remove reported no change for existing icon files")
	}
	for _, name := range []string{"testapp-64.png", "testapp-256.png"} {
		if _, err := os.Stat(filepath.Join(dir, name)); !os.IsNotExist(err) {
			t.Fatalf("icon file %s still exists after Remove", name)
		}
	}
}

func TestRemoveIsIdempotentWhenFilesMissing(t *testing.T) {
	s := &Service{Dir: t.TempDir()}

	removed, err := s.Remove("testapp")
	if err != nil {
		t.Fatalf("Remove returned error for missing files: %v", err)
	}
	if removed {
		t.Fatal("Remove reported a change when no icon files existed")
	}
}

func TestSaveRejectsUndecodableDataURL(t *testing.T) {
	// 解不开的图标数据必须报错。以前这里是静默跳过并返回 (false, nil)，
	// 调用方就以为图标存好了，把 displayIcon 指向一个并不存在的文件。
	s := &Service{Dir: t.TempDir()}
	if _, err := s.Save("testapp", "not-a-data-url", ""); err == nil {
		t.Fatal("Save 接受了解不开的 data URL，应当返回错误")
	}
	if _, err := s.Save("testapp", "data:image/png;base64,!!!", ""); err == nil {
		t.Fatal("Save 接受了非法 base64，应当返回错误")
	}
}

func TestSaveWritesValidPNGDataURL(t *testing.T) {
	// 1x1 透明 PNG
	const tinyPNG = "data:image/png;base64,iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAYAAAAfFcSJAAAADUlEQVR42mP8z8DwHwAFAAH/q842iQAAAABJRU5ErkJggg=="
	dir := t.TempDir()
	s := &Service{Dir: dir}
	saved, err := s.Save("testapp", tinyPNG, tinyPNG)
	if err != nil {
		t.Fatalf("Save returned error for a valid PNG: %v", err)
	}
	if !saved {
		t.Fatal("Save reported no change for a valid PNG data URL")
	}
	for _, name := range []string{"testapp-64.png", "testapp-256.png"} {
		if _, err := os.Stat(filepath.Join(dir, name)); err != nil {
			t.Fatalf("expected %s to be written: %v", name, err)
		}
	}
}
