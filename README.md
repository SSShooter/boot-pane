# bootpane

项目启动器。解决的问题只有一个：项目太多，每次都想不起来该怎么起。

打开应用能看到所有项目，每个项目下面挂着若干个启动按钮——大仓库的前端和后端分开起、uni-app 先起网页端再起小程序端，都是一行配置的事。

## 当前进度

**M0 已完成**：项目列表、配置解析与校验、任务按钮渲染、空/错误状态、AI 生成配置（复制 prompt → 粘贴导入）。

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

默认情况下所有启动按钮都存在本机的全局注册表里，**不需要往仓库里塞任何文件**：

```
%APPDATA%\bootpane\workspace.json
```

每个项目在界面上有个「全局 / 项目内」开关。只有切到「项目内」，才会去读仓库根目录的 `.bootpane.json`——这个文件是可选的，适合想把启动方式跟代码一起提交、供团队共享的场景。两种来源**二选一，不叠加**：切到项目内之后，全局里配的按钮完全不生效。

仓库里放了 `.bootpane.json` 时，即使当前用的是全局配置，界面也会提示一句「这个仓库自带配置，要不要切过去」。

### 用 AI 生成配置

项目详情页右上角有个「生成配置」，点开是两步：

1. **复制 prompt** — 里面写好了 JSON schema 和字段规则。把它拿到**项目根目录下打开的 AI 工具**里粘贴，让模型自己读 `package.json`、`go.mod` 这些文件。不预置仓库摘要，是因为模型现读比我们塞一份可能过期的摘录准得多。
2. **粘回来导入** — 把模型返回的 JSON 粘进去。导入前会跑一遍校验（命令不能为空、`cwd` 必须存在、`dependsOn` 不能指向不存在的任务、不能成环），有问题就逐条列出来且**一条都不写盘**，通过了才存进全局注册表。

### 配置格式

```jsonc
{
  "version": 1,
  "project": {
    "name": "my-shop",
    "description": "uni-app 商城，前端 h5 + 小程序 + 本地 mock 服务",  // 显示在项目头部
    "tags": ["uni-app"]
  },
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

`project.name` 和 `project.description` 是模型生成配置时一并给出的项目元信息：导入时会写进项目（名称显示在侧栏和标题，描述显示在标题下），模型省略或给了空值时保留原值。标签不在这份输出约定里，导入不会改动它；在编辑框里保存则以框里为准，删空就是删空。

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
| M4 | 内置规则扫描器，减少对复制 prompt 的依赖 |
| M5 | 直连 LLM（OpenAI 兼容），省掉手动复制粘贴 |
| M6 | 残留进程清理、日志落盘、快捷键、托盘 |
