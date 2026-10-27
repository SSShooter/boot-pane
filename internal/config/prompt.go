package config

import (
	"strings"
)

// BuildPrompt 生成一段可以直接贴给任意大模型的 prompt，让它产出符合 schema 的启动配置。
//
// 刻意不在 prompt 里塞仓库摘要：让用户把这段内容拿到项目目录下用，模型自己读
// package.json、go.mod 这些文件，比我们生成一份可能已经过期的摘录更准。
// 所以 prompt 的第一段就是使用方式提示，界面上也会再提示一次。
func BuildPrompt(p ResolvedProject) string {
	lines := []string{
		"我要给一个项目配置 bootpane 的启动按钮。bootpane 是项目启动器，每个按钮对应一条启动命令。",
		"",
		"【使用方式】请把这段内容放到项目根目录下使用：",
		"    " + p.Path,
		"在该目录里打开你的 AI 工具再贴这段话，这样你能直接读到 package.json、go.mod、",
		"docker-compose.yml、Makefile 等文件，给出的命令才是真实存在的。",
		"",
		"先通读一遍项目，搞清楚它有哪些独立的启动入口（前端 / 后端 / 小程序端 / mock 服务……），",
		"然后输出一份 JSON。",
		"",
		"只输出一个 json 代码块，不要任何解释文字。",
		"",
		"JSON 结构：",
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
		`      "dependsOn": [],`,
		`      "waitFor": { "type": "port", "port": 5173, "timeout": 60 }`,
		"    }",
		"  ],",
		`  "pipelines": [`,
		`    { "id": "all", "label": "全启", "tasks": ["web"], "mode": "serial" }`,
		"  ]",
		"}",
		"",
		"字段说明：",
		"- id：英文短横线命名，项目内唯一，如 web / mp-weixin / api",
		"- label：按钮上显示的名字，用中文",
		"- group：分组，用来把按钮归类，比如 前端 / 后端 / 小程序",
		"- command：完整的启动命令。必须是仓库里真实存在的脚本——前端只能填 package.json 的",
		"  scripts 里已有的名字（如 pnpm dev、npm run dev:h5），不要编造",
		"- cwd：命令执行的子目录，相对项目根目录；在根目录执行就留空字符串",
		"- dependsOn：必须先启动完成的其他 task id",
		"- waitFor：判断「启动完成」的条件，二选一：",
		`    { "type": "port", "port": 5173, "timeout": 60 }        端口开始监听`,
		`    { "type": "log", "log": "Build complete", "timeout": 120 }   日志出现关键字`,
		"  能给 port 就给 port，端口号要按项目的实际配置填",
		"- pipelines：可选，把多个 task 组合成一键启动，tasks 里填要一起启动的 task id，",
		"  mode 为 serial（顺序）或 parallel（并行）",
		"",
		"几条硬性要求：",
		"1. 有先后依赖的（比如 uni-app 要先起网页端、再起小程序端），用 dependsOn 表达，",
		"   不要用 sleep 或者 && 把命令串起来。",
		"2. 一个仓库有多个启动入口就拆成多个 task，用 group 分开。",
		"3. 不要输出 JSON 注释。",
		"4. 不要写你在仓库里没见过的命令。",
	}

	return strings.Join(lines, "\n") + "\n"
}
