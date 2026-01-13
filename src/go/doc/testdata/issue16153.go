// 版权所有 2017 The Go Authors。保留所有权利。
// 本源代码的使用受 BSD 风格许可证约束，
// 该许可证可在 LICENSE 文件中找到。

package issue16153

// original test case
const (
	x1 uint8 = 255
	Y1       = 256
)

// variations
const (
	x2 uint8 = 255
	Y2
)

const (
	X3 int64 = iota
	Y3       = 1
)

const (
	X4 int64 = iota
	Y4
)
