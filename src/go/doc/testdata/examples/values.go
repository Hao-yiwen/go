// 版权所有 2021 The Go Authors。保留所有权利。
// 本源代码的使用受 BSD 风格许可证约束，
// 该许可证可在 LICENSE 文件中找到。

package foo_test

// Variable declaration with fewer values than names.

func f() (int, int) {
	return 1, 2
}

var a, b = f()

// Need two examples to hit playExample.

func ExampleA() {
	_ = a
}

func ExampleB() {
}
