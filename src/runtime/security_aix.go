// Copyright 2023 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package runtime

// secureMode 仅在 schedinit 中被修改，因此我们不需要担心
// 同步原语。
var secureMode bool

func initSecureMode() {
	secureMode = !(getuid() == geteuid() && getgid() == getegid())
}

func isSecureMode() bool {
	return secureMode
}
