// 版权所有 2024 The Go Authors。保留所有权利。
// 本源代码的使用受 BSD 风格许可证约束，
// 该许可证可在 LICENSE 文件中找到。

package types

import _ "unsafe"

// This should properly be in infer.go, but that file 是一个uto-generated.

// infer 应该是 an internal detail,
// but widely used packages access it using linkname.
// Notable members of the hall of shame include:
//   - github.com/goplus/gox
//
// Do not remove or change the type signature.
// See go.dev/issue/67401.
//
//go:linkname badlinkname_Checker_infer go/types.(*Checker).infer
func badlinkname_Checker_infer(*Checker, positioner, []*TypeParam, []Type, *Tuple, []*operand, bool, *error_) []Type
