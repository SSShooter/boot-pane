# bootpane 项目约定

## 项目性质

Wails3 桌面应用，做「项目启动器」：列出项目，每个项目挂多个启动按钮，按钮由 JSON 配置定义。

## 硬性约定

- **平台差异只允许出现在 `internal/pty` 和 `internal/proctree` 的 build tag 文件里**。
  业务层代码不写 `runtime.GOOS` 判断，只依赖 `pty.Session` / `proctree.Group` 接口。
- **配置分两层**：全局注册表存「有哪些项目」，仓库内 `.bootpane.json` 存「怎么启动」。
  合并按 `task.id`，仓库优先，全局层只补缺（标 `origin: "local"`）。
- **`waitFor` 是就绪判定的唯一依据**，不用固定 sleep 猜启动时间。
- **日志推送必须节流**（攒 50ms 或 32KB 批量推），否则高频输出会打满 Wails 事件通道。
- 前端调后端统一走 `frontend/src/lib/api.ts`，不在组件里直接 import bindings。
  这样纯浏览器跑 `npm run dev` 时能自动切到 `devmock.ts` 假数据。

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
