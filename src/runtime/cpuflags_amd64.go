// Copyright 2015 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package runtime

import (
	"internal/cpu"
)

var memmoveBits uint8

const (
	// avxSupported 表示 CPU 支持 AVX 指令。
	avxSupported = 1 << 0

	// repmovsPreferred 表示 REP MOVSx 指令在
	// CPU 上更有效。
	repmovsPreferred = 1 << 1
)

func init() {
	// 这里我们假设在具有 FSRM 和 ERMS 功能的现代 CPU 上，
	// 使用 REP MOVSB 指令复制 2KB 或更大的数据块
	// 会更有效，以避免必须跟上 CPU 代数。
	// 因此，我们可能保留一个 BlockList 机制以确保不符合此情况的微架构
	// 可能在将来出现。
	// 我们首先在 Intel CPU 上启用它，将来可能支持更多平台。
	isERMSNiceCPU := isIntel
	useREPMOV := isERMSNiceCPU && cpu.X86.HasERMS && cpu.X86.HasFSRM
	if cpu.X86.HasAVX {
		memmoveBits |= avxSupported
	}
	if useREPMOV {
		memmoveBits |= repmovsPreferred
	}
}
