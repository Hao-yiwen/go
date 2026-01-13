// 版权所有 2021 The Go Authors。保留所有权利。
// 本源代码的使用受 BSD 风格许可证约束，
// 该许可证可在 LICENSE 文件中找到。

package foo_test

const (
	a = iota
	b
)

const (
	c = 3
	d = 4
)

const (
	e = iota
	f
)

// The example refers to only one of the constants in the iota group, but we
// must keep all of them because of the iota. The second group of constants can
// be trimmed. The third has an iota, but is unused, so it can be eliminated.

func Example() {
	_ = b
	_ = d
}

// Need two examples to hit the playExample function.

func Example2() {
}
