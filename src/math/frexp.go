// 版权所有 2009 The Go Authors。保留所有权利。
// 本源代码的使用受 BSD 风格许可证约束，
// 该许可证可在 LICENSE 文件中找到。

package math

// Frexp 将 f 分解为规范化分数和 2 的整数幂。
// 它返回满足 f == frac × 2**exp 的 frac 和 exp，
// 其中 frac 的绝对值在区间 [½, 1) 内。
//
// 特殊情况：
//
//	Frexp(±0) = ±0, 0
//	Frexp(±Inf) = ±Inf, 0
//	Frexp(NaN) = NaN, 0
func Frexp(f float64) (frac float64, exp int) {
	if haveArchFrexp {
		return archFrexp(f)
	}
	return frexp(f)
}

func frexp(f float64) (frac float64, exp int) {
	// 特殊情况
	switch {
	case f == 0:
		return f, 0 // 正确返回 -0
	case IsInf(f, 0) || IsNaN(f):
		return f, 0
	}
	f, exp = normalize(f)
	x := Float64bits(f)
	exp += int((x>>shift)&mask) - bias + 1
	x &^= mask << shift
	x |= (-1 + bias) << shift
	frac = Float64frombits(x)
	return
}
