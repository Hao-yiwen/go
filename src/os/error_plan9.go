// 版权所有 2019 The Go Authors。保留所有权利。
// 本源代码的使用受 BSD 风格许可证约束，
// 该许可证可在 LICENSE 文件中找到。

package os

import "syscall"

type syscallErrorType = syscall.ErrorString

var errENOSYS = syscall.NewError("function not implemented")
var errERANGE = syscall.NewError("out of range")
var errENOMEM = syscall.NewError("cannot allocate memory")
