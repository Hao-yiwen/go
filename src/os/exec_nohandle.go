// 版权所有 2024 The Go Authors。保留所有权利。
// 本源代码的使用受 BSD 风格许可证约束，
// 该许可证可在 LICENSE 文件中找到。

//go:build !linux && !windows

package os

func (ph *processHandle) closeHandle() {
	panic("internal error: unexpected call to closeHandle")
}
