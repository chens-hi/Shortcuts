package service

import (
	"encoding/json"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/chens-hi/shortcuts/internal/config"
	"github.com/chens-hi/shortcuts/internal/deployer"
	"github.com/chens-hi/shortcuts/internal/icons"
	"github.com/chens-hi/shortcuts/internal/logger"
	"github.com/chens-hi/shortcuts/internal/model"
	"github.com/chens-hi/shortcuts/internal/packagebuilder"
	"github.com/chens-hi/shortcuts/internal/provider"
	"github.com/chens-hi/shortcuts/internal/repository"
)

type EntryInput struct {
	Type            model.EntryType `json:"type"`
	AppName         string          `json:"appname"`
	DisplayName     string          `json:"displayName"`
	Description     string          `json:"description"`
	DesiredState    string          `json:"desiredState"`
	Config          json.RawMessage `json:"config"`
	Protocol        string          `json:"protocol"`
	Port            string          `json:"port"`
	Path            string          `json:"path"`
	URL             string          `json:"url"`
	OpenMode        string          `json:"openMode"`
	Icon64Data      string          `json:"icon64Data"`
	Icon256Data     string          `json:"icon256Data"`
	RemoveIcon      bool            `json:"removeIcon"`
	DisplayIcon     string          `json:"displayIcon"`
	DeployedVersion string          `json:"deployedVersion"`
	Version         string          `json:"version"`
	// Provided 记录本次请求中"显式出现"的字段名。
	// 它用来区分"未提供"（更新时保留旧值）和"显式提供"（含空字符串，表示清空）。
	// 由请求解析层（inputFromQuery / mergeBodyInput）填充「显式提供」的字段名。
	Provided map[string]bool `json:"-"`
}

// provided 判断指定字段是否在本次请求中显式出现。
// 不做「字段非空即视为已提供」的回退：那会把「没提供」和「提供空值」混为一谈，
// 正是字段合并语义里最容易出错的地方。该回退在生产路径从不触发，
// 只是让测试里直接构造 EntryInput 时少写一行。
func (in EntryInput) provided(field string) bool {
	return in.Provided[field]
}

type SaveResult struct {
	Entry   model.Entry
	Changed bool
	// Created 为 true 表示这次是新建（HTTP 层据此返回 201，更新返回 200）。
	// 以前无论新建还是更新都固定返回 201，状态码不反映实际动作。
	Created bool
}

// DeleteResult 描述一次删除的结果。
// Entries 是不含被删条目的新列表；Child 是随之尝试卸载的子应用名。
// StillInstalled 为 true 表示配置已删除但子应用未能随之卸载成功，
// UninstallDetail 记录卸载失败时的诊断输出，供调用方向用户解释残留原因。
type DeleteResult struct {
	Entries         []model.Entry
	Child           string
	StillInstalled  bool
	UninstallDetail string
}

type ActionResult struct {
	Action              string      `json:"action,omitempty"`
	AppName             string      `json:"appName,omitempty"`
	Code                string      `json:"code,omitempty"`
	Via                 string      `json:"via,omitempty"`
	Output              string      `json:"output,omitempty"`
	Deployed            bool        `json:"deployed,omitempty"`
	ReplacedExisting    bool        `json:"replacedExisting,omitempty"`
	Disabled            bool        `json:"disabled,omitempty"`
	RestoredStopped     bool        `json:"restoredStopped,omitempty"`
	VerificationWarning bool        `json:"verificationWarning,omitempty"`
	Entry               model.Entry `json:"entry,omitempty"`
}

type Service struct {
	Config            config.Config
	Repo              repository.EntryRepository
	Providers         *provider.Registry
	Builder           *packagebuilder.Builder
	Deployer          deployer.AppCenter
	Icons             *icons.Service
	Log               *logger.Logger
	PollDelay         time.Duration
	InstallAttempts   int
	UninstallAttempts int
	childBusy         map[string]bool
	mu                sync.Mutex
}

// NewService 组装服务。iconSvc 由调用方传入：打包器和服务必须共用同一个实例，
// 否则两处各自按 cfg 建一个，配置一改就可能各自指向不同的目录。
func NewService(cfg config.Config, repo repository.EntryRepository, providers *provider.Registry, builder *packagebuilder.Builder, dep deployer.AppCenter, iconSvc *icons.Service) *Service {
	cfg = cfg.WithResolvedPaths()
	return &Service{
		Config:            cfg,
		Repo:              repo,
		Providers:         providers,
		Builder:           builder,
		Deployer:          dep,
		Log:               logger.New(),
		Icons:             iconSvc,
		PollDelay:         time.Second,
		InstallAttempts:   120,
		UninstallAttempts: 30,
		childBusy:         map[string]bool{},
	}
}

// withRepoWrite 把 Load → 修改 → Save 的临界区串行化。
//
// 每条写入路径都是"整份读改写"：没有这把锁时，一个耗时卸载的删除请求
// 还没落盘，另一个请求就用自己 Load 到的旧快照整份保存，把刚删的条目
// 又写回文件（丢失更新）。锁只覆盖读写文件本身，绝不包住耗时的安装或
// 卸载命令 —— 否则慢命令会堵住所有写请求。
//
// 注意 s.mu 同时被 claimChild / releaseChild 使用；两者持锁顺序一致
// （都是直接获取，不嵌套），因此不会死锁。apply 内可以调用只读的
// Repo.Load 和 validateAgainst，但不能再取 mu（sync.Mutex 不可重入）。
func (s *Service) withRepoWrite(apply func(entries []model.Entry) ([]model.Entry, error)) ([]model.Entry, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	entries, err := s.Repo.Load()
	if err != nil {
		return nil, err
	}
	next, err := apply(entries)
	if err != nil {
		return nil, err
	}
	if err := s.Repo.Save(next); err != nil {
		return nil, err
	}
	return next, nil
}

func (s *Service) ListEntries() ([]model.Entry, error) {
	entries, err := s.Repo.Load()
	if err != nil {
		return nil, err
	}
	apps, err := s.installedApps()
	if err != nil {
		return nil, err
	}
	for i := range entries {
		entries[i] = decorate(entries[i], apps, s.Config.MainAppname)
	}
	return entries, nil
}

func (s *Service) CreateEntry(input EntryInput) (SaveResult, error) {
	entry, err := s.normalizeInput(input, nil)
	if err != nil {
		return SaveResult{}, err
	}
	if strings.TrimSpace(entry.AppName) == "" {
		return SaveResult{}, model.Errorf(model.ErrValidation, "appname is required")
	}
	entry = entry.Normalize()
	// 版本号由服务端定：以前允许调用方自带 version/deployedVersion 直接入库，
	// 伪造 deployedVersion == version 就能扭曲"是否需要重新部署"的判定。
	entry.Version = model.BumpVersion("")
	entry.DeployedVersion = ""
	iconProvided := input.Icon64Data != "" || input.Icon256Data != ""
	var saved model.Entry
	if _, err := s.withRepoWrite(func(entries []model.Entry) ([]model.Entry, error) {
		if err := s.validateAgainst(entries, entry, ""); err != nil {
			return nil, err
		}
		if iconProvided {
			if _, err := s.Icons.Save(entry.AppName, input.Icon64Data, input.Icon256Data); err != nil {
				return nil, err
			}
			entry.DisplayIcon = s.iconBasePath() + entry.AppName
		}
		saved = entry
		return append(entries, entry), nil
	}); err != nil {
		return SaveResult{}, err
	}
	apps, _ := s.installedApps()
	return SaveResult{Entry: decorate(saved, apps, s.Config.MainAppname), Changed: true, Created: true}, nil
}

func (s *Service) SaveEntry(input EntryInput) (SaveResult, error) {
	appName := strings.TrimSpace(input.AppName)
	if appName == "" {
		return s.CreateEntry(input)
	}
	existing, err := s.Repo.Load()
	if err != nil {
		return SaveResult{}, err
	}
	if _, ok := findEntry(existing, appName); ok {
		return s.UpdateEntry(appName, input)
	}
	return s.CreateEntry(input)
}

func (s *Service) UpdateEntry(appName string, input EntryInput) (SaveResult, error) {
	var entry model.Entry
	var changed bool
	if _, err := s.withRepoWrite(func(entries []model.Entry) ([]model.Entry, error) {
		old, ok := findEntry(entries, appName)
		if !ok {
			return nil, model.Errorf(model.ErrNotFound, "entry not found")
		}
		next, err := s.normalizeInput(input, &old)
		if err != nil {
			return nil, err
		}
		providerObj, err := s.Providers.Get(next.Type)
		if err != nil {
			return nil, err
		}
		configChanged := providerObj.Changed(old, next)
		iconProvided := input.Icon64Data != "" || input.Icon256Data != ""
		c := configChanged || iconProvided || input.RemoveIcon
		if c {
			next.Version = model.BumpVersion(old.Version)
		}
		next.DeployedVersion = old.DeployedVersion
		// displayIcon 的取值已在 normalizeInput 里定好（显式提供则用新值，
		// 否则保留旧值），这里不再做"空则回落旧值"——那会把显式清空又填回去。
		// 先只把 displayIcon 字段算对，磁盘上的图标文件等校验通过后再动。
		// 之前是先写/删图标再校验，一旦校验失败就留下"图标已经换了、条目没保存"
		// 的错位状态，界面上看到的是新图标，下次部署用的却是旧配置。
		if iconProvided {
			next.DisplayIcon = s.iconBasePath() + appName
		} else if input.RemoveIcon {
			// displayIcon 的语义是"当前存在自定义图标"，删除就是清空。
			next.DisplayIcon = ""
		}
		if err := s.validateAgainst(entries, next, appName); err != nil {
			return nil, err
		}
		if iconProvided {
			if _, err := s.Icons.Save(appName, input.Icon64Data, input.Icon256Data); err != nil {
				return nil, err
			}
		} else if input.RemoveIcon {
			if _, err := s.Icons.Remove(appName); err != nil {
				return nil, err
			}
		}
		idx, ok := indexOfPointer(entries, appName)
		if !ok {
			return nil, model.Errorf(model.ErrNotFound, "entry not found")
		}
		entries[idx] = next
		entry = next
		changed = c
		return entries, nil
	}); err != nil {
		return SaveResult{}, err
	}
	apps, _ := s.installedApps()
	return SaveResult{Entry: decorate(entry, apps, s.Config.MainAppname), Changed: changed}, nil
}

// DeleteEntry 删除一条配置，并尽力卸载随之部署的子应用。
//
// 配置落盘和子应用卸载被拆成两段：落盘在写锁内先完成（快、原子），
// 卸载在锁外尽力执行（可能耗时数分钟）。这样卸载超时、网关断连甚至
// 进程崩溃都不会让配置删不掉 —— 旧实现先卸载后落盘，卸载一卡住文件
// 就没被改写，用户看到的就是"删了过一会又出现"。
// 卸载失败不返回 error，改由 StillInstalled / UninstallDetail 上报。
func (s *Service) DeleteEntry(appName string) (DeleteResult, error) {
	// 先解析出子应用名并占位。删除会卸载子应用、还会删掉它的构建目录，
	// 如果此刻该子应用正在部署安装，就会把正在装的应用卸掉、把正在写的目录删掉，
	// 最后留下一个界面上管不到的孤儿子应用。
	existing, err := s.Repo.Load()
	if err != nil {
		return DeleteResult{}, err
	}
	entry, ok := findEntry(existing, appName)
	if !ok {
		return DeleteResult{}, model.Errorf(model.ErrNotFound, "entry not found")
	}
	child := entry.ChildAppname(s.Config.MainAppname)
	// 新机制创建的 folder 条目从未部署过子应用，DeployedVersion 为空时不该
	// 发起一次注定失败的卸载；只有旧机制留下的子应用才需要随之清理。
	uninstallChild := child
	if entry.Type == model.EntryTypeFolder && entry.DeployedVersion == "" {
		uninstallChild = ""
	}
	if uninstallChild != "" {
		if !s.claimChild(uninstallChild) {
			return DeleteResult{}, model.Errorf(model.ErrDeploy, "子应用 %s 正在操作中，请稍后再试", uninstallChild)
		}
		defer s.releaseChild(uninstallChild)
	}

	updated, err := s.withRepoWrite(func(entries []model.Entry) ([]model.Entry, error) {
		idx, ok := indexOfPointer(entries, appName)
		if !ok {
			return nil, model.Errorf(model.ErrNotFound, "entry not found")
		}
		// 显式构造新切片，不复用原数组：append(entries[:idx], ...) 在
		// idx==0 时会与原切片共享底层数组，调用方再改写会互相影响。
		next := make([]model.Entry, 0, len(entries)-1)
		return append(append(next, entries[:idx]...), entries[idx+1:]...), nil
	})
	if err != nil {
		return DeleteResult{}, err
	}
	stillInstalled, detail := s.bestEffortUninstall(uninstallChild)
	// 图标文件跟着条目一起删：/icon/<appname> 是按名字直接读盘的，
	// 不删的话同名重建的条目会顶着上一条的图标，PNG 也会永久残留。
	if _, err := s.Icons.Remove(appName); err != nil {
		s.Log.Warn("icon cleanup failed after delete", "appname", appName, "err", err.Error())
	}
	// 构建目录清理是尽力而为。失败（例如子进程仍持有文件句柄）
	// 不应阻塞删除，但要记录日志，避免残留目录悄悄堆积。
	if err := os.RemoveAll(filepath.Join(s.Config.BuildDir, child)); err != nil {
		s.Log.Warn("build directory cleanup failed after delete", "appname", child, "err", err.Error())
	}
	apps, _ := s.installedApps()
	for i := range updated {
		updated[i] = decorate(updated[i], apps, s.Config.MainAppname)
	}
	return DeleteResult{Entries: updated, Child: uninstallChild, StillInstalled: stillInstalled, UninstallDetail: detail}, nil
}

// bestEffortUninstall 尽力卸载一个子应用，永不中止调用方。
// Stop 失败也要继续尝试 Uninstall：应用可能已经停了但命令返回非零，
// 直接中止会留下一个半卸载的应用。
func (s *Service) bestEffortUninstall(child string) (stillInstalled bool, detail string) {
	if child == "" {
		return false, ""
	}
	// 查不到列表时不能当成「没装」直接跳过卸载（那正是降级 map 时代的错），
	// 照常尝试卸载，最终状态以轮询结果为准。
	apps, listErr := s.installedApps()
	if listErr == nil {
		if _, installed := apps[child]; !installed {
			return false, ""
		}
	} else {
		s.Log.Warn("list installed apps failed before uninstall, trying anyway", "child", child, "err", listErr.Error())
	}
	stopResult := s.Deployer.Stop(child)
	if !stopResult.OK {
		s.Log.Warn("stop child before uninstall failed", "child", child, "output", deployer.FormatOutput(stopResult, 1000))
	}
	uninstallResult := s.Deployer.Uninstall(child)
	// 命令返回非零不等于没卸掉（Stop 之后立刻 Uninstall 常返回非零），
	// 真实状态以轮询为准；轮询也确认还在，才报 stillInstalled。
	if !s.waitForUninstall(child) {
		detail = deployer.FormatOutput(uninstallResult, 2000)
		if uninstallResult.OK {
			detail = "uninstall command returned success, but child app still exists"
		}
		s.Log.Warn("child app still installed after delete", "child", child, "detail", detail)
		return true, detail
	}
	return false, ""
}

func (s *Service) DeployEntry(appName string) (ActionResult, error) {
	entries, err := s.Repo.Load()
	if err != nil {
		return ActionResult{}, err
	}
	idx, ok := indexOfPointer(entries, appName)
	if !ok {
		return ActionResult{}, model.Errorf(model.ErrNotFound, "entry not found")
	}
	entry := entries[idx]
	// 文件夹快捷方式由桌面端创建，服务端没有可安装的子应用。
	// 必须在 claimChild 之前返回，否则会留下一个永远不会释放的占位。
	if entry.Type == model.EntryTypeFolder {
		return ActionResult{}, model.Errorf(model.ErrValidation, "folder shortcuts are created on the desktop client")
	}
	child := entry.ChildAppname(s.Config.MainAppname)
	if !s.claimChild(child) {
		return ActionResult{AppName: child, Code: string(model.ErrDeploy), Output: "deploy already in progress"},
			model.Errorf(model.ErrDeploy, "deploy already in progress for %s", child)
	}
	defer s.releaseChild(child)

	// Already installed at the version this entry last recorded as deployed means
	// there is nothing to do. Skipping here also avoids rebuilding the child
	// package; without it "deploy all" rebuilt and reinstalled every shortcut on
	// every call, which is what pushed that path past the client timeout.
	apps, _ := s.installedApps()
	if version, ok := s.childAtCurrentVersion(apps, child, entry); ok {
		s.Log.Info("deploy skipped, already installed", "child", child, "version", version)
		// Code 让前端能区分「真的执行了一次部署」和「本来就已经是最新版本」：
		// 两者都是 Deployed=true，只看得懂 Deployed 的话，用户点部署会看到
		// 「已部署」却什么都没发生。
		return ActionResult{
			AppName:  child,
			Deployed: true,
			Code:     "DEPLOY_SKIPPED",
			Output:   "already deployed at " + version,
			Entry:    decorate(entry, apps, s.Config.MainAppname),
		}, nil
	}
	spec, err := s.spec(entry)
	if err != nil {
		return ActionResult{}, err
	}
	// 部署被关掉时不必构建：早先这个检查放在 Build 之后，等于每次都白拼一整个包目录。
	if s.Config.DeployDisabled {
		return ActionResult{AppName: child, Code: string(model.ErrDeployDisabled), Disabled: true}, nil
	}
	s.Log.Info("deploy start", "appname", appName, "type", entry.Type, "version", entry.Version)
	built, err := s.Builder.Build(entry, spec)
	if err != nil {
		s.Log.Error("build failed", "appname", appName, "err", err.Error())
		return ActionResult{}, err
	}
	s.Log.Info("build done", "dir", built.Dir, "appName", built.AppName)
	// 除"安装超时"以外，任何出口都要清掉这个包目录：构建失败之外的失败路径
	// （安装报错、未被 appcenter 列出）以前都会把整包留在 build/ 里。
	// 超时则刻意保留，因为 appcenter-cli 可能还在后台从该目录拷贝文件。
	installTimedOut := false
	defer func() {
		if !installTimedOut {
			_ = os.RemoveAll(built.Dir)
		}
	}()
	// Reload: the list above was taken before the build, and the build can take
	// a while, so the child may have appeared or disappeared in the meantime.
	apps, _ = s.installedApps()
	replacedExisting := false
	if _, exists := apps[built.AppName]; exists {
		s.Log.Info("replacing existing child app", "appName", built.AppName)
		_ = s.Deployer.Stop(built.AppName)
		removeResult := s.Deployer.Uninstall(built.AppName)
		// 旧子应用没真卸掉就往上装，appcenter 里会同时存在新旧两份，
		// 而安装后的验证只查"名字在列表里"，会把残留的旧包当成部署成功
		// 并写入 DeployedVersion。所以这里以轮询到的真实状态为准，还在就中止。
		if !s.waitForUninstall(built.AppName) {
			s.Log.Warn("uninstall old child failed, aborting replace",
				"appName", built.AppName, "output", deployer.FormatOutput(removeResult, 2000))
			return ActionResult{}, model.Errorf(model.ErrDeploy,
				"旧子应用 %s 卸载失败，已中止替换：%s", built.AppName, deployer.FormatOutput(removeResult, 2000))
		}
		replacedExisting = true
		s.Log.Info("uninstall old child done", "ok", replacedExisting)
	}
	// Install the generated package directory directly with appcenter-cli.
	s.Log.Info("installing local package dir", "dir", built.Dir)
	result := s.Deployer.InstallLocal(built.Dir)
	installTimedOut = result.Timeout
	s.Log.Info("install-local result",
		"ok", result.OK, "timeout", result.Timeout, "code", result.Code, "via", result.Via,
		"missing", result.Missing,
		"stdoutLen", len(result.Stdout), "stderrLen", len(result.Stderr))
	s.Log.Info("install-local stdout+stderr (full)", "output", logger.SanitizeOutput(result.Stdout+"\n"+result.Stderr, 8000))
	output := deployer.FormatOutput(result, 8000)
	if !result.OK && !result.Timeout {
		agentOK, _ := s.Deployer.Ping()
		s.Log.Error("install failed", "via", result.Via, "code", result.Code, "agentOK", agentOK, "output", output, "error", result.Error)
		detail := fmt.Sprintf("install-local via=%s code=%d output=%s", result.Via, result.Code, output)
		// 进程没能启动时 output 是空的，只靠 via/code 定位不了问题；
		// CommandResultFromExec 现在会把启动失败的真实 err 放进 Error。
		if result.Error != "" {
			detail += " err=" + result.Error
		}
		return ActionResult{}, model.Errorf(model.ErrDeploy, detail)
	}
	if result.Timeout {
		s.Log.Warn("install-local timed out, appcenter may still be installing in background")
	}
	// Only mark the entry deployed after appcenter-cli actually completes and fnOS lists
	// the child app. Timeout is a real failure state even if the client saw the request.
	verified := result.OK && s.waitForInstall(built.AppName)
	s.Log.Info("waitForInstall result", "verified", verified, "appName", built.AppName)
	installDirExists := false
	if childAppDir := filepath.Join(s.Config.AppInstallRoot, built.AppName); childAppDir != "" {
		installDirExists = dirExists(childAppDir)
	}
	s.Log.Info("child app dir check", "path", filepath.Join(s.Config.AppInstallRoot, built.AppName), "exists", installDirExists)
	appsAfter, _ := s.installedApps()
	s.Log.Info("appcenter list after install", "totalApps", len(appsAfter), "childInList", appsAfter[built.AppName].AppName != "")
	// Record the deployed version only after fnOS actually lists the child app.
	// Otherwise a slow or half-finished install would leave the UI showing a
	// version that was never installed, and the next deploy would skip it.
	if result.OK && verified {
		entry.DeployedVersion = entry.Version
		if _, err := s.withRepoWrite(func(cur []model.Entry) ([]model.Entry, error) {
			i, ok := indexOfPointer(cur, appName)
			if !ok {
				return nil, model.Errorf(model.ErrNotFound, "entry not found")
			}
			cur[i].DeployedVersion = entry.Version
			return cur, nil
		}); err != nil {
			return ActionResult{}, err
		}
		restoredStopped := false
		if entry.DesiredState == model.StateStopped {
			restoredStopped = s.Deployer.Stop(built.AppName).OK
		}
		apps, _ = s.installedApps()
		return ActionResult{
			Deployed:         true,
			AppName:          built.AppName,
			ReplacedExisting: replacedExisting,
			RestoredStopped:  restoredStopped,
			Code:             "DEPLOY_DONE",
			Via:              result.Via,
			Output:           output,
			Entry:            decorate(entry, apps, s.Config.MainAppname),
		}, nil
	}

	// 只有真的超时才用超时码。安装命令成功、但 appcenter 始终没列出子应用是另一种
	// 状态，早先一律写成 ErrTimeout，前端提示和实际原因对不上。
	failCode := "DEPLOY_UNVERIFIED"
	if result.Timeout {
		failCode = string(model.ErrTimeout)
	}
	s.Log.Warn("deploy failed",
		"appName", built.AppName, "code", failCode, "timeout", result.Timeout, "verified", verified,
		"installDirExists", installDirExists, "appRoot", s.Config.AppInstallRoot)
	return ActionResult{
		Deployed:            false,
		AppName:             built.AppName,
		Code:                failCode,
		Via:                 result.Via,
		Output:              withInstallPath(output, s.Config.AppInstallRoot, built.AppName, installDirExists),
		VerificationWarning: true,
	}, nil
}

func dirExists(path string) bool {
	info, err := os.Stat(path)
	return err == nil && info.IsDir()
}

func withInstallPath(output string, appRoot string, appName string, dirExists bool) string {
	appDir := filepath.Join(appRoot, appName)
	detail := fmt.Sprintf("child app dir %s exists=%v", appDir, dirExists)
	return strings.TrimSpace(output + "\n" + detail)
}

// claimChild 为某个子应用占位，表示"此刻正在对它做操作"。
// 部署会一直阻塞在 install 轮询上，删除/卸载/启停/预览都可能耗时数十秒，
// 两个操作叠在同一个子应用上就会互相拆台：典型的是部署装到一半被删除，
// 子应用被卸掉、构建目录被删掉，最后剩一个界面上管不到的孤儿。
// 占位失败说明已有别的操作在进行，调用方应直接拒绝而不是并行去做。
func (s *Service) claimChild(childAppname string) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.childBusy == nil {
		s.childBusy = map[string]bool{}
	}
	if childAppname == "" || s.childBusy[childAppname] {
		return false
	}
	s.childBusy[childAppname] = true
	return true
}

func (s *Service) releaseChild(childAppname string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.childBusy, childAppname)
}

func (s *Service) SetRuntime(appName, action string) (ActionResult, error) {
	entries, err := s.Repo.Load()
	if err != nil {
		return ActionResult{}, err
	}
	idx, ok := indexOfPointer(entries, appName)
	if !ok {
		return ActionResult{}, model.Errorf(model.ErrNotFound, "entry not found")
	}
	child := entries[idx].ChildAppname(s.Config.MainAppname)
	if child != "" {
		if !s.claimChild(child) {
			return ActionResult{}, model.Errorf(model.ErrDeploy, "子应用 %s 正在操作中，请稍后再试", child)
		}
		defer s.releaseChild(child)
	}
	var result deployer.CommandResult
	if action == "start" {
		result = s.Deployer.Start(child)
	} else if action == "stop" {
		result = s.Deployer.Stop(child)
	} else {
		return ActionResult{}, model.Errorf(model.ErrValidation, "invalid action")
	}
	if !result.OK {
		return ActionResult{}, model.Errorf(model.ErrRuntime, deployer.FormatOutput(result, 4000))
	}
	// Start/Stop 在锁外执行；只有 DesiredState 的落盘需要写锁。
	updated := entries[idx]
	if _, err := s.withRepoWrite(func(cur []model.Entry) ([]model.Entry, error) {
		i, ok := indexOfPointer(cur, appName)
		if !ok {
			return nil, model.Errorf(model.ErrNotFound, "entry not found")
		}
		if action == "stop" {
			cur[i].DesiredState = model.StateStopped
		} else {
			cur[i].DesiredState = model.StateRunning
		}
		updated = cur[i]
		return cur, nil
	}); err != nil {
		return ActionResult{}, err
	}
	apps, _ := s.installedApps()
	// 过去式不要用 action+"ed" 拼：stop 会拼成 "stoped"，接口返回的 action
	// 字段就带着这个错词。action 在上面已校验只有 start/stop 两种，写死映射。
	past := "started"
	if action == "stop" {
		past = "stopped"
	}
	return ActionResult{Action: past, AppName: child, Via: result.Via, Code: action + "_done", Entry: decorate(updated, apps, s.Config.MainAppname)}, nil
}

func (s *Service) UninstallEntry(appName string) (ActionResult, error) {
	entries, err := s.Repo.Load()
	if err != nil {
		return ActionResult{}, err
	}
	idx, ok := indexOfPointer(entries, appName)
	if !ok {
		return ActionResult{}, model.Errorf(model.ErrNotFound, "entry not found")
	}
	child := entries[idx].ChildAppname(s.Config.MainAppname)
	if child != "" {
		if !s.claimChild(child) {
			return ActionResult{}, model.Errorf(model.ErrDeploy, "子应用 %s 正在操作中，请稍后再试", child)
		}
		defer s.releaseChild(child)
	}
	stopResult := s.Deployer.Stop(child)
	uninstallResult := s.Deployer.Uninstall(child)
	if !uninstallResult.OK {
		code := "UNINSTALL_FAILED_MANUAL"
		if stopResult.OK {
			code = "UNINSTALL_STOPPED_FALLBACK"
		}
		return ActionResult{Action: code, AppName: child, Code: code, Via: uninstallResult.Via, Output: deployer.FormatOutput(uninstallResult, 2000)}, nil
	}
	if !s.waitForUninstall(child) {
		return ActionResult{Action: "uninstall-unverified", AppName: child, Code: "UNINSTALL_NOT_REMOVED", Output: deployer.FormatOutput(uninstallResult, 2000)}, nil
	}
	// 卸载命令在锁外完成；只有字段回写的落盘需要写锁。
	if _, err := s.withRepoWrite(func(cur []model.Entry) ([]model.Entry, error) {
		i, ok := indexOfPointer(cur, appName)
		if !ok {
			return nil, model.Errorf(model.ErrNotFound, "entry not found")
		}
		cur[i].DeployedVersion = ""
		cur[i].DesiredState = model.StateRunning
		return cur, nil
	}); err != nil {
		return ActionResult{}, err
	}
	return ActionResult{Action: "uninstalled", AppName: child, Code: "UNINSTALL_DONE", Output: deployer.FormatOutput(uninstallResult, 2000)}, nil
}

// BumpEntryVersion 递增一条配置的版本号，使已部署版本变为过时、
// 下次部署会重新构建子应用。图标上传后调用。
func (s *Service) BumpEntryVersion(appName string) error {
	_, err := s.withRepoWrite(func(cur []model.Entry) ([]model.Entry, error) {
		i, ok := indexOfPointer(cur, appName)
		if !ok {
			return nil, model.Errorf(model.ErrNotFound, "entry not found")
		}
		cur[i].Version = model.BumpVersion(cur[i].Version)
		return cur, nil
	})
	return err
}

func (s *Service) PackagePreview(appName string) (map[string]any, error) {
	entries, err := s.Repo.Load()
	if err != nil {
		return nil, err
	}
	entry, ok := findEntry(entries, appName)
	if !ok {
		return nil, model.Errorf(model.ErrNotFound, "entry not found")
	}
	// 预览会往部署用的同一个 build/<child> 目录里构建，必须和部署互斥，
	// 否则预览会把正在安装的目录整个清掉重建。
	child := entry.ChildAppname(s.Config.MainAppname)
	if child != "" {
		if !s.claimChild(child) {
			return nil, model.Errorf(model.ErrDeploy, "子应用 %s 正在操作中，请稍后再试", child)
		}
		defer s.releaseChild(child)
	}
	spec, err := s.spec(entry)
	if err != nil {
		return nil, err
	}
	built, err := s.Builder.Build(entry, spec)
	if err != nil {
		return nil, err
	}
	files, _ := listFiles(built.Dir)
	return map[string]any{
		"appName":  built.AppName,
		"dir":      built.Dir,
		"manifest": readFileText(filepath.Join(built.Dir, "manifest")),
		"uiConfig": readFileText(filepath.Join(built.Dir, "app", "ui", "config")),
		"files":    files,
	}, nil
}

func (s *Service) InstalledApps() map[string]deployer.InstalledApp {
	apps, _ := s.installedApps()
	return apps
}

func (s *Service) normalizeInput(input EntryInput, existing *model.Entry) (model.Entry, error) {
	applyEntryConfigQuery(&input)
	entryType := input.Type
	if entryType == "" {
		if existing != nil {
			entryType = existing.Type
		} else {
			entryType = model.EntryTypeInternalPort
		}
	}
	appName := strings.TrimSpace(input.AppName)
	if existing != nil {
		appName = existing.AppName
	}
	displayName := strings.TrimSpace(input.DisplayName)
	if displayName == "" && existing != nil {
		displayName = existing.DisplayName
	}
	// 更新时未显式提供 description 则保留旧值，
	// 避免"只改端口"这类部分更新把描述误清空。
	description := input.Description
	if existing != nil && !input.provided("description") {
		description = existing.Description
	}
	configRaw := input.Config
	if len(configRaw) == 0 && existing != nil {
		configRaw = existing.Config
	}
	entry := model.Entry{
		Type: entryType, Version: input.Version, AppName: appName, DisplayName: displayName,
		DisplayIcon: input.DisplayIcon, Description: description, DesiredState: input.DesiredState,
		DeployedVersion: input.DeployedVersion, Config: configRaw,
	}
	if existing != nil {
		entry.Version = existing.Version
		// displayIcon 只在本次没显式提供时保留旧值。以前这里无条件覆盖，
		// 更新时传 displayIcon 永远无效，UpdateEntry 里那条"空则回落旧值"
		// 的处理因此成了死代码。
		if !input.provided("displayIcon") {
			entry.DisplayIcon = existing.DisplayIcon
		}
		entry.DeployedVersion = existing.DeployedVersion
		// entryKey 只能由桌面端创建后回写，编辑名称/路径不能把它抹掉。
		entry.DesktopEntryKey = existing.DesktopEntryKey
	}
	// 更新时未显式提供 desiredState 则保留旧值，
	// 否则"已停止"的条目改个描述就会被静默改回运行中。
	if existing != nil && !input.provided("desiredState") {
		entry.DesiredState = existing.DesiredState
	}
	if entry.DesiredState == "" {
		entry.DesiredState = model.StateRunning
	}
	if entry.Version == "" {
		entry.Version = "1.0.0"
	}
	// 配置合并的优先级统一成一条：显式提供的顶层字段覆盖 config 里的同名字段。
	// 以前两条路径正好相反——external_url 以顶层 url 为准重建配置，internal_port
	// 却要求「没带 config」才接受顶层改动（带了就整段丢弃），同一个请求在两种
	// 类型上得到相反的结果。
	configFieldProvided := input.provided("protocol") || input.provided("port") ||
		input.provided("path") || input.provided("url") || input.provided("openMode")
	switch {
	case entry.Type == model.EntryTypeExternalURL && input.URL != "":
		merged, err := s.configFromInput(entry.Type, input)
		if err != nil {
			return model.Entry{}, err
		}
		entry.Config = merged
	case len(entry.Config) == 0:
		merged, err := s.configFromInput(entry.Type, input)
		if err != nil {
			return model.Entry{}, err
		}
		entry.Config = merged
	case configFieldProvided:
		merged, err := s.mergeConfigFields(entry.Type, entry.Config, input)
		if err != nil {
			return model.Entry{}, err
		}
		entry.Config = merged
	}
	return entry, nil
}

func applyEntryConfigQuery(input *EntryInput) {
	input.Config = json.RawMessage(strings.TrimSpace(string(input.Config)))
	if len(input.Config) == 0 || !json.Valid(input.Config) {
		return
	}
	var fields model.EntryConfigInput
	if err := json.Unmarshal(input.Config, &fields); err != nil {
		return
	}
	if input.Protocol == "" {
		input.Protocol = fields.Protocol
	}
	if input.Port == "" {
		input.Port = fields.Port
	}
	if input.Path == "" {
		input.Path = fields.Path
	}
	if input.URL == "" {
		input.URL = fields.URL
	}
	if input.OpenMode == "" {
		input.OpenMode = fields.OpenMode
	}
}

func (s *Service) configFromInput(entryType model.EntryType, input EntryInput) (json.RawMessage, error) {
	return model.ConfigFromInput(entryType, model.EntryConfigInput{
		Protocol: input.Protocol,
		Port:     input.Port,
		Path:     input.Path,
		URL:      input.URL,
		OpenMode: input.OpenMode,
	})
}

// mergeConfigFields 把本次显式提供的配置字段合并到旧配置上，未提供的字段保留旧值。
// 用于"只改一个字段"的部分更新：若直接按本次输入重建配置，
// 调用方没传的旧字段（如只改端口时未传的 protocol/path）会被丢掉。
func (s *Service) mergeConfigFields(entryType model.EntryType, existing json.RawMessage, input EntryInput) (json.RawMessage, error) {
	old := model.EntryConfigInput{}
	if len(existing) > 0 {
		if err := json.Unmarshal(existing, &old); err != nil {
			return nil, model.Errorf(model.ErrValidation, "invalid existing config: %v", err)
		}
	}
	next := old
	if input.provided("port") {
		next.Port = strings.TrimSpace(input.Port)
	}
	if input.provided("protocol") {
		next.Protocol = strings.TrimSpace(input.Protocol)
	}
	if input.provided("path") {
		next.Path = strings.TrimSpace(input.Path)
	}
	if input.provided("url") {
		next.URL = strings.TrimSpace(input.URL)
	}
	if input.provided("openMode") {
		next.OpenMode = strings.TrimSpace(input.OpenMode)
	}
	return model.ConfigFromInput(entryType, next)
}

func (s *Service) iconBasePath() string {
	prefix := s.Config.GatewayPath()
	if prefix == "" {
		prefix = "/"
	}
	if prefix == "/" {
		return "/icon/"
	}
	return prefix + "/icon/"
}

// validateAgainst 对调用方已经 Load 好的快照做校验，
// 避免在持写锁的临界区里再读一次盘。
func (s *Service) validateAgainst(entries []model.Entry, entry model.Entry, selfAppname string) error {
	for _, other := range entries {
		if other.AppName == selfAppname {
			continue
		}
		if other.AppName == entry.AppName {
			return model.Errorf(model.ErrValidation, "appname already exists")
		}
	}
	providerObj, err := s.Providers.Get(entry.Type)
	if err != nil {
		return err
	}
	if err := providerObj.Validate(entry); err != nil {
		return err
	}
	// 文件夹快捷方式的目标路径必须真实存在且是目录。格式校验由 provider 完成
	// （它无法访问 agent，且对 /vol1 共享目录无读权限、本地 stat 不可靠）；
	// 存在性必须走 root agent。agent 不可用时按 fail-closed 拒绝，不放行可能
	// 不存在的路径。这里在写锁内做一次快速 stat，agent 就绪时几乎瞬时完成。
	if entry.Type == model.EntryTypeFolder {
		folderConfig, err := provider.DecodeConfig[model.FolderConfig](entry.Config)
		if err != nil {
			return err
		}
		valid, _, err := s.ValidateFolderPath(folderConfig.Path)
		if err != nil {
			return model.Errorf(model.ErrRuntime, "cannot validate folder path: %s", err.Error())
		}
		if !valid {
			return model.Errorf(model.ErrValidation, "folder path does not exist or is not a directory")
		}
	}
	return nil
}

func (s *Service) spec(entry model.Entry) (provider.ApplicationSpec, error) {
	providerObj, err := s.Providers.Get(entry.Type)
	if err != nil {
		return provider.ApplicationSpec{}, err
	}
	return providerObj.Spec(entry, s.Config.MainAppname)
}

// installedApps 返回 appcenter 报出的已安装应用列表。
// 列表失败就是失败：这里曾经退回扫 AppInstallRoot 目录，得到一个没有版本、
// 没有状态的降级 map 并返回 nil error，于是所有 `apps, _ :=` 的调用方都把
// 「查不到」当成「没装」——删除条目时据此跳过卸载还报 StillInstalled=false。
func (s *Service) installedApps() (map[string]deployer.InstalledApp, error) {
	return s.Deployer.List()
}

func (s *Service) waitForInstall(appName string) bool {
	pollCount := 0
	return deployer.Wait(func() bool {
		apps, err := s.installedApps()
		pollCount++
		// 列表失败不等于「已经装好」：继续等，不要把查询失败判成成功。
		if err != nil {
			s.Log.Warn("waitForInstall list failed", "appName", appName, "attempt", pollCount, "err", err.Error())
			return false
		}
		_, ok := apps[appName]
		if pollCount <= 3 || pollCount%10 == 0 {
			s.Log.Info("waitForInstall poll", "appName", appName, "attempt", pollCount, "found", ok, "totalApps", len(apps))
		}
		return ok
	}, s.InstallAttempts, s.PollDelay)
}

func (s *Service) waitForUninstall(appName string) bool {
	pollCount := 0
	return deployer.Wait(func() bool {
		apps, err := s.installedApps()
		pollCount++
		// 列表失败不等于「已经卸干净」：以前这里把查询失败当成卸载成功，
		// 于是一个仍在的应用被记成已卸载。
		if err != nil {
			s.Log.Warn("waitForUninstall list failed", "appName", appName, "attempt", pollCount, "err", err.Error())
			return false
		}
		_, ok := apps[appName]
		if pollCount <= 3 || pollCount%5 == 0 {
			s.Log.Info("waitForUninstall poll", "appName", appName, "attempt", pollCount, "stillExists", ok)
		}
		return !ok
	}, s.UninstallAttempts, s.PollDelay)
}

// installedVersion 是"已装版本"的唯一取法：优先用 appcenter 报出来的版本，
// 取不到就回落到本地记录的 DeployedVersion。
// childAtCurrentVersion 与 decorate 以前各自写了一遍这个判定，只要改一处，
// "能不能跳过部署"的判断和界面上显示的状态就会对不上。
func installedVersion(installed deployer.InstalledApp, entry model.Entry) string {
	if version := strings.TrimSpace(installed.Version); version != "" {
		return version
	}
	return entry.DeployedVersion
}

// childAtCurrentVersion reports whether the child app is installed and already
// at the version the entry recorded as deployed.
func (s *Service) childAtCurrentVersion(apps map[string]deployer.InstalledApp, child string, entry model.Entry) (string, bool) {
	installed, ok := apps[child]
	if !ok {
		return "", false
	}
	version := installedVersion(installed, entry)
	if version == "" || version != entry.Version {
		return "", false
	}
	return version, true
}

func decorate(entry model.Entry, apps map[string]deployer.InstalledApp, mainApp string) model.Entry {
	child := entry.ChildAppname(mainApp)
	entry.ShortcutAppname = child
	// 文件夹快捷方式不再有子应用：状态只由桌面项标识决定，运行状态对它没有意义。
	if entry.Type == model.EntryTypeFolder {
		if strings.TrimSpace(entry.DesktopEntryKey) == "" {
			entry.Status = model.StatusUndeployed
		} else {
			entry.Status = model.StatusDeployed
		}
		entry.Runtime = model.RuntimeUnknown
		return entry
	}
	installed, ok := apps[child]
	if !ok {
		entry.Status = model.StatusUndeployed
		entry.Runtime = model.RuntimeUnknown
		return entry
	}
	version := installedVersion(installed, entry)
	if version == "" || version == entry.Version {
		entry.Status = model.StatusDeployed
	} else {
		entry.Status = model.StatusOutdated
	}
	switch installed.Status {
	case model.RuntimeRunning:
		entry.Runtime = model.RuntimeRunning
	case model.RuntimeStopped:
		entry.Runtime = model.RuntimeStopped
	default:
		entry.Runtime = model.RuntimeUnknown
	}
	entry.InstalledVersion = version
	entry.RawStatus = installed.Status
	return entry
}

// indexOfPointer 是按 appname 查找条目的唯一实现，需要就地修改时用它；
// findEntry 只是拿值的薄封装。此前三个函数各写一遍同样的线性查找。
func indexOfPointer(entries []model.Entry, appName string) (int, bool) {
	for i := range entries {
		if entries[i].AppName == appName {
			return i, true
		}
	}
	return -1, false
}

func findEntry(entries []model.Entry, appName string) (model.Entry, bool) {
	if i, ok := indexOfPointer(entries, appName); ok {
		return entries[i], true
	}
	return model.Entry{}, false
}

func listFiles(root string) ([]string, error) {
	files := []string{}
	err := filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(root, path)
		if err != nil {
			return err
		}
		if rel == "." {
			return nil
		}
		if d.IsDir() {
			files = append(files, rel+"/")
		} else {
			files = append(files, rel)
		}
		return nil
	})
	if err != nil {
		return files, err
	}
	sort.Strings(files)
	return files, nil
}

func readFileText(path string) string {
	data, err := os.ReadFile(path)
	if err != nil {
		return ""
	}
	return string(data)
}
