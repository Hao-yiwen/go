// Copyright 2021 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

//go:build asan

package runtime

import (
	"internal/runtime/sys"
	"unsafe"
)

// 公共地址清理器 API。
func ASanRead(addr unsafe.Pointer, len int) {
	sp := sys.GetCallerSP()
	pc := sys.GetCallerPC()
	doasanread(addr, uintptr(len), sp, pc)
}

func ASanWrite(addr unsafe.Pointer, len int) {
	sp := sys.GetCallerSP()
	pc := sys.GetCallerPC()
	doasanwrite(addr, uintptr(len), sp, pc)
}

// 运行时的私有接口。
const asanenabled = true
const asanenabledBit = 1

// asan{read,write} 是 nosplit 的，因为它们可能在 fork 和 exec 之间被调用，
// 此时栈不能增长。参见 issue #50391。

//go:linkname asanread
//go:nosplit
func asanread(addr unsafe.Pointer, sz uintptr) {
	sp := sys.GetCallerSP()
	pc := sys.GetCallerPC()
	doasanread(addr, sz, sp, pc)
}

//go:linkname asanwrite
//go:nosplit
func asanwrite(addr unsafe.Pointer, sz uintptr) {
	sp := sys.GetCallerSP()
	pc := sys.GetCallerPC()
	doasanwrite(addr, sz, sp, pc)
}

//go:noescape
func doasanread(addr unsafe.Pointer, sz, sp, pc uintptr)

//go:noescape
func doasanwrite(addr unsafe.Pointer, sz, sp, pc uintptr)

//go:noescape
func asanunpoison(addr unsafe.Pointer, sz uintptr)

//go:noescape
func asanpoison(addr unsafe.Pointer, sz uintptr)

//go:noescape
func asanregisterglobals(addr unsafe.Pointer, n uintptr)

//go:noescape
func lsanregisterrootregion(addr unsafe.Pointer, n uintptr)

//go:noescape
func lsanunregisterrootregion(addr unsafe.Pointer, n uintptr)

func lsandoleakcheck()

// 这些从 asan_GOARCH.s 调用
//
//go:cgo_import_static __asan_read_go
//go:cgo_import_static __asan_write_go
//go:cgo_import_static __asan_unpoison_go
//go:cgo_import_static __asan_poison_go
//go:cgo_import_static __asan_register_globals_go
//go:cgo_import_static __lsan_register_root_region_go
//go:cgo_import_static __lsan_unregister_root_region_go
//go:cgo_import_static __lsan_do_leak_check_go
