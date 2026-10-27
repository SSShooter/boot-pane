# bootpane 项目约定

## 项目性质

Wails3 桌面应用，做「项目启动器」：列出项目，每个项目挂多个启动按钮，按钮由 JSON 配置定义。

## 硬性约定

- **平台差异只允许出现在 `internal/pty` 和 `internal/proctree` 的 build tag 文件里**。
  业务层代码不写 `runtime.GOOS` 判断，只依赖 `pty.Session` / `proctree.Group` 接口。
- **配置默认全局**：按钮存在 `%APPDATA%\bootpane\workspace.json`，不往仓库塞文件。
  仓库内 `.bootpane.json` 要用户显式打开 `UseRepoConfig` 才生效，且**二选一不叠加**
  （仓库配置完全取代全局配置）。
- **`waitFor` 是就绪判定的唯一依据**，不用固定 sleep 猜启动时间。
- **日志推送必须节流**（攒 50ms 或 32KB 批量推），否则高频输出会打满 Wails 事件通道。
- 前端调后端统一走 `frontend/src/lib/api.ts`，不在组件里直接 import bindings。
  这样纯浏览器跑 `npm run dev` 时能自动切到 `devmock.ts` 假数据。
- **Wails service 方法不要用 `(T, error)` 返回业务错误**：error 非空时 T 会被丢弃，
  前端拿不到细节。业务错误放返回值里（参考 `ImportResult`），error 只用于系统级失败。

## 常用命令

```bash
wails3 dev                                    # 起桌面应用
cd frontend && npm run dev                    # 只调 UI（走 mock 数据，9245 端口）
wails3 generate bindings -ts -d frontend/bindings .   # Go 改了服务方法后重新生成
go test ./internal/...                        # 配置模块单测
```

## 待办

- M1：PTY + Job Object + 日志流
- Wails v3.0.0 与本机 WebView2 155 的崩溃问题待查（详见 2026-10-27 日志）
