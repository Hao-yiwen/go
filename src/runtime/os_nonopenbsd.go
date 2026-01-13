// Copyright 2018 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

//go:build !openbsd

package runtime

// osStackAlloc 在 s 被用作堆栈内存前执行特定于操作系统的初始化。
func osStackAlloc(s *mspan) {
}

// osStackFree 在 s 返回到堆前撤销 osStackAlloc 的效果。
func osStackFree(s *mspan) {
}
