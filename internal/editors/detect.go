// Package editors 探测本机装了哪些能「打开目录」的编辑器。
//
// 目的是让设置页只列用户真正装了的编辑器，而不是给一份写死的、其中大半没装
// 的清单。两层判断：
//  1. 内置清单：PATH 上有对应的命令行，或已知安装位置上真的存在启动垫片；
//  2. 兜底扫描：在应用安装目录里按「VS Code 家族垫片指纹」找，覆盖清单
//     没列举到的分支编辑器（Antigravity、Kiro 这类）。
//
// 命中后返回的 Command 可以直接存进 settings.openWith。
//
// 这里只做「找得到」，不启动任何程序：探测是打开设置页时的顺手动作，
// 拉起 IDE 窗口属于用户点「打开目录」之后的副作用。
package editors

import (
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
)

// 检测来源。
const (
	// SourcePath 表示命令来自 PATH，存下来的是裸命令名（如 `code`）。
	SourcePath = "path"
	// SourceInstall 表示命令来自内置清单里的安装位置，存下来的是绝对路径。
	SourceInstall = "install"
	// SourceScan 表示命令是扫描应用目录发现的，存下来的是绝对路径。
	SourceScan = "scan"
)

// Editor 是本机检测到的一个编辑器。
type Editor struct {
	ID   string `json:"id"`
	Name string `json:"name"`

	// Command 是可以直接存进 settings.openWith 的命令行。
	//
	// PATH 命中时是裸命令名，安装位置和扫描结果命中时是加了引号的绝对路径。
	Command string `json:"command"`

	// Source 说明这个命令是怎么来的，取值见 SourcePath / SourceInstall / SourceScan。
	// 界面上不一定展示，但排查「为什么给的是这条命令」时得靠它。
	Source string `json:"source"`
}

// Detect 扫一遍本机可能存在的编辑器，返回按名称排序的结果。
//
// 不做缓存：调用发生在打开设置页这类低频动作上，几十次 stat 的代价可以忽略；
// 而用户刚装完编辑器就点「重新扫描」时，缓存反而会让结果对不上。
func Detect() []Editor {
	list := make([]Editor, 0, 8)
	takenPath := make(map[string]bool)
	takenCLI := make(map[string]bool)

	for _, c := range catalog() {
		if e, ok := c.probe(); ok {
			list = append(list, e)
			if e.Source == SourcePath {
				takenCLI[strings.ToLower(e.Command)] = true
			} else {
				takenPath[filepath.Clean(unquotePath(e.Command))] = true
			}
		}
	}

	// 兜底扫描补上清单没列举到的分支编辑器。同一份安装如果已经通过 PATH
	// 或清单命中就跳过，否则下拉框里会出现两个「VS Code」。
	for _, e := range scanCodeForks(forkRoots()) {
		path := filepath.Clean(unquotePath(e.Command))
		stem := strings.TrimSuffix(filepath.Base(path), filepath.Ext(path))
		if takenPath[path] || takenCLI[strings.ToLower(stem)] {
			continue
		}
		takenPath[path] = true
		list = append(list, e)
	}

	sort.Slice(list, func(i, j int) bool {
		return strings.ToLower(list[i].Name) < strings.ToLower(list[j].Name)
	})
	return list
}

// candidate 描述一个编辑器「可能以哪些形式存在」。
//
// clis 和 paths 的顺序就是优先级：先看 PATH。PATH 命中时存裸命令名，
// 编辑器升级、换安装目录都不影响；安装位置命中时只能存绝对路径，换了目录
// 就会失效，但那也比完全找不到强。
type candidate struct {
	id    string
	name  string
	clis  []string
	paths []string
}

// probe 依次尝试 PATH 和安装位置，命中第一个就返回。
func (c candidate) probe() (Editor, bool) {
	for _, name := range c.clis {
		if _, err := exec.LookPath(name); err == nil {
			return Editor{ID: c.id, Name: c.name, Command: name, Source: SourcePath}, true
		}
	}
	for _, path := range c.paths {
		if path == "" {
			continue
		}
		if info, err := os.Stat(path); err == nil && !info.IsDir() {
			return Editor{ID: c.id, Name: c.name, Command: quotePath(path), Source: SourceInstall}, true
		}
	}
	return Editor{}, false
}

// quotePath 给带空格的路径加双引号。
//
// 设置里存的是命令行，splitCommandLine 只认双引号；`C:\Program Files\...`
// 不加引号会被拆成两段，前半段当成程序名，点「打开目录」直接报找不到命令。
func quotePath(path string) string {
	if strings.ContainsAny(path, " \t") {
		return `"` + path + `"`
	}
	return path
}

// unquotePath 去掉 quotePath 加上的引号。
//
// 用来比较两条命令是不是同一个程序：清单里存的是原始路径，扫描结果是
// 加过引号的，直接比字符串会把同一份安装算成两个。
func unquotePath(command string) string {
	return strings.Trim(command, `"`)
}
