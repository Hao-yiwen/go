// Copyright 2025 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

//go:build !linux

package runtime

import _ "unsafe" // 用于 go:linkname

//go:linkname syscall_runtimeClearenv syscall.runtimeClearenv
func syscall_runtimeClearenv(env map[string]int) {
	// 系统没有 clearenv(3)，所以通过手动取消设置所有变量来模拟它。
	for k := range env {
		syscall_runtimeUnsetenv(k)
	}
}
