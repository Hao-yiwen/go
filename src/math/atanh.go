// 版权所有 2010 The Go Authors。保留所有权利。
// 本源代码的使用受 BSD 风格许可证约束，
// 该许可证可在 LICENSE 文件中找到。

package math

// 原始 C 代码、长注释以及下面的常量
// 来自 FreeBSD 的 /usr/src/lib/msun/src/e_atanh.c
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
// __ieee754_atanh(x)
// 方法：
//	1. 通过 atanh(-x) = -atanh(x) 将 x 归约为正
//	2. 对于 x>=0.5
//	            1              2x                          x
//	atanh(x) = --- * log(1 + -------) = 0.5 * log1p(2 * --------)
//	            2             1 - x                      1 - x
//
//	对于 x<0.5
//	atanh(x) = 0.5*log1p(2x+2x*x/(1-x))
//
// 特殊情况：
//	如果 |x| > 1，atanh(x) 是带信号的 NaN；
//	atanh(NaN) 是不带信号的 NaN；
//	atanh(+-1) 是带信号的 +-INF。
//

// Atanh 返回 x 的反双曲正切值。
//
// 特殊情况：
//
//	Atanh(1) = +Inf
//	Atanh(±0) = ±0
//	Atanh(-1) = -Inf
//	Atanh(x) = NaN 如果 x < -1 或 x > 1
//	Atanh(NaN) = NaN
func Atanh(x float64) float64 {
	if haveArchAtanh {
		return archAtanh(x)
	}
	return atanh(x)
}

func atanh(x float64) float64 {
	const NearZero = 1.0 / (1 << 28) // 2**-28
	// 特殊情况
	switch {
	case x < -1 || x > 1 || IsNaN(x):
		return NaN()
	case x == 1:
		return Inf(1)
	case x == -1:
		return Inf(-1)
	}
	sign := false
	if x < 0 {
		x = -x
		sign = true
	}
	var temp float64
	switch {
	case x < NearZero:
		temp = x
	case x < 0.5:
		temp = x + x
		temp = 0.5 * Log1p(temp+temp*x/(1-x))
	default:
		temp = 0.5 * Log1p((x+x)/(1-x))
	}
	if sign {
		temp = -temp
	}
	return temp
}
