// 版权所有 2022 The Go Authors。保留所有权利。
// 本源代码的使用受 BSD 风格许可证约束，
// 该许可证可在 LICENSE 文件中找到。

package net

import "internal/syscall/unix"

const cgoAddrInfoFlags = (unix.AI_CANONNAME | unix.AI_V4MAPPED | unix.AI_ALL) & unix.AI_MASK
