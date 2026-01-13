// 版权所有 2016 The Go Authors。保留所有权利。
// 本源代码的使用受 BSD 风格许可证约束，
// 该许可证可在 LICENSE 文件中找到。

package issue13742

import (
	"go/ast"
	. "go/ast"
)

// Both F0 and G0 should appear as functions.
func F0(Node)  {}
func G0() Node { return nil }

// Both F1 and G1 should appear as functions.
func F1(ast.Node)  {}
func G1() ast.Node { return nil }
