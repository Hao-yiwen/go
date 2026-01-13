// Copyright 2009 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// Malloc 小大小类。
//
// 概述见 malloc.go。
// 关于我们如何决定使用哪些大小类，另见 mksizeclasses.go。

package runtime

import "internal/runtime/gc"

// 返回 mallocgc 在请求该大小时将分配的内存块的大小，
// 减去元数据的任何内联空间。
func roundupsize(size uintptr, noscan bool) (reqSize uintptr) {
	reqSize = size
	if reqSize <= maxSmallSize-gc.MallocHeaderSize {
		// 小对象。
		if !noscan && reqSize > gc.MinSizeForMallocHeader { // !noscan && !heapBitsInSpan(reqSize)
			reqSize += gc.MallocHeaderSize
		}
		// (reqSize - size) 要么是 mallocHeaderSize，要么是 0。我们需要从结果中减去 mallocHeaderSize
		// 如果我们有一个，因为 mallocgc 会将其加回来。
		if reqSize <= gc.SmallSizeMax-8 {
			return uintptr(gc.SizeClassToSize[gc.SizeToSizeClass8[divRoundUp(reqSize, gc.SmallSizeDiv)]]) - (reqSize - size)
		}
		return uintptr(gc.SizeClassToSize[gc.SizeToSizeClass128[divRoundUp(reqSize-gc.SmallSizeMax, gc.LargeSizeDiv)]]) - (reqSize - size)
	}
	// 大对象。将 reqSize 对齐到下一页。检查溢出。
	reqSize += pageSize - 1
	if reqSize < size {
		return size
	}
	return reqSize &^ (pageSize - 1)
}
