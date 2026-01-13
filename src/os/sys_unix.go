// 版权所有 2014 The Go Authors。保留所有权利。
// 本源代码的使用受 BSD 风格许可证约束，
// 该许可证可在 LICENSE 文件中找到。

//go:build unix

package os

// supportsCloseOnExec 报告平台是否支持
// O_CLOEXEC 标志。
// 在 Darwin 上，O_CLOEXEC 标志在 OS X 10.7（Darwin 11.0.0）中引入。
// 见 https://support.apple.com/kb/HT1633。
// 在 FreeBSD 上，O_CLOEXEC 标志在版本 8.3 中引入。
const supportsCloseOnExec = true
