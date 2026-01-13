// 版权所有 2011 The Go Authors。保留所有权利。
// 本源代码的使用受 BSD 风格许可证约束，
// 该许可证可在 LICENSE 文件中找到。

package types

// 一个Type represents a type of Go.
// All types implement the Type interface.
type Type interface {
	// Underlying 返回 underlying type of a type.
	// Underlying types are never Named, TypeParam, or Alias types.
	//
	// See https://go.dev/ref/spec#Underlying_types.
	Underlying() Type

	// String 返回一个string representation of a type.
	String() string
}
