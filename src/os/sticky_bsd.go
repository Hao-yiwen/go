// 版权所有 2014 The Go Authors。保留所有权利。
// 本源代码的使用受 BSD 风格许可证约束，
// 该许可证可在 LICENSE 文件中找到。

//go:build aix || darwin || dragonfly || freebsd || (js && wasm) || netbsd || openbsd || solaris || wasip1

package os

// 根据 sticky(8)，open(2) 和 mkdir(2) 都不会创建
// 设置了 sticky 位的文件。
const supportsCreateWithStickyBit = false
