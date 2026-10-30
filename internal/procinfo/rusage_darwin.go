//go:build darwin

package procinfo

/*
#include <libproc.h>
#include <mach/mach_time.h>
*/
import "C"
import (
	"fmt"
	"sync"
	"time"
	"unsafe"
)

// timebase 保存 Mach 绝对时间到纳秒的换算系数。
//
// 实测（对照 wait4 的 rusage，单位确定是微秒）证明 proc_pid_rusage 返回的
// ri_user_time / ri_system_time 不是文档里暗示的纳秒，而是 Mach 绝对时间
// tick：Apple Silicon 上时间基准是 24MHz（每 tick ≈ 41.67ns），换算前会
// 少算 24 倍；Intel 上 numer/denom 都是 1，恰好等于纳秒。
var timebase struct {
	once  sync.Once
	numer uint64
	denom uint64
}

func nsPerTick() (numer, denom uint64) {
	timebase.once.Do(func() {
		var tb C.mach_timebase_info_data_t
		C.mach_timebase_info(&tb)
		timebase.numer, timebase.denom = uint64(tb.numer), uint64(tb.denom)
		if timebase.numer == 0 || timebase.denom == 0 {
			timebase.numer, timebase.denom = 1, 1
		}
	})
	return timebase.numer, timebase.denom
}

// Rusage 查询单个进程的累计 CPU 时间和常驻内存。
//
// proc_pid_rusage 是 libproc 提供的正式接口，一次调用同时给出两者，
// ri_resident_size 单位是字节。注意它只反映该进程自己，不含已退出的
// 子进程。
//
// 用 RUSAGE_INFO_V0 就够了：CPU 时间和常驻内存都在 v0 里，后几个版本
// 加的是磁盘 IO、子进程耗时等这里用不到的字段。
func Rusage(pid int) (time.Duration, uint64, error) {
	var info C.struct_rusage_info_v0
	ret, errno := C.proc_pid_rusage(C.int(pid), C.RUSAGE_INFO_V0, (*C.rusage_info_t)(unsafe.Pointer(&info)))
	if ret != 0 {
		return 0, 0, fmt.Errorf("proc_pid_rusage(%d) 失败: %w", pid, errno)
	}
	ticks := uint64(info.ri_user_time) + uint64(info.ri_system_time)
	numer, denom := nsPerTick()
	// 先乘后除：Apple Silicon 上 numer/denom = 1/24，整数除法直接除会归零。
	cpu := time.Duration(ticks * numer / denom)
	return cpu, uint64(info.ri_resident_size), nil
}
