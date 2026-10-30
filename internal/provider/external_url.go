package provider

import (
	"fmt"
	"strings"

	"github.com/chens-hi/shortcuts/internal/model"
)

// shellQuote 把 s 包成 bash 单引号字面量。
//
// 这里绝对不能用 fmt 的 %q：它会产出 Go 风格的双引号字符串，而 bash 在双引号
// 内部照样做命令替换，用户填一个 http://a/$(curl${IFS}x) 这样的合法 URL 就能在
// CGI 运行时执行任意命令。单引号内 bash 不做任何展开，安全。
func shellQuote(s string) string {
	return "'" + strings.ReplaceAll(s, "'", `'\''`) + "'"
}

type ExternalURLProvider struct{}

func (p ExternalURLProvider) Type() model.EntryType {
	return model.EntryTypeExternalURL
}

func (p ExternalURLProvider) Validate(entry model.Entry) error {
	if err := entry.Validate(); err != nil {
		return err
	}
	config, err := DecodeConfig[model.ExternalURLConfig](entry.Config)
	if err != nil {
		return err
	}
	if !model.IsValidURL(config.URL) {
		return model.Errorf(model.ErrValidation, "url must be a valid http or https address")
	}
	openMode := config.OpenMode
	if openMode == "" {
		openMode = model.OpenModeURL
	}
	if openMode != model.OpenModeURL && openMode != model.OpenModeIframe {
		return model.Errorf(model.ErrValidation, "openMode must be url or iframe")
	}
	return nil
}

func (p ExternalURLProvider) Spec(entry model.Entry, mainAppname string) (ApplicationSpec, error) {
	config, err := DecodeConfig[model.ExternalURLConfig](entry.Config)
	if err != nil {
		return ApplicationSpec{}, err
	}
	cgiScript := fmt.Sprintf("#!/bin/bash\n\nTARGET_URL=%s\n\nprintf 'Status: 302 Found\\r\\n'\nprintf 'Location: %%s\\r\\n' \"$TARGET_URL\"\nprintf 'Content-Type: text/plain; charset=utf-8\\r\\n'\nprintf '\\r\\n'\nprintf 'Redirecting to %%s\\n' \"$TARGET_URL\"\n", shellQuote(config.URL))
	childAppname := entry.ChildAppname(mainAppname)
	uiKey := childAppname
	cgiPath := "/cgi/ThirdParty/" + childAppname + "/index.cgi"

	return ApplicationSpec{
		DisplayName: entry.DisplayName,
		Description: entry.Description,
		Version:     entry.Version,
		CGIScript:   cgiScript,
		UIConfig: map[string]any{
			".url": map[string]any{
				uiKey: map[string]any{
					"title":    entry.DisplayName,
					"icon":     "images/icon_{0}.png",
					"type":     config.OpenMode,
					"protocol": "http",
					"url":      cgiPath,
					"allUsers": true,
					"control":  map[string]any{"accessPerm": "readonly"},
				},
			},
		},
	}, nil
}

func (p ExternalURLProvider) Changed(old model.Entry, next model.Entry) bool {
	// DesiredState 不计入"配置变化"：它由启停接口改，且 SetRuntime 改同一个
	// 字段并不 bump 版本；算进来会让"改一次启停"导致下次部署整包重装。
	if old.DisplayName != next.DisplayName ||
		old.Description != next.Description ||
		old.DisplayIcon != next.DisplayIcon {
		return true
	}
	// 解码失败按"有变化"处理：以前错误被吞掉，两边都解成零值就被判成无变化，
	// 配置其实变了却既不 bump 版本也不重建子应用。
	oldConfig, oldErr := DecodeConfig[model.ExternalURLConfig](old.Config)
	nextConfig, nextErr := DecodeConfig[model.ExternalURLConfig](next.Config)
	if oldErr != nil || nextErr != nil {
		return true
	}
	if oldConfig.OpenMode == "" {
		oldConfig.OpenMode = model.OpenModeURL
	}
	if nextConfig.OpenMode == "" {
		nextConfig.OpenMode = model.OpenModeURL
	}
	return oldConfig != nextConfig
}
