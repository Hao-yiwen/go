// Copyright 2017 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package runtime

import _ "unsafe" // 用于 go:linkname

//go:linkname boring_runtime_arg0 crypto/internal/boring.runtime_arg0
func boring_runtime_arg0() string {
	// 在 Windows 上，argslice 未设置，找到 argv0 会很麻烦。
	if len(argslice) == 0 {
		return ""
	}
	return argslice[0]
}
