// 版权所有 2012 The Go Authors。保留所有权利。
// 本源代码的使用受 BSD 风格许可证约束，
// 该许可证可在 LICENSE 文件中找到。

// Test cases for sort order of declarations.

package d

// C2 应该是 third.
const C2 = 2

// V2 应该是 third.
var V2 int

// CBx constants should appear before CAx constants.
const (
	CB2 = iota // before CB1
	CB1        // before CB0
	CB0        // at end
)

// VBx variables should appear before VAx variables.
var (
	VB2 int // before VB1
	VB1 int // before VB0
	VB0 int // at end
)

const (
	// Single const declarations inside ()'s are considered ungrouped
	// and show up in sorted order.
	Cungrouped = 0
)

var (
	// Single var declarations inside ()'s are considered ungrouped
	// and show up in sorted order.
	Vungrouped = 0
)

// T2 应该是 third.
type T2 struct{}

// Grouped types are sorted nevertheless.
type (
	// TG2 应该是 third.
	TG2 struct{}

	// TG1 应该是 second.
	TG1 struct{}

	// TG0 应该是 first.
	TG0 struct{}
)

// F2 应该是 third.
func F2() {}
