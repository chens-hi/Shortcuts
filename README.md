# 快捷图标

[![GitHub release](https://img.shields.io/github/v/release/chens-hi/shortcuts?label=release)](https://github.com/chens-hi/shortcuts/releases/latest)
[![License](https://img.shields.io/github/license/chens-hi/shortcuts)](LICENSE)
![fnOS](https://img.shields.io/badge/fnOS-%E2%89%A5%201.2.0-1f6feb)

## 功能

一键在飞牛 fnOS 桌面上创建快捷原生图标。

## 安装

fnOS **1.2.0 及以上**，需要**管理员**账号（应用仅对管理员可见）。

从 [Releases](https://github.com/chens-hi/shortcuts/releases/latest) 下载 `.fpk` → fnOS「应用中心 → 手动安装」→ 桌面出现「快捷图标」，点开即可添加配置。

## 开发

仓库使用 Go 标准库（无第三方依赖）+ 原生 JS 前端。

### 构建与打包

```powershell
.\tools\bump-version.ps1            # 可选：升版本号
.\tools\package-shortcuts.ps1       # 出包 dist/shortcuts-<version>.fpk
.\tools\check-package.ps1 -PackagePath dist/shortcuts-<version>.fpk
```

包为 amd64 + arm64 合并（manifest `platform=all`），单包全平台通吃。

当前测试包版本：`1.1.2`。

### 测试与校验

```bash
go test ./...
go vet ./...
```

```powershell
.\tools\validate-ui.ps1
.\tools\check-encoding.ps1
```

### 发布

`push` 到 `main` 且改动包含 `shortcuts/manifest` 会触发 GitHub Actions，自动同步版本 → 全平台打包 → 校验 → 发布。

## 许可

[MIT](LICENSE)。本项目是第三方应用，与飞牛官方无关。
