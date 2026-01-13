// 版权所有 2009 The Go Authors。保留所有权利。
// 本源代码的使用受 BSD 风格许可证约束，
// 该许可证可在 LICENSE 文件中找到。

package math

/*
	浮点数反正切。
*/

// 原始 C 代码、长注释以及下面的常量来自
// http://netlib.sandia.gov/cephes/cmath/atan.c，可从
// http://www.netlib.org/cephes/cmath.tgz 获取。
// Go 代码是原始 C 代码的一个版本。
//
// atan.c
// 反圆正切（反正切）
//
// 概要：
// double x, y, atan();
// y = atan( x );
//
// 描述：
// 返回正切值为 x 的弧度角，范围在 -pi/2 和 +pi/2 之间。
//
// 范围归约是从三个区间归约到从零到 0.66 的区间。
// 近似使用 4/5 次有理函数，形式为
// x + x**3 P(x)/Q(x)。
//
// 精度：
//                      相对误差：
// 算术运算   定义域    测试次数  峰值     均方根
//    DEC       -10, 10   50000     2.4e-17  8.3e-18
//    IEEE      -10, 10   10^6      1.8e-16  5.0e-17
//
// Cephes 数学库版本 2.8：2000 年 6 月
// 版权所有 1984, 1987, 1989, 1992, 2000 Stephen L. Moshier
//
// http://netlib.sandia.gov/cephes/ 的 readme 文件说：
//    本存档中的某些软件可能来自书籍《数学函数的方法和
// 程序》（Prentice-Hall 或 Simon & Schuster
// International，1989）或来自 Cephes 数学库，
// 一个商业产品。无论哪种情况，它都受作者版权保护。
// 您在这里看到的内容可以自由使用，但不提供支持或
// 保证。
//
//   书中两个已知的印刷错误已在此处的
// gamma 函数和不完全 beta 积分的
// 源代码清单中修复。
//
//   Stephen L. Moshier
//   moshier@na-net.ornl.gov

// xatan 计算在 [0, 0.66] 范围内有效的级数。
func xatan(x float64) float64 {
	const (
		P0 = -8.750608600031904122785e-01
		P1 = -1.615753718733365076637e+01
		P2 = -7.500855792314704667340e+01
		P3 = -1.228866684490136173410e+02
		P4 = -6.485021904942025371773e+01
		Q0 = +2.485846490142306297962e+01
		Q1 = +1.650270098316988542046e+02
		Q2 = +4.328810604912902668951e+02
		Q3 = +4.853903996359136964868e+02
		Q4 = +1.945506571482613964425e+02
	)
	z := x * x
	z = z * ((((P0*z+P1)*z+P2)*z+P3)*z + P4) / (((((z+Q0)*z+Q1)*z+Q2)*z+Q3)*z + Q4)
	z = x*z + x
	return z
}

// satan 将其参数（已知为正）归约到
// [0, 0.66] 范围并调用 xatan。
func satan(x float64) float64 {
	const (
		Morebits = 6.123233995736765886130e-17 // pi/2 = PIO2 + Morebits
		Tan3pio8 = 2.41421356237309504880      // tan(3*pi/8)
	)
	if x <= 0.66 {
		return xatan(x)
	}
	if x > Tan3pio8 {
		return Pi/2 - xatan(1/x) + Morebits
	}
	return Pi/4 + xatan((x-1)/(x+1)) + 0.5*Morebits
}

// Atan 返回 x 的反正切值，以弧度表示。
//
// 特殊情况：
//
//	Atan(±0) = ±0
//	Atan(±Inf) = ±Pi/2
func Atan(x float64) float64 {
	if haveArchAtan {
		return archAtan(x)
	}
	return atan(x)
}

func atan(x float64) float64 {
	if x == 0 {
		return x
	}
	if x > 0 {
		return satan(x)
	}
	return -satan(-x)
}
