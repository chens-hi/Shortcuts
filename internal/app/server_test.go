package app

import (
	"encoding/json"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"path/filepath"
	"reflect"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/chens-hi/shortcuts/internal/config"
	"github.com/chens-hi/shortcuts/internal/deployer"
	"github.com/chens-hi/shortcuts/internal/icons"
	"github.com/chens-hi/shortcuts/internal/logger"
	"github.com/chens-hi/shortcuts/internal/model"
	"github.com/chens-hi/shortcuts/internal/provider"
	"github.com/chens-hi/shortcuts/internal/repository"
	"github.com/chens-hi/shortcuts/internal/service"
)

func TestHandleEntriesCreateViaQuery(t *testing.T) {
	server := handlerForServerTest(t)
	query := entryCreateQuery()
	req := httptest.NewRequest(http.MethodPost, "/app/shortcuts/api/entries?"+query.Encode(), nil)
	rr := httptest.NewRecorder()
	stripHandler(server.router, entryGatewayPrefix()).ServeHTTP(rr, req)

	if rr.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d body=%s", rr.Code, rr.Body.String())
	}
	if !strings.Contains(rr.Body.String(), `"appname": "moviepilot"`) {
		t.Fatalf("missing saved app name: %s", rr.Body.String())
	}
	if !strings.Contains(rr.Body.String(), `"changed": true`) {
		t.Fatalf("expected changed true: %s", rr.Body.String())
	}
}

func TestHandleEntriesCreateDirectHTTPViaQuery(t *testing.T) {
	server := handlerForServerTest(t)
	ts := httptest.NewServer(stripHandler(server.router, entryGatewayPrefix()))
	t.Cleanup(ts.Close)

	client := &http.Client{Timeout: 3 * time.Second}
	query := entryCreateQuery()
	request, err := http.NewRequest(http.MethodPost, ts.URL+"/api/entries?"+query.Encode(), nil)
	if err != nil {
		t.Fatalf("new request: %v", err)
	}

	response, err := client.Do(request)
	if err != nil {
		t.Fatalf("direct POST: %v", err)
	}
	defer response.Body.Close()

	body, err := io.ReadAll(response.Body)
	if err != nil {
		t.Fatalf("read response: %v", err)
	}
	if response.StatusCode != http.StatusCreated {
		t.Fatalf("expected 201, got %d body=%s", response.StatusCode, string(body))
	}
	if !strings.Contains(string(body), `"appname": "moviepilot"`) {
		t.Fatalf("missing saved app name: %s", string(body))
	}
}

func TestHandleEntriesCreateExternalConfigOpenModeViaQuery(t *testing.T) {
	server := handlerForServerTest(t)
	query := externalEntryCreateQuery()
	req := httptest.NewRequest(http.MethodPost, "/app/shortcuts/api/entries?"+query.Encode(), nil)
	rr := httptest.NewRecorder()
	stripHandler(server.router, entryGatewayPrefix()).ServeHTTP(rr, req)

	if rr.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d body=%s", rr.Code, rr.Body.String())
	}
	if !strings.Contains(rr.Body.String(), `"openMode": "iframe"`) {
		t.Fatalf("missing saved external openMode: %s", rr.Body.String())
	}
}

func TestHandleIconUploadSavesIcon(t *testing.T) {
	server := handlerForServerTest(t)

	// First create an entry via query
	query := entryCreateQuery()
	createReq := httptest.NewRequest(http.MethodPost, "/app/shortcuts/api/entries?"+query.Encode(), nil)
	createRR := httptest.NewRecorder()
	stripHandler(server.router, entryGatewayPrefix()).ServeHTTP(createRR, createReq)
	if createRR.Code != http.StatusCreated {
		t.Fatalf("create entry failed: %d %s", createRR.Code, createRR.Body.String())
	}

	// Extract appname from response
	createBody := createRR.Body.String()
	appNameStart := strings.Index(createBody, `"appname": "`)
	if appNameStart < 0 {
		t.Fatalf("missing appname in create response: %s", createBody)
	}
	appNameStart += len(`"appname": "`)
	appNameEnd := strings.Index(createBody[appNameStart:], `"`)
	if appNameEnd < 0 {
		t.Fatalf("malformed appname in create response: %s", createBody)
	}
	appName := createBody[appNameStart : appNameStart+appNameEnd]

	// Upload icon via POST /api/entries/{appname}/icon
	iconBody := `{"icon64Data":"data:image/png;base64,AAAA","icon256Data":"data:image/png;base64,BBBB"}`
	iconReq := httptest.NewRequest(http.MethodPost, "/app/shortcuts/api/entries/"+appName+"/icon", strings.NewReader(iconBody))
	iconReq.Header.Set("Content-Type", "application/json")
	iconRR := httptest.NewRecorder()
	stripHandler(server.router, entryGatewayPrefix()).ServeHTTP(iconRR, iconReq)

	if iconRR.Code != http.StatusOK {
		t.Fatalf("expected icon upload 200, got %d body=%s", iconRR.Code, iconRR.Body.String())
	}
}

func TestHandleIconUploadEmptyBodyReturnsError(t *testing.T) {
	server := handlerForServerTest(t)
	req := httptest.NewRequest(http.MethodPost, "/app/shortcuts/api/entries/test-appname/icon", nil)
	rr := httptest.NewRecorder()
	stripHandler(server.router, entryGatewayPrefix()).ServeHTTP(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 for empty icon body, got %d", rr.Code)
	}
}

func entryGatewayPrefix() string {
	return "/app/shortcuts"
}

func TestHandleValidateFolder(t *testing.T) {
	app := newStubAppCenter()
	server := handlerForServerTest(t, app)
	serve := func(path string) *httptest.ResponseRecorder {
		req := httptest.NewRequest(http.MethodGet, "/app/shortcuts/api/validate-folder?path="+url.QueryEscape(path), nil)
		rr := httptest.NewRecorder()
		stripHandler(server.router, entryGatewayPrefix()).ServeHTTP(rr, req)
		return rr
	}

	// 默认 stub 报告路径存在 → valid=true。
	if rr := serve("/vol1/1000/照片"); rr.Code != http.StatusOK || !strings.Contains(rr.Body.String(), `"valid": true`) {
		t.Fatalf("expected valid=true 200, got %d body=%s", rr.Code, rr.Body.String())
	}

	// 注入 stat 失败（路径不存在/不是目录）→ valid=false。
	app.statResult = &deployer.CommandResult{OK: false, Error: "path does not exist or is not a directory"}
	if rr := serve("/vol1/1000/不存在"); rr.Code != http.StatusOK || !strings.Contains(rr.Body.String(), `"valid": false`) {
		t.Fatalf("expected valid=false 200, got %d body=%s", rr.Code, rr.Body.String())
	}

	// 格式非法路径 → valid=false（格式校验在服务端本地完成，无需 agent）。
	if rr := serve("vol1/1000"); rr.Code != http.StatusOK || !strings.Contains(rr.Body.String(), `"valid": false`) {
		t.Fatalf("expected valid=false 200 for bad format, got %d body=%s", rr.Code, rr.Body.String())
	}

	// agent 不可用 → 503，fail-closed。
	app.statResult = &deployer.CommandResult{Unavailable: true, Error: "unavailable: agent not configured"}
	if rr := serve("/vol1/1000/照片"); rr.Code != http.StatusServiceUnavailable {
		t.Fatalf("expected 503 when agent unavailable, got %d body=%s", rr.Code, rr.Body.String())
	}

	// 方法不允许 → 405。
	req := httptest.NewRequest(http.MethodPost, "/app/shortcuts/api/validate-folder", nil)
	rr := httptest.NewRecorder()
	stripHandler(server.router, entryGatewayPrefix()).ServeHTTP(rr, req)
	if rr.Code != http.StatusMethodNotAllowed {
		t.Fatalf("expected 405, got %d", rr.Code)
	}
}

// stubAppCenter 让 handler 测试不依赖宿主机上的 appcenter-cli。
// 真实 Manager 的 List 在没装 CLI 的机器上会返回错误，而 /api/entries 与
// /api/deploy-all 都靠 List 取列表，测试会变成「这台机器装没装 CLI」的探测。
// 同时记录调用，便于断言某条路径没有触发安装。
type stubAppCenter struct {
	mu        sync.Mutex
	installed map[string]deployer.InstalledApp
	calls     []string
	// statResult 覆盖 StatDir 的返回值；nil 时 StatDir 默认报告路径存在且是目录。
	statResult *deployer.CommandResult
}

func newStubAppCenter(installed ...deployer.InstalledApp) *stubAppCenter {
	apps := map[string]deployer.InstalledApp{}
	for _, app := range installed {
		apps[app.AppName] = app
	}
	return &stubAppCenter{installed: apps}
}

func (s *stubAppCenter) call(name string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.calls = append(s.calls, name)
}

func (s *stubAppCenter) Calls() []string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return append([]string{}, s.calls...)
}

func (s *stubAppCenter) List() (map[string]deployer.InstalledApp, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := map[string]deployer.InstalledApp{}
	for name, app := range s.installed {
		out[name] = app
	}
	return out, nil
}

func (s *stubAppCenter) InstallLocal(dir string) deployer.CommandResult {
	s.call("install:" + filepath.Base(dir))
	return deployer.CommandResult{OK: true}
}
func (s *stubAppCenter) Start(appName string) deployer.CommandResult {
	s.call("start:" + appName)
	return deployer.CommandResult{OK: true}
}
func (s *stubAppCenter) Stop(appName string) deployer.CommandResult {
	s.call("stop:" + appName)
	return deployer.CommandResult{OK: true}
}
func (s *stubAppCenter) Uninstall(appName string) deployer.CommandResult {
	s.call("uninstall:" + appName)
	s.mu.Lock()
	delete(s.installed, appName)
	s.mu.Unlock()
	return deployer.CommandResult{OK: true}
}
func (s *stubAppCenter) Ping() (bool, deployer.CommandResult) {
	return true, deployer.CommandResult{OK: true}
}
func (s *stubAppCenter) StatDir(dir string) deployer.CommandResult {
	s.call("stat-dir:" + dir)
	if s.statResult != nil {
		return *s.statResult
	}
	return deployer.CommandResult{OK: true, Stdout: dir}
}

func handlerForServerTest(t *testing.T, apps ...*stubAppCenter) *Server {
	t.Helper()
	appCenter := newStubAppCenter()
	if len(apps) > 0 {
		appCenter = apps[0]
	}
	root := t.TempDir()
	cfg := config.Config{
		MainAppname:    "shortcuts",
		GatewayPrefix:  "/app/shortcuts",
		DataFile:       filepath.Join(root, "shortcuts.json"),
		IconDir:        filepath.Join(root, "icons"),
		BuildDir:       filepath.Join(root, "build"),
		AppInstallRoot: root,
		TrimUID:        -1,
		TrimGID:        -1,
	}
	providers := provider.NewRegistry()
	providers.Register(provider.InternalPortProvider{})
	providers.Register(provider.ExternalURLProvider{})
	providers.Register(provider.FolderProvider{})
	iconSvc := &icons.Service{Dir: cfg.IconDir}
	svc := service.NewService(cfg, repository.NewJSONRepository(cfg.DataFile), providers, nil, appCenter, iconSvc)
	server := &Server{Config: cfg, Svc: svc, router: http.NewServeMux(), log: logger.New()}
	server.register()
	return server
}

func entryCreateQuery() url.Values {
	query := url.Values{}
	query.Set("appname", "moviepilot")
	query.Set("displayName", "MoviePilot")
	query.Set("config", `{"protocol":"http","port":"13000","path":"/","openMode":"iframe"}`)
	return query
}

func externalEntryCreateQuery() url.Values {
	query := url.Values{}
	query.Set("appname", "baidu")
	query.Set("displayName", "百度一下")
	query.Set("type", "external_url")
	query.Set("url", "https://www.baidu.com")
	query.Set("config", `{"url":"https://www.baidu.com","openMode":"iframe"}`)
	return query
}

// 默认不监听 TCP：界面只走 unix socket 网关，开 TCP 没人用，还会平白多出
// 「端口被占用导致启动失败」的风险（以前端口绑不上是致命错误）。
func TestListenTCPDisabledByDefault(t *testing.T) {
	s := &Server{Config: config.Config{Port: 23330}, log: logger.New()}
	listener, addr, err := s.listenTCP()
	if err != nil {
		t.Fatalf("Host 为空时不应尝试监听: %v", err)
	}
	if listener != nil {
		_ = listener.Close()
		t.Fatal("Host 为空时不应打开 TCP 监听")
	}
	if addr != "" {
		t.Fatalf("未监听时不该有地址，实际 %q", addr)
	}
}

func TestListenTCPWhenHostConfigured(t *testing.T) {
	s := &Server{Config: config.Config{Host: "127.0.0.1", Port: 0}, log: logger.New()}
	listener, addr, err := s.listenTCP()
	if err != nil {
		t.Fatalf("显式配置 Host 后应能监听: %v", err)
	}
	if listener == nil {
		t.Fatal("显式配置 Host 后应返回 listener")
	}
	defer listener.Close()
	if !strings.HasPrefix(addr, "127.0.0.1:") {
		t.Fatalf("地址不符合预期: %q", addr)
	}
}

// 端口被占用时只返回错误，由调用方降级为纯网关模式；绝不能 panic 或退出。
func TestListenTCPReportsOccupiedPort(t *testing.T) {
	occupied, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("准备占用端口失败: %v", err)
	}
	defer occupied.Close()
	port := occupied.Addr().(*net.TCPAddr).Port

	s := &Server{Config: config.Config{Host: "127.0.0.1", Port: port}, log: logger.New()}
	listener, addr, err := s.listenTCP()
	if listener != nil {
		_ = listener.Close()
	}
	if err == nil {
		t.Fatalf("端口已被占用时应返回错误，addr=%q", addr)
	}
}

func decodeEntryNames(t *testing.T, body []byte) []string {
	t.Helper()
	var response struct {
		Entries []model.Entry `json:"entries"`
	}
	if err := json.Unmarshal(body, &response); err != nil {
		t.Fatalf("decode entries: %v body=%s", err, body)
	}
	names := make([]string, 0, len(response.Entries))
	for _, entry := range response.Entries {
		names = append(names, entry.AppName)
	}
	return names
}

func TestHandleEntriesReorder(t *testing.T) {
	app := newStubAppCenter()
	server := handlerForServerTest(t, app)
	serve := func(req *http.Request) *httptest.ResponseRecorder {
		rr := httptest.NewRecorder()
		stripHandler(server.router, entryGatewayPrefix()).ServeHTTP(rr, req)
		return rr
	}
	for _, name := range []string{"alpha", "bravo", "charlie"} {
		query := entryCreateQuery()
		query.Set("appname", name)
		query.Set("displayName", name)
		rr := serve(httptest.NewRequest(http.MethodPost, "/app/shortcuts/api/entries?"+query.Encode(), nil))
		if rr.Code != http.StatusCreated {
			t.Fatalf("create %s: %d %s", name, rr.Code, rr.Body.String())
		}
	}

	want := []string{"charlie", "alpha", "bravo"}
	payload, err := json.Marshal(map[string]any{"order": want})
	if err != nil {
		t.Fatal(err)
	}
	rr := serve(httptest.NewRequest(http.MethodPut, "/app/shortcuts/api/entries", strings.NewReader(string(payload))))
	if rr.Code != http.StatusOK {
		t.Fatalf("reorder: %d %s", rr.Code, rr.Body.String())
	}
	if got := decodeEntryNames(t, rr.Body.Bytes()); !reflect.DeepEqual(got, want) {
		t.Fatalf("reorder response order = %v, want %v", got, want)
	}

	rr = serve(httptest.NewRequest(http.MethodGet, "/app/shortcuts/api/entries", nil))
	if rr.Code != http.StatusOK {
		t.Fatalf("list after reorder: %d %s", rr.Code, rr.Body.String())
	}
	if got := decodeEntryNames(t, rr.Body.Bytes()); !reflect.DeepEqual(got, want) {
		t.Fatalf("persisted order = %v, want %v", got, want)
	}
}

func TestHandleEntriesReorderRejectsInvalidRequests(t *testing.T) {
	server := handlerForServerTest(t)
	serve := func(body string) (int, string) {
		req := httptest.NewRequest(http.MethodPut, "/app/shortcuts/api/entries", strings.NewReader(body))
		rr := httptest.NewRecorder()
		stripHandler(server.router, entryGatewayPrefix()).ServeHTTP(rr, req)
		return rr.Code, rr.Body.String()
	}
	for _, name := range []string{"alpha", "bravo"} {
		query := entryCreateQuery()
		query.Set("appname", name)
		query.Set("displayName", name)
		req := httptest.NewRequest(http.MethodPost, "/app/shortcuts/api/entries?"+query.Encode(), nil)
		rr := httptest.NewRecorder()
		stripHandler(server.router, entryGatewayPrefix()).ServeHTTP(rr, req)
		if rr.Code != http.StatusCreated {
			t.Fatalf("create %s: %d %s", name, rr.Code, rr.Body.String())
		}
	}

	cases := []struct {
		name string
		body string
	}{
		{"malformed", `{`},
		{"missing", `{}`},
		{"null", `{"order":null}`},
		{"short", `{"order":["alpha"]}`},
		{"duplicate", `{"order":["alpha","alpha"]}`},
		{"unknown", `{"order":["alpha","zulu"]}`},
		{"too-large", `{"order":["` + strings.Repeat("a", maxReorderBody) + `"]}`},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			code, body := serve(tc.body)
			if code != http.StatusBadRequest {
				t.Fatalf("%s: code = %d body=%s, want 400", tc.name, code, body)
			}
		})
	}

	// 被拒绝的请求不能动到已存顺序。
	rr := httptest.NewRecorder()
	stripHandler(server.router, entryGatewayPrefix()).ServeHTTP(rr, httptest.NewRequest(http.MethodGet, "/app/shortcuts/api/entries", nil))
	if got := decodeEntryNames(t, rr.Body.Bytes()); !reflect.DeepEqual(got, []string{"alpha", "bravo"}) {
		t.Fatalf("rejected requests changed order: %v", got)
	}
}

func TestHandleEntriesCollectionMethods(t *testing.T) {
	server := handlerForServerTest(t)
	serve := func(req *http.Request) *httptest.ResponseRecorder {
		rr := httptest.NewRecorder()
		stripHandler(server.router, entryGatewayPrefix()).ServeHTTP(rr, req)
		return rr
	}

	// PATCH 集合仍然不支持。
	if rr := serve(httptest.NewRequest(http.MethodPatch, "/app/shortcuts/api/entries", nil)); rr.Code != http.StatusMethodNotAllowed {
		t.Fatalf("PATCH /api/entries = %d, want 405", rr.Code)
	}

	// 新增排序接口不能截走单条目的 PUT。
	query := entryCreateQuery()
	if rr := serve(httptest.NewRequest(http.MethodPost, "/app/shortcuts/api/entries?"+query.Encode(), nil)); rr.Code != http.StatusCreated {
		t.Fatalf("create: %d %s", rr.Code, rr.Body.String())
	}
	update := url.Values{}
	update.Set("displayName", "Alpha2")
	rr := serve(httptest.NewRequest(http.MethodPut, "/app/shortcuts/api/entries/moviepilot?"+update.Encode(), nil))
	if rr.Code != http.StatusOK {
		t.Fatalf("PUT /api/entries/<name> = %d %s, want 200", rr.Code, rr.Body.String())
	}
	if !strings.Contains(rr.Body.String(), `"displayName": "Alpha2"`) {
		t.Fatalf("single entry update did not save: %s", rr.Body.String())
	}
}
