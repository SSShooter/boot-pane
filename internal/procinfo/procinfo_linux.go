//go:build linux

package procinfo

import (
	"bytes"
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"
)

// List 逐个读 /proc/<pid>/stat 拼出全量进程快照。
//
// /proc 目录本身就是进程表的实时视图：读的过程中有进程退出、新起都是
// 正常情况，读不到 stat 的条目直接跳过，不算错误。
func List() ([]Proc, error) {
	entries, err := os.ReadDir("/proc")
	if err != nil {
		return nil, err
	}

	out := make([]Proc, 0, len(entries))
	for _, e := range entries {
		pid, err := strconv.Atoi(e.Name())
		if err != nil || pid <= 0 {
			continue // 非 <pid> 命名的条目（sys、irq 之类）不关心
		}
		s, err := readStat(pid)
		if err != nil {
			continue // 进程刚好退出，跳过
		}
		out = append(out, s.Proc)
	}
	return out, nil
}

// Rusage 从 /proc/<pid>/stat 取累计 CPU 时间和常驻内存。
//
// utime / stime 的单位是 USER_HZ（用户态恒为 100，与内核 CONFIG_HZ 无关）；
// rss 字段单位是页，乘上页大小得到字节数。
func Rusage(pid int) (time.Duration, uint64, error) {
	s, err := readStat(pid)
	if err != nil {
		return 0, 0, err
	}
	return s.cpuTime(), s.rssBytes(), nil
}

// procStat 是解析完的一条 /proc/<pid>/stat。Proc 是对外快照，
// 多出来的三个原始字段只服务于 Rusage。
type procStat struct {
	Proc
	utimeTicks uint64 // 用户态 CPU 时间，USER_HZ 为单位
	stimeTicks uint64 // 内核态 CPU 时间
	rssPages   uint64 // 常驻内存，页为单位
}

func (s procStat) cpuTime() time.Duration {
	return time.Duration(s.utimeTicks+s.stimeTicks) * time.Second / userHZ
}

func (s procStat) rssBytes() uint64 {
	return s.rssPages * uint64(os.Getpagesize())
}

// userHZ 是用户态看到的时钟频率。Linux 把它固定在 100（USER_HZ），
// 内核自己的 CONFIG_HZ 调成多少都不影响这个值，可以直接当常量用。
const userHZ = 100

// readStat 解析 /proc/<pid>/stat。
//
// comm 字段是括号包裹的进程名，本身可以包含空格甚至括号，所以不能按
// 空格直接切：先定位最后一个 ')'，之前是 pid 和 comm，之后才是各数值字段。
func readStat(pid int) (procStat, error) {
	data, err := os.ReadFile(fmt.Sprintf("/proc/%d/stat", pid))
	if err != nil {
		return procStat{}, err
	}
	start := bytes.IndexByte(data, '(')
	end := bytes.LastIndexByte(data, ')')
	if start < 0 || end < start {
		return procStat{}, fmt.Errorf("/proc/%d/stat 格式异常", pid)
	}

	fields := strings.Fields(string(data[end+1:]))
	// fields 相对 stat 手册的偏移（去掉 pid 和 comm 之后重新从 0 数）：
	// 0=state 1=ppid 2=pgrp 11=utime 12=stime 21=rss
	if len(fields) < 22 {
		return procStat{}, fmt.Errorf("/proc/%d/stat 字段不足", pid)
	}

	ppid, err := strconv.Atoi(fields[1])
	if err != nil {
		return procStat{}, err
	}
	pgid, err := strconv.Atoi(fields[2])
	if err != nil {
		return procStat{}, err
	}
	utime, err := strconv.ParseUint(fields[11], 10, 64)
	if err != nil {
		return procStat{}, err
	}
	stime, err := strconv.ParseUint(fields[12], 10, 64)
	if err != nil {
		return procStat{}, err
	}
	rss, err := strconv.ParseUint(fields[21], 10, 64)
	if err != nil {
		return procStat{}, err
	}

	return procStat{
		Proc: Proc{
			PID:  pid,
			PPID: ppid,
			PGID: pgid,
			Name: string(data[start+1 : end]),
			Live: fields[0] != "Z",
		},
		utimeTicks: utime,
		stimeTicks: stime,
		rssPages:   rss,
	}, nil
}
