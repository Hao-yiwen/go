// Copyright 2018 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

//go:build js

package runtime

// resetMemoryDataView 向 JS 前端信号，表示 WebAssembly 的 memory.grow 指令已被使用。
// 这允许前端用新的 DataView 对象替换旧的 DataView 对象。
//
//go:wasmimport gojs runtime.resetMemoryDataView
func resetMemoryDataView()
