# 外部链接快捷方式（external_url）

把一个外部 URL 生成为桌面上的独立图标。点击后经 302 跳转到目标地址。

## 字段与校验

表单见 `shortcuts/app/ui/www/index.html`（`.type-external` 区块），提交见 `shortcuts/app/ui/www/app.js`（`config={url, openMode}`）。

| 字段 | 规则 |
| --- | --- |
| `appname` | `^[a-z][a-z0-9-]{1,11}$` |
| `displayName` | 必填，≤12 个字符 |
| `url` | 必须 `http`/`https`，host 为合法 IP 或 ≥2 段域名（`model.IsValidURL`） |
| `openMode` | 缺省补 `url`；`url`（新标签）或 `iframe`（桌面内嵌） |

校验入口：`internal/provider/external_url.go` 的 `Validate` 与 `model.ConfigFromInput`。

## 点击行为

桌面打开 `/cgi/ThirdParty/<child>/index.cgi`，CGI 返回 `302 Found` + `Location: <目标URL>`，浏览器跳到目标地址。`iframe` 模式即内嵌窗口内完成重定向，`url` 模式新标签。

## 生成物

`internal/provider/external_url.go` 的 `Spec`：

- 动态生成 bash CGI（`CGIScript`），写到子包 `app/ui/index.cgi`（0755）：

```bash
#!/bin/bash
TARGET_URL='<shellQuote(url)>'
printf 'Status: 302 Found\r\n'
printf 'Location: %s\r\n' "$TARGET_URL"
printf 'Content-Type: text/plain; charset=utf-8\r\n'
printf '\r\n'
printf 'Redirecting to %s\n' "$TARGET_URL"
```

- `UIConfig` 用 `type` = openMode、`protocol` = `http`、`url` = `/cgi/ThirdParty/<child>/index.cgi`、`allUsers` = true。
- 因生成了 CGI，`builder.go` 额外生成 `install_callback` / `config_callback` / `upgrade_callback` 三个回调脚本：安装/改配/升级后用 heredoc 重写 `${TRIM_APPDEST}/app/ui/config` 与 `index.cgi` 并 `chmod +x`（`internal/packagebuilder/builder.go` 的 `generateCallback`）。
- 模板目录 `shortcuts/app/templates/external-url/`。

## 注意事项

- **安全要点（shellQuote）**：CGI 脚本里包 URL 用的是 `internal/provider/external_url.go` 的 `shellQuote`，**绝不能用 `fmt` 的 `%q`**。`%q` 产出 Go 风格双引号字符串，bash 在双引号内部照样做命令替换——用户填 `http://a/$(curl${IFS}x)` 这样的合法 URL 就能在 CGI 运行时执行任意命令。单引号内 bash 不做任何展开，安全。
- **`openMode` 空值归一化为 `url`**：`Validate` 与 `Changed` 比较前都要归一化，否则空值和 `url` 被判为不同。
- **`Changed()` 不计入 `desiredState`**：同 `internal_port`，避免「改一次启停」导致整包重装。
- **解码失败按「有变化」处理**：同 `internal_port`。
- **部分更新合并语义**：同 `internal_port`（`service.go` 的 `mergeConfigFields`）。
