// 版权所有 2010 The Go Authors。保留所有权利。
// 本源代码的使用受 BSD 风格许可证约束，
// 该许可证可在 LICENSE 文件中找到。

package math

// 原始 C 代码、长注释以及下面的常量
// 来自 FreeBSD 的 /usr/src/lib/msun/src/e_acosh.c
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
// __ieee754_acosh(x)
// 方法：
//	基于
//	        acosh(x) = log [ x + sqrt(x*x-1) ]
//	我们有
//	        acosh(x) := log(x)+ln2,	如果 x 很大；否则
//	        acosh(x) := log(2x-1/(sqrt(x*x-1)+x)) 如果 x>2；否则
//	        acosh(x) := log1p(t+sqrt(2.0*t+t*t)); 其中 t=x-1。
//
// 特殊情况：
//	如果 x<1，acosh(x) 是带信号的 NaN。
//	acosh(NaN) 是不带信号的 NaN。
//

// Acosh 返回 x 的反双曲余弦值。
//
// 特殊情况：
//
//	Acosh(+Inf) = +Inf
//	Acosh(x) = NaN 如果 x < 1
//	Acosh(NaN) = NaN
func Acosh(x float64) float64 {
	if haveArchAcosh {
		return archAcosh(x)
	}
	return acosh(x)
}

func acosh(x float64) float64 {
	const Large = 1 << 28 // 2**28
	// 第一个 case 是特殊情况
	switch {
	case x < 1 || IsNaN(x):
		return NaN()
	case x == 1:
		return 0
	case x >= Large:
		return Log(x) + Ln2 // x > 2**28
	case x > 2:
		return Log(2*x - 1/(x+Sqrt(x*x-1))) // 2**28 > x > 2
	}
	t := x - 1
	return Log1p(t + Sqrt(2*t+t*t)) // 2 >= x > 1
}
