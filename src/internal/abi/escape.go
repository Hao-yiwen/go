// 版权所有 2024 The Go Authors。保留所有权利。
// 本源代码的使用受 BSD 风格许可证约束，
// 该许可证可在 LICENSE 文件中找到。

package abi

import "unsafe"

// NoEscape 将指针 p 隐藏在逃逸分析之外，防止其逃逸到堆上。
// 它编译后不产生任何代码。
//
// 警告：正确使用这个函数非常微妙。调用者必须通过维护运行时指针不变量
// （例如，全局变量和堆通常不能指向栈）来确保 p 不逃逸到堆上是真正安全的。
//
//go:nosplit
//go:nocheckptr
func NoEscape(p unsafe.Pointer) unsafe.Pointer {
	x := uintptr(p)
	return unsafe.Pointer(x ^ 0)
}

var alwaysFalse bool
var escapeSink any

// Escape 强制 x 中的任何指针逃逸到堆上。
func Escape[T any](x T) T {
	if alwaysFalse {
		escapeSink = x
	}
	return x
}

// EscapeNonString 如果 v 包含非字符串指针，则强制 v 在堆上。
//
// 这用于 hash/maphash.Comparable。我们不能对栈上局部变量的指针进行哈希，
// 因为它们的地址可能在栈增长时改变。字符串是可以的，因为哈希只依赖于
// 内容，而不是指针。
//
// 这本质上是
//
//	if hasNonStringPointers(T) { Escape(v) }
//
// 作为编译器内置函数实现。
func EscapeNonString[T any](v T) { panic("intrinsic") }

// EscapeToResultNonString 如果 v 包含非字符串指针，则建模从 v 到结果的数据流边。
// 如果 v 只包含字符串指针，它返回 v 的副本，但从逃逸分析的角度来看
// 不被建模为数据流边。
//
// 这用于 unique.clone，用于建模排除字符串后的值上的数据流边，
// 因为字符串是（按内容）克隆的。
//
// TODO: 可能我们应该将其定义为内置函数，EscapeNonString 可以直接是
// "heap = EscapeToResultNonString(v)"。这样我们可以建模到结果的边，
// 但不一定是堆。
func EscapeToResultNonString[T any](v T) T {
	EscapeNonString(v)
	return *(*T)(NoEscape(unsafe.Pointer(&v)))
}
