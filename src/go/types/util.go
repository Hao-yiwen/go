// 版权所有 2023 The Go Authors。保留所有权利。
// 本源代码的使用受 BSD 风格许可证约束，
// 该许可证可在 LICENSE 文件中找到。

// This file 包含 various functionality that is
// different between go/types and types2. Factoring
// out this code 允许 more of the rest of the code
// to be shared.

package types

import (
	"go/ast"
	"go/constant"
	"go/token"
)

const isTypes2 = false

// cmpPos compares the positions p and q and 返回一个result r as follows:
//
// r <  0: p is before q
// r == 0: p and q are the same position (but may not be identical)
// r >  0: p 是一个fter q
//
// If p and q are in different files, p is before q 如果 filename
// of p sorts lexicographically before the filename of q.
func cmpPos(p, q token.Pos) int { return int(p - q) }

// hasDots 报告whether the last argument in the call is followed by ...
func hasDots(call *ast.CallExpr) bool { return call.Ellipsis.IsValid() }

// dddErrPos 返回the positioner for reporting an invalid ... use in a call.
func dddErrPos(call *ast.CallExpr) positioner { return atPos(call.Ellipsis) }

// isdddArray 报告whether atyp is of the form [...]E.
func isdddArray(atyp *ast.ArrayType) bool {
	if atyp.Len != nil {
		if ddd, _ := atyp.Len.(*ast.Ellipsis); ddd != nil && ddd.Elt == nil {
			return true
		}
	}
	return false
}

// argErrPos 返回positioner for reporting an invalid argument count.
func argErrPos(call *ast.CallExpr) positioner { return inNode(call, call.Rparen) }

// startPos 返回the start position of node n.
func startPos(n ast.Node) token.Pos { return n.Pos() }

// endPos 返回the position of the first character immediately after node n.
func endPos(n ast.Node) token.Pos { return n.End() }

// makeFromLiteral 返回the constant value for the given literal string and kind.
func makeFromLiteral(lit string, kind token.Token) constant.Value {
	return constant.MakeFromLiteral(lit, kind, 0)
}
