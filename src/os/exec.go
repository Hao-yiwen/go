// 版权所有 2009 The Go Authors。保留所有权利。
// 本源代码的使用受 BSD 风格许可证约束，
// 该许可证可在 LICENSE 文件中找到。

package os

import (
	"errors"
	"internal/testlog"
	"runtime"
	"sync"
	"sync/atomic"
	"syscall"
	"time"
)

var (
	// ErrProcessDone 表示 [Process] 已完成。
	ErrProcessDone = errors.New("os: process already finished")
	// errProcessReleased 表示 [Process] 已被释放。
	errProcessReleased = errors.New("os: process already released")
	// ErrNoHandle 表示 [Process] 没有句柄。
	ErrNoHandle = errors.New("os: process handle unavailable")
)

// processStatus 描述 [Process] 的状态。
type processStatus uint32

const (
	// statusOK 表示 Process 已准备好使用。
	statusOK processStatus = iota

	// statusDone 表示不应使用 PID/句柄，因为
	// 进程已完成（已成功调用 Wait）。
	statusDone

	// statusReleased 表示不应使用 PID/句柄，
	// 因为进程已被释放。
	statusReleased
)

// Process 存储由 [StartProcess] 创建的进程的信息。
type Process struct {
	Pid int

	// state 包含原子进程状态。
	//
	// 它由 processStatus 字段组成，
	// 指示进程是否已完成/释放。
	state atomic.Uint32

	// 仅在 handle 为 nil 时使用
	sigMu sync.RWMutex // 避免 wait 和 signal 之间的竞争

	// handle 如果不为 nil，是指向包含操作系统特定进程句柄的
	// 结构的指针。
	// 此指针在创建 Process 时设置，
	// 之后不再更改。
	// 这是指向单独内存分配的指针，
	// 以便我们可以使用 runtime.AddCleanup。
	handle *processHandle

	// cleanup 用于清理进程句柄。
	cleanup runtime.Cleanup
}

// processHandle 保存指向进程的操作系统句柄。
// 这仅在支持该概念的系统上使用，
// 目前是 Linux 和 Windows。
// 它维护对句柄的引用计数，
// 并在引用降至零时关闭句柄。
type processHandle struct {
	// 实际的句柄。此字段不应直接使用。
	// 而应使用 acquire 和 release 方法。
	//
	// 在 Windows 上，这是由 OpenProcess 返回的句柄。
	// 在 Linux 上，这是一个 pidfd。
	handle uintptr

	// 活动引用数。当此数降至零时，句柄被关闭。
	refs atomic.Int32
}

// acquire 添加一个引用并返回句柄。
// 布尔结果报告 acquire 是否成功；
// 如果句柄已关闭则失败。
// 每次成功调用 acquire 都应与 release 调用配对。
func (ph *processHandle) acquire() (uintptr, bool) {
	for {
		refs := ph.refs.Load()
		if refs < 0 {
			panic("internal error: negative process handle reference count")
		}
		if refs == 0 {
			return 0, false
		}
		if ph.refs.CompareAndSwap(refs, refs+1) {
			return ph.handle, true
		}
	}
}

// release 释放对句柄的一个引用。
func (ph *processHandle) release() {
	for {
		refs := ph.refs.Load()
		if refs <= 0 {
			panic("internal error: too many releases of process handle")
		}
		if ph.refs.CompareAndSwap(refs, refs-1) {
			if refs == 1 {
				ph.closeHandle()
			}
			return
		}
	}
}

// newPIDProcess 返回给定 PID 的 [Process]。
func newPIDProcess(pid int) *Process {
	p := &Process{
		Pid: pid,
	}
	return p
}

// newHandleProcess 返回具有给定 PID 和句柄的 [Process]。
func newHandleProcess(pid int, handle uintptr) *Process {
	ph := &processHandle{
		handle: handle,
	}

	// 将引用计数初始化为 1，
	// 表示来自返回的 Process 的引用。
	ph.refs.Store(1)

	p := &Process{
		Pid:    pid,
		handle: ph,
	}

	p.cleanup = runtime.AddCleanup(p, (*processHandle).release, ph)

	return p
}

// newDoneProcess 返回已标记为完成的给定 PID 的 [Process]。
// 这在 Unix 系统上使用，如果已知进程不存在。
func newDoneProcess(pid int) *Process {
	p := &Process{
		Pid: pid,
	}
	p.state.Store(uint32(statusDone)) // 没有持久引用，因为没有句柄。
	return p
}

// handleTransientAcquire 返回进程句柄，或者
// 如果进程未就绪，返回当前状态。
func (p *Process) handleTransientAcquire() (uintptr, processStatus) {
	if p.handle == nil {
		panic("handleTransientAcquire called in invalid mode")
	}

	status := processStatus(p.state.Load())
	if status != statusOK {
		return 0, status
	}
	h, ok := p.handle.acquire()
	if ok {
		return h, statusOK
	}

	// 这种情况意味着句柄已被关闭。
	// 我们总是在关闭句柄之前将状态设置为非零。
	// 如果我们到达这里，状态必须是在我们刚才检查之后
	// 被设置为非零的。
	status = processStatus(p.state.Load())
	if status == statusOK {
		panic("inconsistent process status")
	}
	return 0, status
}

// handleTransientRelease 释放由 handleTransientAcquire 返回的句柄。
func (p *Process) handleTransientRelease() {
	if p.handle == nil {
		panic("handleTransientRelease called in invalid mode")
	}
	p.handle.release()
}

// pidStatus 返回当前进程状态。
func (p *Process) pidStatus() processStatus {
	if p.handle != nil {
		panic("pidStatus called in invalid mode")
	}

	return processStatus(p.state.Load())
}

// ProcAttr 保存将应用于由 StartProcess 启动的新进程的属性。
type ProcAttr struct {
	// 如果 Dir 非空，子进程在创建进程之前切换到该目录。
	Dir string
	// 如果 Env 非 nil，它以 Environ 返回的格式提供新进程的环境变量。
	// 如果为 nil，将使用 Environ 的结果。
	Env []string
	// Files 指定新进程继承的打开文件。
	// 前三个条目对应于标准输入、标准输出和标准错误。
	// 根据底层操作系统，实现可能支持额外的条目。
	// nil 条目对应于进程启动时该文件被关闭。
	// 在 Unix 系统上，StartProcess 会将这些 File 值更改为
	// 阻塞模式，这意味着 SetDeadline 将停止工作，
	// 调用 Close 不会中断 Read 或 Write。
	Files []*File

	// 操作系统特定的进程创建属性。
	// 注意，设置此字段意味着您的程序可能无法在某些
	// 操作系统上正确执行甚至无法编译。
	Sys *syscall.SysProcAttr
}

// Signal 表示操作系统信号。
// 通常的底层实现取决于操作系统：
// 在 Unix 上是 syscall.Signal。
type Signal interface {
	String() string
	Signal() // 用于与其他 Stringer 区分
}

// Getpid 返回调用者的进程 ID。
func Getpid() int { return syscall.Getpid() }

// Getppid 返回调用者父进程的进程 ID。
func Getppid() int { return syscall.Getppid() }

// FindProcess 通过 pid 查找正在运行的进程。
//
// 它返回的 [Process] 可用于获取有关底层操作系统进程的信息。
//
// 在 Unix 系统上，FindProcess 总是成功并返回给定 pid 的 Process，
// 无论进程是否存在。要测试进程是否实际存在，
// 请查看 p.Signal(syscall.Signal(0)) 是否报告错误。
func FindProcess(pid int) (*Process, error) {
	return findProcess(pid)
}

// StartProcess 使用 name、argv 和 attr 指定的程序、参数和属性
// 启动一个新进程。argv 切片将成为新进程中的 [os.Args]，
// 所以它通常以程序名称开头。
//
// 如果调用的 goroutine 已使用 [runtime.LockOSThread] 锁定了操作系统线程
// 并修改了任何可继承的操作系统级线程状态（例如，Linux 或 Plan 9 命名空间），
// 新进程将继承调用者的线程状态。
//
// StartProcess 是一个低级接口。[os/exec] 包提供更高级的接口。
//
// 如果发生错误，它的类型将是 [*PathError]。
func StartProcess(name string, argv []string, attr *ProcAttr) (*Process, error) {
	testlog.Open(name)
	return startProcess(name, argv, attr)
}

// Release 释放与 [Process] p 关联的任何资源，
// 使其在将来无法使用。
// 只有在不调用 [Process.Wait] 时才需要调用 Release。
func (p *Process) Release() error {
	// 不幸的是，由于历史原因，在 Windows 以外的系统上，
	// Release 将 Pid 字段设置为 -1。
	// 这会导致竞争检测器在并发调用 Release 时报告问题，
	// 但我们现在无法更改它。
	if runtime.GOOS != "windows" {
		p.Pid = -1
	}

	oldStatus := p.doRelease(statusReleased)

	// 为了向后兼容，仅在 Windows 上，
	// 我们在第二次调用 Release 时返回 EINVAL。
	if runtime.GOOS == "windows" {
		if oldStatus == statusReleased {
			return syscall.EINVAL
		}
	}

	return nil
}

// doRelease 释放 [Process]，将状态设置为 newStatus。
// 如果先前的状态不是 statusOK，则不执行任何操作。
// 它返回先前的状态。
func (p *Process) doRelease(newStatus processStatus) processStatus {
	for {
		state := p.state.Load()
		oldStatus := processStatus(state)
		if oldStatus != statusOK {
			return oldStatus
		}

		if !p.state.CompareAndSwap(state, uint32(newStatus)) {
			continue
		}

		// 我们已成功释放 Process。
		// 如果它有句柄，释放我们在 newHandleProcess 中创建的引用。
		if p.handle != nil {
			// 不需要更多清理。
			// 我们必须在调用 release 之前停止清理；
			// 否则清理可能与 release 并发运行，
			// 这会导致引用计数无效，从而引发 panic。
			p.cleanup.Stop()

			p.handle.release()
		}

		return statusOK
	}
}

// Kill 导致 [Process] 立即退出。Kill 不会等待
// Process 实际退出。这只会杀死 Process 本身，
// 不会杀死它可能启动的任何其他进程。
func (p *Process) Kill() error {
	return p.kill()
}

// Wait 等待 [Process] 退出，然后返回描述其状态的
// ProcessState 和错误（如果有）。
// Wait 释放与 Process 关联的任何资源。
// 在大多数操作系统上，Process 必须是当前进程的子进程，
// 否则将返回错误。
func (p *Process) Wait() (*ProcessState, error) {
	return p.wait()
}

// Signal 向 [Process] 发送信号。
// 在 Windows 上发送 [Interrupt] 未实现。
func (p *Process) Signal(sig Signal) error {
	return p.signal(sig)
}

// WithHandle 使用有效的进程句柄作为参数调用提供的函数 f。
// 即使 p 终止，也保证句柄在 f 返回之前引用进程 p。
// 此函数不能在 [Process.Release] 或 [Process.Wait] 之后使用。
//
// 如果不支持进程句柄或句柄不可用，则返回 [ErrNoHandle]。
// 目前，进程句柄在 Linux 5.4 或更高版本（pidfd）和 Windows 上受支持。
func (p *Process) WithHandle(f func(handle uintptr)) error {
	return p.withHandle(f)
}

// UserTime 返回已退出进程及其子进程的用户 CPU 时间。
func (p *ProcessState) UserTime() time.Duration {
	return p.userTime()
}

// SystemTime 返回已退出进程及其子进程的系统 CPU 时间。
func (p *ProcessState) SystemTime() time.Duration {
	return p.systemTime()
}

// Exited 报告程序是否已退出。
// 在 Unix 系统上，如果程序因调用 exit 而退出则报告 true，
// 但如果程序因信号而终止则报告 false。
func (p *ProcessState) Exited() bool {
	return p.exited()
}

// Success 报告程序是否成功退出，
// 例如在 Unix 上退出状态为 0。
func (p *ProcessState) Success() bool {
	return p.success()
}

// Sys 返回有关进程的系统相关退出信息。
// 将其转换为适当的底层类型，例如 Unix 上的 [syscall.WaitStatus]，
// 以访问其内容。
func (p *ProcessState) Sys() any {
	return p.sys()
}

// SysUsage 返回有关已退出进程的系统相关资源使用信息。
// 将其转换为适当的底层类型，例如 Unix 上的 [*syscall.Rusage]，
// 以访问其内容。
// （在 Unix 上，*syscall.Rusage 与 getrusage(2) 手册页中
// 定义的 struct rusage 匹配。）
func (p *ProcessState) SysUsage() any {
	return p.sysUsage()
}
