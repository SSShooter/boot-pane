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
- **个人偏好类状态存 workspace.json 顶层，不要塞进 `Task`/`Project` 定义**。
  任务定义要能跟仓库走（`.bootpane.json` 是团队共享的），星标这类本机习惯一旦混进去，
  切到项目内配置就无处落地。参考 `Workspace.Stars` + `ResolvedProject.StarredTaskIDs`。
- **配置写入只留一个入口**：`ConfigService.SaveConfigJSON(id, raw)`。不要加
  `UpsertTask` / `DeleteTask` 这种细粒度方法——那是第二套写入路径，校验逻辑要写两遍，
  迟早出现「单条删除绕过校验」。加/改/删都靠前端改 JSON 再整份提交。
- **`.dialog` 只给 `max-height` 时，子元素的 `flex: 1` 是失效的**（容器高度由内容决定，
  没有可分配空间）。需要撑满的弹窗（如 JSON 编辑器）必须给死 `height`。

## 验证方式（硬性，别再试错）

**本机 `wails3 dev` 和 `bin/bootpane.exe` 都跑不起来**：WebView2 155.0.4283.62 渲染
进程崩溃循环，窗口空白。这是环境问题，跟代码无关，已经排查过一轮（见 2026-10-27 日志），
不要再尝试、不要再验证、不要提议"跑一下看看"。

所以验证只走这三条：

| 要验什么 | 怎么做 |
|---|---|
| Go 侧行为（进程管理、端口、配置） | `go test ./internal/...`，写集成测试 |
| UI 静态渲染 | `cd frontend && npm run dev`（9245 端口，走 devmock 假数据）<br>再用 Chrome headless 截图：`--headless=new --disable-gpu --window-size=1180,900 --screenshot=... --virtual-time-budget=6000 http://127.0.0.1:9245/` |
| UI 交互（点击、输入、状态变化） | **CDP 驱动 headless Chrome**，见下 |

**CDP 交互验证**（playwright-cli 没装，别去装）：Chrome 加
`--headless=new --remote-debugging-port=9333 --user-data-dir=<临时目录>`，
Node 22 自带全局 `WebSocket` / `fetch`，连上 `/json/list` 的
`webSocketDebuggerUrl` 发 `Runtime.evaluate` / `Page.captureScreenshot` 即可。
改 Svelte 的 `bind:value` 要用 `HTMLTextAreaElement.prototype` 的 value setter
再 `dispatchEvent(new Event('input', {bubbles:true}))`，直接赋 `.value` 不会触发。

需要看交互后的静态状态时，也可以临时在 `App.svelte` 的 onMount 里加一行
`if (isPreview) void app.openLog(...)`，截完图立刻删掉（优先用 CDP，不用改代码）。

`wails3 generate bindings` 可以正常用（纯代码生成，不起窗口）。

## 常用命令

```bash
cd frontend && npm run dev                    # 只调 UI（走 mock 数据，9245 端口）
wails3 generate bindings -ts -d frontend/bindings .   # Go 改了服务方法后重新生成
go test ./internal/... -timeout 180s          # 全部 Go 测试
cd frontend && npm run check && npm run build # 前端类型检查 + 构建
```

`npm run dev` 和 `wails3 dev` **都占 9245 且都是 strictPort，不能同时开**，
同时开后者直接报 `bind: Only one usage of each socket address`。
调 UI 要独立端口时用 `npm run dev -- --port 9247`。

**`npm run dev` 报 `SAFE_DELETE_BULK_CONFIRM_REQUIRED`**（vite 想清 `node_modules/.vite/deps_ssr`
被沙箱批量删除保护拦下）时，不要删、直接改名：
`mv frontend/node_modules/.vite frontend/node_modules/.vite.bak`，vite 会重建一份新的。

## 前端样式约定

- 样式全在 `frontend/public/style.css`。**它在 `public/` 下，vite 不做 HMR**，
  改完必须整页刷新（Ctrl+Shift+R）。只改 CSS 却看到「改了没生效」，先怀疑缓存。
- `.task-grid` 最小轨道 320px 是算出来的下限：卡片底部一行
  「状态 + CPU/内存 + 日志 + 停止」需要约 300px，低于这个值就会挤爆。
- 卡片底部**禁止 `flex-wrap: wrap`**。状态文字长度会变（运行中 / 等待就绪 / 已退出），
  一换行按钮组就在两行之间来回跳。正确做法：按钮组 `margin-left: auto; flex: none`
  固定靠右，状态文字 `flex: 0 1 auto; min-width: 0` 收缩省略（文字要包一层 `.state-text`，
  因为 flex 容器的 `text-overflow` 对裸文本节点无效）。
- 命令行走 `overflow-wrap: break-word`，**不要用 `break-all`**——后者把每个字符都当断点，
  会出现 `ExecutionP / olicy` 这种词中间硬断。

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
