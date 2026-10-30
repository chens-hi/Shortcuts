package model

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestEntryNormalizeDoesNotInjectDefaultIcon(t *testing.T) {
	entry := NewEntry(EntryTypeInternalPort, "demo", "Demo", []byte(`{"protocol":"http","port":"80","path":"/"}`))

	entry = entry.Normalize()

	if entry.DisplayIcon != "" {
		t.Fatalf("DisplayIcon = %q, want empty for default icon state", entry.DisplayIcon)
	}
}

func TestBumpVersion(t *testing.T) {
	cases := map[string]string{
		"1.0.0":     "1.0.1",
		"1.0.172":   "1.0.173",
		"  2.10.9 ": "2.10.10",
		"":          "1.0.0",
		"1.0":       "1.0.0",
		"1.0.1.2":   "1.0.0",
		"a.b.c":     "1.0.0",
		"1.0.x":     "1.0.0",
	}
	for in, want := range cases {
		if got := BumpVersion(in); got != want {
			t.Fatalf("BumpVersion(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestIsValidPort(t *testing.T) {
	for _, port := range []string{"1", "80", "13000", "65535", "00080"} {
		if !IsValidPort(port) {
			t.Fatalf("IsValidPort(%q) = false, want true", port)
		}
	}
	for _, port := range []string{"", "0", "65536", "99999", "abc", "1a", "-1", "1.5", "1 0", "123456"} {
		if IsValidPort(port) {
			t.Fatalf("IsValidPort(%q) = true, want false", port)
		}
	}
}

func TestIsValidURL(t *testing.T) {
	for _, rawURL := range []string{
		"http://127.0.0.1:13000",
		"http://192.168.1.20",
		"https://www.example.com",
		"https://sub.example.com/path?q=1",
	} {
		if !IsValidURL(rawURL) {
			t.Fatalf("IsValidURL(%q) = false, want true", rawURL)
		}
	}
	for _, rawURL := range []string{
		"",
		"not a url",
		"ftp://example.com",
		"file:///tmp/x",
		"http://",
		"http://example",
		"http://-bad-.example.com",
		"http://example..com",
	} {
		if IsValidURL(rawURL) {
			t.Fatalf("IsValidURL(%q) = true, want false", rawURL)
		}
	}
}

func TestIsValidFolderPath(t *testing.T) {
	for _, path := range []string{"/vol1/1000", "/vol1/1000/照片 备份", "/" + strings.Repeat("a", 500)} {
		if !IsValidFolderPath(path) {
			t.Fatalf("IsValidFolderPath(%q) = false, want true", path)
		}
	}
	for _, path := range []string{"", "/", "vol1/1000", "C:/data", "/vol1/\tx", "/vol1/\n x", "/" + strings.Repeat("a", 513)} {
		if IsValidFolderPath(path) {
			t.Fatalf("IsValidFolderPath(%q) = true, want false", path)
		}
	}
}

func TestConfigFromInputFolder(t *testing.T) {
	raw, err := ConfigFromInput(EntryTypeFolder, EntryConfigInput{Path: " /vol1/1000 "})
	if err != nil {
		t.Fatalf("ConfigFromInput = %v, want nil", err)
	}
	var config FolderConfig
	if err := json.Unmarshal(raw, &config); err != nil {
		t.Fatalf("folder config is not valid JSON: %v", err)
	}
	if config.Path != "/vol1/1000" {
		t.Fatalf("path = %q, want /vol1/1000 (trimmed)", config.Path)
	}

	for _, input := range []EntryConfigInput{{Path: ""}, {Path: "relative"}} {
		if _, err := ConfigFromInput(EntryTypeFolder, input); err == nil {
			t.Fatalf("ConfigFromInput(%+v) = nil error, want validation error", input)
		}
	}
}
