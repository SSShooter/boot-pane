# bootpane

项目启动器。解决的问题只有一个：项目太多，每次都想不起来该怎么起。

打开应用能看到所有项目，每个项目下面挂着若干个启动按钮——大仓库的前端和后端分开起、uni-app 先起网页端再起小程序端，都是一行配置的事。

## 当前进度

**M0 已完成**：项目列表、双层配置合并、配置校验、任务按钮渲染、空/错误状态。

**M1 待做**：进程启动、PTY 日志流、进程树终止。现在点「启动」会提示进程管理尚未接入。

## 运行

```bash
wails3 dev
```

只想调界面的话，直接跑前端开发服务器，会走内置假数据：

```bash
cd frontend && npm run dev   # http://127.0.0.1:9245
```

## 配置

配置分两层，因为它们属于不同的东西：

| 层 | 位置 | 存什么 | 进 git |
|---|---|---|---|
| 全局注册表 | `%APPDATA%\bootpane\workspace.json` | 有哪些项目、路径、标签、个人覆盖的按钮 | 否 |
| 仓库配置 | `<repo>/.bootpane.json` | 这个项目怎么启动，团队共享 | 是 |

合并时按 `task.id` 匹配，仓库配置优先；全局层只补仓库里没有的 id，界面上会标一个「本地」角标。

仓库根目录放一个 `.bootpane.json` 就能定义按钮：

```jsonc
{
  "version": 1,
  "project": { "name": "my-shop", "tags": ["uni-app"] },
  "tasks": [
    {
      "id": "web",
      "label": "启动网页端",
      "group": "前端",              // 按钮分组
      "command": "pnpm dev:h5",
      "cwd": ".",                  // 相对项目根目录
      "env": { "PORT": "5173" },
      "waitFor": { "type": "port", "port": 5173, "timeout": 60 },
      "singleton": true            // 默认 true，防止同一按钮起两份
    },
    {
      "id": "mp-weixin",
      "label": "启动小程序端",
      "group": "小程序",
      "command": "pnpm dev:mp-weixin",
      "dependsOn": ["web"],        // 等 web 就绪后再启动
      "waitFor": { "type": "log", "log": "Build complete", "timeout": 120 }
    }
  ],
  "pipelines": [
    { "id": "all", "label": "全启", "tasks": ["web", "mp-weixin"], "mode": "serial" }
  ]
}
```

`waitFor` 是这里最关键的一个字段：它定义了「什么算启动完成」。没有它就只能够靠 `sleep 3` 猜，而猜是不准的——端口没起来就启动下一个，下一个必然失败。

## 代码结构

```
main.go                    应用入口、窗口、服务注册
app_services.go            暴露给前端的 ConfigService / ProcessService
internal/config/           两层配置、合并、校验
internal/process/          任务状态机与进程管理（M1 填充）
internal/pty/              伪终端接口，Windows/Unix 分实现
internal/proctree/         进程树管理接口，Windows/Unix 分实现
internal/discovery/        仓库扫描与配置生成（M4 填充）
frontend/src/              Svelte 界面
```

`pty` 和 `proctree` 把平台差异收在两个接口后面，业务代码里不出现 `runtime.GOOS` 判断——现在只实现 Windows，后面补 macOS/Linux 只需新增 `_unix.go` 文件。

## 路线图

| 阶段 | 内容 |
|---|---|
| M1 | 启动 + PTY 日志流 + 停止（Job Object 杀整棵树） |
| M2 | `dependsOn` / `waitFor` / pipeline 编排 |
| M3 | 配置编辑界面（表单 + 原始 JSON） |
| M4 | 仓库扫描：规则引擎 + 校验器 + diff 预览 |
| M5 | LLM 增强（OpenAI 兼容，复用校验器拦幻觉） |
| M6 | 残留进程清理、日志落盘、快捷键、托盘 |
