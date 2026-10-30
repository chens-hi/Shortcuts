package app

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/chens-hi/shortcuts/internal/appmeta"
	"github.com/chens-hi/shortcuts/internal/config"
	"github.com/chens-hi/shortcuts/internal/deployer"
	"github.com/chens-hi/shortcuts/internal/icons"
	"github.com/chens-hi/shortcuts/internal/logger"
	"github.com/chens-hi/shortcuts/internal/model"
	"github.com/chens-hi/shortcuts/internal/packagebuilder"
	"github.com/chens-hi/shortcuts/internal/provider"
	"github.com/chens-hi/shortcuts/internal/repository"
	"github.com/chens-hi/shortcuts/internal/service"
	"github.com/chens-hi/shortcuts/internal/unixsock"
)

type Server struct {
	Config      config.Config
	Svc         *service.Service
	router      *http.ServeMux
	AgentClient agentClient
	icons       *icons.Service
	log         *logger.Logger
}

type agentClient interface {
	Ping() (bool, deployer.CommandResult)
}

func NewServer() *Server {
	cfg := config.New()
	cfg = cfg.WithResolvedPaths()
	providerRegistry := provider.NewRegistry()
	providerRegistry.Register(provider.InternalPortProvider{})
	providerRegistry.Register(provider.ExternalURLProvider{})
	providerRegistry.Register(provider.FolderProvider{})
	repo := repository.NewJSONRepository(cfg.DataFile)
	iconService := &icons.Service{Dir: cfg.IconDir, DefaultDir: cfg.DefaultIconDir()}
	builder := &packagebuilder.Builder{
		BuildDir:    cfg.BuildDir,
		TemplateDir: cfg.TemplateDir,
		Icons:       iconService,
		MainAppname: cfg.MainAppname,
	}
	appCenter := deployer.NewManager(deployer.Settings{
		MainAppname:  cfg.MainAppname,
		AgentSocket:  cfg.AgentSocket,
		AppCenterCLI: cfg.AppCenterCLI,
		Timeout:      cfg.CLITimeout,
	})
	svc := service.NewService(cfg, repo, providerRegistry, builder, appCenter, iconService)
	srv := &Server{
		Config:      cfg,
		Svc:         svc,
		router:      http.NewServeMux(),
		AgentClient: appCenter,
		icons:       iconService,
		log:         logger.New(),
	}
	srv.register()
	return srv
}

func (s *Server) Run() error {
	if err := os.MkdirAll(s.Config.BuildDir, 0755); err != nil {
		return err
	}
	gatewayPrefix := s.Config.GatewayPath()
	gatewaySocket := os.Getenv("SOCKET_PATH")
	if gatewaySocket == "" {
		gatewaySocket = filepath.Join(os.TempDir(), "shortcuts-gateway.sock")
	}
	// 只清理真正的 socket 文件，并且不抢已有人监听的 socket，
	// 否则 SOCKET_PATH 配错时会删掉无关文件。
	if err := unixsock.RemoveStale(gatewaySocket); err != nil {
		return err
	}
	listener, err := net.Listen("unix", gatewaySocket)
	if err != nil {
		return err
	}
	defer listener.Close()
	s.log.Info("gateway listener opened", "socket", gatewaySocket, "prefix", gatewayPrefix)
	s.log.Info("resolved app paths",
		"pkgvar", s.Config.Pkgvar,
		"dataFile", s.Config.DataFile,
		"uiWWWDir", s.Config.UIWWWDir,
		"templateDir", s.Config.TemplateDir,
		"iconDir", s.Config.IconDir,
		"defaultIconDir", s.Config.DefaultIconDir(),
		"agentSocket", s.Config.AgentSocket,
	)
	handler := stripHandler(s.router, gatewayPrefix)
	gatewayServer := &http.Server{Handler: handler, ErrorLog: s.log.Std()}

	// TCP 只是本机排查用的附加通道，默认关闭（Config.Host 为空即不监听）。
	// 绑不上绝不阻断主通道：网关这条 unix socket 才是界面唯一的入口。
	if tcpListener, tcpAddr, tcpErr := s.listenTCP(); tcpErr != nil {
		s.log.Error("tcp listener failed, serving gateway socket only", "addr", tcpAddr, "err", tcpErr.Error())
	} else if tcpListener == nil {
		s.log.Info("tcp listener disabled, gateway socket only",
			"hint", "set HOST=127.0.0.1 to enable local TCP access")
	} else {
		defer tcpListener.Close()
		s.log.Info("tcp listener opened", "addr", tcpListener.Addr().String())
		tcpServer := &http.Server{Handler: handler, ErrorLog: s.log.Std()}
		go func() {
			if err := tcpServer.Serve(tcpListener); err != nil && err != http.ErrServerClosed {
				s.log.Error("tcp server stopped", "err", err.Error())
			}
		}()
	}

	// 网关同步 Serve：它是主通道，出错就如实返回给调用方，而不是在 goroutine 里
	// panic 把整个进程崩掉——一个附加的 TCP 通道更不该有这种能力。
	s.log.Info("gateway serving", "socket", gatewaySocket, "uid", logger.SystemUID(), "pkgvar", s.Config.Pkgvar)
	if err := gatewayServer.Serve(listener); err != nil && err != http.ErrServerClosed {
		return err
	}
	return nil
}

// listenTCP 按配置决定要不要开 TCP 监听。Host 为空表示不监听（默认），
// 这时返回 (nil, "", nil)。绑定失败由调用方降级处理，绝不当作致命错误。
func (s *Server) listenTCP() (net.Listener, string, error) {
	if s.Config.Host == "" {
		return nil, "", nil
	}
	addr := net.JoinHostPort(s.Config.Host, itoa(s.Config.Port))
	listener, err := net.Listen("tcp", addr)
	if err != nil {
		return nil, addr, err
	}
	return listener, addr, nil
}

func (s *Server) register() {
	s.router.HandleFunc("/", s.handleRoot)
	s.router.HandleFunc("/api/entries", s.handleEntries)
	s.router.HandleFunc("/api/entries/", s.handleEntryByID)
	s.router.HandleFunc("/api/deploy-all", s.handleDeployAll)
	s.router.HandleFunc("/api/validate-folder", s.handleValidateFolder)
	s.router.HandleFunc("/api/system", s.handleSystem)
	s.router.HandleFunc("/icon/", s.handleIcon)
	s.router.HandleFunc("/icons/", s.handleDefaultIcon)
	s.router.Handle("/static/", http.HandlerFunc(s.serveStatic))
}

func (s *Server) handleRoot(w http.ResponseWriter, r *http.Request) {
	// 未匹配的 /api/* 会落到这个兜底路由。API 路径绝不能回 index.html：
	// 拼错的接口会拿到 200 + 一个 HTML 页面，问题被当成「页面正常」吞掉。
	if strings.HasPrefix(r.URL.Path, "/api/") {
		writeError(w, http.StatusNotFound, model.ErrNotFound)
		return
	}
	if r.Method != http.MethodGet && r.Method != http.MethodHead {
		writeError(w, http.StatusMethodNotAllowed, model.ErrMethod)
		return
	}
	name := strings.Trim(strings.TrimPrefix(r.URL.Path, "/"), "/")
	if name != "" {
		if data, err := s.readUIFile(name); err == nil {
			w.Header().Set("Content-Type", staticMime(name))
			if r.Method == http.MethodHead {
				return
			}
			_, _ = w.Write(data)
			return
		}
	}
	// UI 目录只有 cfg.UIWWWDir 一个候选，读不到就直接用内置的兜底页：
	// 以前还有一层 uiDirFallback，在候选收敛成一条之后它只会把同一个
	// 读失败的 index.html 再读一遍，永远失败。
	data, err := os.ReadFile(filepath.Join(s.Config.UIWWWDir, "index.html"))
	if err != nil {
		s.log.Error("ui index not found", "uiwwwdir", s.Config.UIWWWDir, "err", err.Error())
		data = s.notFoundUIPage()
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if r.Method == http.MethodHead {
		return
	}
	_, _ = w.Write(data)
}

func (s *Server) handleEntries(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		entries, err := s.Svc.ListEntries()
		if err != nil {
			writeError(w, http.StatusInternalServerError, model.ErrService)
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"entries": entries, "mainAppname": s.Config.MainAppname})
	case http.MethodPost:
		s.handleSave(w, r, "")
	default:
		writeError(w, http.StatusMethodNotAllowed, model.ErrMethod)
	}
}

func (s *Server) handleSave(w http.ResponseWriter, r *http.Request, appName string) {
	q := r.URL.Query()
	// 路径里的 appname 是权威来源。之前这个参数取了却完全没用，落库用的是
	// query/body 里的 appname，于是 PUT /api/entries/foo 带 body.appname=bar
	// 会把 bar 存进去，等于允许借一次更新改写另一个条目。
	pathAppName := strings.TrimSpace(appName)
	if pathAppName == "" {
		pathAppName = strings.TrimSpace(q.Get("appname"))
	}
	input, inputErr := inputFromQuery(q)
	if inputErr != nil {
		writeValidationDetail(w, inputErr.Error())
		return
	}
	// 合并 JSON body（图标数据，以及 query 里没有的字段）。
	// 请求体读错、超限截断、不是 JSON 对象都要报错：此前这些全被吞掉，
	// 请求按 query-only 数据保存并返回成功，调用方以为 body 里的内容存上了。
	body, err := io.ReadAll(io.LimitReader(r.Body, maxRequestBody+1))
	if err != nil {
		writeValidationDetail(w, "读取请求体失败: "+err.Error())
		return
	}
	if len(body) > maxRequestBody {
		writeValidationDetail(w, "请求体超过 8MB 上限")
		return
	}
	if len(bytes.TrimSpace(body)) > 0 {
		var rawKeys map[string]json.RawMessage
		if err := json.Unmarshal(body, &rawKeys); err != nil {
			writeValidationDetail(w, "请求体不是 JSON 对象: "+err.Error())
			return
		}
		var bodyInput service.EntryInput
		if err := json.Unmarshal(body, &bodyInput); err != nil {
			writeValidationDetail(w, "请求体字段解析失败: "+err.Error())
			return
		}
		mergeBodyInput(&input, bodyInput, rawKeys)
	}
	if pathAppName != "" {
		if bodyAppName := strings.TrimSpace(input.AppName); bodyAppName != "" && bodyAppName != pathAppName {
			writeServiceError(w, model.Errorf(model.ErrValidation,
				"路径中的 appname（%s）与请求中的（%s）不一致", pathAppName, bodyAppName))
			return
		}
		input.AppName = pathAppName
	}
	s.log.Info("save entry", "appname", input.AppName, "type", input.Type, "hasIcon", input.Icon64Data != "")
	result, err := s.Svc.SaveEntry(input)
	if err != nil {
		s.log.Warn("save entry failed", "appname", input.AppName, "err", serviceErrorDetail(err))
		writeServiceError(w, err)
		return
	}
	// 新建返回 201、更新返回 200：状态码要反映实际动作，
	// 以前无论新建还是更新都固定 201。
	status := http.StatusOK
	if result.Created {
		status = http.StatusCreated
	}
	writeJSON(w, status, map[string]any{"entry": result.Entry, "changed": result.Changed})
}

// 请求体上限。读取时多读 1 字节判断是否超限，超限显式报错，不做静默截断。
const maxRequestBody = 8 << 20

// 参与"显式提供"判定的字段名（顺序与 EntryInput 声明一致）。
var entryInputMergeKeys = []string{
	"type", "appname", "displayName", "description", "desiredState", "protocol",
	"port", "path", "url", "openMode", "config", "displayIcon", "deployedVersion", "version",
}

// mergeBodyInput 把 JSON body 中显式出现的字段合并进 query 解析出的 input。
// rawKeys 是 body 的原始键集合，用来区分"未提供"和"显式传空"：
// 例如 description:"" 表示清空描述，省略 description 则表示保留旧值。
// 两种通道（query 与 body）由此保持同一套语义。
func mergeBodyInput(target *service.EntryInput, body service.EntryInput, rawKeys map[string]json.RawMessage) {
	if target.Provided == nil {
		target.Provided = map[string]bool{}
	}
	// body 里出现的键一律拷值并标记「已提供」，与 query 通道同一套语义：
	// 显式传空就是清空，省略才保留旧值。
	//
	// 这里曾经把值拷贝放在「query 里没有 appname 才拷」的条件里，而 Provided
	// 标志无条件置位：query 带 appname 的更新请求（正常路径）里，body 的
	// config/desiredState 等会被标记成「显式提供」但值是空，服务层按「显式
	// 清空」处理，直接把配置抹掉——不是丢字段，是丢数据。
	for key := range rawKeys {
		switch key {
		case "appname":
			target.AppName = body.AppName
		case "type":
			target.Type = body.Type
		case "displayName":
			target.DisplayName = body.DisplayName
		case "description":
			target.Description = body.Description
		case "desiredState":
			target.DesiredState = body.DesiredState
		case "protocol":
			target.Protocol = body.Protocol
		case "port":
			target.Port = body.Port
		case "path":
			target.Path = body.Path
		case "url":
			target.URL = body.URL
		case "openMode":
			target.OpenMode = body.OpenMode
		case "config":
			target.Config = body.Config
		case "displayIcon":
			target.DisplayIcon = body.DisplayIcon
		case "deployedVersion":
			target.DeployedVersion = body.DeployedVersion
		case "version":
			target.Version = body.Version
		default:
			continue
		}
		target.Provided[key] = true
	}
	// 图标数据体量较大，只在 body 显式提供且非空时采用。
	if body.Icon64Data != "" {
		target.Icon64Data = body.Icon64Data
	}
	if body.Icon256Data != "" {
		target.Icon256Data = body.Icon256Data
	}
	if body.RemoveIcon {
		target.RemoveIcon = true
	}
}

// handleIconUpload saves icon data for an entry.
// Body is JSON {icon64Data, icon256Data}.
//
// 读失败、超限、空 body 三种情况分别报错：以前它们被压成一句
// "body is empty or dropped by gateway"，读盘出错时看到的提示是误导性的。
func (s *Server) handleIconUpload(w http.ResponseWriter, r *http.Request, appName string) {
	body, err := io.ReadAll(io.LimitReader(r.Body, maxRequestBody+1))
	if err != nil {
		writeValidationDetail(w, "读取图标请求体失败: "+err.Error())
		return
	}
	if len(body) > maxRequestBody {
		writeValidationDetail(w, "图标请求体超过 8MB 上限")
		return
	}
	if len(bytes.TrimSpace(body)) == 0 {
		writeValidationDetail(w, "图标请求体为空（可能被网关丢弃）")
		return
	}
	var data struct {
		Icon64Data  string `json:"icon64Data"`
		Icon256Data string `json:"icon256Data"`
	}
	if err := json.NewDecoder(bytes.NewReader(body)).Decode(&data); err != nil {
		writeValidationDetail(w, err.Error())
		return
	}
	changed, err := s.Svc.Icons.Save(appName, data.Icon64Data, data.Icon256Data)
	if err != nil {
		writeServiceError(w, err)
		return
	}
	// 图标已经写盘，接着把条目版本 bump 掉（下次部署才会重建子应用）。
	// bump 失败要回滚刚写的图标并如实报错：以前只记一条 warn 就当成功返回，
	// 条目不存在时会留下孤儿图标文件，而调用方收到的是 200。
	if err := s.Svc.BumpEntryVersion(appName); err != nil {
		s.log.Warn("bump entry version after icon upload failed, rolling back icons",
			"appname", appName, "err", err.Error())
		if _, removeErr := s.Svc.Icons.Remove(appName); removeErr != nil {
			s.log.Warn("rollback icon files failed", "appname", appName, "err", removeErr.Error())
		}
		writeServiceError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"changed": changed})
}

func inputFromQuery(q url.Values) (service.EntryInput, error) {
	input := service.EntryInput{
		Type:         model.EntryType(q.Get("type")),
		AppName:      q.Get("appname"),
		DisplayName:  q.Get("displayName"),
		Description:  q.Get("description"),
		DesiredState: q.Get("desiredState"),
		Protocol:     q.Get("protocol"),
		Port:         q.Get("port"),
		Path:         q.Get("path"),
		URL:          q.Get("url"),
		OpenMode:     q.Get("openMode"),
		Version:      q.Get("version"),
	}
	// 记录哪些参数真实出现在 query 里，用来区分"未提供"和"显式传空"。
	// 前端总会带上 description（即使为空），所以"清空描述"仍然有效；
	// 而省略某字段的客户端不会再误清空旧值。
	input.Provided = make(map[string]bool, len(entryInputMergeKeys))
	for _, key := range entryInputMergeKeys {
		input.Provided[key] = q.Has(key)
	}
	if configJSON := strings.TrimSpace(q.Get("config")); configJSON != "" {
		// config 传了就必须是合法 JSON。此前非法配置被静默丢弃，但
		// Provided["config"] 已经置位，服务层按「显式清空」处理，
		// 直接把旧配置抹掉。
		if !json.Valid([]byte(configJSON)) {
			return service.EntryInput{}, errors.New("config 不是合法 JSON")
		}
		input.Config = json.RawMessage(configJSON)
		input.Provided["config"] = true
		var configInput model.EntryConfigInput
		if err := json.Unmarshal([]byte(configJSON), &configInput); err != nil {
			return service.EntryInput{}, fmt.Errorf("config 字段解析失败: %w", err)
		}
		if input.URL == "" {
			input.URL = configInput.URL
		}
		if input.Protocol == "" {
			input.Protocol = configInput.Protocol
		}
		if input.Port == "" {
			input.Port = configInput.Port
		}
		if input.Path == "" {
			input.Path = configInput.Path
		}
		if input.OpenMode == "" {
			input.OpenMode = configInput.OpenMode
		}
	}
	return input, nil
}

func (s *Server) handleEntryByID(w http.ResponseWriter, r *http.Request) {
	path := strings.TrimPrefix(r.URL.Path, "/api/entries/")
	parts := strings.Split(strings.TrimSuffix(path, "/"), "/")
	if len(parts) == 0 || parts[0] == "" {
		writeError(w, http.StatusNotFound, model.ErrNotFound)
		return
	}
	appName := parts[0]
	if len(parts) == 2 {
		switch parts[1] {
		case "package":
			if r.Method != http.MethodGet {
				writeError(w, http.StatusMethodNotAllowed, model.ErrMethod)
				return
			}
			pkg, err := s.Svc.PackagePreview(appName)
			if err != nil {
				writeServiceError(w, err)
				return
			}
			writeJSON(w, http.StatusOK, pkg)
			return
		case "icon":
			if r.Method != http.MethodPost {
				writeError(w, http.StatusMethodNotAllowed, model.ErrMethod)
				return
			}
			s.handleIconUpload(w, r, appName)
			return
		case "desktop-item":
			s.handleDesktopItem(w, r, appName)
			return
		case "deploy", "start", "stop", "uninstall":
			if r.Method != http.MethodPost {
				writeError(w, http.StatusMethodNotAllowed, model.ErrMethod)
				return
			}
			var result service.ActionResult
			var err error
			switch parts[1] {
			case "deploy":
				s.log.Info("deploy entry", "appname", appName)
				result, err = s.Svc.DeployEntry(appName)
			case "start":
				result, err = s.Svc.SetRuntime(appName, "start")
			case "stop":
				result, err = s.Svc.SetRuntime(appName, "stop")
			case "uninstall":
				s.log.Info("uninstall entry", "appname", appName)
				result, err = s.Svc.UninstallEntry(appName)
			}
			if err != nil {
				writeServiceError(w, err)
				return
			}
			writeJSON(w, http.StatusOK, result)
			return
		}
		writeError(w, http.StatusNotFound, model.ErrNotFound)
		return
	}
	// 只允许 /<name> 与 /<name>/<action> 两级；/api/entries/foo/typo/extra
	// 这类更深的路径过去会滑到下面按 /<name> 处理，DELETE 直接删掉 foo。
	if len(parts) > 2 {
		writeError(w, http.StatusNotFound, model.ErrNotFound)
		return
	}
	switch r.Method {
	case http.MethodPut, http.MethodPatch:
		s.handleSave(w, r, appName)
	case http.MethodDelete:
		result, err := s.Svc.DeleteEntry(appName)
		if err != nil {
			writeServiceError(w, err)
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{
			"entries":         result.Entries,
			"removedAppname":  result.Child,
			"stillInstalled":  result.StillInstalled,
			"uninstallDetail": result.UninstallDetail,
		})
	default:
		writeError(w, http.StatusMethodNotAllowed, model.ErrMethod)
	}
}

func (s *Server) handleDeployAll(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeError(w, http.StatusMethodNotAllowed, model.ErrMethod)
		return
	}
	entries, err := s.Svc.ListEntries()
	if err != nil {
		// 列表都拿不到时没有可判断的条目，走统一的服务错误映射，
		// 保留错误码与细节，别塌缩成一句 Internal。
		writeServiceError(w, err)
		return
	}
	results := []service.ActionResult{}
	for _, entry := range entries {
		if entry.Type == model.EntryTypeFolder {
			results = append(results, service.ActionResult{
				AppName: entry.ShortcutAppname,
				Code:    "DEPLOY_SKIPPED_CLIENT",
				Output:  "folder shortcuts are created on the desktop client",
			})
			continue
		}
		action, actionErr := s.Svc.DeployEntry(entry.AppName)
		if actionErr != nil {
			// 单条失败不影响整批，聚合进 results 由前端逐条提示。
			action.Code = string(model.ErrDeploy)
			action.Output = actionErr.Error()
		}
		results = append(results, action)
	}
	fresh, err := s.Svc.ListEntries()
	if err != nil {
		s.log.Warn("list entries after deploy-all failed", "err", serviceErrorDetail(err))
	}
	writeJSON(w, http.StatusOK, map[string]any{"results": results, "entries": fresh})
}

// appVersion 读取主应用 manifest 里的版本号。
// manifest 在安装根的上一级：fnOS 把 fpk 根（manifest、cmd、config、wizard）装到
// /var/apps/<app>/，app.tgz 解到 target/，而 TRIM_APPDEST 指向的是后者。
func (s *Server) appVersion() string {
	path := filepath.Join(filepath.Dir(s.Config.AppInstallRoot), "manifest")
	version, err := appmeta.ReadManifestVersion(path)
	if err != nil {
		s.log.Warn("read app manifest version failed", "path", path, "err", err.Error())
		return ""
	}
	return version
}

// handleValidateFolder 校验文件夹快捷方式的目标路径是否真实存在且是目录。
// 只服务于前端手填路径时的保存前校验；agent 不可用时返回 503，前端按
// 「无法确认存在性」fail-closed，不放行可能不存在的路径。
func (s *Server) handleValidateFolder(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeError(w, http.StatusMethodNotAllowed, model.ErrMethod)
		return
	}
	valid, code, err := s.Svc.ValidateFolderPath(r.URL.Query().Get("path"))
	if err != nil {
		// agent 不可用：无法确认存在性，前端按 fail-closed 不放行。503 比 409 更贴切，
		// 前端只认非 2xx，不会把英文 detail 当成用户文案。
		writeJSON(w, http.StatusServiceUnavailable, map[string]any{"valid": false, "code": "unavailable"})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"valid": valid, "code": code})
}

func (s *Server) handleSystem(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeError(w, http.StatusMethodNotAllowed, model.ErrMethod)
		return
	}
	agentOK, agentResult := s.AgentClient.Ping()
	appVersion := s.appVersion()
	writeJSON(w, http.StatusOK, map[string]any{
		"appVersion":     appVersion,
		"mainAppname":    s.Config.MainAppname,
		"runUid":         logger.SystemUID(),
		"pkgvar":         s.Config.Pkgvar,
		"buildDir":       s.Config.BuildDir,
		"iconDir":        s.Config.IconDir,
		"templateDir":    s.Config.TemplateDir,
		"appInstallRoot": s.Config.AppInstallRoot,
		"appcenterCli":   s.Config.AppCenterCLI,

		"agentSocket":    s.Config.AgentSocket,
		"agentAvailable": agentOK,
		"agentDetail":    agentResult,
		"deployDisabled": s.Config.DeployDisabled,
		"uiWWWDir":       s.Config.UIWWWDir,
		"trimAppDest":    os.Getenv("TRIM_APPDEST"),
		"home":           os.Getenv("HOME"),
		"uiCandidates":   s.uiCandidateDiagnostics(),
		"installedApps":  sortedApps(s.Svc.InstalledApps()),
	})
}

// uiDir 返回配置解析出来的 UI 目录（唯一来源）。
// 这里原先有 12 条硬编码猜测（TRIM_APPDEST/..、HOME/shortcuts、甚至相对路径
// "..\shortcuts\app\ui\www"），是为了绕过"UI 目录没找到"而层层加出来的。
// cfg.UIWWWDir 由 SHORTCUTS_UI_WWW 或 <安装目录>/ui/www 解析而来，而 app.tgz
// 解包出的 ui/www 就在安装目录下（templates 走的是同一个安装目录，已被实际部署
// 验证），一条就够。真的指错了，/api/system 会给出 uiWWWDir，启动日志也会写明。
func (s *Server) uiDir() string {
	return s.Config.UIWWWDir
}

func (s *Server) uiCandidateDiagnostics() []map[string]string {
	dir := s.uiDir()
	item := map[string]string{"path": dir, "hasIndex": "false"}
	if _, err := os.Stat(filepath.Join(dir, "index.html")); err == nil {
		item["hasIndex"] = "true"
	}
	return []map[string]string{item}
}

func (s *Server) notFoundUIPage() []byte {
	dir := s.uiDir()
	has := "false"
	if _, err := os.Stat(filepath.Join(dir, "index.html")); err == nil {
		has = "true"
	}
	page := `<!doctype html><html><head><meta charset="utf-8"></head><body><h1>go-shortcuts UI not found</h1>` +
		`<p>configured uiWWWDir: ` + escapeHTML(dir) + `</p><ul><li>hasIndex=` + has + `, path=` + escapeHTML(dir) + `</li></ul></body></html>`
	return []byte(page)
}

func escapeHTML(value string) string {
	return strings.NewReplacer("&", "&amp;", "<", "&lt;", ">", "&gt;", `"`, "&quot;").Replace(value)
}

func (s *Server) serveStatic(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet && r.Method != http.MethodHead {
		writeError(w, http.StatusMethodNotAllowed, model.ErrMethod)
		return
	}
	name := strings.TrimPrefix(r.URL.Path, "/static/")
	if name == "" || name == "/" {
		writeError(w, http.StatusNotFound, model.ErrNotFound)
		return
	}
	data, err := s.readUIFile(name)
	if err != nil {
		s.log.Warn("static asset not found", "asset", name, "err", err.Error())
		writeError(w, http.StatusNotFound, model.ErrNotFound)
		return
	}
	w.Header().Set("Content-Type", staticMime(name))
	if r.Method == http.MethodHead {
		return
	}
	_, _ = w.Write(data)
}

func (s *Server) readUIFile(name string) ([]byte, error) {
	cleaned := filepath.Clean(name)
	if cleaned == "." || cleaned == ".." || filepath.IsAbs(cleaned) || strings.HasPrefix(cleaned, ".."+string(filepath.Separator)) {
		return nil, os.ErrNotExist
	}
	dir := s.uiDir()
	if dir == "" {
		return nil, os.ErrNotExist
	}
	full := filepath.Join(dir, cleaned)
	// 再校验一次拼出来的路径没有越出 UI 目录（cleaned 里仍可能有分隔符）。
	if rel, err := filepath.Rel(dir, full); err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return nil, os.ErrNotExist
	}
	return os.ReadFile(full)
}

func staticMime(name string) string {
	switch strings.ToLower(filepath.Ext(name)) {
	case ".html", ".htm":
		return "text/html; charset=utf-8"
	case ".js":
		return "application/javascript; charset=utf-8"
	case ".css":
		return "text/css; charset=utf-8"
	case ".svg":
		return "image/svg+xml"
	case ".json":
		return "application/json; charset=utf-8"
	case ".png":
		return "image/png"
	case ".jpg", ".jpeg":
		return "image/jpeg"
	case ".gif":
		return "image/gif"
	case ".ico":
		return "image/x-icon"
	default:
		return "application/octet-stream"
	}
}

func (s *Server) handleIcon(w http.ResponseWriter, r *http.Request) {
	appName := strings.Trim(strings.TrimPrefix(r.URL.Path, "/icon/"), "/")
	if appName == "" {
		writeError(w, http.StatusNotFound, model.ErrNotFound)
		return
	}
	size := 64
	if r.URL.Query().Get("size") == "256" {
		size = 256
	}
	data, err := s.Svc.Icons.Read(appName, size)
	if err != nil {
		writeError(w, http.StatusNotFound, model.ErrResource)
		return
	}
	w.Header().Set("Content-Type", "image/png")
	_, _ = w.Write(data)
}

func (s *Server) handleDefaultIcon(w http.ResponseWriter, r *http.Request) {
	// 尺寸只认文件名后缀（UI 用的就是 child_entry_64.png / _256.png）。
	// 以前用 strings.Contains(name, "256") 做子串匹配，任何含 256 的路径
	// 都会被当成大图标，写错的地址也不会 404。
	size := 0
	switch filepath.Base(r.URL.Path) {
	case "child_entry_64.png":
		size = 64
	case "child_entry_256.png":
		size = 256
	default:
		writeError(w, http.StatusNotFound, model.ErrNotFound)
		return
	}
	data, err := s.icons.Fallback(size)
	if err != nil {
		writeError(w, http.StatusInternalServerError, model.ErrService)
		return
	}
	w.Header().Set("Content-Type", "image/png")
	_, _ = w.Write(data)
}

func writeJSON(w http.ResponseWriter, status int, value any) {
	data, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		writeError(w, http.StatusInternalServerError, model.ErrService)
		return
	}
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_, _ = w.Write(data)
}

func writeError(w http.ResponseWriter, status int, code model.ErrorCode) {
	writeJSON(w, status, map[string]any{"error": code})
}

func writeServiceError(w http.ResponseWriter, err error) {
	var apiErr *model.APIError
	if !asAPIError(err, &apiErr) {
		writeError(w, http.StatusInternalServerError, model.ErrService)
		return
	}
	// 状态码要能区分「可重试的忙/冲突」与真实故障。以前除了 validation /
	// notfound / method 之外一律 500，前端把「子应用正在操作中」当成服务坏了。
	status := http.StatusInternalServerError
	switch apiErr.Code {
	case model.ErrValidation:
		status = http.StatusBadRequest
	case model.ErrNotFound:
		status = http.StatusNotFound
	case model.ErrMethod:
		status = http.StatusMethodNotAllowed
	case model.ErrDeploy, model.ErrUninstall, model.ErrRuntime, model.ErrDelete:
		status = http.StatusConflict
	case model.ErrDeployDisabled:
		status = http.StatusForbidden
	case model.ErrTimeout:
		status = http.StatusGatewayTimeout
	}
	writeJSON(w, status, apiErr)
}

func writeValidationDetail(w http.ResponseWriter, detail string) {
	writeJSON(w, http.StatusBadRequest, model.APIError{Code: model.ErrValidation, Detail: detail})
}

func serviceErrorDetail(err error) string {
	var apiErr *model.APIError
	if asAPIError(err, &apiErr) {
		return fmt.Sprintf("%s %s", apiErr.Code, apiErr.Error())
	}
	return err.Error()
}

func asAPIError(err error, target **model.APIError) bool {
	// 用 errors.As：手写遍历 Unwrap() error 认不出 errors.Join 这类
	// Unwrap() []error 的多重错误，被包装过的服务错误会整段退化成 500。
	return errors.As(err, target)
}

func sortedApps(apps map[string]deployer.InstalledApp) []string {
	names := []string{}
	for name := range apps {
		names = append(names, name)
	}
	for i := 0; i < len(names); i++ {
		for j := i + 1; j < len(names); j++ {
			if names[j] < names[i] {
				names[i], names[j] = names[j], names[i]
			}
		}
	}
	return names
}

func itoa(n int) string {
	return strconv.Itoa(n)
}

func stripHandler(next http.Handler, prefix string) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == prefix || r.URL.Path == prefix+"/" {
			r.URL.Path = "/"
		} else if strings.HasPrefix(r.URL.Path, prefix+"/") {
			r.URL.Path = r.URL.Path[len(prefix):]
		}
		next.ServeHTTP(w, r)
	})
}
