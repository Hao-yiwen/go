// 版权所有 2021 The Go Authors。保留所有权利。
// 本源代码的使用受 BSD 风格许可证约束，
// 该许可证可在 LICENSE 文件中找到。

//go:build ppc64le || ppc64

package reflect

func archFloat32FromReg(reg uint64) float32
func archFloat32ToReg(val float32) uint64
