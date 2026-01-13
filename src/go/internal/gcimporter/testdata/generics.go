// 版权所有 2021 The Go Authors。保留所有权利。
// 本源代码的使用受 BSD 风格许可证约束，
// 该许可证可在 LICENSE 文件中找到。

// This file is used to generate an object file which
// serves as test file for gcimporter_test.go.

package generics

type Any any

var x any

type T[A, B any] struct {
	Left  A
	Right B
}

var X T[int, string] = T[int, string]{1, "hi"}

func ToInt[P interface{ ~int }](p P) int { return int(p) }

var IntID = ToInt[int]

type G[C comparable] int

func ImplicitFunc[T ~int]() {}

type ImplicitType[T ~int] int
