package app

import (
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"strings"
	"testing"

	"github.com/chens-hi/shortcuts/internal/deployer"
	"github.com/chens-hi/shortcuts/internal/model"
)

// realFolderTestPath 创建并返回一个真实存在、通过 IsValidFolderPath 格式检查的目录。
// /tmp 前缀在 Windows 下解析为当前盘符的 \tmp，Linux 下即 /tmp，两边都能真实创建。
func realFolderTestPath(t *testing.T, name string) string {
	t.Helper()
	p := "/tmp/shortcuts-app-" + name
	if err := os.MkdirAll(p, 0o755); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.RemoveAll(p) })
	return p
}

func folderEntryCreateQuery(path string) url.Values {
	query := url.Values{}
	query.Set("appname", "photos")
	query.Set("displayName", "照片")
	query.Set("type", "folder")
	query.Set("path", path)
	query.Set("config", `{"path":"`+path+`"}`)
	return query
}

func createFolderEntryForTest(t *testing.T, server *Server) {
	t.Helper()
	req := httptest.NewRequest(http.MethodPost,
		"/app/shortcuts/api/entries?"+folderEntryCreateQuery(realFolderTestPath(t, "photos")).Encode(), nil)
	rr := httptest.NewRecorder()
	stripHandler(server.router, entryGatewayPrefix()).ServeHTTP(rr, req)
	if rr.Code != http.StatusCreated {
		t.Fatalf("create folder entry: %d %s", rr.Code, rr.Body.String())
	}
}

func TestDesktopItemPutAndDelete(t *testing.T) {
	server := handlerForServerTest(t)
	createFolderEntryForTest(t, server)

	put := httptest.NewRequest(http.MethodPut,
		"/app/shortcuts/api/entries/photos/desktop-item",
		strings.NewReader(`{"entryKey":"folder:57"}`))
	put.Header.Set("Content-Type", "application/json")
	rr := httptest.NewRecorder()
	stripHandler(server.router, entryGatewayPrefix()).ServeHTTP(rr, put)
	if rr.Code != http.StatusOK {
		t.Fatalf("put: %d %s", rr.Code, rr.Body.String())
	}
	if !strings.Contains(rr.Body.String(), `"desktopEntryKey": "folder:57"`) {
		t.Fatalf("key missing in response: %s", rr.Body.String())
	}

	del := httptest.NewRequest(http.MethodDelete, "/app/shortcuts/api/entries/photos/desktop-item", nil)
	rr = httptest.NewRecorder()
	stripHandler(server.router, entryGatewayPrefix()).ServeHTTP(rr, del)
	if rr.Code != http.StatusOK {
		t.Fatalf("delete: %d %s", rr.Code, rr.Body.String())
	}
	if strings.Contains(rr.Body.String(), `"desktopEntryKey"`) {
		t.Fatalf("key should be cleared: %s", rr.Body.String())
	}
}

func TestDesktopItemRejectsInvalidKey(t *testing.T) {
	server := handlerForServerTest(t)
	createFolderEntryForTest(t, server)

	for _, body := range []string{
		`{"entryKey":""}`,
		`{"entryKey":"folder:abc"}`,
		`{"entryKey":"entry:57"}`,
		`not-json`,
	} {
		req := httptest.NewRequest(http.MethodPut,
			"/app/shortcuts/api/entries/photos/desktop-item", strings.NewReader(body))
		rr := httptest.NewRecorder()
		stripHandler(server.router, entryGatewayPrefix()).ServeHTTP(rr, req)
		if rr.Code != http.StatusBadRequest {
			t.Errorf("body %q: status = %d, want 400 (%s)", body, rr.Code, rr.Body.String())
		}
	}
}

func TestDesktopItemRejectsNonFolderEntry(t *testing.T) {
	server := handlerForServerTest(t)
	req := httptest.NewRequest(http.MethodPost,
		"/app/shortcuts/api/entries?"+entryCreateQuery().Encode(), nil)
	rr := httptest.NewRecorder()
	stripHandler(server.router, entryGatewayPrefix()).ServeHTTP(rr, req)
	if rr.Code != http.StatusCreated {
		t.Fatalf("create port entry: %d %s", rr.Code, rr.Body.String())
	}

	put := httptest.NewRequest(http.MethodPut,
		"/app/shortcuts/api/entries/moviepilot/desktop-item",
		strings.NewReader(`{"entryKey":"folder:57"}`))
	rr = httptest.NewRecorder()
	stripHandler(server.router, entryGatewayPrefix()).ServeHTTP(rr, put)
	if rr.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400 (%s)", rr.Code, rr.Body.String())
	}
}

func TestDesktopItemUnknownEntry(t *testing.T) {
	server := handlerForServerTest(t)
	put := httptest.NewRequest(http.MethodPut,
		"/app/shortcuts/api/entries/nope/desktop-item",
		strings.NewReader(`{"entryKey":"folder:57"}`))
	rr := httptest.NewRecorder()
	stripHandler(server.router, entryGatewayPrefix()).ServeHTTP(rr, put)
	if rr.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want 404 (%s)", rr.Code, rr.Body.String())
	}
}

func TestFolderDeployIsRejectedOverHTTP(t *testing.T) {
	// 预置同名旧子应用且版本与条目一致：没有 DeployEntry 守卫时会先命中
	// 「已是最新版本」，返回 200 DEPLOY_SKIPPED；契约要求 400 SC-0001。
	app := newStubAppCenter(deployer.InstalledApp{AppName: "shortcuts-photos", Version: "1.0.0"})
	server := handlerForServerTest(t, app)
	createFolderEntryForTest(t, server)

	req := httptest.NewRequest(http.MethodPost, "/app/shortcuts/api/entries/photos/deploy", nil)
	rr := httptest.NewRecorder()
	stripHandler(server.router, entryGatewayPrefix()).ServeHTTP(rr, req)
	if rr.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400 (%s)", rr.Code, rr.Body.String())
	}
	if !strings.Contains(rr.Body.String(), string(model.ErrValidation)) {
		t.Fatalf("error code missing in response: %s", rr.Body.String())
	}
	for _, call := range app.Calls() {
		if strings.HasPrefix(call, "install:") {
			t.Fatalf("folder deploy must not install anything; calls=%v", app.Calls())
		}
	}
}

func TestDeployAllSkipsFolderEntry(t *testing.T) {
	server := handlerForServerTest(t)
	createFolderEntryForTest(t, server)

	req := httptest.NewRequest(http.MethodPost, "/app/shortcuts/api/deploy-all", nil)
	rr := httptest.NewRecorder()
	stripHandler(server.router, entryGatewayPrefix()).ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d (%s)", rr.Code, rr.Body.String())
	}
	if !strings.Contains(rr.Body.String(), `"code": "DEPLOY_SKIPPED_CLIENT"`) {
		t.Fatalf("folder entry was not skipped: %s", rr.Body.String())
	}
}

func TestDesktopItemPutRejectsOversizedBody(t *testing.T) {
	server := handlerForServerTest(t)
	createFolderEntryForTest(t, server)

	// entryKey 只有十几个字节；这个接口必须自己在读取阶段限住请求体，
	// 否则一个 3 字节的写入接口能拖着 8MB 的公共上限跑。
	body := `{"entryKey":"folder:57","pad":"` + strings.Repeat("a", 600) + `"}`
	req := httptest.NewRequest(http.MethodPut,
		"/app/shortcuts/api/entries/photos/desktop-item", strings.NewReader(body))
	rr := httptest.NewRecorder()
	stripHandler(server.router, entryGatewayPrefix()).ServeHTTP(rr, req)
	if rr.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400 (%s)", rr.Code, rr.Body.String())
	}
	if !strings.Contains(rr.Body.String(), string(model.ErrValidation)) {
		t.Fatalf("error code missing in response: %s", rr.Body.String())
	}
}

func TestDesktopItemGetMethodIsNotAllowed(t *testing.T) {
	server := handlerForServerTest(t)
	createFolderEntryForTest(t, server)

	// 该端点只有 PUT（回写）与 DELETE（清空）两种语义；GET 落到 default 分支，
	// 不能因为「路径认出来了」就被当成读取。
	req := httptest.NewRequest(http.MethodGet,
		"/app/shortcuts/api/entries/photos/desktop-item", nil)
	rr := httptest.NewRecorder()
	stripHandler(server.router, entryGatewayPrefix()).ServeHTTP(rr, req)
	if rr.Code != http.StatusMethodNotAllowed {
		t.Fatalf("status = %d, want 405 (%s)", rr.Code, rr.Body.String())
	}
	if !strings.Contains(rr.Body.String(), string(model.ErrMethod)) {
		t.Fatalf("error code missing in response: %s", rr.Body.String())
	}
}

func TestDesktopItemDeleteUnknownEntry(t *testing.T) {
	server := handlerForServerTest(t)

	// 清空走的是 key="" 分支，跳过格式校验；条目不存在时必须仍然报 404，
	// 否则前端无法区分「没这条捷径」和「已经清空了」。
	req := httptest.NewRequest(http.MethodDelete,
		"/app/shortcuts/api/entries/nope/desktop-item", nil)
	rr := httptest.NewRecorder()
	stripHandler(server.router, entryGatewayPrefix()).ServeHTTP(rr, req)
	if rr.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want 404 (%s)", rr.Code, rr.Body.String())
	}
	if !strings.Contains(rr.Body.String(), string(model.ErrNotFound)) {
		t.Fatalf("error code missing in response: %s", rr.Body.String())
	}
}

func TestDesktopItemDeleteRejectsNonFolderEntry(t *testing.T) {
	server := handlerForServerTest(t)
	req := httptest.NewRequest(http.MethodPost,
		"/app/shortcuts/api/entries?"+entryCreateQuery().Encode(), nil)
	rr := httptest.NewRecorder()
	stripHandler(server.router, entryGatewayPrefix()).ServeHTTP(rr, req)
	if rr.Code != http.StatusCreated {
		t.Fatalf("create port entry: %d %s", rr.Code, rr.Body.String())
	}

	// 只有 folder 条目带桌面项标识；其他类型即使请求合法也要挡住，
	// 否则一次误请求就把端口条目的记录写成无意义的 desktopEntryKey。
	del := httptest.NewRequest(http.MethodDelete,
		"/app/shortcuts/api/entries/moviepilot/desktop-item", nil)
	rr = httptest.NewRecorder()
	stripHandler(server.router, entryGatewayPrefix()).ServeHTTP(rr, del)
	if rr.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400 (%s)", rr.Code, rr.Body.String())
	}
	if !strings.Contains(rr.Body.String(), string(model.ErrValidation)) {
		t.Fatalf("error code missing in response: %s", rr.Body.String())
	}
}
