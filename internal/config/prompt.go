package config

import (
	"strings"

	"bootpane/internal/i18n"
	"bootpane/internal/pty"
)

// promptBuilders 把语言映射到 prompt 生成函数。
//
// prompt 会直接显示在「生成配置」弹窗里，用户还要把它复制到模型那边用。
// 界面是英文却给一段中文 prompt，等于让用户每次先自己翻一遍，所以它跟着
// 界面语言走。新增语言时在这里注册，同时补一份 prompt_<lang>.go。
var promptBuilders = map[i18n.Lang]func(ResolvedProject, string) string{
	i18n.ZH: promptZH,
	i18n.EN: promptEN,
	i18n.PT: promptPT,
	i18n.RU: promptRU,
}

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
//     所以引导模型优先用日志标记（log），端口只在真实读到时才写；
//     waitPort 本身已做 IPv4/IPv6 双栈探测，只绑 [::1] 的服务（如 WXT）不会误判。
//   - shell 名字从 pty 拿，业务层不判断平台。
//
// 各语言版本的内容必须保持一致：只翻措辞，不加也不减规则。
func BuildPrompt(p ResolvedProject) string {
	build, ok := promptBuilders[i18n.Current()]
	if !ok {
		build = promptEN
	}
	return build(p, pty.ShellDescription())
}

// joinPrompt 统一给 prompt 收尾，省得各语言版本各写一次换行。
func joinPrompt(lines []string) string {
	return strings.Join(lines, "\n") + "\n"
}
