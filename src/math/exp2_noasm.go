// 版权所有 2021 The Go Authors。保留所有权利。
// 本源代码的使用受 BSD 风格许可证约束，
// 该许可证可在 LICENSE 文件中找到。

//go:build !arm64 && !loong64

package math

const haveArchExp2 = false

func archExp2(x float64) float64 {
	panic("not implemented")
}
