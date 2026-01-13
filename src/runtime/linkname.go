// Copyright 2024 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package runtime

import _ "unsafe"

// 用于 internal/godebug 和 syscall
//go:linkname write

// 由 cgo 使用
//go:linkname _cgo_panic_internal
//go:linkname cgoAlwaysFalse
//go:linkname cgoUse
//go:linkname cgoKeepAlive
//go:linkname cgoCheckPointer
//go:linkname cgoCheckResult
//go:linkname cgoNoCallback
//go:linkname gobytes
//go:linkname gostringn

// 用于插件
//go:linkname doInit

// 用于 math/bits
//go:linkname overflowError
//go:linkname divideError

// 用于测试
//go:linkname extraMInUse
//go:linkname blockevent
//go:linkname haveHighResSleep
//go:linkname blockUntilEmptyFinalizerQueue
//go:linkname lockedOSThread
