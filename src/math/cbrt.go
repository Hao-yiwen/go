// 版权所有 2009 The Go Authors。保留所有权利。
// 本源代码的使用受 BSD 风格许可证约束，
// 该许可证可在 LICENSE 文件中找到。

package math

// Go 代码是来自 http://www.netlib.org/fdlibm/s_cbrt.c 的
// 原始 C 代码的修改版本，并附带以下声明。
//
// ====================================================
// 版权所有 (C) 1993 Sun Microsystems, Inc. 保留所有权利。
//
// 由 SunSoft（Sun Microsystems, Inc. 的一个业务部门）开发。
// 允许自由使用、复制、修改和分发本软件，
// 前提是保留此声明。
// ====================================================

// Cbrt 返回 x 的立方根。
//
// 特殊情况：
//
//	Cbrt(±0) = ±0
//	Cbrt(±Inf) = ±Inf
//	Cbrt(NaN) = NaN
func Cbrt(x float64) float64 {
	if haveArchCbrt {
		return archCbrt(x)
	}
	return cbrt(x)
}

func cbrt(x float64) float64 {
	const (
		B1             = 715094163                   // (682-0.03306235651)*2**20
		B2             = 696219795                   // (664-0.03306235651)*2**20
		C              = 5.42857142857142815906e-01  // 19/35     = 0x3FE15F15F15F15F1
		D              = -7.05306122448979611050e-01 // -864/1225 = 0xBFE691DE2532C834
		E              = 1.41428571428571436819e+00  // 99/70     = 0x3FF6A0EA0EA0EA0F
		F              = 1.60714285714285720630e+00  // 45/28     = 0x3FF9B6DB6DB6DB6E
		G              = 3.57142857142857150787e-01  // 5/14      = 0x3FD6DB6DB6DB6DB7
		SmallestNormal = 2.22507385850720138309e-308 // 2**-1022  = 0x0010000000000000
	)
	// 特殊情况
	switch {
	case x == 0 || IsNaN(x) || IsInf(x, 0):
		return x
	}

	sign := false
	if x < 0 {
		x = -x
		sign = true
	}

	// 粗略 cbrt 精确到 5 位
	t := Float64frombits(Float64bits(x)/3 + B1<<32)
	if x < SmallestNormal {
		// 次正规数
		t = float64(1 << 54) // 设置 t= 2**54
		t *= x
		t = Float64frombits(Float64bits(t)/3 + B2<<32)
	}

	// 新 cbrt 精确到 23 位
	r := t * t / x
	s := C + r*t
	t *= G + F/(s+E+D/s)

	// 截断到 22 位，使其大于 cbrt(x)
	t = Float64frombits(Float64bits(t)&(0xFFFFFFFFC<<28) + 1<<30)

	// 一步牛顿迭代到 53 位，误差小于 0.667ulps
	s = t * t // t*t 是精确的
	r = x / s
	w := t + t
	r = (r - t) / (w + r) // r-s 是精确的
	t = t + t*r

	// 恢复符号位
	if sign {
		t = -t
	}
	return t
}
