package config

import (
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

// DefaultCLITimeout 是 appcenter-cli 单次调用的默认超时，也是这个超时唯一的
// 默认值来源：deployer.NewManager 的兜底、agent 的 socket 读写 deadline 都从
// 这里取。以前三处各写一遍（180s / 600s / 600s+10s），改一个忘一个就对不上。
const DefaultCLITimeout = 600 * time.Second

type Config struct {
	MainAppname    string
	Host           string
	Port           int
	GatewayPrefix  string
	Pkgvar         string
	DataFile       string
	BuildDir       string
	IconDir        string
	TemplateDir    string
	UIWWWDir       string
	AppInstallRoot string
	AppCenterCLI   string
	AgentSocket    string
	CLITimeout     time.Duration
	DeployDisabled bool
	TrimUID        int
	TrimGID        int
}

func (c Config) GatewayPath() string {
	prefix := c.GatewayPrefix
	if prefix == "" {
		prefix = "/app/" + c.MainAppname
	}
	if prefix != "/" {
		prefix = strings.TrimRight(prefix, "/")
	}
	return prefix
}

func New() Config {
	mainAppname := env("TRIM_APPNAME", "shortcuts")
	if mainAppname == "" {
		mainAppname = "shortcuts"
	}
	appInstallRoot := appInstallRootFromEnv()
	pkgvar := env("TRIM_PKGVAR", filepath.Join(appInstallRoot, "var"))
	if pkgvar == "" {
		pkgvar = filepath.Join(appInstallRoot, "var")
	}
	// 默认不监听 TCP。界面上所有资源与接口都是 /app/<appname>/... 绝对路径，
	// 经 SOCKET_PATH 这个 unix socket 网关转发，这套 handler 又完全没有鉴权，
	// 再开一个 TCP 端口既没人用，又会白担"端口被占用导致启动失败"的风险
	// （端口绑定失败以前是致命错误，会连带把已经起好的网关一起带下去）。
	// 需要在本机直连排查时显式设 HOST=127.0.0.1 打开。
	host := env("HOST", "")
	port := envInt("PORT", 23330)
	if port == 0 {
		port = envInt("SERVICE_PORT", 23330)
	}

	return Config{
		MainAppname:    mainAppname,
		Host:           host,
		Port:           port,
		GatewayPrefix:  env("GATEWAY_PREFIX", "/app/"+mainAppname),
		Pkgvar:         pkgvar,
		DataFile:       env("SHORTCUTS_DATA_FILE", filepath.Join(pkgvar, "shortcuts.json")),
		BuildDir:       env("SHORTCUTS_BUILD_DIR", filepath.Join(pkgvar, "build")),
		IconDir:        env("SHORTCUTS_ICON_DIR", filepath.Join(pkgvar, "icons")),
		TemplateDir:    env("SHORTCUTS_TEMPLATE_DIR", filepath.Join(appInstallRoot, "templates")),
		UIWWWDir:       env("SHORTCUTS_UI_WWW", filepath.Join(appInstallRoot, "ui", "www")),
		AppInstallRoot: appInstallRoot,
		AppCenterCLI:   env("SHORTCUTS_APPCENTER_CLI", "appcenter-cli"),
		AgentSocket:    env("SHORTCUTS_AGENT_SOCKET", filepath.Join(pkgvar, "deploy-agent.sock")),
		CLITimeout:     envDuration("SHORTCUTS_CLI_TIMEOUT", DefaultCLITimeout.Milliseconds()),
		DeployDisabled: env("SHORTCUTS_DEPLOY_DISABLED", "0") == "1",
		TrimUID:        envInt("TRIM_UID", -1),
		TrimGID:        envInt("TRIM_GID", -1),
	}.WithResolvedPaths()
}

// WithResolvedPaths fills in path fields that the caller did not set.
// It never overwrites a non-empty value, so a Config built as a literal with
// explicit paths (as tests do) keeps those paths while still receiving
// environment-derived defaults for the rest. It is idempotent.
//
// Previously this re-derived every field from the environment, which silently
// discarded explicit values; service.NewService then resolved the config a
// second time and moved IconDir out from under the caller, so icon uploads
// failed with a write error on whatever the default directory resolved to.
func (c Config) WithResolvedPaths() Config {
	if c.AppInstallRoot == "" {
		c.AppInstallRoot = appInstallRootFromEnv()
	}
	if c.Pkgvar == "" {
		c.Pkgvar = env("TRIM_PKGVAR", filepath.Join(c.AppInstallRoot, "var"))
	}
	if c.DataFile == "" {
		c.DataFile = env("SHORTCUTS_DATA_FILE", filepath.Join(c.Pkgvar, "shortcuts.json"))
	}
	if c.BuildDir == "" {
		c.BuildDir = env("SHORTCUTS_BUILD_DIR", filepath.Join(c.Pkgvar, "build"))
	}
	if c.IconDir == "" {
		c.IconDir = env("SHORTCUTS_ICON_DIR", filepath.Join(c.Pkgvar, "icons"))
	}
	if c.TemplateDir == "" {
		c.TemplateDir = env("SHORTCUTS_TEMPLATE_DIR", filepath.Join(c.AppInstallRoot, "templates"))
	}
	if c.UIWWWDir == "" {
		c.UIWWWDir = env("SHORTCUTS_UI_WWW", filepath.Join(c.AppInstallRoot, "ui", "www"))
	}
	if c.AgentSocket == "" {
		c.AgentSocket = env("SHORTCUTS_AGENT_SOCKET", filepath.Join(c.Pkgvar, "deploy-agent.sock"))
	}
	return c
}

func (c Config) DefaultIconDir() string {
	return filepath.Join(appInstallRootFromEnv(), "app", "icons")
}

func appInstallRootFromEnv() string {
	return resolveIfRelative(
		env("SHORTCUTS_APP_ROOT", env("TRIM_APPDEST", filepath.Join("/var/apps", "shortcuts"))),
		filepath.Join(os.Getenv("HOME"), "shortcuts"),
	)
}

func resolveIfRelative(path, fallback string) string {
	if filepath.IsAbs(path) {
		return filepath.Clean(path)
	}
	return fallback
}

func env(key, fallback string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return fallback
}

func envInt(key string, fallback int) int {
	if value := os.Getenv(key); value != "" {
		if parsed, err := strconv.Atoi(value); err == nil {
			return parsed
		}
	}
	return fallback
}

func envDuration(key string, fallback int64) time.Duration {
	return time.Duration(envInt(key, int(fallback))) * time.Millisecond
}
