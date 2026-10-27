//go:build windows

// Package win32 集中声明 bootpane 需要的 Win32 API。
//
// 为什么不用 golang.org/x/sys/windows：ConPTY 的 CreatePseudoConsole 和
// PROC_THREAD_ATTRIBUTE_JOB_LIST 在 x/sys 里都没有导出，混用两套声明时结构体
// 布局一旦不一致就是内存踩踏，排查成本极高。所以这里统一手写一份。
//
// 这个包只被 internal/pty 和 internal/proctree 的平台实现引用，业务层不直接依赖。
package win32

import (
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"syscall"
	"unsafe"
)

var (
	modkernel32 = syscall.NewLazyDLL("kernel32.dll")
	modpsapi    = syscall.NewLazyDLL("psapi.dll")
	modiphlpapi = syscall.NewLazyDLL("iphlpapi.dll")

	procCreatePseudoConsole           = modkernel32.NewProc("CreatePseudoConsole")
	procResizePseudoConsole           = modkernel32.NewProc("ResizePseudoConsole")
	procClosePseudoConsole            = modkernel32.NewProc("ClosePseudoConsole")
	procCreatePipe                    = modkernel32.NewProc("CreatePipe")
	procInitializeProcThreadAttrList  = modkernel32.NewProc("InitializeProcThreadAttributeList")
	procUpdateProcThreadAttribute     = modkernel32.NewProc("UpdateProcThreadAttribute")
	procDeleteProcThreadAttributeList = modkernel32.NewProc("DeleteProcThreadAttributeList")
	procCreateProcessW                = modkernel32.NewProc("CreateProcessW")
	procCreateJobObjectW              = modkernel32.NewProc("CreateJobObjectW")
	procSetInformationJobObject       = modkernel32.NewProc("SetInformationJobObject")
	procQueryInformationJobObject     = modkernel32.NewProc("QueryInformationJobObject")
	procTerminateJobObject            = modkernel32.NewProc("TerminateJobObject")
	procOpenProcess                   = modkernel32.NewProc("OpenProcess")
	procGetProcessMemoryInfo          = modpsapi.NewProc("GetProcessMemoryInfo")
	procWaitForSingleObject           = modkernel32.NewProc("WaitForSingleObject")
	procGetExitCodeProcess            = modkernel32.NewProc("GetExitCodeProcess")
	procCloseHandle                   = modkernel32.NewProc("CloseHandle")
	procReadFile                      = modkernel32.NewProc("ReadFile")
	procWriteFile                     = modkernel32.NewProc("WriteFile")
	procGetExtendedTcpTable           = modiphlpapi.NewProc("GetExtendedTcpTable")
	procQueryFullProcessImageNameW    = modkernel32.NewProc("QueryFullProcessImageNameW")
	procCreateToolhelp32Snapshot      = modkernel32.NewProc("CreateToolhelp32Snapshot")
	procProcess32FirstW               = modkernel32.NewProc("Process32FirstW")
	procProcess32NextW                = modkernel32.NewProc("Process32NextW")
	procTerminateProcess              = modkernel32.NewProc("TerminateProcess")
)

// 进程创建标志。
const (
	CreateUnicodeEnvironment   = 0x00000400
	ExtendedStartupInfoPresent = 0x00080000

	// StartfUseStdHandles 让系统使用 STARTUPINFO 里显式给出的标准句柄，
	// 而不是沿用父进程的。ConPTY 场景必须打开，否则子进程会抢父进程的 stdout。
	StartfUseStdHandles = 0x00000100
)

// InvalidHandle 是 INVALID_HANDLE_VALUE，配合 StartfUseStdHandles 表示
// 「不要继承父进程的标准句柄」。
const InvalidHandle = ^syscall.Handle(0)

// 线程属性键。数值来自 ProcThreadAttributeValue 宏：
// 低位是属性序号，第 17 位（0x20000）表示这是「输入属性」。
const (
	ProcThreadAttributeJobList       = 0x0002000D
	ProcThreadAttributePseudoConsole = 0x00020016
)

// Job Object 信息类。
const (
	JobObjectBasicAccountingInformation      = 1
	JobObjectBasicProcessIDList              = 3
	JobObjectBasicAndIOAccountingInformation = 8
	JobObjectExtendedLimitInformation        = 9
)

// JobObjectLimitKillOnJobClose 让 Job 句柄一关，里面所有进程立刻被杀。
// 这是「应用崩了也不留孤儿进程」的兜底，比在退出流程里逐个 kill 可靠得多。
const JobObjectLimitKillOnJobClose = 0x00002000

// 进程访问权限。
const ProcessQueryLimitedInformation = 0x1000

// Infinite 用于 WaitForSingleObject 的无限等待。
const Infinite = 0xFFFFFFFF

// Coord 是 CONSOLE_SCREEN_BUFFER_INFO 那套坐标结构，ConPTY 用它表示终端尺寸。
type Coord struct {
	X int16
	Y int16
}

// StartupInfoEx 是 STARTUPINFOEXW：在 STARTUPINFOW 后面追加属性列表指针。
// 属性列表是 ConPTY 和 Job Object 的唯一挂载入口。
type StartupInfoEx struct {
	syscall.StartupInfo
	AttributeList unsafe.Pointer
}

// ProcessInformation 是 CreateProcessW 的输出。
type ProcessInformation struct {
	Process   syscall.Handle
	Thread    syscall.Handle
	ProcessID uint32
	ThreadID  uint32
}

// JobBasicAccounting 是 JOBOBJECT_BASIC_ACCOUNTING_INFORMATION。
// 时间是 100 纳秒为单位的累计值，进程退出后仍然保留在 Job 记账里。
type JobBasicAccounting struct {
	TotalUserTime             int64
	TotalKernelTime           int64
	ThisPeriodTotalUserTime   int64
	ThisPeriodTotalKernelTime int64
	TotalPageFaultCount       uint32
	TotalProcesses            uint32
	ActiveProcesses           uint32
	TotalTerminatedProcesses  uint32
}

// IOCounters 是 IO_COUNTERS。
type IOCounters struct {
	ReadOperationCount  int64
	WriteOperationCount int64
	OtherOperationCount int64
	ReadTransferCount   int64
	WriteTransferCount  int64
	OtherTransferCount  int64
}

// JobBasicAndIOAccounting 是 JOBOBJECT_BASIC_AND_IO_ACCOUNTING_INFORMATION。
type JobBasicAndIOAccounting struct {
	Basic JobBasicAccounting
	IO    IOCounters
}

// JobBasicLimit 是 JOBOBJECT_BASIC_LIMIT_INFORMATION。
type JobBasicLimit struct {
	PerProcessUserTimeLimit int64
	PerJobUserTimeLimit     int64
	LimitFlags              uint32
	MinimumWorkingSetSize   uintptr
	MaximumWorkingSetSize   uintptr
	ActiveProcessLimit      uint32
	Affinity                uintptr
	PriorityClass           uint32
	SchedulingClass         uint32
}

// JobExtendedLimit 是 JOBOBJECT_EXTENDED_LIMIT_INFORMATION。
type JobExtendedLimit struct {
	BasicLimitInformation JobBasicLimit
	IoInfo                IOCounters
	ProcessMemoryLimit    uintptr
	JobMemoryLimit        uintptr
	PeakProcessMemoryUsed uintptr
	PeakJobMemoryUsed     uintptr
}

// JobProcessIDList 是变长的 JOBOBJECT_BASIC_PROCESS_ID_LIST 头部，
// 实际使用时按需分配一段更大的缓冲区。
type JobProcessIDList struct {
	NumberOfAssignedProcesses uint32
	NumberOfProcessIDsInList  uint32
	ProcessIDList             [1]uintptr
}

// ProcessMemoryCounters 是 PROCESS_MEMORY_COUNTERS。
type ProcessMemoryCounters struct {
	Cb                         uint32
	PageFaultCount             uint32
	PeakWorkingSetSize         uintptr
	WorkingSetSize             uintptr
	QuotaPeakPagedPoolUsage    uintptr
	QuotaPagedPoolUsage        uintptr
	QuotaPeakNonPagedPoolUsage uintptr
	QuotaNonPagedPoolUsage     uintptr
	PagefileUsage              uintptr
	PeakPagefileUsage          uintptr
}

// lastError 把 syscall 的 err 兜底成非 nil，避免调用方拿到 nil 却不知道失败了。
func lastError(err error) error {
	if err != nil && err != syscall.Errno(0) {
		return err
	}
	return syscall.EINVAL
}

// CloseHandle 关闭一个内核句柄，句柄为 0 时静默返回。
func CloseHandle(h syscall.Handle) error {
	if h == 0 {
		return nil
	}
	r1, _, err := procCloseHandle.Call(uintptr(h))
	if r1 == 0 {
		return lastError(err)
	}
	return nil
}

// CreatePipe 创建一对匿名管道，句柄不可继承。
//
// ConPTY 自己会在内部复制它需要的管道句柄，所以这里不需要可继承属性；
// 反过来设成可继承会让子进程意外拿到不该拿的句柄。
func CreatePipe() (read, write syscall.Handle, err error) {
	var r, w syscall.Handle
	r1, _, e := procCreatePipe.Call(
		uintptr(unsafe.Pointer(&r)),
		uintptr(unsafe.Pointer(&w)),
		0, // lpPipeAttributes: 不可继承
		0, // nSize: 用系统默认
	)
	if r1 == 0 {
		return 0, 0, lastError(e)
	}
	return r, w, nil
}

// coordValue 把宽高打包成 COORD 的按值表示。
//
// COORD 是 {SHORT X; SHORT Y;}，只有 4 字节。x64 调用约定下这种小结构体
// 是**按值**塞进寄存器的低 32 位，不能传指针——传指针的话系统会把地址的低
// 16 位当成列数，得到一个离谱的尺寸，ConPTY 直接初始化失败（0xC0000142）。
func coordValue(cols, rows int16) uintptr {
	return uintptr(uint32(uint16(cols)) | uint32(uint16(rows))<<16)
}

// CreatePseudoConsole 创建一个伪终端。
//
// input 是 ConPTY 读的一端（我们往这里写），output 是 ConPTY 写的一端（我们从这里读）。
// 返回的 HPCON 会通过 PROC_THREAD_ATTRIBUTE_PSEUDOCONSOLE 传给子进程。
func CreatePseudoConsole(cols, rows int16, input, output syscall.Handle) (uintptr, error) {
	var hpc uintptr
	hr, _, _ := procCreatePseudoConsole.Call(
		coordValue(cols, rows),
		uintptr(input),
		uintptr(output),
		0, // dwFlags
		uintptr(unsafe.Pointer(&hpc)),
	)
	// 返回的是 HRESULT，高位为 1 表示失败。
	if int32(hr) < 0 {
		return 0, syscall.Errno(hr)
	}
	return hpc, nil
}

// ResizePseudoConsole 同步终端尺寸。
func ResizePseudoConsole(hpc uintptr, cols, rows int16) error {
	hr, _, _ := procResizePseudoConsole.Call(hpc, coordValue(cols, rows))
	if int32(hr) < 0 {
		return syscall.Errno(hr)
	}
	return nil
}

// ClosePseudoConsole 释放伪终端。它会连带终止还挂在 ConPTY 上的进程。
func ClosePseudoConsole(hpc uintptr) {
	if hpc == 0 {
		return
	}
	procClosePseudoConsole.Call(hpc)
}

// CreateJobObject 创建一个匿名 Job Object。
func CreateJobObject() (syscall.Handle, error) {
	h, _, err := procCreateJobObjectW.Call(0, 0)
	if h == 0 {
		return 0, lastError(err)
	}
	return syscall.Handle(h), nil
}

// SetKillOnJobClose 打开 KILL_ON_JOB_CLOSE，让 Job 句柄关闭时清空所有成员进程。
func SetKillOnJobClose(job syscall.Handle) error {
	var info JobExtendedLimit
	info.BasicLimitInformation.LimitFlags = JobObjectLimitKillOnJobClose
	r1, _, err := procSetInformationJobObject.Call(
		uintptr(job),
		JobObjectExtendedLimitInformation,
		uintptr(unsafe.Pointer(&info)),
		unsafe.Sizeof(info),
	)
	if r1 == 0 {
		return lastError(err)
	}
	return nil
}

// TerminateJobObject 终止 Job 里的所有进程，等价于对整棵树一次性 kill。
func TerminateJobObject(job syscall.Handle, exitCode uint32) error {
	r1, _, err := procTerminateJobObject.Call(uintptr(job), uintptr(exitCode))
	if r1 == 0 {
		return lastError(err)
	}
	return nil
}

// QueryJobAccounting 取整个 Job 的累计 CPU 时间。
//
// 这是资源监控里最关键的一步：Job 的记账天然是整棵树的聚合值，
// 不需要枚举进程，也不会漏掉已经退出的中间层（pnpm 退出但 node 还在跑）。
func QueryJobAccounting(job syscall.Handle) (JobBasicAndIOAccounting, error) {
	var info JobBasicAndIOAccounting
	r1, _, err := procQueryInformationJobObject.Call(
		uintptr(job),
		JobObjectBasicAndIOAccountingInformation,
		uintptr(unsafe.Pointer(&info)),
		unsafe.Sizeof(info),
		0,
	)
	if r1 == 0 {
		return info, lastError(err)
	}
	return info, nil
}

// QueryJobPids 列出 Job 里当前活着的进程。
//
// 内存没有聚合 API，只能拿到 PID 列表后逐个查工作集再求和。
func QueryJobPids(job syscall.Handle) ([]uint32, error) {
	// 先按 64 个进程的容量申请，不够再翻倍重试。
	capacity := 64
	for attempt := 0; attempt < 4; attempt++ {
		size := unsafe.Sizeof(JobProcessIDList{}) + uintptr(capacity-1)*unsafe.Sizeof(uintptr(0))
		buf := make([]byte, size)
		var returned uint32
		r1, _, err := procQueryInformationJobObject.Call(
			uintptr(job),
			JobObjectBasicProcessIDList,
			uintptr(unsafe.Pointer(&buf[0])),
			size,
			uintptr(unsafe.Pointer(&returned)),
		)
		if r1 == 0 {
			return nil, lastError(err)
		}
		list := (*JobProcessIDList)(unsafe.Pointer(&buf[0]))
		if list.NumberOfProcessIDsInList < uint32(capacity) {
			out := make([]uint32, 0, list.NumberOfProcessIDsInList)
			ids := unsafe.Slice(&list.ProcessIDList[0], list.NumberOfProcessIDsInList)
			for _, pid := range ids {
				out = append(out, uint32(pid))
			}
			return out, nil
		}
		capacity *= 4
	}
	return nil, syscall.EINVAL
}

// ProcessWorkingSet 取单个进程的物理内存占用（WorkingSetSize）。
func ProcessWorkingSet(pid uint32) (uint64, error) {
	h, _, err := procOpenProcess.Call(ProcessQueryLimitedInformation, 0, uintptr(pid))
	if h == 0 {
		// 进程可能在枚举和查询之间就退出了，这不是错误，交给调用方跳过。
		return 0, lastError(err)
	}
	defer procCloseHandle.Call(h)

	var counters ProcessMemoryCounters
	counters.Cb = uint32(unsafe.Sizeof(counters))
	r1, _, err := procGetProcessMemoryInfo.Call(
		h,
		uintptr(unsafe.Pointer(&counters)),
		unsafe.Sizeof(counters),
	)
	if r1 == 0 {
		return 0, lastError(err)
	}
	return uint64(counters.WorkingSetSize), nil
}

// WaitForSingleObject 阻塞等待句柄进入有信号状态。timeout 单位毫秒。
func WaitForSingleObject(h syscall.Handle, timeout uint32) (uint32, error) {
	r1, _, err := procWaitForSingleObject.Call(uintptr(h), uintptr(timeout))
	if uint32(r1) == 0xFFFFFFFF {
		return 0, lastError(err)
	}
	return uint32(r1), nil
}

// GetExitCode 取进程退出码。
func GetExitCode(h syscall.Handle) (uint32, error) {
	var code uint32
	r1, _, err := procGetExitCodeProcess.Call(uintptr(h), uintptr(unsafe.Pointer(&code)))
	if r1 == 0 {
		return 0, lastError(err)
	}
	return code, nil
}

// ReadFile 从管道读数据。
func ReadFile(h syscall.Handle, buf []byte) (int, error) {
	var n uint32
	r1, _, err := procReadFile.Call(
		uintptr(h),
		uintptr(unsafe.Pointer(&buf[0])),
		uintptr(len(buf)),
		uintptr(unsafe.Pointer(&n)),
		0,
	)
	if r1 == 0 {
		return int(n), lastError(err)
	}
	return int(n), nil
}

// WriteFile 往管道写数据。
func WriteFile(h syscall.Handle, buf []byte) (int, error) {
	if len(buf) == 0 {
		return 0, nil
	}
	var n uint32
	r1, _, err := procWriteFile.Call(
		uintptr(h),
		uintptr(unsafe.Pointer(&buf[0])),
		uintptr(len(buf)),
		uintptr(unsafe.Pointer(&n)),
		0,
	)
	if r1 == 0 {
		return int(n), lastError(err)
	}
	return int(n), nil
}

// ProcSpec 描述一次原生进程创建。
type ProcSpec struct {
	AppName     string // 可执行文件完整路径，空表示从 CommandLine 推断
	CommandLine string // 完整命令行，必须带 argv[0]
	Dir         string // 工作目录
	Env         []string
	JobHandle   uintptr // 非 0 时在同一 CreateProcess 里挂进 Job
	PseudoCon   uintptr // 非 0 时在同一 CreateProcess 里接上 ConPTY
}

// CreateProcess 创建进程，并在同一次调用里挂上 ConPTY 和 Job Object。
//
// 这是整个 M1 的地基。分开做（先启动、再 AssignProcessToJobObject）会留下一个
// 竞态窗口：`pnpm` 有可能在被挂进 Job 之前就 fork 出 node，那个 node 就逃逸了，
// 之后 kill 整棵树也杀不掉它。用属性列表一次搞定才没有这个窗口。
func CreateProcess(spec ProcSpec) (ProcessInformation, error) {
	var pi ProcessInformation

	attrCount := uint32(0)
	if spec.PseudoCon != 0 {
		attrCount++
	}
	if spec.JobHandle != 0 {
		attrCount++
	}

	var attrBuf []byte
	var attrList unsafe.Pointer
	if attrCount > 0 {
		var size uintptr
		// 第一次调用只为拿所需缓冲区大小，必然返回 FALSE，属预期行为。
		procInitializeProcThreadAttrList.Call(0, uintptr(attrCount), 0, uintptr(unsafe.Pointer(&size)))
		if size == 0 {
			return pi, syscall.EINVAL
		}
		attrBuf = make([]byte, size)
		attrList = unsafe.Pointer(&attrBuf[0])
		r1, _, err := procInitializeProcThreadAttrList.Call(
			uintptr(attrList), uintptr(attrCount), 0, uintptr(unsafe.Pointer(&size)))
		if r1 == 0 {
			return pi, lastError(err)
		}
		defer procDeleteProcThreadAttributeList.Call(uintptr(attrList))

		if spec.PseudoCon != 0 {
			// 注意：PSEUDOCONSOLE 的 lpValue 是 HPCON 值本身，不是它的地址。
			// HPCON 本身就是个指针类型，再取一次地址会把栈地址当成句柄传下去，
			// 表现是进程照常启动但 ConPTY 完全没接管输出，排查起来非常隐蔽。
			// 而 JOB_LIST 要求的是「句柄数组」的指针，两者语义不同，别写成一样。
			r1, _, err := procUpdateProcThreadAttribute.Call(
				uintptr(attrList), 0, ProcThreadAttributePseudoConsole,
				spec.PseudoCon, unsafe.Sizeof(spec.PseudoCon), 0, 0)
			if r1 == 0 {
				return pi, lastError(err)
			}
		}
		if spec.JobHandle != 0 {
			job := spec.JobHandle
			r1, _, err := procUpdateProcThreadAttribute.Call(
				uintptr(attrList), 0, ProcThreadAttributeJobList,
				uintptr(unsafe.Pointer(&job)), unsafe.Sizeof(job), 0, 0)
			if r1 == 0 {
				return pi, lastError(err)
			}
		}
	}

	var si StartupInfoEx
	si.Cb = uint32(unsafe.Sizeof(si))
	si.AttributeList = attrList

	// 关键一步：Windows 有个历史遗留行为——即使 bInheritHandles 为 FALSE，
	// 标准输入输出错误句柄**仍然会被继承**，除非显式声明 STARTF_USESTDHANDLES。
	//
	// 不设的话，从终端里启动 bootpane 时，子进程会继承终端的 stdout，
	// 日志就写到终端去了，ConPTY 一条都收不到（现象极具迷惑性：进程明明
	// 在 ConPTY 里，窗口标题都改得动，但 echo 的输出凭空消失）。
	// 这里把三个句柄标成无效，等于告诉系统「别继承，让子进程自己找 console」。
	si.Flags = StartfUseStdHandles
	si.StdInput = InvalidHandle
	si.StdOutput = InvalidHandle
	si.StdErr = InvalidHandle

	cmdline, err := syscall.UTF16FromString(spec.CommandLine)
	if err != nil {
		return pi, err
	}

	var appPtr *uint16
	if spec.AppName != "" {
		appPtr, err = syscall.UTF16PtrFromString(spec.AppName)
		if err != nil {
			return pi, err
		}
	}

	var dirPtr *uint16
	if spec.Dir != "" {
		dirPtr, err = syscall.UTF16PtrFromString(spec.Dir)
		if err != nil {
			return pi, err
		}
	}

	var envBlock []uint16
	if spec.Env != nil {
		envBlock = BuildEnvBlock(spec.Env)
	}

	var envPtr uintptr
	if envBlock != nil {
		envPtr = uintptr(unsafe.Pointer(&envBlock[0]))
	}

	flags := uint32(ExtendedStartupInfoPresent)
	if envBlock != nil {
		flags |= CreateUnicodeEnvironment
	}

	r1, _, callErr := procCreateProcessW.Call(
		uintptr(unsafe.Pointer(appPtr)),
		uintptr(unsafe.Pointer(&cmdline[0])),
		0, // lpProcessAttributes
		0, // lpThreadAttributes
		0, // bInheritHandles: ConPTY 通过属性传递，不需要继承
		uintptr(flags),
		envPtr,
		uintptr(unsafe.Pointer(dirPtr)),
		uintptr(unsafe.Pointer(&si)),
		uintptr(unsafe.Pointer(&pi)),
	)
	
	if r1 == 0 {
		return pi, lastError(callErr)
	}

	// 显式保活：上面把指针转成了 uintptr，GC 看不见引用。
	// 一旦提前回收，CreateProcessW 读到的就是已释放内存。
	runtime.KeepAlive(cmdline)
	runtime.KeepAlive(envBlock)
	runtime.KeepAlive(attrBuf)

	return pi, nil
}

// BuildEnvBlock 把 "K=V" 列表编码成 Windows 要求的 UTF-16 环境块：
// 每项以 \0 结尾，整块再以一个额外的 \0 收尾（即双 null 结尾）。
func BuildEnvBlock(env []string) []uint16 {
	sorted := make([]string, 0, len(env))
	for _, kv := range env {
		if kv == "" || !strings.Contains(kv, "=") {
			continue
		}
		sorted = append(sorted, kv)
	}
	// Windows 要求环境块近似按字母序排列，否则某些 CRT 做二分查找会漏项。
	sort.Slice(sorted, func(i, j int) bool {
		return strings.ToUpper(sorted[i]) < strings.ToUpper(sorted[j])
	})

	out := make([]uint16, 0, 64)
	for _, kv := range sorted {
		u, err := syscall.UTF16FromString(kv)
		if err != nil {
			continue
		}
		out = append(out, u...)
	}
	// 空环境也要保证至少两个 \0，否则会被当成「继承父环境」。
	out = append(out, 0)
	if len(out) == 1 {
		out = append(out, 0)
	}
	return out
}

// ---------- 端口监听者查询 ----------

// TCP 表查询相关常量。
const (
	AfInet  = 2
	AfInet6 = 23

	// TcpTableOwnerPIDListener 只返回处于 LISTEN 状态且带 OwningPID 的行，
	// 比全连接表小得多。
	TcpTableOwnerPIDListener = 3

	errorInsufficientBuffer = 122
)

// MibTcpRowOwnerPID 是 IPv4 监听表的一行（MIB_TCPROW_OWNER_PID）。
type MibTcpRowOwnerPID struct {
	State      uint32
	LocalAddr  uint32
	LocalPort  uint32 // 网络字节序
	RemoteAddr uint32
	RemotePort uint32
	OwningPID  uint32
}

// MibTcp6RowOwnerPID 是 IPv6 监听表的一行（MIB_TCP6ROW_OWNER_PID）。
type MibTcp6RowOwnerPID struct {
	LocalAddr     [16]byte
	LocalScopeID  uint32
	LocalPort     uint32 // 网络字节序
	RemoteAddr    [16]byte
	RemoteScopeID uint32
	RemotePort    uint32
	State         uint32
	OwningPID     uint32
}

// ntohsPort 把网络字节序的端口字段还原成主机序。
//
// 字段虽然是 DWORD，但只有低 16 位有效，而且存的是大端，所以要手工交换。
func ntohsPort(v uint32) int {
	return int(v&0xFF)<<8 | int((v>>8)&0xFF)
}

// ListeningPorts 返回本机所有处于监听状态的 TCP 端口 → 进程 ID。
//
// IPv4 和 IPv6 都要查：node / vite 默认监听 `::`，只查 IPv4 会漏掉一大半
// 前端项目。同名端口 IPv4 优先，够用——同一个端口被两个进程监听的情况极罕见。
func ListeningPorts() map[int]uint32 {
	out := make(map[int]uint32, 32)
	collectTcp4(out)
	collectTcp6(out)
	return out
}

func collectTcp4(out map[int]uint32) {
	var size uint32
	// 第一次调用只为拿所需缓冲区大小，返回 ERROR_INSUFFICIENT_BUFFER 属预期。
	procGetExtendedTcpTable.Call(
		0,
		uintptr(unsafe.Pointer(&size)),
		0,
		AfInet,
		TcpTableOwnerPIDListener,
		0,
	)
	if size == 0 {
		return
	}

	buf := make([]byte, size)
	r1, _, _ := procGetExtendedTcpTable.Call(
		uintptr(unsafe.Pointer(&buf[0])),
		uintptr(unsafe.Pointer(&size)),
		0,
		AfInet,
		TcpTableOwnerPIDListener,
		0,
	)
	if r1 != 0 {
		return
	}

	count := *(*uint32)(unsafe.Pointer(&buf[0]))
	if count == 0 {
		return
	}
	rowSize := unsafe.Sizeof(MibTcpRowOwnerPID{})
	base := unsafe.Add(unsafe.Pointer(&buf[0]), 4)
	for i := uint32(0); i < count; i++ {
		row := (*MibTcpRowOwnerPID)(unsafe.Add(base, uintptr(i)*rowSize))
		port := ntohsPort(row.LocalPort)
		if port <= 0 {
			continue
		}
		if _, exists := out[port]; !exists {
			out[port] = row.OwningPID
		}
	}
}

func collectTcp6(out map[int]uint32) {
	var size uint32
	procGetExtendedTcpTable.Call(
		0,
		uintptr(unsafe.Pointer(&size)),
		0,
		AfInet6,
		TcpTableOwnerPIDListener,
		0,
	)
	if size == 0 {
		return
	}

	buf := make([]byte, size)
	r1, _, _ := procGetExtendedTcpTable.Call(
		uintptr(unsafe.Pointer(&buf[0])),
		uintptr(unsafe.Pointer(&size)),
		0,
		AfInet6,
		TcpTableOwnerPIDListener,
		0,
	)
	if r1 != 0 {
		return
	}

	count := *(*uint32)(unsafe.Pointer(&buf[0]))
	if count == 0 {
		return
	}
	rowSize := unsafe.Sizeof(MibTcp6RowOwnerPID{})
	base := unsafe.Add(unsafe.Pointer(&buf[0]), 4)
	for i := uint32(0); i < count; i++ {
		row := (*MibTcp6RowOwnerPID)(unsafe.Add(base, uintptr(i)*rowSize))
		port := ntohsPort(row.LocalPort)
		if port <= 0 {
			continue
		}
		if _, exists := out[port]; !exists {
			out[port] = row.OwningPID
		}
	}
}

// ProcessName 返回进程的可执行文件名（不含路径）。查不到时返回空串。
//
// 用 QueryFullProcessImageNameW 而不是读 PE 头：只需要文件名，
// PROCESS_QUERY_LIMITED_INFORMATION 权限就够，对系统进程也不会失败。
func ProcessName(pid uint32) string {
	if pid == 0 {
		return ""
	}
	h, _, _ := procOpenProcess.Call(ProcessQueryLimitedInformation, 0, uintptr(pid))
	if h == 0 {
		return ""
	}
	defer procCloseHandle.Call(h)

	buf := make([]uint16, 512)
	size := uint32(len(buf))
	r1, _, _ := procQueryFullProcessImageNameW.Call(
		h,
		0,
		uintptr(unsafe.Pointer(&buf[0])),
		uintptr(unsafe.Pointer(&size)),
	)
	if r1 == 0 {
		return ""
	}
	return filepath.Base(syscall.UTF16ToString(buf[:size]))
}

// ---------- 进程树遍历与终止 ----------

// Toolhelp 快照与终止权限。
const (
	Th32csSnapProcess = 0x00000002
	ProcessTerminate  = 0x0001
	InvalidPIDValue   = 0xFFFFFFFF
)

// ProcessEntry32 是 PROCESSENTRY32W。
//
// DefaultHeapID 是 ULONG_PTR，Go 会自动按 8 字节对齐，和 C 布局一致（总长 564）。
type ProcessEntry32 struct {
	Size            uint32
	CntUsage        uint32
	ProcessID       uint32
	DefaultHeapID   uintptr
	ModuleID        uint32
	CntThreads      uint32
	ParentProcessID uint32
	PriClassBase    int32
	Flags           uint32
	ExeFile         [260]uint16
}

// ProcessNode 是进程树里的一个节点。
type ProcessNode struct {
	PID  int
	PPID int
	Name string
}

// SnapshotProcesses 取一份全量进程快照。
//
// 用它而不是 NtQueryInformationProcess 逐个查父进程：一次快照就能拿到全表，
// 遍历几百个进程也只有一次系统调用。
func SnapshotProcesses() ([]ProcessNode, error) {
	snap, _, err := procCreateToolhelp32Snapshot.Call(Th32csSnapProcess, 0)
	if snap == uintptr(syscall.InvalidHandle) || snap == 0 {
		return nil, lastError(err)
	}
	defer procCloseHandle.Call(snap)

	var entry ProcessEntry32
	entry.Size = uint32(unsafe.Sizeof(entry))

	r1, _, callErr := procProcess32FirstW.Call(snap, uintptr(unsafe.Pointer(&entry)))
	if r1 == 0 {
		// 一个进程都没有时返回 ERROR_NO_MORE_FILES，按空列表处理。
		if errno, ok := callErr.(syscall.Errno); ok && errno == 18 {
			return nil, nil
		}
		return nil, lastError(callErr)
	}

	out := make([]ProcessNode, 0, 256)
	for {
		out = append(out, ProcessNode{
			PID:  int(entry.ProcessID),
			PPID: int(entry.ParentProcessID),
			Name: syscall.UTF16ToString(entry.ExeFile[:]),
		})
		r1, _, callErr = procProcess32NextW.Call(snap, uintptr(unsafe.Pointer(&entry)))
		if r1 == 0 {
			if errno, ok := callErr.(syscall.Errno); ok && errno == 18 {
				break
			}
			break
		}
	}
	return out, nil
}

// TerminateByPID 强制结束单个进程。
func TerminateByPID(pid int, exitCode uint32) error {
	if pid <= 0 {
		return syscall.EINVAL
	}
	h, _, err := procOpenProcess.Call(ProcessTerminate|ProcessQueryLimitedInformation, 0, uintptr(pid))
	if h == 0 {
		return lastError(err)
	}
	defer procCloseHandle.Call(h)

	r1, _, callErr := procTerminateProcess.Call(h, uintptr(exitCode))
	if r1 == 0 {
		return lastError(callErr)
	}
	return nil
}
