// Copyright 2014 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

//go:build !arm && !arm64 && !mips64 && !mips64le && !mips && !mipsle && !wasm

package runtime

// 注意：cputicks 不保证单调递增！特别是，我们在某些操作系统/架构组合上
// 注意到 CPU 之间的偏差。参见 issue 8976。
func cputicks() int64
