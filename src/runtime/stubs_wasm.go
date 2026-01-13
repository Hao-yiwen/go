// Copyright 2024 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package runtime

// pause 将 SP 设置为 newsp 并暂停 Go 的 WebAssembly 代码的执行，
// 直到触发事件或回调到 Go。
//
// 注意：pause 的尾声从堆栈中弹出 8 个字节，因此当返回到主机时，
// SP 是 newsp+8。
// 如果我们想设置 SP，使得当它回调到 Go 时，Go 函数看起来是从
// pause 的调用者的调用者调用的，然后使用 newsp = internal/runtime/sys.GetCallerSP()-16
// 调用 pause（另外 8 字节是推送到堆栈的返回 PC）。
func pause(newsp uintptr)
