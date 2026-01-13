// Copyright 2019 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package runtime

import (
	"internal/runtime/syscall/linux"
	"unsafe"
)

type riscvHWProbePairs = struct {
	key   int64
	value uint64
}

// TODO: 考虑是否使用 riscv_hwprobe 的 VDSO 条目。
// 存在一个 riscv_hwprobe 的 VDSO 条目，应该允许我们完全避免 syscall，
// 因为它可以处理调用者仅请求所有核心都支持的扩展的情况，这正是我们在这里所做的。
// 但是，由于我们只调用这个 syscall 一次，实现 VDSO 调用可能不值得付出额外的努力。

//go:linkname internal_cpu_riscvHWProbe internal/cpu.riscvHWProbe
func internal_cpu_riscvHWProbe(pairs []riscvHWProbePairs, flags uint) bool {
	// sys_RISCV_HWPROBE 从 golang.org/x/sys/unix/zsysnum_linux_riscv64.go 复制。
	const sys_RISCV_HWPROBE uintptr = 258

	if len(pairs) == 0 {
		return false
	}
	// 传入 cpuCount 为 0 和 cpu 为 nil 确保仅返回所有核心都支持的扩展，
	// 这是我们在 internal/cpu 中想要的行为。
	_, _, e1 := linux.Syscall6(sys_RISCV_HWPROBE, uintptr(unsafe.Pointer(&pairs[0])), uintptr(len(pairs)), uintptr(0), uintptr(unsafe.Pointer(nil)), uintptr(flags), 0)
	return e1 == 0
}
