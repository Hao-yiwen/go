// 版权所有 2024 The Go Authors。保留所有权利。
// 本源代码的使用受 BSD 风格许可证约束，
// 该许可证可在 LICENSE 文件中找到。

//go:build netbsd

package os

import "syscall"

// isNoFollowErr 报告 err 是否可能由 O_NOFOLLOW 阻止打开操作所导致。
func isNoFollowErr(err error) bool {
	// NetBSD 返回 EFTYPE，但也检查其他可能性。
	switch err {
	case syscall.ELOOP, syscall.EMLINK, syscall.EFTYPE:
		return true
	}
	return false
}
