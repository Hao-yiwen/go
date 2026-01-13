// 版权所有 2020 The Go Authors。保留所有权利。
// 本源代码的使用受 BSD 风格许可证约束，
// 该许可证可在 LICENSE 文件中找到。

package abi

import (
	"internal/goarch"
	"unsafe"
)

// RegArgs 是一个结构体，为当前架构上的每个参数
// 和返回值寄存器提供空间。
//
// 汇编代码知道 RegArgs 前两个字段的布局。
//
// RegArgs 还包含额外的空间用于保存指针，
// 因为在某些情况下仅将指针保存在整数寄存器空间中
// 可能不安全。
type RegArgs struct {
	// 这些槽位中的值应该精确地表示它们在寄存器中的
	// 逐位表示形式。
	//
	// 这意味着在大端架构上，整数值应该在槽位的高位。
	// 浮点数通常直接表示，但某些架构对窄宽度浮点值
	// 有特殊处理（例如，它们首先被提升，或者需要进行 NaN 装箱）。
	Ints   [IntArgRegs]uintptr  // 无类型整数寄存器
	Floats [FloatArgRegs]uint64 // 无类型浮点寄存器

	// 此点以上的字段对汇编代码是已知的。

	// Ptrs 是一个与 Ints 重复但具有指针类型的空间，
	// 用于通过将类型设为 unsafe.Pointer 使在寄存器中
	// 传递或返回的指针对 GC 可见。
	Ptrs [IntArgRegs]unsafe.Pointer

	// ReturnIsPtr 是一个位图，指示哪些寄存器在 reflectcall
	// 的返回路径上包含或将包含指针。第 i 位指示第 i 个
	// 寄存器是否包含或将包含有效的 Go 指针。
	ReturnIsPtr IntArgRegBitmap
}

func (r *RegArgs) Dump() {
	print("Ints:")
	for _, x := range r.Ints {
		print(" ", x)
	}
	println()
	print("Floats:")
	for _, x := range r.Floats {
		print(" ", x)
	}
	println()
	print("Ptrs:")
	for _, x := range r.Ptrs {
		print(" ", x)
	}
	println()
}

// IntRegArgAddr 返回 r.Ints[reg] 内部的一个指针，该指针针对
// 大小为 argSize 的参数进行了适当的偏移。
//
// argSize 必须非零、能够放入寄存器中，且为 2 的幂次方。
//
// 此方法是用于处理不同 CPU 架构字节序的辅助函数，
// 因为在大端架构中，小于字长的参数需要"对齐"到寄存器的
// 高位边缘才能被 CPU 正确解释。
func (r *RegArgs) IntRegArgAddr(reg int, argSize uintptr) unsafe.Pointer {
	if argSize > goarch.PtrSize || argSize == 0 || argSize&(argSize-1) != 0 {
		panic("invalid argSize")
	}
	offset := uintptr(0)
	if goarch.BigEndian {
		offset = goarch.PtrSize - argSize
	}
	return unsafe.Pointer(uintptr(unsafe.Pointer(&r.Ints[reg])) + offset)
}

// IntArgRegBitmap 是一个足够大的位图，可以为每个
// 整数参数/返回值寄存器保存一位。
type IntArgRegBitmap [(IntArgRegs + 7) / 8]uint8

// Set 将位图的第 i 位设置为 1。
func (b *IntArgRegBitmap) Set(i int) {
	b[i/8] |= uint8(1) << (i % 8)
}

// Get 返回位图的第 i 位是否被设置。
//
// nosplit 是因为它在极其敏感的上下文中被调用，
// 例如在 reflectcall 的返回路径上。
//
//go:nosplit
func (b *IntArgRegBitmap) Get(i int) bool {
	// 计算 p=&b[i/8]，但不进行边界检查。我们没有足够的栈空间来做这个。
	p := (*byte)(unsafe.Pointer(uintptr(unsafe.Pointer(b)) + uintptr(i/8)))
	return *p&(uint8(1)<<(i%8)) != 0
}
