// 版权所有 2021 The Go Authors。保留所有权利。
// 本源代码的使用受 BSD 风格许可证约束，
// 该许可证可在 LICENSE 文件中找到。

//go:build ppc64 || ppc64le

package abi

const (
	// 参见 abi_generic.go。

	// R3 - R10, R14 - R17。
	IntArgRegs = 12

	// F1 - F12。
	FloatArgRegs = 12

	EffectiveFloatRegSize = 8
)
