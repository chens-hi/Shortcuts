package packagebuilder

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"text/template"

	"github.com/chens-hi/shortcuts/internal/model"
)

// manifestTemplateFile 是子应用 manifest 模板，放在模板目录根部，
// 所有子应用类型共用。改字段只需编辑这个文件，不必改 Go 代码。
const manifestTemplateFile = "manifest.tmpl"

type manifestData struct {
	AppName     string
	Version     string
	DisplayName string
	Desc        string
	// Extra 是 manifest 的动态 key=value 字段。固定字段留在模板里，
	// 会随类型变化的（ctl_stop、micro_app）走这里，模板不再硬编码。
	Extra map[string]string
}

// manifestDataFor 组装 manifest 数据。动态字段默认值里 desktop_applaunchname
// 跟随应用名，与 ui/config 的入口键保持一致；类型可通过 ExtraManifest 覆盖。
func manifestDataFor(appName string, entry model.Entry, extra map[string]string) manifestData {
	desc := strings.TrimSpace(entry.Description)
	if desc == "" {
		desc = fmt.Sprintf("%s 快捷入口", entry.DisplayName)
	}
	merged := map[string]string{
		"ctl_stop":              "true",
		"desktop_applaunchname": appName,
	}
	for key, value := range extra {
		merged[key] = value
	}
	return manifestData{
		AppName:     appName,
		Version:     entry.Version,
		DisplayName: oneLine(entry.DisplayName),
		Desc:        oneLine(desc),
		Extra:       merged,
	}
}

// renderManifest 用模板目录根部的 manifest.tmpl 渲染子应用 manifest。
// 模板随应用包一起下发，改字段不需要重新编译。
func (b *Builder) renderManifest(appName string, entry model.Entry, extraManifest map[string]string) (string, error) {
	path := filepath.Join(b.TemplateDir, manifestTemplateFile)
	raw, err := os.ReadFile(path)
	if err != nil {
		return "", fmt.Errorf("read manifest template: %w", err)
	}
	// 模板只解析一次，数据值里的 {{ 不会被二次解析。
	tmpl, err := template.New(manifestTemplateFile).Parse(string(raw))
	if err != nil {
		return "", fmt.Errorf("parse manifest template %s: %w", path, err)
	}
	var out bytes.Buffer
	if err := tmpl.Execute(&out, manifestDataFor(appName, entry, extraManifest)); err != nil {
		return "", fmt.Errorf("render manifest template %s: %w", path, err)
	}
	// Extra range 块会在文件尾多出一个空行，裁掉后统一补单个结尾换行。
	return strings.TrimRight(out.String(), " \t\n") + "\n", nil
}
