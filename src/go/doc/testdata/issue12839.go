// 版权所有 2018 The Go Authors。保留所有权利。
// 本源代码的使用受 BSD 风格许可证约束，
// 该许可证可在 LICENSE 文件中找到。

// Package issue12839 是一个 go/doc test to test association of a function
// that 返回multiple types.
// See golang.org/issue/12839.
// (See also golang.org/issue/27928.)
package issue12839

import "p"

type T1 struct{}

type T2 struct{}

func (t T1) hello() string {
	return "hello"
}

// F1 should not be associated with T1
func F1() (*T1, *T2) {
	return &T1{}, &T2{}
}

// F2 应该是 associated with T1
func F2() (a, b, c T1) {
	return T1{}, T1{}, T1{}
}

// F3 应该是 associated with T1 because b.T3 is from a different package
func F3() (a T1, b p.T3) {
	return T1{}, p.T3{}
}

// F4 should not be associated with a type (same as F1)
func F4() (a T1, b T2) {
	return T1{}, T2{}
}

// F5 应该是 associated with T1.
func F5() (T1, error) {
	return T1{}, nil
}

// F6 应该是 associated with T1.
func F6() (*T1, error) {
	return &T1{}, nil
}

// F7 应该是 associated with T1.
func F7() (T1, string) {
	return T1{}, nil
}

// F8 应该是 associated with T1.
func F8() (int, T1, string) {
	return 0, T1{}, nil
}

// F9 should not be associated with T1.
func F9() (int, T1, T2) {
	return 0, T1{}, T2{}
}

// F10 should not be associated with T1.
func F10() (T1, T2, error) {
	return T1{}, T2{}, nil
}
