// 版权所有 2023 The Go Authors。保留所有权利。
// 本源代码的使用受 BSD 风格许可证约束，
// 该许可证可在 LICENSE 文件中找到。

//go:build wasm

package os

import "syscall"

// Pipe 返回一对连接的文件；从 r 读取返回写入到 w 的字节。
// 它返回文件和一个错误，如果有的话。
func Pipe() (r *File, w *File, err error) {
	// GOOS=js 和 GOOS=wasip1 都不支持管道。
	return nil, nil, NewSyscallError("pipe", syscall.ENOSYS)
}
