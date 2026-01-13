// Copyright 2010 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package runtime

import "unsafe"

func sbrk(n uintptr) unsafe.Pointer {
	// 来自 /sys/src/libc/9sys/sbrk.c 的 Plan 9 sbrk
	bl := bloc
	n = memRound(n)
	if bl+n > blocMax {
		if brk_(unsafe.Pointer(bl+n)) < 0 {
			return nil
		}
		blocMax = bl + n
	}
	bloc += n
	return unsafe.Pointer(bl)
}
