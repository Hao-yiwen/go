// 版权所有 2009 The Go Authors。保留所有权利。
// 本源代码的使用受 BSD 风格许可证约束，
// 该许可证可在 LICENSE 文件中找到。

// 进程等。

package os

import (
	"internal/testlog"
	"runtime"
	"syscall"
)

// Args 保存命令行参数，从程序名称开始。
var Args []string

func init() {
	if runtime.GOOS == "windows" {
		// 在 exec_windows.go 中初始化。
		return
	}
	Args = runtime_args()
}

func runtime_args() []string // 在 runtime 包中

// Getuid 返回调用者的数字用户 ID。
//
// 在 Windows 上，它返回 -1。
func Getuid() int { return syscall.Getuid() }

// Geteuid 返回调用者的数字有效用户 ID。
//
// 在 Windows 上，它返回 -1。
func Geteuid() int { return syscall.Geteuid() }

// Getgid 返回调用者的数字组 ID。
//
// 在 Windows 上，它返回 -1。
func Getgid() int { return syscall.Getgid() }

// Getegid 返回调用者的数字有效组 ID。
//
// 在 Windows 上，它返回 -1。
func Getegid() int { return syscall.Getegid() }

// Getgroups 返回调用者所属组的数字 ID 列表。
//
// 在 Windows 上，它返回 [syscall.EWINDOWS]。
// 请参阅 [os/user] 包以获取可能的替代方案。
func Getgroups() ([]int, error) {
	gids, e := syscall.Getgroups()
	return gids, NewSyscallError("getgroups", e)
}

// Exit 使当前程序以给定的状态码退出。
// 按照惯例，代码零表示成功，非零表示错误。
// 程序立即终止；延迟函数不会运行。
//
// 为了可移植性，状态码应在 [0, 125] 范围内。
func Exit(code int) {
	if code == 0 && testlog.PanicOnExit0() {
		// 我们被告知在调用 os.Exit(0) 时 panic。
		// 这用于使过早意外调用 os.Exit(0) 的测试失败。
		panic("unexpected call to os.Exit(0) during test")
	}

	// 通知运行时 os.Exit 正在被调用。如果启用了 -race，
	// 这将给竞争检测器一个使程序失败的机会（有竞争的程序
	// 无权成功完成）。如果启用了覆盖率，则此调用将
	// 使我们能够写出覆盖率数据文件。
	runtime_beforeExit(code)

	syscall.Exit(code)
}

func runtime_beforeExit(exitCode int) // 在 runtime 中实现
