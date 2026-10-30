package model

import (
	"encoding/json"
	"net"
	"net/url"
	"regexp"
	"strconv"
	"strings"
)

type EntryType string

const (
	EntryTypeInternalPort EntryType = "internal_port"
	EntryTypeExternalURL  EntryType = "external_url"
	EntryTypeFolder       EntryType = "folder"
)

const (
	StateRunning = "running"
	StateStopped = "stopped"

	OpenModeIframe = "iframe"
	OpenModeURL    = "url"

	StatusUndeployed = "undeployed"
	StatusDeployed   = "deployed"
	StatusOutdated   = "outdated"

	RuntimeRunning = "running"
	RuntimeStopped = "stopped"
	RuntimeUnknown = "unknown"
)

type Entry struct {
	Type             EntryType       `json:"type"`
	Version          string          `json:"version"`
	AppName          string          `json:"appname"`
	DisplayName      string          `json:"displayName"`
	DisplayIcon      string          `json:"displayIcon"`
	Description      string          `json:"description"`
	DesiredState     string          `json:"desiredState"`
	DeployedVersion  string          `json:"deployedVersion,omitempty"`
	Config           json.RawMessage `json:"config"`
	ShortcutAppname  string          `json:"shortcutAppname,omitempty"`
	Status           string          `json:"status,omitempty"`
	Runtime          string          `json:"runtime,omitempty"`
	InstalledVersion string          `json:"installedVersion,omitempty"`
	RawStatus        string          `json:"rawStatus,omitempty"`
	DesktopEntryKey  string          `json:"desktopEntryKey,omitempty"`
}

type InternalPortConfig struct {
	Port     string `json:"port"`
	Protocol string `json:"protocol"`
	Path     string `json:"path"`
	OpenMode string `json:"openMode"`
}

type ExternalURLConfig struct {
	URL      string `json:"url"`
	OpenMode string `json:"openMode"`
}

type FolderConfig struct {
	Path string `json:"path"`
}

type EntryConfigInput struct {
	Port     string `json:"port"`
	Protocol string `json:"protocol"`
	Path     string `json:"path"`
	URL      string `json:"url"`
	OpenMode string `json:"openMode"`
}

var appNamePattern = regexp.MustCompile(`^[a-z][a-z0-9-]{1,11}$`)

// AppNameValid 判断 appname 是否符合命名规则。数据文件里的名字会被拼进
// 构建/安装路径（filepath.Join(BuildDir, child)），带路径分隔符的脏名字
// 能越出 build 目录，所以加载时也要单独校验一次。
func AppNameValid(appName string) bool {
	return appNamePattern.MatchString(appName)
}

func NewEntry(entryType EntryType, appName, displayName string, config json.RawMessage) Entry {
	return Entry{
		Type:         entryType,
		Version:      "1.0.0",
		AppName:      appName,
		DisplayName:  strings.TrimSpace(displayName),
		DisplayIcon:  "",
		Description:  "",
		DesiredState: StateRunning,
		Config:       config,
	}
}

func (e Entry) Validate() error {
	if !AppNameValid(e.AppName) {
		return Errorf(ErrValidation, "appname must start with a lowercase letter and contain only lowercase letters, numbers, and hyphens")
	}
	if strings.TrimSpace(e.DisplayName) == "" {
		return Errorf(ErrValidation, "displayName is required")
	}
	if len([]rune(strings.TrimSpace(e.DisplayName))) > 12 {
		return Errorf(ErrValidation, "displayName must be 12 characters or less")
	}
	if e.Type == "" {
		return Errorf(ErrValidation, "type is required")
	}
	if len(e.Config) == 0 {
		return Errorf(ErrValidation, "config is required")
	}
	if e.DesiredState == "" {
		e.DesiredState = StateRunning
	}
	if e.DesiredState != StateRunning && e.DesiredState != StateStopped {
		return Errorf(ErrValidation, "desiredState must be running or stopped")
	}
	return nil
}

func ConfigFromInput(entryType EntryType, input EntryConfigInput) (json.RawMessage, error) {
	if entryType == EntryTypeExternalURL {
		rawURL := strings.TrimSpace(input.URL)
		if rawURL == "" {
			return nil, Errorf(ErrValidation, "url is required")
		}
		if !IsValidURL(rawURL) {
			return nil, Errorf(ErrValidation, "url must be a valid http or https address")
		}
		openMode := strings.TrimSpace(input.OpenMode)
		if openMode == "" {
			openMode = OpenModeURL
		}
		if openMode != OpenModeURL && openMode != OpenModeIframe {
			return nil, Errorf(ErrValidation, "openMode must be url or iframe")
		}
		return json.Marshal(ExternalURLConfig{URL: rawURL, OpenMode: openMode})
	}
	if entryType == EntryTypeFolder {
		folderPath := strings.TrimSpace(input.Path)
		if folderPath == "" {
			return nil, Errorf(ErrValidation, "path is required")
		}
		if !IsValidFolderPath(folderPath) {
			return nil, Errorf(ErrValidation, "path must be an absolute folder path starting with /")
		}
		return json.Marshal(FolderConfig{Path: folderPath})
	}
	if strings.TrimSpace(input.Port) == "" {
		return nil, Errorf(ErrValidation, "port is required")
	}
	if strings.TrimSpace(input.Protocol) == "" {
		return nil, Errorf(ErrValidation, "protocol is required")
	}
	if strings.TrimSpace(input.Path) == "" {
		return nil, Errorf(ErrValidation, "path is required")
	}
	return json.Marshal(InternalPortConfig{
		Protocol: strings.TrimSpace(input.Protocol),
		Port:     strings.TrimSpace(input.Port),
		Path:     strings.TrimSpace(input.Path),
		OpenMode: input.OpenMode,
	})
}

func (e Entry) ChildAppname(mainAppname string) string {
	return mainAppname + "-" + e.AppName
}

func (e Entry) Normalize() Entry {
	if e.Type == "" {
		e.Type = EntryTypeInternalPort
	}
	if e.Version == "" {
		e.Version = "1.0.0"
	}
	if e.DesiredState == "" {
		e.DesiredState = StateRunning
	}
	e.Status = StatusUndeployed
	e.Runtime = RuntimeUnknown
	return e
}

func BumpVersion(version string) string {
	parts := strings.Split(strings.TrimSpace(version), ".")
	if len(parts) != 3 {
		return "1.0.0"
	}
	if !isDigits(parts[0]) || !isDigits(parts[1]) || !isDigits(parts[2]) {
		return "1.0.0"
	}
	patch, err := strconv.Atoi(parts[2])
	if err != nil {
		return "1.0.0"
	}
	return parts[0] + "." + parts[1] + "." + strconv.Itoa(patch+1)
}

func isDigits(value string) bool {
	if value == "" {
		return false
	}
	for _, r := range value {
		if r < '0' || r > '9' {
			return false
		}
	}
	return true
}

func IsValidPort(port string) bool {
	if len(port) < 1 || len(port) > 5 {
		return false
	}
	value := 0
	for _, r := range port {
		if r < '0' || r > '9' {
			return false
		}
		value = value*10 + int(r-'0')
	}
	return value >= 1 && value <= 65535
}

// IsValidFolderPath 校验文件夹快捷方式的目标路径：必须是绝对内部路径。
// 根目录不是快捷方式的目标（宿主的默认起始位置），同样拒绝。
// 路径会被构建进子应用启动页并传给 openFileManager，控制字符会破坏
// 生成页面的语法，这里直接拒绝。
func IsValidFolderPath(path string) bool {
	if path == "" || path == "/" || !strings.HasPrefix(path, "/") {
		return false
	}
	if len(path) > 512 {
		return false
	}
	for _, r := range path {
		if r < 0x20 || r == 0x7f {
			return false
		}
	}
	return true
}

func IsValidURL(rawURL string) bool {
	if rawURL == "" {
		return false
	}
	parsed, err := url.Parse(rawURL)
	if err != nil || parsed.Scheme == "" || parsed.Host == "" {
		return false
	}
	if parsed.Scheme != "http" && parsed.Scheme != "https" {
		return false
	}
	host := parsed.Hostname()
	if host == "" {
		return false
	}
	if ip := net.ParseIP(host); ip != nil {
		return true
	}
	if len(host) > 253 {
		return false
	}
	labels := strings.Split(strings.ToLower(host), ".")
	for _, label := range labels {
		if len(label) < 1 || len(label) > 63 {
			return false
		}
		if !regexp.MustCompile(`^[a-z0-9]([a-z0-9-]*[a-z0-9])?$`).MatchString(label) {
			return false
		}
	}
	return len(labels) >= 2
}
