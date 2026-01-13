// Copyright 2019 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

//go:build mips || mipsle

package runtime

// 仅从汇编调用；为 go vet 声明。
func load_g()
func save_g()

// getfp 返回其调用者的帧指针寄存器，如果未实现则返回 0。
// TODO: 使其成为编译器内在函数
//
//go:nosplit
func getfp() uintptr { return 0 }
