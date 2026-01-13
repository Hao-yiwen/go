// 版权所有 2021 The Go Authors。保留所有权利。
// 本源代码的使用受 BSD 风格许可证约束，
// 该许可证可在 LICENSE 文件中找到。

//go:build 386 || amd64 || arm64 || loong64 || ppc64 || ppc64le || riscv64 || s390x || wasm

package math

const haveArchFloor = true

func archFloor(x float64) float64

const haveArchCeil = true

func archCeil(x float64) float64

const haveArchTrunc = true

func archTrunc(x float64) float64
