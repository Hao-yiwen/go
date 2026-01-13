// 版权所有 2023 The Go Authors。保留所有权利。
// 本源代码的使用受 BSD 风格许可证约束，
// 该许可证可在 LICENSE 文件中找到。

//go:build !gccgo

package abi

// FuncPC* 内置函数。
//
// 注意：在使用插件的程序中，FuncPC* 可能为同一函数返回不同的值
// （因为地址空间中实际上有同一函数的多个副本）。为安全起见，
// 不要在任何 == 表达式中使用此函数的结果。只有将结果用作
// 开始执行代码的地址才是安全的。

// FuncPCABI0 返回函数 f 的入口 PC，f 必须是定义为 ABI0 的函数的
// 直接引用。否则会产生编译时错误。
//
// 作为编译器内置函数实现。
func FuncPCABI0(f any) uintptr

// FuncPCABIInternal 返回函数 f 的入口 PC。如果 f 是函数的直接引用，
// 它必须定义为 ABIInternal。否则会产生编译时错误。如果 f 不是
// 已定义函数的直接引用，则假定 f 是一个 func 值。否则行为未定义。
//
// 作为编译器内置函数实现。
func FuncPCABIInternal(f any) uintptr
