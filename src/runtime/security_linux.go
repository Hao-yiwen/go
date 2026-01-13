// Copyright 2023 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package runtime

import _ "unsafe"

func initSecureMode() {
	// 我们已在 sysauxv 中初始化了 secureMode 布尔值。
}

func isSecureMode() bool {
	return secureMode
}
