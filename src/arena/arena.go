// Copyright 2022 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

//go:build goexperiment.arenas

/*
arena 包提供了为一组 Go 值分配内存并一次性安全地释放该空间的能力。该功能的目的是
提高效率：在垃圾回收之前手动释放内存会延迟回收周期。回收周期更少意味着垃圾收集器的
CPU 成本发生得更少。

此包中的此功能主要由 Arena 类型捕获。Arena 为 Go 值分配大块内存，因此它们可能
对分配少量小 Go 值来说是低效的。它们最好用于批量分配，每次使用时分配的内存在 MiB
级别。

请注意，通过允许这种有限形式的手动内存分配，use-after-free 错误对于常规 Go 值
是可能的。此包通过防止重用已释放的内存区域来限制这些 use-after-free 错误的影响，
直到垃圾收集器能够确定这样做是安全的。通常，use-after-free 错误会导致故障和有用的
错误消息，但此包保留不强制释放内存故障的权利。这意味着此包的有效实现就是以运行时
通常的方式分配所有内存，实际上它保留了偶尔对某些 Go 值这样做的权利。
*/
package arena

import (
	"internal/reflectlite"
	"unsafe"
)

// Arena 代表一起分配和释放的 Go 值的集合。Arena 对于提高效率很有用，因为它们可以
// 被手动释放回运行时，但任何从已释放的 Arena 中获得的内存一旦发生这种情况就不能被访问。
// Arena 一旦不再被引用就会自动释放，因此必须保持活动状态（参见 runtime.KeepAlive），
// 直到从其分配的任何内存不再需要为止。
//
// Arena 必须永远不能被多个 goroutine 并发使用。
type Arena struct {
	a unsafe.Pointer
}

// NewArena 分配一个新的 arena。
func NewArena() *Arena {
	return &Arena{a: runtime_arena_newArena()}
}

// Free 释放 arena（以及从 arena 分配的所有对象），以便 arena 支持的内存可以在
// 没有垃圾回收开销的情况下相当快速地被重新使用。应用程序在释放 arena 之后不能对其
// 调用任何方法。
func (a *Arena) Free() {
	runtime_arena_arena_Free(a.a)
	a.a = nil
}

// New 在提供的 arena 中创建一个新的 *T。在释放 arena 之后，*T 不能被使用。
// 访问释放后的值可能会导致故障，但也不保证会发生故障。
func New[T any](a *Arena) *T {
	return runtime_arena_arena_New(a.a, reflectlite.TypeOf((*T)(nil))).(*T)
}

// MakeSlice 使用提供的容量和长度创建一个新的 []T。在释放 arena 之后，[]T 不能
// 被使用。释放后访问切片的底层存储可能会导致故障，但这种故障也不保证会发生。
func MakeSlice[T any](a *Arena, len, cap int) []T {
	var sl []T
	runtime_arena_arena_Slice(a.a, &sl, cap)
	return sl[:len]
}

// Clone 制作输入值的浅拷贝，该副本不再绑定到它可能从中分配的任何 arena，
// 并返回该副本。如果它不是从 arena 分配的，则返回原值。此函数对于更容易让
// arena 分配的值比其 arena 活得更久很有用。T 必须是指针、切片或字符串，
// 否则此函数将 panic。
func Clone[T any](s T) T {
	return runtime_arena_heapify(s).(T)
}

//go:linkname reflect_arena_New reflect.arena_New
func reflect_arena_New(a *Arena, typ any) any {
	return runtime_arena_arena_New(a.a, typ)
}

//go:linkname runtime_arena_newArena
func runtime_arena_newArena() unsafe.Pointer

//go:linkname runtime_arena_arena_New
func runtime_arena_arena_New(arena unsafe.Pointer, typ any) any

// 标记为 noescape 以避免逃逸切片头。
//
//go:noescape
//go:linkname runtime_arena_arena_Slice
func runtime_arena_arena_Slice(arena unsafe.Pointer, slice any, cap int)

//go:linkname runtime_arena_arena_Free
func runtime_arena_arena_Free(arena unsafe.Pointer)

//go:linkname runtime_arena_heapify
func runtime_arena_heapify(any) any
