// Copyright 2015 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package runtime

func checkgoarm() {
	return // TODO(minux)
}

//go:nosplit
func cputicks() int64 {
	// runtime·nanotime() 是 CPU 计时的粗略近似，但对分析器来说是足够的。
	return nanotime()
}
