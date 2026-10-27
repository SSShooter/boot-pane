// Package prockill 终止任意进程及其整棵子树。
//
// 为什么需要独立一层：proctree 管的是「我们自己启动、已经挂进 Job Object」的进程，
// 但重启外部实例时面对的是完全不受我们管的进程——只能靠 Toolhelp 快照摸清父子关系，
// 再逐个 TerminateProcess。两种场景的生命周期管理方式完全不同，不能混用。
package prockill

import "errors"

// ErrUnsupported 表示当前平台尚未实现。
var ErrUnsupported = errors.New("当前平台不支持终止外部进程")

// Killed 是被终止的一个进程。
type Killed struct {
	PID  int    `json:"pid"`
	Name string `json:"name"`
}

// Tree 返回 pid 及其所有子孙进程，**子进程排在父进程之前**。
//
// 顺序是刻意安排的：先杀子再杀父。反过来父进程先死的话，某些守护型脚本
// 会在退出前把子进程重启一遍，结果就是杀了个寂寞。
func Tree(root int) ([]Killed, error) {
	return tree(root)
}

// Kill 终止 pid 及其所有子孙进程，返回实际被终止的进程（子在前父在后）。
//
// 已经自行退出的进程会被跳过，不算失败——重启场景下这很常见。
func Kill(root int) ([]Killed, error) {
	return kill(root)
}
