// 版权所有 2009 The Go Authors。保留所有权利。
// 本源代码的使用受 BSD 风格许可证约束，
// 该许可证可在 LICENSE 文件中找到。

//go:build aix || darwin

package os

import "syscall"

// Pipe 返回一对连接的文件；从 r 读取返回写入到 w 的字节。
// 它返回文件和一个错误，如果有的话。
func Pipe() (r *File, w *File, err error) {
	var p [2]int

	// 关于锁的说明，请参见 ../syscall/exec.go。
	syscall.ForkLock.RLock()
	e := syscall.Pipe(p[0:])
	if e != nil {
		syscall.ForkLock.RUnlock()
		return nil, nil, NewSyscallError("pipe", e)
	}
	syscall.CloseOnExec(p[0])
	syscall.CloseOnExec(p[1])
	syscall.ForkLock.RUnlock()

	return newFile(p[0], "|0", kindPipe, false), newFile(p[1], "|1", kindPipe, false), nil
}
