// 版权所有 2016 The Go Authors。保留所有权利。
// 本源代码的使用受 BSD 风格许可证约束，
// 该许可证可在 LICENSE 文件中找到。

package p

// The underlying type of Error 是 underlying type of error.
// Make sure we can import th是一个gain without problems.
type Error error

func F() Error { return nil }
