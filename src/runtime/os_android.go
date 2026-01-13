// Copyright 2014 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package runtime

import _ "unsafe" // 用于 go:cgo_export_static 和 go:cgo_export_dynamic

// 导出 main 函数。
//
// 由 app 包使用，用于启动通过 JNI 加载的纯 Go Android 应用。
// 参见 golang.org/x/mobile/app。

//go:cgo_export_static main.main
//go:cgo_export_dynamic main.main
