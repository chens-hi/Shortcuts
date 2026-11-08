package model

import (
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
