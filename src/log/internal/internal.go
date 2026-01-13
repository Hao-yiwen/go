// 版权所有 2023 The Go Authors。保留所有权利。
// 本源代码的使用受 BSD 风格许可证约束，
// 该许可证可在 LICENSE 文件中找到。

// internal 包包含 log 和 log/slog 共同使用的定义。
package internal

// DefaultOutput 持有一个调用默认 log.Logger 输出函数的函数。
// 它允许 slog.defaultHandler 调用 log 包的未导出函数。
var DefaultOutput func(pc uintptr, data []byte) error
