// 版权所有 2024 The Go Authors。保留所有权利。
// 本源代码的使用受 BSD 风格许可证约束，
// 该许可证可在 LICENSE 文件中找到。

//go:build asan

package asan

import (
	"unsafe"
)

const Enabled = true

//go:linkname Read runtime.asanread
func Read(addr unsafe.Pointer, len uintptr)

//go:linkname Write runtime.asanwrite
func Write(addr unsafe.Pointer, len uintptr)
