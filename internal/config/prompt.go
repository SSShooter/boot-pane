package config

import (
	"strings"

	"bootpane/internal/pty"
)

// BuildPrompt 生成一段可以直接贴给任意大模型的 prompt，让它产出符合 schema 的启动配置。
//
// 刻意不在 prompt 里塞仓库摘要：让用户把这段内容拿到项目目录下用，模型自己读
// package.json、go.mod 这些文件，比我们生成一份可能已经过期的摘录更准。
//
// 下面几条都是踩过坑才写进去的，改之前先看一遍：
//   - 不提 dependsOn / pipelines。这两个字段目前没有执行方（Manager.Start 只接
//     单个 task，前端也没有 pipeline 入口），prompt 里一写就等于引导用户配一个
//     点了不生效的东西。等 M2 接上调度再放回来。
//   - 必须讲清楚「改端口用 env」。命令最终由 shell 执行，Windows 下
//     `PORT=5173 pnpm dev` 这种前缀赋值直接报错。
//   - waitFor 要给出「拿不准就省略」的出路。猜错端口的后果不是简单失败：
//     Manager.fail 只改状态不杀进程，用户会看到「失败」而服务其实是活的。
//   - shell 名字从 pty 拿，业务层不判断平台。
func BuildPrompt(p ResolvedProject) string {
	shell := pty.ShellDescription()

	lines := []string{
		"我要给下面这个项目配置 bootpane 的启动按钮。bootpane 是项目启动器，一个按钮对应一条长期运行的启动命令。",
		"",
		"项目根目录：",
		"    " + p.Path,
		"",
		"请在这个目录下读文件（package.json、go.mod、docker-compose.yml、Makefile、vite.config.*、",
		".env*、各种 lockfile），命令必须来自你真正读到的内容，不要凭经验猜。",
		"",
		"【找什么】",
		"找出这个项目里需要长期运行的进程入口：dev server、watch、mock 服务、后端 API。每个入口一个 task。",
		"不要列一次性任务——install、build、lint、test、格式化这类跑完就退出的，不属于启动器。",
		"",
		"【命令怎么填】",
		"- 包管理器看 lockfile 决定：pnpm-lock.yaml 用 pnpm，yarn.lock 用 yarn，",
		"  package-lock.json 用 npm，bun.lockb 用 bun。不要默认用 npm。",
		"- command 只能填 package.json 的 scripts 里已存在的脚本名（如 pnpm dev:h5），",
		"  或仓库里真实存在的可执行文件。读不到就不要写。",
		"- cwd 是命令执行的子目录，相对项目根目录，必须是真实存在的目录；",
		"  不要以 / 开头，不要用 .. 往上跳。在根目录执行就留空字符串。",
		"- 要改端口、传参数，用 env 字段，不要塞进 command。",
		"",
		"【端口从哪找】",
		"依次看：vite.config.* / webpack.config.* 的 server.port、package.json scripts 里的 --port、",
		".env* 里的 PORT 或 VITE_PORT、docker-compose.yml 的 ports、",
		"后端的 application.yml / config.toml / appsettings.json。",
		"找不到就不要写端口。",
		"",
		"【就绪判定 waitFor】",
		"bootpane 靠它判断「启动完成」，三选一：",
		`1. 确定了端口：{ "type": "port", "port": 5173, "timeout": 60 }`,
		"2. 端口不确定，但启动输出里有稳定出现、且只在就绪之后才出现的原文：",
		`   { "type": "log", "log": "ready in", "timeout": 120 }`,
		"   log 是大小写敏感的原文子串匹配，不是正则，不要改写大小写。",
		"3. 两者都拿不准：整个省略 waitFor 字段。",
		"timeout 单位是秒。猜一个端口比不写更糟——猜错会让任务被标成失败，而进程其实还在跑。",
		"",
		"【运行环境】",
		"命令由 " + shell + "执行，这个 shell 有几个硬约束：",
		"- 不要写 PORT=5173 pnpm dev 这种前缀赋值，cmd 不认，改用 env 字段",
		"- 不要用单引号，不要用 & 把命令放到后台",
		"- 命令里尽量别带双引号，会被 shell 吞掉一层",
		"- 一个 task 一条命令，不要用 && 或 ; 把多个服务串起来",
		"",
		"【硬性要求】",
		"1. id 只用小写字母、数字、短横线，项目内唯一。",
		"2. label 用中文，是按钮上显示的名字。",
		"3. group 用来归类按钮，比如 前端 / 后端 / 小程序。",
		"4. 不同 task 不要用同一个端口。",
		"5. task 数量控制在 1~5 个，只保留真正需要常驻的入口。",
		`6. 如果这个项目里找不到任何需要长期运行的入口，输出 {"version": 1, "tasks": []}，`,
		"   不要为了凑数编命令。",
		"",
		"【输出】",
		"只输出一个 json 代码块，不要任何解释文字，不要注释。结构如下：",
		"",
		"{",
		`  "version": 1,`,
		`  "tasks": [`,
		"    {",
		`      "id": "web",`,
		`      "label": "启动网页端",`,
		`      "group": "前端",`,
		`      "command": "pnpm dev:h5",`,
		`      "cwd": "",`,
		`      "env": { "PORT": "5173" },`,
		`      "waitFor": { "type": "port", "port": 5173, "timeout": 60 }`,
		"    },",
		"    {",
		`      "id": "api",`,
		`      "label": "启动后端",`,
		`      "group": "后端",`,
		`      "command": "go run ./cmd/api",`,
		`      "cwd": "server",`,
		`      "waitFor": { "type": "log", "log": "listening on", "timeout": 60 }`,
		"    }",
		"  ]",
		"}",
		"",
		"字段一览：id / label / group / command / cwd / env / waitFor，",
		"其中 group、cwd、env、waitFor 都可以省略。",
	}

	return strings.Join(lines, "\n") + "\n"
}
