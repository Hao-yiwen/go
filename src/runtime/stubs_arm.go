// Copyright 2018 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package runtime

import "unsafe"

// 从编译器生成的代码中调用；为 go vet 声明。
func udiv()
func _div()
func _divu()
func _mod()
func _modu()

// 仅从汇编调用；为 go vet 声明。
func usplitR0()
func load_g()
func save_g()
func emptyfunc()
func _initcgo()
func read_tls_fallback()

//go:noescape
func asmcgocall_no_g(fn, arg unsafe.Pointer)

// getfp 返回其调用者的帧指针寄存器，如果未实现则返回 0。
// TODO: 使其成为编译器内在函数
//
//go:nosplit
func getfp() uintptr { return 0 }
