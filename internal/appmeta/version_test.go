package appmeta_test

import (
	"os"
	"path/filepath"
	"testing"
)

import (
	"github.com/chens-hi/shortcuts/internal/appmeta"
)

func TestReadManifestVersion(t *testing.T) {
	dir := t.TempDir()
	manifest := filepath.Join(dir, "manifest")
	if err := os.WriteFile(manifest, []byte("appname=shortcuts\nversion=1.0.40\n\nchangelog=test\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	version, err := appmeta.ReadManifestVersion(manifest)
	if err != nil {
		t.Fatal(err)
	}
	if version != "1.0.40" {
		t.Fatalf("version = %q", version)
	}
}
