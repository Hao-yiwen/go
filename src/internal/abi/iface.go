// 版权所有 2023 The Go Authors。保留所有权利。
// 本源代码的使用受 BSD 风格许可证约束，
// 该许可证可在 LICENSE 文件中找到。

package abi

import "unsafe"

// 每个非空接口类型的第一个字包含一个 *ITab。
// 它记录底层具体类型（Type）、它正在实现的接口类型（Inter），
// 以及一些辅助信息。
//
// 分配在非垃圾回收内存中
type ITab struct {
	Inter *InterfaceType
	Type  *Type
	Hash  uint32     // Type.Hash 的副本。用于类型 switch。
	Fun   [1]uintptr // 可变大小。fun[0]==0 表示 Type 没有实现 Inter。
}

// EmptyInterface 描述 "interface{}" 或 "any" 的布局。
// 这些与非空接口的表示方式不同，因为第一个字总是指向 abi.Type。
type EmptyInterface struct {
	Type *Type
	Data unsafe.Pointer
}

// NonEmptyInterface 描述包含任何方法的接口的布局。
type NonEmptyInterface struct {
	ITab *ITab
	Data unsafe.Pointer
}

// CommonInterface 描述 [EmptyInterface] 和 [NonEmptyInterface] 的共同布局。
type CommonInterface struct {
	// *ITab 或 *Type，未导出以避免意外使用。
	_ unsafe.Pointer

	Data unsafe.Pointer
}
