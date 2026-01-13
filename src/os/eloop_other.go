// 版权所有 2024 The Go Authors。保留所有权利。
// 本源代码的使用受 BSD 风格许可证约束，
// 该许可证可在 LICENSE 文件中找到。

//go:build aix || darwin || dragonfly || freebsd || linux || openbsd || solaris || wasip1

package os

import (
	"runtime"
	"syscall"
)

// isNoFollowErr 报告 err 是否可能由 O_NOFOLLOW 阻止打开操作所导致。
func isNoFollowErr(err error) bool {
	switch err {
	case syscall.ELOOP, syscall.EMLINK:
		return true
	}
	if runtime.GOOS == "dragonfly" {
		// Dragonfly 似乎在这种情况下从 openat 返回 EINVAL。
		if err == syscall.EINVAL {
			return true
		}
	}
	return false
}
