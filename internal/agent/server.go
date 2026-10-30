package agent

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/chens-hi/shortcuts/internal/config"
	"github.com/chens-hi/shortcuts/internal/deployer"
	"github.com/chens-hi/shortcuts/internal/logger"
	"github.com/chens-hi/shortcuts/internal/model"
	"github.com/chens-hi/shortcuts/internal/unixsock"
)

// maxRequestFrame 限制一帧请求的大小：能连上 socket 的对端可以是任何进程，
// 不能让它无限发数据把 root 进程的内存撑爆。
const maxRequestFrame = 64 << 10

// maxConcurrentCLI 限制并发执行的 root CLI 数量，超出的请求排队等待。
const maxConcurrentCLI = 4

// socketDeadlineMargin 在 CLI 超时之外再留一点余量，让客户端先收到命令超时的
// 结果帧，而不是连接被直接切断。
const socketDeadlineMargin = 10 * time.Second

type Server struct {
	cfg config.Config
	log *logger.Logger
	sem chan struct{}
}

// 请求协议直接复用 deployer.Command：两端同一份定义，字段不会各自漂移。
func NewServer() *Server {
	cfg := config.New()
	return &Server{cfg: cfg, log: logger.New(), sem: make(chan struct{}, maxConcurrentCLI)}
}

func (s *Server) Run() error {
	if err := os.MkdirAll(filepath.Dir(s.cfg.AgentSocket), 0755); err != nil {
		return err
	}
	if err := unixsock.RemoveStale(s.cfg.AgentSocket); err != nil {
		return err
	}
	listener, err := net.Listen("unix", s.cfg.AgentSocket)
	if err != nil {
		return err
	}
	if err := os.Chmod(s.cfg.AgentSocket, 0660); err != nil {
		return err
	}
	if s.cfg.TrimUID >= 0 && s.cfg.TrimGID >= 0 {
		// chown 失败不能当没事：socket 停在 root:root 0660 时非特权的服务端
		// 永远连不上，之后每个操作都静默降级成直连 CLI，没人知道为什么。
		if err := chownSocket(s.cfg.AgentSocket, s.cfg.TrimUID, s.cfg.TrimGID); err != nil {
			s.log.Warn("chown agent socket failed, unprivileged client may not connect",
				"socket", s.cfg.AgentSocket, "err", err.Error())
		}
	}
	s.log.Info("deploy agent listening", "socket", s.cfg.AgentSocket, "uid", systemRunUID(), "cli", s.cfg.AppCenterCLI)
	for {
		conn, err := listener.Accept()
		if err != nil {
			// EMFILE 之类的瞬时错误不该让整个 root 部署通道退场。
			var ne net.Error
			if errors.As(err, &ne) && ne.Temporary() {
				s.log.Warn("agent accept temporary error", "err", err.Error())
				time.Sleep(100 * time.Millisecond)
				continue
			}
			return err
		}
		go s.handle(conn)
	}
}

func (s *Server) handle(conn net.Conn) {
	defer conn.Close()
	_ = conn.SetDeadline(time.Now().Add(s.cfg.CLITimeout + socketDeadlineMargin))
	// 限长读取：超过 maxRequestFrame 还没有换行就直接断开。
	line, err := bufio.NewReader(io.LimitReader(conn, maxRequestFrame)).ReadString('\n')
	if err != nil {
		return
	}
	var req deployer.Command
	if err := json.Unmarshal([]byte(line), &req); err != nil {
		s.log.Warn("agent invalid json request", "error", err.Error())
		s.reply(conn, deployer.CommandResult{OK: false, Error: "invalid json"})
		return
	}
	// 入参来自对端，值里的换行能伪造日志行，清洗后再记。
	s.log.Info("agent request received", "action", req.Action,
		"dir", logger.SanitizeOutput(req.Dir, 200), "appname", logger.SanitizeOutput(req.AppName, 200))
	// 并发封顶：每个请求都会拉起一条 root CLI，没有上限就是资源耗尽点。
	// stat-dir 不启动任何 CLI（只 os.Stat），也不占并发槽：路径校验走保存链路，
	// 一旦被卡住的 appcenter 命令占满槽位，保存会被拖到 CLI 超时（分钟级）才反馈。
	if req.Action != "stat-dir" {
		s.sem <- struct{}{}
		defer func() { <-s.sem }()
	}
	s.reply(conn, s.dispatch(req))
}

func (s *Server) dispatch(req deployer.Command) deployer.CommandResult {
	switch req.Action {
	case "ping":
		return deployer.CommandResult{OK: true, Via: "agent", Stdout: s.cfg.AgentSocket, Code: 0}
	case "list":
		return s.run([]string{"list"}, "")
	case "install":
		// install-local 安装的是当前工作目录，不接路径参数。
		// 之前的 build-fpk / install-fpk 两个动作是"先把目录打包成 fpk 再安装"
		// 那条废弃路径留下的，其中 install-fpk 完全没有校验入参，已随整条链删除。
		//
		// root 边界：必须执行校验后解析出来的路径。此前校验用 EvalSymlinks
		// 的结果、执行却仍用原始 req.Dir，校验和执行之间 build/<child> 被换成
		// 软链就能把 root 引到 build 目录之外。
		resolved, ok := s.resolveInstallDir(req.Dir)
		if !ok {
			return deployer.CommandResult{OK: false, Via: "agent", Error: "install dir not allowed"}
		}
		return s.run([]string{"install-local"}, resolved)
	case "start", "stop", "uninstall":
		if !s.appNameAllowed(req.AppName) {
			return deployer.CommandResult{OK: false, Via: "agent", Error: "appname not allowed"}
		}
		return s.run([]string{req.Action, req.AppName}, "")
	case "stat-dir":
		return statDir(req.Dir)
	default:
		return deployer.CommandResult{OK: false, Via: "agent", Error: "unknown action"}
	}
}

// statDir 校验一个目录真实存在且是目录。只用 os.Stat，不执行任何命令：
// root 进程读 /vol1 共享目录不受降权用户限制。路径格式沿用
// model.IsValidFolderPath，与 server 侧保存校验共用同一条规则，避免两边
// 各自一份正则漂移。
func statDir(path string) deployer.CommandResult {
	if !model.IsValidFolderPath(path) {
		return deployer.CommandResult{OK: false, Via: "agent", Error: "stat dir path not allowed"}
	}
	info, err := os.Stat(path)
	if err != nil || !info.IsDir() {
		return deployer.CommandResult{OK: false, Via: "agent", Error: "path does not exist or is not a directory"}
	}
	return deployer.CommandResult{OK: true, Via: "agent", Stdout: path}
}

func (s *Server) run(args []string, cwd string) deployer.CommandResult {
	ctx, cancel := context.WithTimeout(context.Background(), s.cfg.CLITimeout)
	defer cancel()
	cmd := exec.CommandContext(ctx, s.cfg.AppCenterCLI, args...)
	if cwd != "" {
		// 用解析后校验过的真实路径直接作为工作目录。
		// 之前为防 TOCTOU 用 openDirFD + /proc/self/fd/3 作 chdir 目标：
		// 设备上报 install-local code=-1 空输出（命令根本没起来），chdir
		// 到 /proc/self/fd/3 在 fnOS 容器环境不可靠。而能利用该竞态的人
		// 本就有 build/ 的写权限，直接改写 manifest 即可，fd 方案的边际
		// 收益不值得拿稳定性换。路径校验（resolveInstallDir）仍然保留，
		// 执行的始终是校验后的路径，不是调用方原始入参。
		cmd.Dir = cwd
	}
	var stdout, stderr strings.Builder
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	err := cmd.Run()
	// 超时只在命令确实失败时判定：命令成功、ctx 又恰好过期时不能把成功结果
	// 改判成 Timeout。
	timedOut := err != nil && errors.Is(ctx.Err(), context.DeadlineExceeded)
	// 命令没能启动时（chdir 失败、权限、信号等）只记 exitCode 会把真实原因
	// 丢掉，UI 上只能看到一个 code=-1——把 err 原文也留下来。
	if err != nil {
		s.log.Warn("agent command failed", "args", strings.Join(args, " "), "cwd", cwd,
			"timeout", timedOut, "err", err.Error())
	}
	s.log.Info("agent command finished", "args", strings.Join(args, " "), "cwd", cwd, "exitCode", deployer.ExitCode(err), "timeout", timedOut)
	s.log.Info("agent command stdout", "stdout", logger.SanitizeOutput(stdout.String(), 8000))
	if stderr.String() != "" {
		s.log.Warn("agent command stderr", "stderr", logger.SanitizeOutput(stderr.String(), 4000))
	}
	return deployer.CommandResultFromExec(err, timedOut, s.cfg.AppCenterCLI, "agent", stdout.String(), stderr.String())
}

// resolveInstallDir 校验安装目录并返回真正要执行的路径。
// 只允许 build 目录下一层、且名字符合子应用命名规则的目录。
func (s *Server) resolveInstallDir(dir string) (string, bool) {
	base, err := filepath.EvalSymlinks(s.cfg.BuildDir)
	if err != nil {
		return "", false
	}
	abs, err := filepath.Abs(dir)
	if err != nil {
		return "", false
	}
	// 目标目录可能还没创建，EvalSymlinks 会失败，此时解析父目录再拼回名字。
	// 只有「确实不存在、且不是软链」才允许这样拼：悬空软链也走这条路的话，
	// 按链接名就能通过校验，与「目录存在就必须解析它自身」的规则相悖。
	resolved, err := filepath.EvalSymlinks(abs)
	if err != nil {
		if !os.IsNotExist(err) {
			return "", false
		}
		if fi, lerr := os.Lstat(abs); lerr == nil && fi.Mode()&os.ModeSymlink != 0 {
			return "", false
		}
		parent, parentErr := filepath.EvalSymlinks(filepath.Dir(abs))
		if parentErr != nil {
			return "", false
		}
		resolved = filepath.Join(parent, filepath.Base(abs))
	}
	if filepath.Dir(resolved) != base {
		return "", false
	}
	if !s.appNameAllowed(filepath.Base(resolved)) {
		return "", false
	}
	return resolved, true
}

// appNameAllowed 复用 deployer 里的规则，避免 root agent 与客户端各维护一份正则。
func (s *Server) appNameAllowed(appName string) bool {
	return deployer.IsChildAppName(s.cfg.MainAppname, appName)
}

func (s *Server) reply(conn net.Conn, result deployer.CommandResult) {
	data, err := json.Marshal(result)
	if err != nil {
		// 回包序列化失败要留痕：否则客户端只看到连接关闭，无从分辨原因。
		s.log.Warn("agent reply marshal failed", "err", err.Error())
		data = []byte(`{"OK":false,"Error":"reply marshal failed"}`)
	}
	if _, err := conn.Write(append(data, '\n')); err != nil {
		s.log.Warn("agent reply write failed", "err", err.Error())
	}
}
