// Copyright 2009 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package runtime

import (
	"internal/goexperiment"
	"internal/runtime/atomic"
	"unsafe"
)

// 这些函数不能有 go:noescape 注解，
// 因为虽然 ptr 不逃逸，但 new 会逃逸。
// 如果 new 被标记为不逃逸，编译器将对存储的指针值
// 做出错误的逃逸分析决策。

// atomicwb 在原子指针写入之前执行写屏障。
// 调用者应该用 "if writeBarrier.enabled" 保护调用。
//
// atomicwb 应该是一个内部实现细节，
// 但许多广泛使用的包通过 linkname 访问它。
// 耻辱榜上的知名成员包括：
//   - github.com/bytedance/gopkg
//   - github.com/songzhibin97/gkit
//
// 请勿删除或更改类型签名。
// 参见 go.dev/issue/67401。
//
//go:linkname atomicwb
//go:nosplit
func atomicwb(ptr *unsafe.Pointer, new unsafe.Pointer) {
	slot := (*uintptr)(unsafe.Pointer(ptr))
	buf := getg().m.p.ptr().wbBuf.get2()
	buf[0] = *slot
	buf[1] = uintptr(new)
}

// atomicstorep 以原子方式执行 *ptr = new 并调用写屏障。
//
//go:nosplit
func atomicstorep(ptr unsafe.Pointer, new unsafe.Pointer) {
	if writeBarrier.enabled {
		atomicwb((*unsafe.Pointer)(ptr), new)
	}
	if goexperiment.CgoCheck2 {
		cgoCheckPtrWrite((*unsafe.Pointer)(ptr), new)
	}
	atomic.StorepNoWB(noescape(ptr), new)
}

// atomic_storePointer 是 internal/runtime/atomic.UnsafePointer.Store 的实现
// （类似 StoreNoWB 但带有写屏障）。
//
//go:nosplit
//go:linkname atomic_storePointer internal/runtime/atomic.storePointer
func atomic_storePointer(ptr *unsafe.Pointer, new unsafe.Pointer) {
	atomicstorep(unsafe.Pointer(ptr), new)
}

// atomic_casPointer 是 internal/runtime/atomic.UnsafePointer.CompareAndSwap 的实现
// （类似 CompareAndSwapNoWB 但带有写屏障）。
//
//go:nosplit
//go:linkname atomic_casPointer internal/runtime/atomic.casPointer
func atomic_casPointer(ptr *unsafe.Pointer, old, new unsafe.Pointer) bool {
	if writeBarrier.enabled {
		atomicwb(ptr, new)
	}
	if goexperiment.CgoCheck2 {
		cgoCheckPtrWrite(ptr, new)
	}
	return atomic.Casp1(ptr, old, new)
}

// 与上面类似，但用 sync/atomic 的 uintptr 操作实现。
// 我们不能直接调用运行时例程，因为竞态检测器期望
// 能够拦截 sync/atomic 形式但不是运行时形式。

//go:linkname sync_atomic_StoreUintptr sync/atomic.StoreUintptr
func sync_atomic_StoreUintptr(ptr *uintptr, new uintptr)

//go:linkname sync_atomic_StorePointer sync/atomic.StorePointer
//go:nosplit
func sync_atomic_StorePointer(ptr *unsafe.Pointer, new unsafe.Pointer) {
	if writeBarrier.enabled {
		atomicwb(ptr, new)
	}
	if goexperiment.CgoCheck2 {
		cgoCheckPtrWrite(ptr, new)
	}
	sync_atomic_StoreUintptr((*uintptr)(unsafe.Pointer(ptr)), uintptr(new))
}

//go:linkname sync_atomic_SwapUintptr sync/atomic.SwapUintptr
func sync_atomic_SwapUintptr(ptr *uintptr, new uintptr) uintptr

//go:linkname sync_atomic_SwapPointer sync/atomic.SwapPointer
//go:nosplit
func sync_atomic_SwapPointer(ptr *unsafe.Pointer, new unsafe.Pointer) unsafe.Pointer {
	if writeBarrier.enabled {
		atomicwb(ptr, new)
	}
	if goexperiment.CgoCheck2 {
		cgoCheckPtrWrite(ptr, new)
	}
	old := unsafe.Pointer(sync_atomic_SwapUintptr((*uintptr)(noescape(unsafe.Pointer(ptr))), uintptr(new)))
	return old
}

//go:linkname sync_atomic_CompareAndSwapUintptr sync/atomic.CompareAndSwapUintptr
func sync_atomic_CompareAndSwapUintptr(ptr *uintptr, old, new uintptr) bool

//go:linkname sync_atomic_CompareAndSwapPointer sync/atomic.CompareAndSwapPointer
//go:nosplit
func sync_atomic_CompareAndSwapPointer(ptr *unsafe.Pointer, old, new unsafe.Pointer) bool {
	if writeBarrier.enabled {
		atomicwb(ptr, new)
	}
	if goexperiment.CgoCheck2 {
		cgoCheckPtrWrite(ptr, new)
	}
	return sync_atomic_CompareAndSwapUintptr((*uintptr)(noescape(unsafe.Pointer(ptr))), uintptr(old), uintptr(new))
}
