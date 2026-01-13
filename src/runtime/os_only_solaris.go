// Copyright 2019 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// 不适用于 illumos 的 Solaris 代码。

//go:build !illumos

package runtime

func getCPUCount() int32 {
	n := int32(sysconf(__SC_NPROCESSORS_ONLN))
	if n < 1 {
		return 1
	}

	return n
}
