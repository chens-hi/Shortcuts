package config

import "testing"

func TestWithResolvedPathsKeepsExplicitValues(t *testing.T) {
	in := Config{
		MainAppname: "shortcuts",
		IconDir:     "D:\\explicit\\icons",
		BuildDir:    "D:\\explicit\\build",
		DataFile:    "D:\\explicit\\shortcuts.json",
	}

	out := in.WithResolvedPaths()

	if out.IconDir != "D:\\explicit\\icons" {
		t.Fatalf("IconDir = %q, want explicit value preserved", out.IconDir)
	}
	if out.BuildDir != "D:\\explicit\\build" {
		t.Fatalf("BuildDir = %q, want explicit value preserved", out.BuildDir)
	}
	if out.DataFile != "D:\\explicit\\shortcuts.json" {
		t.Fatalf("DataFile = %q, want explicit value preserved", out.DataFile)
	}
}

func TestWithResolvedPathsFillsEmptyFields(t *testing.T) {
	out := Config{MainAppname: "shortcuts"}.WithResolvedPaths()

	if out.AppInstallRoot == "" {
		t.Fatal("AppInstallRoot = empty, want resolved default")
	}
	if out.Pkgvar == "" {
		t.Fatal("Pkgvar = empty, want resolved default")
	}
	if out.IconDir == "" {
		t.Fatal("IconDir = empty, want resolved default")
	}
	if out.BuildDir == "" {
		t.Fatal("BuildDir = empty, want resolved default")
	}
	if out.DataFile == "" {
		t.Fatal("DataFile = empty, want resolved default")
	}
}

func TestWithResolvedPathsIsIdempotent(t *testing.T) {
	once := Config{MainAppname: "shortcuts"}.WithResolvedPaths()
	twice := once.WithResolvedPaths()

	if once != twice {
		t.Fatalf("WithResolvedPaths is not idempotent: first=%#v second=%#v", once, twice)
	}
}
