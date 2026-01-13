// Copyright 2024 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

//go:build amd64 || arm64

package runtime

import _ "unsafe"

// 从 Go 1.22 开始，下面的符号被发现通过 linkname 在外部被拉取。
// 我们在这里提供一个推送 linkname，以保持它们可以通过拉取 linkname 访问。
// 这可能在将来改变。请不要在新代码中依赖它们。

//go:linkname vdsoClockgettimeSym
