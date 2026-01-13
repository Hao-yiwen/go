// 版权所有 2020 The Go Authors。保留所有权利。
// 本源代码的使用受 BSD 风格许可证约束，
// 该许可证可在 LICENSE 文件中找到。

//go:build !goexperiment.regabiargs && !amd64 && !arm64 && !loong64 && !ppc64 && !ppc64le && !riscv64

package abi

const (
	// ABI 相关常量。
	//
	// 在通用情况下，这些值都是零，
	// 这使得它们可以优雅地降级到 ABI0。

	// IntArgRegs 是专门用于传递整数参数值的寄存器数量。
	// 返回值寄存器与参数寄存器相同，所以这个数字也用于返回值。
	IntArgRegs = 0

	// FloatArgRegs 是专门用于传递浮点参数值的寄存器数量。
	// 返回值寄存器与参数寄存器相同，所以这个数字也用于返回值。
	FloatArgRegs = 0

	// EffectiveFloatRegSize 从 ABI 的角度描述当前平台上
	// 浮点寄存器的宽度。
	//
	// 由于 Go 只支持 32 位和 64 位浮点原语，
	// 这个数字应该是 0、4 或 8。0 表示 ABI 没有浮点寄存器，
	// 或者浮点值将通过软浮点 ABI 传递。
	//
	// 对于支持更大浮点寄存器宽度的平台，
	// 例如 x87 的 80 位"寄存器"（虽然我们目前不支持 x87），
	// 使用 8。
	EffectiveFloatRegSize = 0
)
