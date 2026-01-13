// 版权所有 2017 The Go Authors。保留所有权利。
// 本源代码的使用受 BSD 风格许可证约束，
// 该许可证可在 LICENSE 文件中找到。

//go:build amd64

package math

import "internal/cpu"

var useFMA = cpu.X86.HasAVX && cpu.X86.HasFMA
