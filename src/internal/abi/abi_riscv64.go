// 版权所有 2022 The Go Authors。保留所有权利。
// 本源代码的使用受 BSD 风格许可证约束，
// 该许可证可在 LICENSE 文件中找到。

package abi

const (
	// 参见 abi_generic.go。

	// X8 - X23
	IntArgRegs = 16

	// F8 - F23。
	FloatArgRegs = 16

	EffectiveFloatRegSize = 8
)
