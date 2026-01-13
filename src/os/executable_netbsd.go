// 版权所有 2024 The Go Authors。保留所有权利。
// 本源代码的使用受 BSD 风格许可证约束，
// 该许可证可在 LICENSE 文件中找到。

package os

// 来自 NetBSD 的 <sys/sysctl.h>
const (
	_CTL_KERN           = 1
	_KERN_PROC_ARGS     = 48
	_KERN_PROC_PATHNAME = 5
)

var executableMIB = [4]int32{_CTL_KERN, _KERN_PROC_ARGS, -1, _KERN_PROC_PATHNAME}
