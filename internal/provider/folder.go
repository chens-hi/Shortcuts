package provider

import (
	"github.com/chens-hi/shortcuts/internal/model"
)

// FolderProvider 生成「文件夹」条目。文件夹快捷方式不再安装子应用：
// 桌面原生 folder 项由应用页面在宿主内通过 app.query 创建，点击时由桌面
// 直接打开文件管理器，不产生任何窗口。
type FolderProvider struct{}

func (p FolderProvider) Type() model.EntryType {
	return model.EntryTypeFolder
}

func (p FolderProvider) Validate(entry model.Entry) error {
	if err := entry.Validate(); err != nil {
		return err
	}
	config, err := DecodeConfig[model.FolderConfig](entry.Config)
	if err != nil {
		return err
	}
	if !model.IsValidFolderPath(config.Path) {
		return model.Errorf(model.ErrValidation, "path must be an absolute folder path other than /")
	}
	// 不做目录存在性校验：服务端进程对 /vol1 下的共享目录没有读取权限
	// （设计上后端不读目录，打开目录由桌面 openFileManager 负责），os.Stat
	// 会对真实存在的路径误报失败。真实目录的保证来自桌面选择器——能选中的
	// 一定存在；这里只做能可靠判断的格式校验。
	return nil
}

// Spec 不再生成子应用：folder 条目由桌面端在宿主内创建，没有可打包的内容。
// 部署与预览最终都会走到这里，返回错误是兜底——任何遗漏的调用方都能拿到明确
// 原因，而不是静默打包一个没人用的子应用。
func (p FolderProvider) Spec(entry model.Entry, mainAppname string) (ApplicationSpec, error) {
	return ApplicationSpec{}, model.Errorf(model.ErrValidation, "folder shortcuts are created on the desktop client")
}

func (p FolderProvider) Changed(old model.Entry, next model.Entry) bool {
	// DesiredState 不计入"配置变化"：它由启停接口改，且 SetRuntime 改同一个
	// 字段并不 bump 版本；算进来会让"改一次启停"导致下次部署整包重装。
	if old.DisplayName != next.DisplayName ||
		old.Description != next.Description ||
		old.DisplayIcon != next.DisplayIcon {
		return true
	}
	// 解码失败按"有变化"处理：以前错误被吞掉，两边都解成零值就被判成无变化，
	// 配置其实变了却既不 bump 版本也不重建子应用。
	oldConfig, oldErr := DecodeConfig[model.FolderConfig](old.Config)
	nextConfig, nextErr := DecodeConfig[model.FolderConfig](next.Config)
	if oldErr != nil || nextErr != nil {
		return true
	}
	return oldConfig != nextConfig
}
