// Copyright 2013 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package runtime

import (
	"internal/abi"
	"unsafe"
)

func lwp_mcontext_init(mc *mcontextt, stk unsafe.Pointer, mp *m, gp *g, fn uintptr) {
	// 用于 LWP 的机器依赖 mcontext 初始化。
	mc.__gregs[_REG_R15] = uint32(abi.FuncPCABI0(lwp_tramp))
	mc.__gregs[_REG_R13] = uint32(uintptr(stk))
	mc.__gregs[_REG_R0] = uint32(uintptr(unsafe.Pointer(mp)))
	mc.__gregs[_REG_R1] = uint32(uintptr(unsafe.Pointer(gp)))
	mc.__gregs[_REG_R2] = uint32(fn)
}

func checkgoarm() {
	// TODO(minux): 类似于 os_linux_arm.go 中的 FP 检查。

	// osinit 还未被调用，所以 numCPUStartup 未设置：必须直接
	// 使用 getCPUCount。
	if getCPUCount() > 1 && goarm < 7 {
		print("runtime: this system has multiple CPUs and must use\n")
		print("atomic synchronization instructions. Recompile using GOARM=7.\n")
		exit(1)
	}
}

//go:nosplit
func cputicks() int64 {
	// runtime·nanotime() 是 CPU 计时的粗略近似，但对分析器来说是足够的。
	return nanotime()
}
