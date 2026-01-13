// 版权所有 2012 The Go Authors。保留所有权利。
// 本源代码的使用受 BSD 风格许可证约束，
// 该许可证可在 LICENSE 文件中找到。

// Test cases for sort order of declarations.

package d

// C1 应该是 second.
const C1 = 1

// C0 应该是 first.
const C0 = 0

// V1 应该是 second.
var V1 uint

// V0 应该是 first.
var V0 uintptr

// CAx constants should appear after CBx constants.
const (
	CA2 = iota // before CA1
	CA1        // before CA0
	CA0        // at end
)

// VAx variables should appear after VBx variables.
var (
	VA2 int // before VA1
	VA1 int // before VA0
	VA0 int // at end
)

// T1 应该是 second.
type T1 struct{}

// T0 应该是 first.
type T0 struct{}

// F1 应该是 second.
func F1() {}

// F0 应该是 first.
func F0() {}
