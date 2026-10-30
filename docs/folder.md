# 文件夹快捷方式（folder）

把 NAS 内部文件夹（如 `/vol1/1000/照片`）生成为桌面上的**原生文件夹项**：双击图标由桌面自身打开文件管理器对应目录，全程不产生任何应用窗口。

这是三种类型里唯一**不生成子应用、不走部署链路**的：文件夹项由应用页面在飞牛桌面宿主内通过 `@trimjs/web-app` 的 `app.query` 调桌面内部接口直接创建。

## 字段与校验

表单见 `shortcuts/app/ui/www/index.html`（`.type-folder` 区块），提交见 `shortcuts/app/ui/www/app.js`（`config={path}`）。

| 字段 | 规则 |
| --- | --- |
| `appname` | `^[a-z][a-z0-9-]{1,11}$` |
| `displayName` | 必填，≤12 个字符 |
| `path` | 必须以 `/` 开头、≤512 字符、无控制字符（`model.IsValidFolderPath`）；**保存时校验真实存在且是目录** |

- **`openMode` 隐藏**：固定 iframe（SDK 只在桌面宿主内可用），UI 不给选打开方式（`app.js` 的 `toggleTypeFields`）。
- **自定义图标不适用**：桌面项用系统文件夹图标，表单隐藏图标上传区（`app.js` 的 `toggleTypeFields`）。
- appname 自动建议：按路径末段 slug（`app.js`）。

### 路径输入与存在性校验

**路径允许手动输入**：路径输入框不再因宿主可用而只读（`app.js` 的 `updateFolderBrowseButton` 只控制「浏览」按钮显隐），手填与「浏览」二选一；「浏览」只是快捷填法，不是唯一来源。

**保存时校验**（`app.js` 的 `saveEntry` folder 分支 → `validateFolderPath` → `GET /api/validate-folder`）：

1. 服务端 `Service.ValidateFolderPath`（`internal/service/desktop_item.go`）先做格式校验（`model.IsValidFolderPath`），再做存在性校验。
2. 存在性由 **root agent** 判定：`Manager.StatDir`（`internal/deployer/appcenter.go`）发 `stat-dir` 动作，agent 侧 `statDir`（`internal/agent/server.go`）只 `os.Stat` 判「存在且是目录」。**必须走 agent**——server 进程按 `TRIM_USERNAME` 降权运行、对 `/vol1` 共享目录无读权限，本地 `os.Stat` 会对真实目录误报失败。
3. 返回**机器码** `code`（`""` 有效 / `bad_format` 格式非法 / `not_found` 不存在或不是目录），前端映射 i18n 文案（`app.js` 的 `folderValidationMessage`）；**不要返回人读英文文案**——前端直接显示会出英文 toast（已踩坑）。
4. **fail-closed**：agent 不可用（`Unavailable`）时接口返回 503 + `code=unavailable`，前端提示「无法校验文件夹路径」并拒绝保存，不放行可能不存在的路径。

**agent 调用超时**：`stat-dir` 单次调用上限 `statDirAgentTimeout = 10s`（`internal/deployer/appcenter.go`），远小于 `DefaultCLITimeout=600s`；且 `stat-dir` **不占 agent 的 CLI 并发槽**（`internal/agent/server.go` 的 `handle` 对 `stat-dir` 跳过 `s.sem`）。否则一旦 appcenter 命令卡住占满槽位，路径校验会排队到分钟级，拖慢保存（已踩坑）。前端 `validateFolderPath` 超时 15s，大于后端上限，避免前后端竞相判超时。

**写锁说明**：存在性校验在 `Service.validateAgainst`（`internal/service/service.go`）内、持有写锁时调用 agent。正常毫秒级返回；最坏情况（agent 卡住）被 10s 上限封顶，写锁最多占 10s，不会像 600s 那样阻塞其它操作。

## 浏览选择（真机踩坑）

「浏览选择」按钮走宿主桥 `hostApp.pickFile`，按钮只在宿主桥可用时显示（`app.js` 的 `ensureHostApp`，懒加载 `import(API_BASE + "/sdk/trim-web-app.js")`；独立浏览器 `isStandaloneWeb=true` 时标记不可用）。

**必须用免授权的 `pickFile({directory:true})`（直接返回 `string[]`）**，踩坑记录（`app.js` 的 `browseFolderPath` 注释）：

- `pickUserFile` 确认后走主机授权登记，第三方应用实测必报「应用鉴权异常」（补全 `api-scope`、卸载重装均无效）——该环节对第三方应用不可用。
- `pickUserFile` 参数校验与文档不一致，缺 `sidebarGroup` 报 errno 10030「请求参数错误」。
- 本应用只需路径字符串（`openFileManager` 官方明确不做权限校验，后端不读目录），不需要授权登记，所以优先 `pickFile`。

调用（`app.js` 的 `browseFolderPath`）：

`hostApp.pickFile({directory:true})`（免授权，唯一调用路径——`pickUserFile` 对第三方应用鉴权异常、缺 `sidebarGroup` 报 10030，不用它回退）。

**取消/关闭静默**：选择器关闭返回空或「Operation failed」都没有路径，一律静默结束，不弹错误提示、不重试（快速关闭曾被「秒回=没弹窗」启发式误判而重弹选择器）。返回形状宽松解析（`extractPickedFolder`：字符串 / 数组 / `data` 数组 / 对象内 `path`/`filePath`/`fullPath`/`dir`）。

## 创建 / 销毁 / 重建链路

桌面内部接口常量 `DESKTOP_REQ`（`app.js`）：`appcgi.sac.entry.v1.getUserDesktop` / `addUserDesktopEntry` / `delUserDesktopEntry`（未公开接口）。

**创建**（`app.js` 的 `createDesktopItem`）：

1. 已有 key 且索引确认存在 → 幂等返回。
2. 索引可用时按 `name===displayName && folderPath===path` 认领已存在项（防服务端数据被清后重复创建；不能抢别的条目已认领的 key）。
3. `addUserDesktopEntry({itemType:"folder", name, folderPath, sort:999999})` → 返回 `data.entryKey`（形如 `folder:57`）。
4. `PUT /api/entries/{appname}/desktop-item` 回写 `entryKey`。
5. 回写失败 → `delUserDesktopEntry` 回滚刚建的项；回滚也失败时提示「桌面图标可能仍在，请手动删除」。

**销毁**（`removeDesktopItem`）：`delUserDesktopEntry` 后用实时列表确认删除（`result=succ` 但项还在也算失败）→ `DELETE .../desktop-item` 清 key。

**重建**（`rebuildDesktopItem`）：旧项未确认消失前不清 key（否则服务端记「未创建」而桌面项还在，再建时名字/路径变了认领不上，留下新旧两项）；移除后必须把 key 传 `null` 再创建。

**编辑保存**：名称/路径变化触发删旧建新（`app.js`）。**删除条目前**先销毁桌面项；宿主不可用时降级为「定义照删 + 提示手动清理」。

## 服务端 key 管理

- `PUT` / `DELETE /api/entries/{appname}/desktop-item`（`internal/app/desktop_item.go`）。body 上限 512B；**PUT 拒绝空串**（清空只能走 DELETE，防误请求静默抹掉凭据）。
- `Service.SetDesktopEntryKey`（`internal/service/desktop_item.go`）：只对 folder 类型；key 校验 `^folder:[0-9]{1,20}$`（只校验形状，不解析序号——序号由桌面端生成，回写只是认领凭据）。
- **不做去空白规整**：宿主回写的标识必须精确匹配，带空白的输入是脏数据，静默修剪会让本地记录和宿主记住的标识悄悄分叉。
- 空值与写入同走写锁，防「销毁」和「重建」并发互相覆盖。

## 状态机

服务端只由 `DesktopEntryKey` 决定（`service.go` 的 `decorate` folder 分支）：

- key 空 → `undeployed`（「未创建」）
- key 非空 → `deployed`（「已创建」）
- `runtime` 恒 `unknown`（folder 没有子应用进程，运行状态无意义）

「桌面项丢失」是前端用实时桌面列表覆盖的显示态（`app.js` 的 `folderEntryStatus`），**不回写**；索引拿不到时按服务端状态显示，不臆测。

## 桌面实时刷新

创建/销毁后应用向桌面补投一次 `user-desktop-entry-change` 事件，图标即时增减、不需手动 F5。机制依赖桌面未公开的事件约定，失败静默降级，此时按 F5 仍可刷新。

## 第一代实现历史（已废弃）

旧实现（commit `63e8740`，工作区已删除）是每个 folder 条目装一个 `micro_app` 子应用 + SDK 内联跳转页，点击 `openFileManager` 后立即关窗。真机踩坑（写文档必录）：

- **`micro_app=true` + `ctl_stop=false` → 桌面不出图标**（`micro_app=true` 本身无碍，已二分证实）。`ctl_stop` 保持默认 `true`。
- **入口键 `<child>.main` + 尾斜杠同样不出桌面图标**；裸键 + 无尾斜杠一直正常。
- **CGI 环境拿不到 `TRIM_APPDEST`**（它只保证给 cmd 生命周期脚本）→ 构建时把安装路径 baked 进脚本回退值：`BASE_PATH="${TRIM_APPDEST:-/var/apps/<child>/target}/www"`；否则真机 404 黑屏。
- **点击必然产生一个应用窗口**，即使「发打开请求立即关窗」仍闪屏——这是第二代重做的动机。
- `openFileManager` 需系统 ≥1.2.04011 / 桌面 App ≥34.0。

**遗留清理**：新机制创建的 folder 条目从未部署过子应用，`DeployedVersion` 为空时不发起注定失败的卸载；只有旧机制留下的子应用才在删除条目时随之清理（`service.go` 的 `DeleteEntry`）。

## 已知限制

- 桌面项接口为未公开的桌面内部 CGI，无兼容承诺；宿主升级可能失效。
- 桌面项每用户私有、无自定义字段，服务端拿不到用户身份 → **单管理员假设**：第二个管理员会覆盖第一个的 key 导致孤儿图标。
- **URL 直开文件管理器不可行**：实测抓 NAS 前端代码，FM 运行时 chunk 0 处读 `URLSearchParams`/`location.hash`/`search`；桌面壳 URL 参数只认 `?theme=`；社区无深链格式。
- 主应用 `shortcuts/app/ui/config` 入口 `allUsers:false`（仅管理员可见应用）；`shortcuts/config/resource` 的 `api-scope` 为 `["trim.file.userAccess","trim.file.userAcl"]`（pickFile 所需）。
