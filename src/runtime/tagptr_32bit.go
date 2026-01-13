// Copyright 2014 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

//go:build 386 || arm || mips || mipsle || (gccgo && (ppc || s390))

package runtime

import "unsafe"

// taggedPointer 的数字标签中存储的位数
const taggedPointerBits = 32

// 标签中允许的位数。
const tagBits = 32

// 在 32 位系统上，taggedPointer 有一个 32 位指针和 32 位计数。

// taggedPointerPack 从指针和标签创建 taggedPointer。
// 不适合结果的标签位被丢弃。
func taggedPointerPack(ptr unsafe.Pointer, tag uintptr) taggedPointer {
	return taggedPointer(uintptr(ptr))<<32 | taggedPointer(tag)
}

// Pointer 从 taggedPointer 返回指针。
func (tp taggedPointer) pointer() unsafe.Pointer {
	return unsafe.Pointer(uintptr(tp >> 32))
}

// Tag 从 taggedPointer 返回标签。
func (tp taggedPointer) tag() uintptr {
	return uintptr(tp)
}
