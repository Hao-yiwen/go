// 版权所有 2021 The Go Authors。保留所有权利。
// 本源代码的使用受 BSD 风格许可证约束，
// 该许可证可在 LICENSE 文件中找到。

package x

import "internal/abi"

func Fn0() // 在汇编中定义

func Fn1() {}

var FnExpr func()

func test() {
	_ = abi.FuncPCABI0(Fn0)           // 第 16 行，无错误
	_ = abi.FuncPCABIInternal(Fn0)    // 第 17 行，错误
	_ = abi.FuncPCABI0(Fn1)           // 第 18 行，错误
	_ = abi.FuncPCABIInternal(Fn1)    // 第 19 行，无错误
	_ = abi.FuncPCABI0(FnExpr)        // 第 20 行，错误
	_ = abi.FuncPCABIInternal(FnExpr) // 第 21 行，无错误
}
