package appmeta

import (
	"fmt"
	"os"
	"strings"
)

func ReadManifestVersion(path string) (string, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return "", fmt.Errorf("read manifest: %w", err)
	}

	for _, line := range strings.Split(string(data), "\n") {
		line = strings.TrimSpace(strings.TrimSuffix(line, "\r"))
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		if version, ok := strings.CutPrefix(line, "version="); ok {
			version = strings.TrimSpace(version)
			if version == "" {
				return "", fmt.Errorf("empty manifest version")
			}
			return version, nil
		}
	}

	return "", fmt.Errorf("manifest version missing")
}
