// 版权所有 2023 The Go Authors。保留所有权利。
// 本源代码的使用受 BSD 风格许可证约束，
// 该许可证可在 LICENSE 文件中找到。

package abi

import (
	"unsafe"
)

// 多个包共用的 Map 常量
// runtime/runtime-gdb.py:MapTypePrinter 包含它自己的副本
const (
	// group.slot 计数中的位数。
	MapGroupSlotsBits = 3

	// 一个组中的槽位数。
	MapGroupSlots = 1 << MapGroupSlotsBits // 8

	// 保持内联的最大 key 或 elem 大小（而不是为每个元素分配内存）。
	// 必须能放入 uint8。
	MapMaxKeyBytes  = 128
	MapMaxElemBytes = 128

	ctrlEmpty = 0b10000000
	bitsetLSB = 0x0101010101010101

	// 所有槽位都为空时控制字的值。
	MapCtrlEmpty = bitsetLSB * uint64(ctrlEmpty)
)

type MapType struct {
	Type
	Key   *Type
	Elem  *Type
	Group *Type // 表示槽位组的内部类型
	// 用于对键进行哈希的函数 (key 指针, seed) -> hash
	Hasher    func(unsafe.Pointer, uintptr) uintptr
	GroupSize uintptr // == Group.Size_
	SlotSize  uintptr // key/elem 槽位的大小
	ElemOff   uintptr // elem 在 key/elem 槽位中的偏移量
	Flags     uint32
}

// 标志值
const (
	MapNeedKeyUpdate = 1 << iota
	MapHashMightPanic
	MapIndirectKey
	MapIndirectElem
)

func (mt *MapType) NeedKeyUpdate() bool { // 如果覆盖时需要更新 key 则为 true
	return mt.Flags&MapNeedKeyUpdate != 0
}
func (mt *MapType) HashMightPanic() bool { // 如果哈希函数可能 panic 则为 true
	return mt.Flags&MapHashMightPanic != 0
}
func (mt *MapType) IndirectKey() bool { // 存储指向 key 的指针而不是 key 本身
	return mt.Flags&MapIndirectKey != 0
}
func (mt *MapType) IndirectElem() bool { // 存储指向 elem 的指针而不是 elem 本身
	return mt.Flags&MapIndirectElem != 0
}
