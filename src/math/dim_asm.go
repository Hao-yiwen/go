// 版权所有 2021 The Go Authors。保留所有权利。
// 本源代码的使用受 BSD 风格许可证约束，
// 该许可证可在 LICENSE 文件中找到。

//go:build amd64 || arm64 || loong64 || riscv64 || s390x

package math

const haveArchMax = true

func archMax(x, y float64) float64

const haveArchMin = true

func archMin(x, y float64) float64
