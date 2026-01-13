// 版权所有 2010 The Go Authors。保留所有权利。
// 本源代码的使用受 BSD 风格许可证约束，
// 该许可证可在 LICENSE 文件中找到。

package math

// Copysign 返回具有 f 的幅度
// 和 sign 的符号的值。
func Copysign(f, sign float64) float64 {
	const signBit = 1 << 63
	return Float64frombits(Float64bits(f)&^signBit | Float64bits(sign)&signBit)
}
