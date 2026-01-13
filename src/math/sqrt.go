// 版权所有 2009 The Go Authors。保留所有权利。
// 本源代码的使用受 BSD 风格许可证约束，
// 该许可证可在 LICENSE 文件中找到。

package math

// 原始 C 代码和下面的长注释来自
// FreeBSD 的 /usr/src/lib/msun/src/e_sqrt.c，
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
// __ieee754_sqrt(x)
// 返回正确舍入的 sqrt。
//           -----------------------------------------
//           | 如果有硬件 sqrt，请使用硬件 sqrt       |
//           -----------------------------------------
// 方法：
//   使用整数运算的逐位方法。（慢，但可移植）
//   1. 规范化
//      将 x 缩放到 [1,4) 区间的 y，使用 2 的偶次幂：
//      找到一个整数 k 使得 1 <= (y=x*2**(2k)) < 4，则
//              sqrt(x) = 2**k * sqrt(y)
//   2. 逐位计算
//      令 q  = sqrt(y) 截断到二进制小数点后 i 位 (q = 1)，
//           i                                        0
//                                     i+1         2
//          s  = 2*q , 且        y  =  2   * ( y - q  )。         (1)
//           i      i            i                 i
//
//      要从 q  计算 q   ，需检查是否
//            i       i+1
//
//                            -(i+1) 2
//                      (q + 2      )  <= y。                    (2)
//                        i
//                                                            -(i+1)
//      如果 (2) 为假，则 q   = q ；否则 q   = q  + 2      。
//                         i+1   i         i+1   i
//
//      通过一些代数运算，不难看出
//      (2) 等价于
//                             -(i+1)
//                      s  +  2       <= y                       (3)
//                       i                i
//
//      (3) 的优点是 s  和 y  可以通过
//                    i      i
//      以下递推公式计算：
//          如果 (3) 为假
//
//          s     =  s  ,       y    = y   ;                     (4)
//           i+1      i          i+1    i
//
//      否则，
//                         -i                      -(i+1)
//          s     =  s  + 2  ,  y    = y  -  s  - 2              (5)
//           i+1      i          i+1    i     i
//
//      可以很容易地用归纳法证明 (4) 和 (5)。
//      注意：由于 (3) 的左侧只包含 i+2 位，
//            因此在 (3) 中不需要进行完整的（53 位）比较。
//   3. 最终舍入
//      生成 53 位结果后，我们再计算一位。
//      结合余数，我们可以确定结果是精确的、
//      大于 1/2ulp 还是小于 1/2ulp
//      （它永远不会等于 1/2ulp）。
//      可以通过检查对于某些浮点数 "huge" 和 "tiny"，
//      huge + tiny 是否等于 huge，以及 huge - tiny 是否
//      等于 huge 来检测舍入模式。
//
//
// 注意：舍入模式检测已省略。常量 "mask"、"shift"
// 和 "bias" 在 src/math/bits.go 中定义。

// Sqrt 返回 x 的平方根。
//
// 特殊情况：
//
//	Sqrt(+Inf) = +Inf
//	Sqrt(±0) = ±0
//	Sqrt(x < 0) = NaN
//	Sqrt(NaN) = NaN
func Sqrt(x float64) float64 {
	return sqrt(x)
}

// 注意：在 Sqrt 是单条指令的系统上，编译器可能会
// 将直接调用转换为直接使用该指令。

func sqrt(x float64) float64 {
	// 特殊情况
	switch {
	case x == 0 || IsNaN(x) || IsInf(x, 1):
		return x
	case x < 0:
		return NaN()
	}
	ix := Float64bits(x)
	// 规范化 x
	exp := int((ix >> shift) & mask)
	if exp == 0 { // 次正规数 x
		for ix&(1<<shift) == 0 {
			ix <<= 1
			exp--
		}
		exp++
	}
	exp -= bias // 去除指数偏置
	ix &^= mask << shift
	ix |= 1 << shift
	if exp&1 == 1 { // 奇数指数，将 x 加倍使其变为偶数
		ix <<= 1
	}
	exp >>= 1 // exp = exp/2，平方根的指数
	// 逐位生成 sqrt(x)
	ix <<= 1
	var q, s uint64               // q = sqrt(x)
	r := uint64(1 << (shift + 1)) // r = 从 MSB 到 LSB 移动的位
	for r != 0 {
		t := s + r
		if t <= ix {
			s = t + r
			ix -= t
			q += r
		}
		ix <<= 1
		r >>= 1
	}
	// 最终舍入
	if ix != 0 { // 有余数，结果不精确
		q += q & 1 // 根据额外位进行舍入
	}
	ix = q>>1 + uint64(exp-1+bias)<<shift // 有效数字 + 带偏置的指数
	return Float64frombits(ix)
}
