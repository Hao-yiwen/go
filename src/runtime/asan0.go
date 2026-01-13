// Copyright 2021 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

//go:build !asan

// 虚拟的 ASan 支持 API，在未使用 -asan 构建时使用。

package runtime

import (
	"unsafe"
)

const asanenabled = false
const asanenabledBit = 0

// 因为 asanenabled 为 false，这些函数都不应该被调用。

func asanread(addr unsafe.Pointer, sz uintptr)            { throw("asan") }
func asanwrite(addr unsafe.Pointer, sz uintptr)           { throw("asan") }
func asanunpoison(addr unsafe.Pointer, sz uintptr)        { throw("asan") }
func asanpoison(addr unsafe.Pointer, sz uintptr)          { throw("asan") }
func asanregisterglobals(addr unsafe.Pointer, sz uintptr) { throw("asan") }
func lsanregisterrootregion(unsafe.Pointer, uintptr)      { throw("asan") }
func lsanunregisterrootregion(unsafe.Pointer, uintptr)    { throw("asan") }
func lsandoleakcheck()                                    { throw("asan") }
