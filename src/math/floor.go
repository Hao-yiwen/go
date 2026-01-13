// 版权所有 2009 The Go Authors。保留所有权利。
// 本源代码的使用受 BSD 风格许可证约束，
// 该许可证可在 LICENSE 文件中找到。

package math

// Floor 返回小于或等于 x 的最大整数值。
//
// 特殊情况：
//
//	Floor(±0) = ±0
//	Floor(±Inf) = ±Inf
//	Floor(NaN) = NaN
func Floor(x float64) float64 {
	if haveArchFloor {
		return archFloor(x)
	}
	return floor(x)
}

func floor(x float64) float64 {
	if x == 0 || IsNaN(x) || IsInf(x, 0) {
		return x
	}
	if x < 0 {
		d, fract := Modf(-x)
		if fract != 0.0 {
			d = d + 1
		}
		return -d
	}
	d, _ := Modf(x)
	return d
}

// Ceil 返回大于或等于 x 的最小整数值。
//
// 特殊情况：
//
//	Ceil(±0) = ±0
//	Ceil(±Inf) = ±Inf
//	Ceil(NaN) = NaN
func Ceil(x float64) float64 {
	if haveArchCeil {
		return archCeil(x)
	}
	return ceil(x)
}

func ceil(x float64) float64 {
	return -Floor(-x)
}

// Trunc 返回 x 的整数部分。
//
// 特殊情况：
//
//	Trunc(±0) = ±0
//	Trunc(±Inf) = ±Inf
//	Trunc(NaN) = NaN
func Trunc(x float64) float64 {
	if haveArchTrunc {
		return archTrunc(x)
	}
	return trunc(x)
}

func trunc(x float64) float64 {
	if Abs(x) < 1 {
		return Copysign(0, x)
	}

	b := Float64bits(x)
	e := uint(b>>shift)&mask - bias

	// 保留高 12+e 位（整数部分）；清除其余位。
	if e < 64-12 {
		b &^= 1<<(64-12-e) - 1
	}
	return Float64frombits(b)
}

// Round 返回最接近的整数，四舍五入时远离零。
//
// 特殊情况：
//
//	Round(±0) = ±0
//	Round(±Inf) = ±Inf
//	Round(NaN) = NaN
func Round(x float64) float64 {
	// Round 是以下实现的更快版本：
	//
	// func Round(x float64) float64 {
	//   t := Trunc(x)
	//   if Abs(x-t) >= 0.5 {
	//     return t + Copysign(1, x)
	//   }
	//   return t
	// }
	bits := Float64bits(x)
	e := uint(bits>>shift) & mask
	if e < bias {
		// 舍入 abs(x) < 1，包括次正规数。
		bits &= signMask // +-0
		if e == bias-1 {
			bits |= uvone // +-1
		}
	} else if e < bias+shift {
		// 舍入任何包含小数部分 [0,1) 的 abs(x) >= 1。
		//
		// 指数较大的数字保持不变，因为它们
		// 必须是整数、无穷大或 NaN。
		const half = 1 << (shift - 1)
		e -= bias
		bits += half >> e
		bits &^= fracMask >> e
	}
	return Float64frombits(bits)
}

// RoundToEven 返回最接近的整数，平局时舍入到偶数。
//
// 特殊情况：
//
//	RoundToEven(±0) = ±0
//	RoundToEven(±Inf) = ±Inf
//	RoundToEven(NaN) = NaN
func RoundToEven(x float64) float64 {
	// RoundToEven 是以下实现的更快版本：
	//
	// func RoundToEven(x float64) float64 {
	//   t := math.Trunc(x)
	//   odd := math.Remainder(t, 2) != 0
	//   if d := math.Abs(x - t); d > 0.5 || (d == 0.5 && odd) {
	//     return t + math.Copysign(1, x)
	//   }
	//   return t
	// }
	bits := Float64bits(x)
	e := uint(bits>>shift) & mask
	if e >= bias {
		// 舍入 abs(x) >= 1。
		// - 没有小数部分的大数、无穷大和 NaN 保持不变。
		// - 在截断前加上 0.499.. 或 0.5，取决于截断后的
		//   数字是偶数还是奇数（分别对应）。
		const halfMinusULP = (1 << (shift - 1)) - 1
		e -= bias
		bits += (halfMinusULP + (bits>>(shift-e))&1) >> e
		bits &^= fracMask >> e
	} else if e == bias-1 && bits&fracMask != 0 {
		// 舍入 0.5 < abs(x) < 1。
		bits = bits&signMask | uvone // +-1
	} else {
		// 舍入 abs(x) <= 0.5，包括次正规数。
		bits &= signMask // +-0
	}
	return Float64frombits(bits)
}
