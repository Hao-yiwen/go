// 版权所有 2018 The Go Authors。保留所有权利。
// 本源代码的使用受 BSD 风格许可证约束，
// 该许可证可在 LICENSE 文件中找到。

//go:build js && wasm

package os

// supportsCloseOnExec 报告平台是否支持
// O_CLOEXEC 标志。
const supportsCloseOnExec = false
