// 版权所有 2019 The Go Authors。保留所有权利。
// 本源代码的使用受 BSD 风格许可证约束，
// 该许可证可在 LICENSE 文件中找到。

//go:build !plan9

package os

import "syscall"

type syscallErrorType = syscall.Errno

const (
	errENOSYS = syscall.ENOSYS
	errERANGE = syscall.ERANGE
	errENOMEM = syscall.ENOMEM
)
