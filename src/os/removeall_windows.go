// 版权所有 2025 The Go Authors。保留所有权利。
// 本源代码的使用受 BSD 风格许可证约束，
// 该许可证可在 LICENSE 文件中找到。

//go:build windows

package os

import "syscall"

func isErrNoFollow(err error) bool {
	return err == syscall.ELOOP
}

func newDirFile(fd syscall.Handle, name string) (*File, error) {
	return newFile(fd, name, kindOpenFile, false), nil
}
