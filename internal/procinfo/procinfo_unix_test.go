//go:build !windows

package procinfo

import (
	"os"
	"os/exec"
	"syscall"
	"testing"
	"time"
)

// TestListContainsSelf 验证全量枚举里能找到自己，且 PGID / PPID 与
// 系统调用返回的一致——这是 proctree 按 pgid 聚合、prockill 按 ppid
// 爬树的数据基础，错一个两个功能就全废。
func TestListContainsSelf(t *testing.T) {
	procs, err := List()
	if err != nil {
		t.Fatalf("枚举进程失败: %v", err)
	}

	for _, p := range procs {
		if p.PID != os.Getpid() {
			continue
		}
		if p.PGID != syscall.Getpgrp() {
			t.Fatalf("PGID 不一致：枚举 %d，系统调用 %d", p.PGID, syscall.Getpgrp())
		}
		if p.PPID != os.Getppid() {
			t.Fatalf("PPID 不一致：枚举 %d，系统调用 %d", p.PPID, os.Getppid())
		}
		if !p.Live {
			t.Fatal("自己不该是 zombie")
		}
		if p.Name == "" {
			t.Fatal("进程名为空")
		}
		return
	}
	t.Fatalf("全量枚举（%d 条）里找不到自己（pid=%d）", len(procs), os.Getpid())
}

// TestRusageSelf 验证单个进程的资源查询基本可用：测试进程活着，
// 常驻内存不可能为 0。
func TestRusageSelf(t *testing.T) {
	cpu, rss, err := Rusage(os.Getpid())
	if err != nil {
		t.Fatalf("查询自身资源失败: %v", err)
	}
	if rss == 0 {
		t.Fatal("常驻内存不应为 0")
	}
	if cpu < 0 {
		t.Fatalf("CPU 时间不应为负：%v", cpu)
	}
	t.Logf("self: cpu=%v rss=%d bytes", cpu, rss)
}

// TestRusageCPUUnits 验证 ri_user_time 的单位确实是纳秒。
//
// 让子进程空转约 1.2s 再查它的累计 CPU：单核满负荷应该接近 1.2s。
// 如果单位搞错（比如实际是毫秒），这里只会查出 1.2ms 量级，直接失败。
func TestRusageCPUUnits(t *testing.T) {
	if testing.Short() {
		t.Skip("short 模式跳过")
	}
	cmd := exec.Command("/bin/sh", "-c", "end=$((SECONDS+3)); while [ $SECONDS -lt $end ]; do :; done")
	if err := cmd.Start(); err != nil {
		t.Fatalf("启动空转子进程失败: %v", err)
	}
	defer func() {
		_ = cmd.Process.Kill()
		_ = cmd.Wait()
	}()

	time.Sleep(1200 * time.Millisecond)

	cpu, _, err := Rusage(cmd.Process.Pid)
	if err != nil {
		t.Fatalf("查询空转进程失败: %v", err)
	}
	if cpu < 500*time.Millisecond {
		t.Fatalf("空转 1.2s 后 CPU 时间只有 %v，单位换算大概率不对", cpu)
	}
}
