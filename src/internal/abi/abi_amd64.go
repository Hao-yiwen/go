// 版权所有 2020 The Go Authors。保留所有权利。
// 本源代码的使用受 BSD 风格许可证约束，
// 该许可证可在 LICENSE 文件中找到。

package abi

const (
	// 参见 abi_generic.go。

	// RAX, RBX, RCX, RDI, RSI, R8, R9, R10, R11。
	IntArgRegs = 9

	// X0 -> X14。
	FloatArgRegs = 15

	// 我们使用支持 64 位浮点运算的 SSE2 寄存器。
	EffectiveFloatRegSize = 8
)
