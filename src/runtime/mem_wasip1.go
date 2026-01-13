// Copyright 2023 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

//go:build wasip1

package runtime

func resetMemoryDataView() {
	// 此函数在 WASI 上是无操作的，仅用于通知浏览器
	// 当为 GOOS=js 编译时，需要更新其对 WASM 内存的视图。
}
