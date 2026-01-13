// Copyright 2015 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

//go:build ppc64 || ppc64le

package runtime

// crosscall_ppc64 调用 runtime 来设置 Go runtime 期望的寄存器，
// 因此它调用的符号需要导出以便外部链接能够正常工作。
//
//go:cgo_export_static _cgo_reginit
