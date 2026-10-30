package deployer

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"os"
	"os/exec"
	"regexp"
	"strings"
	"time"

	"github.com/chens-hi/shortcuts/internal/config"
)

type InstalledApp struct {
	AppName     string
	DisplayName string
	Version     string
	Status      string
	Dependency  string
}

type CommandResult struct {
	OK      bool
	Code    int
	Stdout  string
	Stderr  string
	Timeout bool
	Missing bool
	Via     string
	Error   string
	// Unavailable 表示请求根本没到达 agent（未配置、连不上、写失败），
	// 命令确定没有执行，调用方可以安全回落到直连 CLI。
	// 反过来：响应读丢/解不出时命令可能已经执行，绝不能标 Unavailable
	// 再跑一次直连——一次请求会被安装两遍。
	Unavailable bool
}

type Command struct {
	Action  string `json:"action"`
	Dir     string `json:"dir,omitempty"`
	AppName string `json:"appname,omitempty"`
}

type AppCenter interface {
	List() (map[string]InstalledApp, error)
	InstallLocal(dir string) CommandResult
	Start(appName string) CommandResult
	Stop(appName string) CommandResult
	Uninstall(appName string) CommandResult
	Ping() (bool, CommandResult)
	StatDir(dir string) CommandResult
}

// Settings 只保留真正被读到的东西。此前还有四个字段：BuildDir 与 AppRoot
// 从未被读过；FnpackBinary 只服务于"先打包成 fpk 再安装"那条已经废弃的路径；
// InstallVolume 算出来却从没传给 appcenter-cli，安装卷配置实际不生效。
type Settings struct {
	MainAppname  string
	AgentSocket  string
	AppCenterCLI string
	Timeout      time.Duration
}

type Manager struct {
	settings Settings
}

// socketDeadlineMargin 在 CLI 超时之外再留一点余量，让对端先收到命令超时的
// 结果帧，而不是连接被直接切断。
const socketDeadlineMargin = 10 * time.Second

func NewManager(settings Settings) *Manager {
	if settings.Timeout <= 0 {
		settings.Timeout = config.DefaultCLITimeout
	}
	if settings.AppCenterCLI == "" {
		settings.AppCenterCLI = "appcenter-cli"
	}
	return &Manager{settings: settings}
}

// List 返回已安装应用。真实失败（CLI 不可用、agent 连不上、命令非零退出）
// 必须作为 error 返回：之前这里无论成败都返回 nil error，调用方那条
// "List 失败就退回扫目录" 的分支因此永远走不到，appcenter 一坏就静默变成空列表。
func (m *Manager) List() (map[string]InstalledApp, error) {
	result := m.callAgent(Command{Action: "list"})
	if result.Unavailable {
		result = m.runCli([]string{"list"})
		result.Via = "direct"
	}
	if !result.OK {
		return nil, fmt.Errorf("appcenter list failed: %s", FormatOutput(result, 400))
	}
	return ParseAppCenterList(result.Stdout), nil
}

// InstallLocal 安装一个已经构建好的子应用包目录。
// appcenter-cli 的 install-local 不接路径参数，它安装的是当前工作目录，
// 所以这里（以及 agent 侧）都是把 cwd 设为包目录、arguments 只留子命令。
//
// 只有「请求没到达 agent」才回落直连：响应读丢/解不出时命令可能已经在
// agent 上执行过，再跑一次直连等于一次请求安装两遍。以前这里靠 Error 文案
// 含 "unavailable" 判断，把「执行过但结果丢了」也当成了「没执行」。
func (m *Manager) InstallLocal(dir string) CommandResult {
	resp := m.callAgent(Command{Action: "install", Dir: dir})
	if !resp.Unavailable {
		return resp
	}
	result := m.runCommand(m.settings.AppCenterCLI, []string{"install-local"}, dir)
	result.Via = "direct"
	return result
}

func (m *Manager) Start(appName string) CommandResult {
	return m.appAction("start", appName)
}

func (m *Manager) Stop(appName string) CommandResult {
	return m.appAction("stop", appName)
}

func (m *Manager) Uninstall(appName string) CommandResult {
	return m.appAction("uninstall", appName)
}

// appAction 一律先校验名字再决定执行方式。名字不合法就是拒绝：
// 之前校验不通过会绕过 agent 静默直连 CLI 执行同一个动作，校验成了
// 「用不用 agent」的路由开关，而不是拒绝条件。
func (m *Manager) appAction(action, appName string) CommandResult {
	if !m.appNameAllowed(appName) {
		return CommandResult{Code: -1, Error: action + ": appname not allowed: " + appName}
	}
	if result := m.callAgent(Command{Action: action, AppName: appName}); !result.Unavailable {
		return result
	}
	result := m.runCli([]string{action, appName})
	result.Via = "direct"
	return result
}

func (m *Manager) Ping() (bool, CommandResult) {
	if m.settings.AgentSocket == "" {
		return false, CommandResult{Error: "agent not configured"}
	}
	resp := m.callAgent(Command{Action: "ping"})
	return resp.OK, resp
}

// statDirAgentTimeout 是 stat-dir 单次 agent 调用的超时上限。stat 不启动任何
// CLI，正常毫秒级返回；给一个远小于 DefaultCLITimeout（600s）的上限，避免
// agent 侧被其它卡住的命令拖累时，路径校验也拖到分钟级才反馈。上限须小于
// 前端 validateFolderPath 的 15s 超时，让后端先返回而不是前后端竞相判超时。
const statDirAgentTimeout = 10 * time.Second

// StatDir 校验一个目录真实存在且是目录。server 进程按 TRIM_USERNAME 降权运行，
// 对 /vol1 下的共享目录没有读权限，直接 os.Stat 会对真实目录误报失败；存在性
// 必须由 root agent 判定。agent 不可用时不回落直连——本机进程同样没权限，
// 直连只会把「真实存在」误判成失败，所以直接向调用方报告不可用。
func (m *Manager) StatDir(dir string) CommandResult {
	return m.callAgentWithTimeout(Command{Action: "stat-dir", Dir: dir}, statDirAgentTimeout)
}

func (m *Manager) runCli(args []string) CommandResult {
	return m.runCommand(m.settings.AppCenterCLI, args, "")
}

func (m *Manager) runCommand(command string, args []string, cwd string) CommandResult {
	ctx, cancel := context.WithTimeout(context.Background(), m.settings.Timeout)
	defer cancel()
	cmd := exec.CommandContext(ctx, command, args...)
	if cwd != "" {
		cmd.Dir = cwd
	}
	stdout := &strings.Builder{}
	stderr := &strings.Builder{}
	cmd.Stdout = stdout
	cmd.Stderr = stderr
	err := cmd.Run()
	// 超时只在命令确实失败时判定：命令成功、ctx 又恰好过期时不能把成功结果
	// 改判成 Timeout。
	timedOut := err != nil && errors.Is(ctx.Err(), context.DeadlineExceeded)
	return CommandResultFromExec(err, timedOut, command, "", stdout.String(), stderr.String())
}

// CommandResultFromExec 把一次外部命令的执行结果归类成统一的 CommandResult。
// 客户端直接跑 appcenter-cli、root agent 也跑同一条命令，两边各写一份归类逻辑时
// Missing/Timeout/OK 的语义很容易走偏（agent 侧还多带一个 Via），所以收敛到一处。
func CommandResultFromExec(err error, timedOut bool, cliName, via, stdout, stderr string) CommandResult {
	if errors.Is(err, exec.ErrNotFound) {
		if stderr == "" {
			stderr = cliName + " not found"
		}
		return CommandResult{Code: -1, Stdout: stdout, Stderr: stderr, Missing: true, Via: via}
	}
	if timedOut {
		return CommandResult{Code: -1, Stdout: stdout, Stderr: stderr, Timeout: true, Via: via}
	}
	result := CommandResult{OK: err == nil, Code: ExitCode(err), Stdout: stdout, Stderr: stderr, Via: via}
	var exitErr *exec.ExitError
	if err != nil && !errors.As(err, &exitErr) {
		// 进程根本没起来（chdir 失败、权限、信号等）：ExitCode 只会是 -1、
		// 输出为空，调用方拿到的是一个无法定位的 code=-1。把 err 原文带上，
		// 让日志和接口 detail 能看到真实原因。
		result.Error = err.Error()
	}
	return result
}

func (m *Manager) callAgent(command Command) CommandResult {
	return m.callAgentWithTimeout(command, m.settings.Timeout+socketDeadlineMargin)
}

func (m *Manager) callAgentWithTimeout(command Command, deadline time.Duration) CommandResult {
	if m.settings.AgentSocket == "" {
		return CommandResult{Unavailable: true, Error: "unavailable: agent not configured"}
	}
	conn, err := net.DialTimeout("unix", m.settings.AgentSocket, 2*time.Second)
	if err != nil {
		return CommandResult{Unavailable: true, Error: "unavailable: " + err.Error()}
	}
	defer conn.Close()
	_ = conn.SetDeadline(time.Now().Add(deadline))
	data, err := json.Marshal(command)
	if err != nil {
		return CommandResult{Error: "invalid command: " + err.Error()}
	}
	if _, err := io.WriteString(conn, string(data)+"\n"); err != nil {
		return CommandResult{Unavailable: true, Error: "unavailable: " + err.Error()}
	}
	// 从这里开始命令可能已经在 agent 上执行：读丢/解不出都不能标
	// Unavailable——调用方会据此回落直连，把同一次操作再执行一遍。
	response, err := io.ReadAll(conn)
	if err != nil {
		return CommandResult{Error: "agent response lost: " + err.Error()}
	}
	var result CommandResult
	if err := json.Unmarshal(response, &result); err != nil {
		return CommandResult{Error: "invalid agent response"}
	}
	return result
}

// IsChildAppName 判断 appName 是否是主应用的子应用（形如 mainAppname-xxx）。
// 客户端与 root agent 必须共用这一条规则：两边各写一份正则，一旦漂移就会出现
// "客户端放行、agent 拒绝"，或者更糟的反向放行。
func IsChildAppName(mainAppname, appName string) bool {
	if mainAppname == "" {
		return false
	}
	pattern := `^` + regexp.QuoteMeta(mainAppname) + `-[a-z0-9][a-z0-9-]{0,63}$`
	return regexp.MustCompile(pattern).MatchString(appName)
}

func (m *Manager) appNameAllowed(appName string) bool {
	return IsChildAppName(m.settings.MainAppname, appName)
}

func ParseAppCenterList(output string) map[string]InstalledApp {
	apps := map[string]InstalledApp{}
	for _, line := range strings.Split(output, "\n") {
		if !strings.Contains(line, "│") && !strings.Contains(line, "|") {
			continue
		}
		cells := strings.Split(strings.ReplaceAll(line, "│", "|"), "|")
		if len(cells) < 6 {
			continue
		}
		appName := strings.TrimSpace(cells[1])
		if appName == "" || appName == "APP NAME" {
			continue
		}
		// 首个字符必须是字母或数字：ASCII 表格的分隔行（|----|----|…）
		// 会落进 cells[1]，纯标点的「名字」不是应用。表头 APP NAME 已在上一条挡掉。
		if !regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._-]{0,63}$`).MatchString(appName) {
			continue
		}
		apps[appName] = InstalledApp{
			AppName:     appName,
			DisplayName: strings.TrimSpace(cells[2]),
			Version:     strings.TrimSpace(cells[3]),
			Status:      strings.ToLower(strings.TrimSpace(cells[4])),
			Dependency:  strings.TrimSpace(cells[5]),
		}
	}
	return apps
}

// ExitCode 取命令退出码：成功为 0，非零退出返回真实退出码，其它情况（启动失败、
// 超时被杀）一律 -1。客户端与 agent 共用这一份，之前 agent 侧少了 nil 判断，
// 成功时日志里会打出 exitCode=-1 而结果又是 OK，自相矛盾。
func ExitCode(err error) int {
	if err == nil {
		return 0
	}
	var exitErr *exec.ExitError
	if errors.As(err, &exitErr) {
		return exitErr.ExitCode()
	}
	return -1
}

func ListInstalledAppNames(appRoot string) []string {
	entries, err := os.ReadDir(appRoot)
	if err != nil {
		return []string{}
	}
	names := []string{}
	for _, entry := range entries {
		// 只有目录才算「已安装应用」：文件和隐藏项混进来会污染卸载核对，
		// 一个 README 文件会被当成待卸载的应用名。
		if !entry.IsDir() || strings.HasPrefix(entry.Name(), ".") {
			continue
		}
		names = append(names, entry.Name())
	}
	return names
}

func Wait(condition func() bool, attempts int, delay time.Duration) bool {
	for i := 0; i < attempts; i++ {
		if condition() {
			return true
		}
		time.Sleep(delay)
	}
	return condition()
}

// FormatOutput 截取输出尾部用于提示与日志。按 rune 截：按字节截会切开
// 中文字符，产出非法 UTF-8；max 做下限保护，负数不再越界 panic。
func FormatOutput(result CommandResult, max int) string {
	output := strings.TrimSpace(fmt.Sprintf("%s\n%s", result.Stdout, result.Stderr))
	if max < 0 {
		max = 0
	}
	runes := []rune(output)
	if len(runes) <= max {
		return output
	}
	return string(runes[len(runes)-max:])
}
