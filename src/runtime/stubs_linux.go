// Copyright 2017 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

//go:build linux

package runtime

import "unsafe"

func sbrk0() uintptr

// 仅从 write_err_android.go 调用，但在 sys_linux_*.s 中定义；
// 在此声明（而不是在 write_err_android.go 中）是为了在非 Android 构建上进行 go vet。
// 返回值是原始系统调用结果，可能编码一个错误号。
//
//go:noescape
func access(name *byte, mode int32) int32
func connect(fd int32, addr unsafe.Pointer, len int32) int32
func socket(domain int32, typ int32, prot int32) int32
