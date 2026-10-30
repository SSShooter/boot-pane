//go:build darwin

package procinfo

import (
	"bytes"

	"golang.org/x/sys/unix"
)

// List 走 sysctl kern.proc.all 一次性拿全所有进程的 kinfo_proc。
//
// 相比反复拉起 ps 解析文本，一次内核调用既便宜又稳定。macOS 没有可读的
// /proc，这是纯 Go 能拿到的最全的进程表。
func List() ([]Proc, error) {
	kps, err := unix.SysctlKinfoProcSlice("kern.proc.all")
	if err != nil {
		return nil, err
	}

	out := make([]Proc, 0, len(kps))
	for i := range kps {
		kp := &kps[i]
		if kp.Proc.P_pid <= 0 {
			continue
		}
		out = append(out, Proc{
			PID:  int(kp.Proc.P_pid),
			PPID: int(kp.Eproc.Ppid),
			PGID: int(kp.Eproc.Pgid),
			Name: commName(kp.Proc.P_comm[:]),
			// P_stat：0 空槽、1 SIDL、2 SRUN、3 SSLEEP、4 SSTOP、5 SZOMB。
			// zombie 进程实体已死，只剩退出码等父进程来收，视为不存在。
			Live: kp.Proc.P_stat > 0 && kp.Proc.P_stat != 5,
		})
	}
	return out, nil
}

// commName 取 P_comm 定长数组里的 C 字符串。数组尾部可能没有清零，
// 必须按第一个 NUL 截断，否则会带出上一轮残留的脏字节。
func commName(b []byte) string {
	if i := bytes.IndexByte(b, 0); i >= 0 {
		b = b[:i]
	}
	return string(b)
}
