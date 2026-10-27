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

## 验证方式（硬性，别再试错）

**本机 `wails3 dev` 和 `bin/bootpane.exe` 都跑不起来**：WebView2 155.0.4283.62 渲染
进程崩溃循环，窗口空白。这是环境问题，跟代码无关，已经排查过一轮（见 2026-10-27 日志），
不要再尝试、不要再验证、不要提议"跑一下看看"。

所以验证只走这两条：

| 要验什么 | 怎么做 |
|---|---|
| Go 侧行为（进程管理、端口、配置） | `go test ./internal/...`，写集成测试 |
| UI 渲染 | `cd frontend && npm run dev`（9245 端口，走 devmock 假数据）<br>再用 Chrome headless 截图：`--headless=new --disable-gpu --window-size=1180,900 --screenshot=... --virtual-time-budget=6000 http://127.0.0.1:9245/` |

需要看交互后的状态（比如日志抽屉展开），临时在 `App.svelte` 的 onMount 里加一行
`if (isPreview) void app.openLog(...)`，截完图立刻删掉。

`wails3 generate bindings` 可以正常用（纯代码生成，不起窗口）。

## 常用命令

```bash
cd frontend && npm run dev                    # 只调 UI（走 mock 数据，9245 端口）
wails3 generate bindings -ts -d frontend/bindings .   # Go 改了服务方法后重新生成
go test ./internal/... -timeout 180s          # 全部 Go 测试
cd frontend && npm run check && npm run build # 前端类型检查 + 构建
```

## 待办

- M2：`DependsOn` 依赖链、Pipeline 串/并行启动（M1 只做了单任务）
- WebView2 崩溃：需要用户在自己机器上处理，不由 agent 碰

## Win32 层约定

- 所有 Win32 声明收敛在 `internal/win32`，业务层不直接 import。
- 不用 `golang.org/x/sys/windows`：CreatePseudoConsole、PROC_THREAD_ATTRIBUTE_JOB_LIST
  都没导出，混用两套声明容易出现结构体布局不一致。
- 平台差异只出现在 `internal/{pty,proctree,win32,portinfo}` 的 build tag 文件里。

## 状态语义约定

- **任务状态 ≠ 服务状态**。`runs` 只反映「bootpane 启动的那个进程」，
  端口占用反映「服务是否活着」，两者会不一致（脚本幂等跳过是典型场景）。
  界面必须分别表达，不允许用任务状态去暗示服务状态。
- 新增任何「是不是在跑」的判断，先想清楚问的是哪一个。
