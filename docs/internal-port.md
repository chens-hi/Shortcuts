# 端口映射快捷方式（internal_port）

把一个局域网服务（NAS 内的 `协议://IP:端口/路径`）生成为桌面上的独立图标。用户填端口、协议、访问路径，桌面图标由 fnOS 桌面按子应用的 `ui/config` 渲染。

## 字段与校验

表单见 `shortcuts/app/ui/www/index.html`（`.type-internal` 区块），提交见 `shortcuts/app/ui/www/app.js` 的提交函数（`config={port, protocol, path, openMode}`）。

| 字段 | 规则 |
| --- | --- |
| `appname` | `^[a-z][a-z0-9-]{1,11}$`，`model.AppNameValid` |
| `displayName` | 必填，≤12 个字符 |
| `port` | 1-65535，`model.IsValidPort` |
| `protocol` | `http` 或 `https` |
| `path` | 以 `/` 或 `?` 开头（见下） |
| `openMode` | `iframe`（桌面内嵌窗口）或 `url`（新标签打开） |

校验入口：`internal/provider/internal_port.go` 的 `Validate` 与 `model.ConfigFromInput`。校验失败返回 `ErrValidation`。

`path` 允许纯查询串（如 `?k=<token>`）：fnOS 桌面拼出 `host:port?k=…` 后，浏览器按空路径（即 `/`）请求，与 `/?k=…` 等价。因此不强制以 `/` 开头，`?` 开头也合法，且原样透传、不补斜杠。

## 点击行为

桌面图标由桌面按子应用 `ui/config` 的 `.url` 条目渲染，本仓库没有拼 URL 的代码。`type=iframe` 时桌面内嵌窗口打开 `protocol://<nas>:<port><path>`，`type=url` 时新标签打开。

## 生成物

`internal/provider/internal_port.go` 的 `Spec` 只生成 `UIConfig`，不生成 CGI（子应用只是桌面入口，没有真实进程）：

```json
{
  ".url": {
    "<child>": {
      "title": "<displayName>",
      "icon": "images/icon_{0}.png",
      "type": "<iframe|url>",
      "protocol": "<http|https>",
      "port": "<port>",
      "url": "<path>",
      "allUsers": true,
      "control": { "accessPerm": "readonly" }
    }
  }
}
```

- `UIConfig` 写到子包 `app/ui/config`，见 `internal/packagebuilder/builder.go`。
- 子应用名 = `shortcuts-<appname>`（`model.Entry.ChildAppname`）。
- 模板目录 `shortcuts/app/templates/internal-port/`：`cmd/main` 为 no-op 脚本（start/stop/status 全部 exit 0），`config/privilege` 以 root 运行，`config/resource` 为 `{}`。
- 图标：`Spec` 未指定 CGI 时仍写 4 个图标文件（包根 `ICON.PNG`/`ICON_256.PNG` + `app/ui/images/icon_64.png`/`icon_256.png`），见 `builder.go` 的图标循环。

## 部署链路

`Service.DeployEntry` → `packagebuilder.Builder.Build` → `deployer.Manager.InstallLocal`（`appcenter-cli install-local`，cwd=构建目录）→ root agent 白名单执行 → 轮询 appcenter 列表确认后才写 `DeployedVersion`。

## 注意事项

- **路径透传**：`path` 以 `?` 开头时不补 `/`、原样进入 `UIConfig` 的 `url` 字段（见 `internal_port.go` 注释）。
- **`Changed()` 不计入 `desiredState`**：它由启停接口改，且 `SetRuntime` 改同一字段并不 bump 版本；算进来会让「改一次启停」导致下次部署整包重装。
- **解码失败按「有变化」处理**：以前错误被吞掉、两边都解成零值就被判无变化，配置其实变了却既不 bump 版本也不重建子应用。
- **部分更新合并语义**：显式提供的顶层字段覆盖 config 同名字段；只改端口时未传的 `protocol`/`path` 不能被丢掉（`service.go` 的 `mergeConfigFields`）。
- **部署幂等**：已装且版本一致返回 `DEPLOY_SKIPPED`，不重建包（`service.go` 的 `DeployEntry`）。
