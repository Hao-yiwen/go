// 版权所有 2021 The Go Authors。保留所有权利。
// 本源代码的使用受 BSD 风格许可证约束，
// 该许可证可在 LICENSE 文件中找到。

//go:build !ppc64 && !ppc64le && !riscv64 && !s390x

package reflect

import "unsafe"

// 本文件实现了 float32 值到其寄存器表示的直接转换。
// 此转换适用于 amd64 和 arm64。它也被选择用于零参数寄存器的情况，
// 但不会被使用。

func archFloat32FromReg(reg uint64) float32 {
	i := uint32(reg)
	return *(*float32)(unsafe.Pointer(&i))
}

func archFloat32ToReg(val float32) uint64 {
	return uint64(*(*uint32)(unsafe.Pointer(&val)))
}
