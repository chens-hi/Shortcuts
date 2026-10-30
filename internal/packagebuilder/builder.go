package packagebuilder

import (
	"encoding/json"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	"github.com/chens-hi/shortcuts/internal/icons"
	"github.com/chens-hi/shortcuts/internal/model"
	"github.com/chens-hi/shortcuts/internal/provider"
)

type Result struct {
	AppName string
	Dir     string
}

type Builder struct {
	BuildDir    string
	TemplateDir string
	Icons       *icons.Service
	MainAppname string
}

func (b *Builder) Build(entry model.Entry, spec provider.ApplicationSpec) (Result, error) {
	appName := entry.ChildAppname(b.MainAppname)
	destDir := filepath.Join(b.BuildDir, appName)
	if err := os.RemoveAll(destDir); err != nil {
		return Result{}, model.Errorf(model.ErrDeploy, err.Error())
	}
	templateDir := filepath.Join(b.TemplateDir, templateName(entry.Type))
	if err := b.copyTemplates(templateDir, destDir, spec.TemplateVars); err != nil {
		return Result{}, model.Errorf(model.ErrDeploy, err.Error())
	}
	manifest, err := b.renderManifest(appName, entry, spec.ExtraManifest)
	if err != nil {
		return Result{}, model.Errorf(model.ErrDeploy, err.Error())
	}
	if err := os.WriteFile(filepath.Join(destDir, "manifest"), []byte(manifest), 0644); err != nil {
		return Result{}, model.Errorf(model.ErrDeploy, err.Error())
	}
	uiConfig, err := json.MarshalIndent(spec.UIConfig, "", "  ")
	if err != nil {
		return Result{}, model.Errorf(model.ErrDeploy, err.Error())
	}
	uiConfigDir := filepath.Join(destDir, "app", "ui")
	imageDir := filepath.Join(uiConfigDir, "images")
	if err := os.MkdirAll(imageDir, 0755); err != nil {
		return Result{}, model.Errorf(model.ErrDeploy, err.Error())
	}
	if err := os.WriteFile(filepath.Join(uiConfigDir, "config"), append(uiConfig, '\n'), 0644); err != nil {
		return Result{}, model.Errorf(model.ErrDeploy, err.Error())
	}
	if spec.CGIScript != "" {
		cgiPath := filepath.Join(uiConfigDir, "index.cgi")
		if err := os.WriteFile(cgiPath, []byte(spec.CGIScript), 0755); err != nil {
			return Result{}, model.Errorf(model.ErrDeploy, err.Error())
		}
		if err := os.Chmod(cgiPath, 0755); err != nil {
			return Result{}, model.Errorf(model.ErrDeploy, err.Error())
		}
		// Generate callback scripts that re-write config + CGI after install/config-change/upgrade
		callbackContent := generateCallback(string(uiConfig), spec.CGIScript)
		for _, name := range []string{"install_callback", "config_callback", "upgrade_callback"} {
			cbPath := filepath.Join(destDir, "cmd", name)
			if err := os.WriteFile(cbPath, []byte(callbackContent), 0755); err != nil {
				return Result{}, model.Errorf(model.ErrDeploy, err.Error())
			}
			if err := os.Chmod(cbPath, 0755); err != nil {
				return Result{}, model.Errorf(model.ErrDeploy, err.Error())
			}
		}
		// Keep template privilege/resource; generated shortcuts must match the
		// reference shortcut template and run as root.
	}
	// 类型专属的静态文件。键来自各 provider 的常量，但路径拼进包目录前
	// 仍做一次越界检查，防手滑写穿。
	for rel, content := range spec.ExtraFiles {
		clean := filepath.Clean(rel)
		if filepath.IsAbs(clean) || clean == ".." || strings.HasPrefix(clean, ".."+string(filepath.Separator)) {
			return Result{}, model.Errorf(model.ErrDeploy, "invalid extra file path: %s", rel)
		}
		dest := filepath.Join(destDir, "app", clean)
		if err := os.MkdirAll(filepath.Dir(dest), 0755); err != nil {
			return Result{}, model.Errorf(model.ErrDeploy, err.Error())
		}
		if err := os.WriteFile(dest, []byte(content), 0644); err != nil {
			return Result{}, model.Errorf(model.ErrDeploy, err.Error())
		}
	}
	for size, name := range map[int]string{64: "ICON.PNG", 256: "ICON_256.PNG"} {
		icon, err := b.iconForEntry(entry.AppName, size)
		if err != nil {
			return Result{}, model.Errorf(model.ErrDeploy, err.Error())
		}
		if err := os.WriteFile(filepath.Join(destDir, name), icon, 0644); err != nil {
			return Result{}, model.Errorf(model.ErrDeploy, err.Error())
		}
		if err := os.WriteFile(filepath.Join(imageDir, fmt.Sprintf("icon_%d.png", size)), icon, 0644); err != nil {
			return Result{}, model.Errorf(model.ErrDeploy, err.Error())
		}
	}
	cmdDir := filepath.Join(destDir, "cmd")
	if err := filepath.WalkDir(cmdDir, func(path string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return err
		}
		return os.Chmod(path, 0755)
	}); err != nil {
		return Result{}, model.Errorf(model.ErrDeploy, err.Error())
	}
	return Result{AppName: appName, Dir: destDir}, nil
}

func templateName(entryType model.EntryType) string {
	return strings.ReplaceAll(string(entryType), "_", "-")
}

func generateCallback(configJSON string, cgiScript string) string {
	return "#!/bin/bash\n" +
		"\n" +
		"cat > \"${TRIM_APPDEST}/app/ui/config\" <<'JSON'\n" +
		configJSON + "\n" +
		"JSON\n" +
		"\n" +
		"cat > \"${TRIM_APPDEST}/app/ui/index.cgi\" <<'CGI'\n" +
		cgiScript +
		"\nCGI\n" +
		"\n" +
		"chmod +x \"${TRIM_APPDEST}/app/ui/index.cgi\"\n" +
		"\n" +
		"exit 0\n"
}

func (b *Builder) iconForEntry(appName string, size int) ([]byte, error) {
	if b.Icons != nil {
		data, err := b.Icons.Read(appName, size)
		if err == nil {
			return data, nil
		}
		if data, err := b.Icons.Fallback(size); err == nil {
			return data, nil
		}
	}
	return nil, model.Errorf(model.ErrDeploy, "missing icon")
}

func (b *Builder) copyTemplates(srcDir, destDir string, vars map[string]string) error {
	if err := os.MkdirAll(destDir, 0755); err != nil {
		return err
	}
	var replacer *strings.Replacer
	if len(vars) > 0 {
		pairs := make([]string, 0, len(vars)*2)
		for placeholder, value := range vars {
			pairs = append(pairs, placeholder, value)
		}
		replacer = strings.NewReplacer(pairs...)
	}
	return filepath.WalkDir(srcDir, func(src string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			return nil
		}
		rel, err := filepath.Rel(srcDir, src)
		if err != nil {
			return err
		}
		if strings.HasSuffix(rel, ".tmpl") {
			rel = strings.TrimSuffix(rel, ".tmpl")
		}
		dest := filepath.Join(destDir, rel)
		if err := os.MkdirAll(filepath.Dir(dest), 0755); err != nil {
			return err
		}
		data, err := os.ReadFile(src)
		if err != nil {
			return err
		}
		// 占位符是 ASCII 字面量，对二进制文件不会误替换；
		// 转换 string 不改变字节，非 UTF-8 内容也原样保留。
		if replacer != nil {
			data = []byte(replacer.Replace(string(data)))
		}
		mode := fs.FileMode(d.Type())
		if d.Type()&fs.ModePerm == 0 {
			mode = 0644
		}
		return os.WriteFile(dest, data, mode)
	})
}

func oneLine(value string) string {
	return strings.TrimSpace(strings.NewReplacer("\r", " ", "\n", " ").Replace(value))
}
