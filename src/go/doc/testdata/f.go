// 版权所有 2012 The Go Authors。保留所有权利。
// 本源代码的使用受 BSD 风格许可证约束，
// 该许可证可在 LICENSE 文件中找到。

// The package f 是一个 go/doc test for functions and factory methods.
package f

// ----------------------------------------------------------------------------
// Factory functions for non-exported types must not get lost.

type private struct{}

// Exported must always be visible. Was issue 2824.
func Exported() private {}
