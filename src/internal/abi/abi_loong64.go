// 版权所有 2023 The Go Authors。保留所有权利。
// 本源代码的使用受 BSD 风格许可证约束，
// 该许可证可在 LICENSE 文件中找到。

package abi

const (
	// 参见 abi_generic.go。

	// R4 - R19
	IntArgRegs = 16

	// F0 - F15
	FloatArgRegs = 16

	EffectiveFloatRegSize = 8
)
