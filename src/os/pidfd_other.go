// 版权所有 2023 The Go Authors。保留所有权利。
// 本源代码的使用受 BSD 风格许可证约束，
// 该许可证可在 LICENSE 文件中找到。

//go:build (unix && !linux) || (js && wasm) || wasip1 || windows

package os

import "syscall"

func ensurePidfd(sysAttr *syscall.SysProcAttr) (*syscall.SysProcAttr, bool) {
	return sysAttr, false
}

func getPidfd(_ *syscall.SysProcAttr, _ bool) (uintptr, bool) {
	return 0, false
}

func pidfdFind(_ int) (uintptr, error) {
	return 0, syscall.ENOSYS
}

func (_ *Process) pidfdWait() (*ProcessState, error) {
	panic("unreachable")
}

func (_ *Process) pidfdSendSignal(_ syscall.Signal) error {
	panic("unreachable")
}
