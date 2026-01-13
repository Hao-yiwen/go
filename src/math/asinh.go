// 版权所有 2010 The Go Authors。保留所有权利。
// 本源代码的使用受 BSD 风格许可证约束，
// 该许可证可在 LICENSE 文件中找到。

package math

// 原始 C 代码、长注释以及下面的常量
// 来自 FreeBSD 的 /usr/src/lib/msun/src/s_asinh.c
// 并附带以下声明。Go 代码是原始 C 代码的简化版本。
//
// ====================================================
// 版权所有 (C) 1993 Sun Microsystems, Inc. 保留所有权利。
//
// 由 SunPro（Sun Microsystems, Inc. 的一个业务部门）开发。
// 允许自由使用、复制、修改和分发本软件，
// 前提是保留此声明。
// ====================================================
//
//
// asinh(x)
// 方法：
//	基于
//	        asinh(x) = sign(x) * log [ |x| + sqrt(x*x+1) ]
//	我们有
//	asinh(x) := x  如果  1+x*x=1,
//	         := sign(x)*(log(x)+ln2) 对于大的 |x|，否则
//	         := sign(x)*log(2|x|+1/(|x|+sqrt(x*x+1))) 如果 |x|>2，否则
//	         := sign(x)*log1p(|x| + x**2/(1 + sqrt(1+x**2)))
//

// Asinh 返回 x 的反双曲正弦值。
//
// 特殊情况：
//
//	Asinh(±0) = ±0
//	Asinh(±Inf) = ±Inf
//	Asinh(NaN) = NaN
func Asinh(x float64) float64 {
	if haveArchAsinh {
		return archAsinh(x)
	}
	return asinh(x)
}

func asinh(x float64) float64 {
	const (
		Ln2      = 6.93147180559945286227e-01 // 0x3FE62E42FEFA39EF
		NearZero = 1.0 / (1 << 28)            // 2**-28
		Large    = 1 << 28                    // 2**28
	)
	// 特殊情况
	if IsNaN(x) || IsInf(x, 0) {
		return x
	}
	sign := false
	if x < 0 {
		x = -x
		sign = true
	}
	var temp float64
	switch {
	case x > Large:
		temp = Log(x) + Ln2 // |x| > 2**28
	case x > 2:
		temp = Log(2*x + 1/(Sqrt(x*x+1)+x)) // 2**28 > |x| > 2.0
	case x < NearZero:
		temp = x // |x| < 2**-28
	default:
		temp = Log1p(x + x*x/(1+Sqrt(1+x*x))) // 2.0 > |x| > 2**-28
	}
	if sign {
		temp = -temp
	}
	return temp
}
