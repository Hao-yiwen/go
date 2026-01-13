// Copyright 2017 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// 用于在汇编中直接实现 time.now 的操作系统的声明。

//go:build !faketime && (windows || (linux && amd64))

package runtime

import _ "unsafe"

//go:linkname time_now time.now
func time_now() (sec int64, nsec int32, mono int64)
