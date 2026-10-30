package provider

import (
	"strings"

	"github.com/chens-hi/shortcuts/internal/model"
)

type InternalPortProvider struct{}

func (p InternalPortProvider) Type() model.EntryType {
	return model.EntryTypeInternalPort
}

func (p InternalPortProvider) Validate(entry model.Entry) error {
	if err := entry.Validate(); err != nil {
		return err
	}
	config, err := DecodeConfig[model.InternalPortConfig](entry.Config)
	if err != nil {
		return err
	}
	if !model.IsValidPort(config.Port) {
		return model.Errorf(model.ErrValidation, "port must be between 1 and 65535")
	}
	if config.Protocol != "http" && config.Protocol != "https" {
		return model.Errorf(model.ErrValidation, "protocol must be http or https")
	}
	// 允许纯查询串（如 Magpie 的 "?k=<token>"）：fnOS 拼出 host:port?k=… 后
	// 浏览器会按空路径（即 /）请求，与 /?k=… 等价。
	if config.Path == "" ||
		(!strings.HasPrefix(config.Path, "/") && !strings.HasPrefix(config.Path, "?")) {
		return model.Errorf(model.ErrValidation, "path must start with / or ?")
	}
	if config.OpenMode != model.OpenModeIframe && config.OpenMode != model.OpenModeURL {
		return model.Errorf(model.ErrValidation, "openMode must be iframe or url")
	}
	return nil
}

func (p InternalPortProvider) Spec(entry model.Entry, mainAppname string) (ApplicationSpec, error) {
	config, err := DecodeConfig[model.InternalPortConfig](entry.Config)
	if err != nil {
		return ApplicationSpec{}, err
	}
	return ApplicationSpec{
		DisplayName: entry.DisplayName,
		Description: entry.Description,
		Version:     entry.Version,
		OpenMode:    config.OpenMode,
		UIConfig: map[string]any{
			".url": map[string]any{
				entry.ChildAppname(mainAppname): map[string]any{
					"title":    entry.DisplayName,
					"icon":     "images/icon_{0}.png",
					"type":     config.OpenMode,
					"protocol": config.Protocol,
					"port":     config.Port,
					"url":      config.Path,
					"allUsers": true,
					"control":  map[string]any{"accessPerm": "readonly"},
				},
			},
		},
	}, nil
}

func (p InternalPortProvider) Changed(old model.Entry, next model.Entry) bool {
	// DesiredState 不计入"配置变化"：它由启停接口改，且 SetRuntime 改同一个
	// 字段并不 bump 版本；算进来会让"改一次启停"导致下次部署整包重装。
	if old.DisplayName != next.DisplayName ||
		old.Description != next.Description ||
		old.DisplayIcon != next.DisplayIcon {
		return true
	}
	// 解码失败按"有变化"处理：以前错误被吞掉，两边都解成零值就被判成无变化，
	// 配置其实变了却既不 bump 版本也不重建子应用。
	oldConfig, oldErr := DecodeConfig[model.InternalPortConfig](old.Config)
	nextConfig, nextErr := DecodeConfig[model.InternalPortConfig](next.Config)
	if oldErr != nil || nextErr != nil {
		return true
	}
	return oldConfig != nextConfig
}
