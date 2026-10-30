package service

import (
	"fmt"
	"regexp"

	"github.com/chens-hi/shortcuts/internal/model"
)

// ValidateFolderPath 校验文件夹快捷方式的目标路径真实存在且是目录。
// 格式校验在本地（model.IsValidFolderPath）；存在性必须走 root agent，
// 因为 server 进程按 TRIM_USERNAME 降权运行，对 /vol1 共享目录没有读权限，
// 直接 os.Stat 会对真实目录误报失败。
//
// 返回的 code 是给前端映射文案的机器码："" 有效，"bad_format" 格式非法，
// "not_found" 不存在或不是目录。不要返回人读英文文案——前端直接把它当提示
// 显示会出英文 toast。agent 不可用时返回 error，调用方按「无法确认存在性」
// 处理（fail-closed），而不是放行一个可能不存在的路径。
func (s *Service) ValidateFolderPath(path string) (bool, string, error) {
	if !model.IsValidFolderPath(path) {
		return false, "bad_format", nil
	}
	result := s.Deployer.StatDir(path)
	if result.Unavailable {
		return false, "", fmt.Errorf("folder validation unavailable: %s", result.Error)
	}
	if result.OK {
		return true, "", nil
	}
	return false, "not_found", nil
}

// 宿主返回的桌面项标识形如 folder:57。服务端只校验形状，不解析序号：
// 序号由桌面端生成，回写只是一个认领凭据。
var desktopEntryKeyPattern = regexp.MustCompile(`^folder:[0-9]{1,20}$`)

// SetDesktopEntryKey 落盘 folder 条目的桌面项标识；key 传空表示清空。
// 空值也必须走同一个写锁，避免"移除桌面项"和"重建"并发时互相覆盖。
// 不做去空白规整：宿主回写的标识必须精确匹配，带空白的输入是脏数据，
// 静默修剪会让本地记录和宿主记住的标识悄悄分叉。
func (s *Service) SetDesktopEntryKey(appName, key string) (model.Entry, error) {
	if key != "" && !desktopEntryKeyPattern.MatchString(key) {
		return model.Entry{}, model.Errorf(model.ErrValidation, "invalid desktop entry key")
	}
	var updated model.Entry
	if _, err := s.withRepoWrite(func(entries []model.Entry) ([]model.Entry, error) {
		idx, ok := indexOfPointer(entries, appName)
		if !ok {
			return nil, model.Errorf(model.ErrNotFound, "entry not found")
		}
		if entries[idx].Type != model.EntryTypeFolder {
			return nil, model.Errorf(model.ErrValidation, "desktop entry key is only supported for folder shortcuts")
		}
		entries[idx].DesktopEntryKey = key
		updated = entries[idx]
		return entries, nil
	}); err != nil {
		return model.Entry{}, err
	}
	apps, _ := s.installedApps()
	return decorate(updated, apps, s.Config.MainAppname), nil
}
