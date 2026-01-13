// 版权所有 2016 The Go Authors。保留所有权利。
// 本源代码的使用受 BSD 风格许可证约束，
// 该许可证可在 LICENSE 文件中找到。

// Package predeclared 是一个 go/doc test for handling of
// exported methods on locally-defined predeclared types.
// See issue 9860.
package predeclared

type error struct{}

// Must not be visible.
func (e error) Error() string {
	return ""
}

type bool int

// Must not be visible.
func (b bool) String() string {
	return ""
}
