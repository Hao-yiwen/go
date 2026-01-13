// 版权所有 2019 The Go Authors。保留所有权利。
// 本源代码的使用受 BSD 风格许可证约束，
// 该许可证可在 LICENSE 文件中找到。

package math

import "math/bits"

func zero(x uint64) uint64 {
	if x == 0 {
		return 1
	}
	return 0
	// 无分支版本：
	// return ((x>>1 | x&1) - 1) >> 63
}

func nonzero(x uint64) uint64 {
	if x != 0 {
		return 1
	}
	return 0
	// 无分支版本：
	// return 1 - ((x>>1|x&1)-1)>>63
}

func shl(u1, u2 uint64, n uint) (r1, r2 uint64) {
	r1 = u1<<n | u2>>(64-n) | u2<<(n-64)
	r2 = u2 << n
	return
}

func shr(u1, u2 uint64, n uint) (r1, r2 uint64) {
	r2 = u2>>n | u1<<(64-n) | u1>>(n-64)
	r1 = u1 >> n
	return
}

// shrcompress 将双字值的低 n+1 位压缩为单个位。
// 结果等于值右移 n 位，但结果的第 0 位
// 设置为低 n+1 位的按位或。
func shrcompress(u1, u2 uint64, n uint) (r1, r2 uint64) {
	// TODO: 这里的性能对这些分支的顺序/位置非常敏感。
	// n == 0 足够常见，应该在快速路径中。
	// 也许需要进行更多测量来找到最佳顺序/位置？
	switch {
	case n == 0:
		return u1, u2
	case n == 64:
		return 0, u1 | nonzero(u2)
	case n >= 128:
		return 0, nonzero(u1 | u2)
	case n < 64:
		r1, r2 = shr(u1, u2, n)
		r2 |= nonzero(u2 & (1<<n - 1))
	case n < 128:
		r1, r2 = shr(u1, u2, n)
		r2 |= nonzero(u1&(1<<(n-64)-1) | u2)
	}
	return
}

func lz(u1, u2 uint64) (l int32) {
	l = int32(bits.LeadingZeros64(u1))
	if l == 64 {
		l += int32(bits.LeadingZeros64(u2))
	}
	return l
}

// split 将 b 分解为符号、带偏置的指数和尾数。
// 它为正规值向尾数添加隐含的 1 位，
// 并规范化次正规值。
func split(b uint64) (sign uint32, exp int32, mantissa uint64) {
	sign = uint32(b >> 63)
	exp = int32(b>>52) & mask
	mantissa = b & fracMask

	if exp == 0 {
		// 如果是次正规数，则规范化值。
		shift := uint(bits.LeadingZeros64(mantissa) - 11)
		mantissa <<= shift
		exp = 1 - int32(shift)
	} else {
		// 添加隐含的 1 位
		mantissa |= 1 << 52
	}
	return
}

// FMA 返回 x * y + z，仅进行一次舍入计算。
// （即 FMA 返回 x、y 和 z 的融合乘加结果。）
func FMA(x, y, z float64) float64 {
	bx, by, bz := Float64bits(x), Float64bits(y), Float64bits(z)

	// 涉及 Inf 或 NaN 或零。最多发生一次舍入。
	if x == 0.0 || y == 0.0 || bx&uvinf == uvinf || by&uvinf == uvinf {
		return x*y + z
	}
	// 单独处理 z == 0.0。
	// 加零通常不会改变原始值。
	// 但是，负零是个例外。（例如 (-0) + (+0) = (+0)）
	// 这适用于 x * y 为负且下溢的情况。
	if z == 0.0 {
		return x * y
	}
	// 单独处理非有限的 z。计算 x*y+z 时，
	// 如果 x 和 y 是有限的，但 z 是无穷大，结果应始终为 z。
	if bz&uvinf == uvinf {
		return z
	}

	// 输入是（次）正规数。
	// 将 x、y、z 分解为符号、指数、尾数。
	xs, xe, xm := split(bx)
	ys, ye, ym := split(by)
	zs, ze, zm := split(bz)

	// 计算乘积 p = x*y，表示为符号、指数、双字尾数。
	// 从指数开始。"是正规数" 位尚未减去。
	pe := xe + ye - bias + 1

	// pm1:pm2 是乘积 p 的双字尾数。
	// 左移以在乘积中保留最高位。实际上
	// 将 106 位乘积左移 21 位。
	pm1, pm2 := bits.Mul64(xm<<10, ym<<11)
	zm1, zm2 := zm<<10, uint64(0)
	ps := xs ^ ys // 乘积符号

	// 规范化到第 62 位
	is62zero := uint((^pm1 >> 62) & 1)
	pm1, pm2 = shl(pm1, pm2, is62zero)
	pe -= int32(is62zero)

	// 交换加法操作数使 |p| >= |z|
	if pe < ze || pe == ze && pm1 < zm1 {
		ps, pe, pm1, pm2, zs, ze, zm1, zm2 = zs, ze, zm1, zm2, ps, pe, pm1, pm2
	}

	// 特殊情况：如果 p == -z，由于两个操作数都不为零，结果始终为 +0。
	if ps != zs && pe == ze && pm1 == zm1 && pm2 == zm2 {
		return 0
	}

	// 对齐有效数字
	zm1, zm2 = shrcompress(zm1, zm2, uint(pe-ze))

	// 计算结果有效数字，必要时规范化。
	var m, c uint64
	if ps == zs {
		// 加法 (pm1:pm2) + (zm1:zm2)
		pm2, c = bits.Add64(pm2, zm2, 0)
		pm1, _ = bits.Add64(pm1, zm1, c)
		pe -= int32(^pm1 >> 63)
		pm1, m = shrcompress(pm1, pm2, uint(64+pm1>>63))
	} else {
		// 减法 (pm1:pm2) - (zm1:zm2)
		// TODO: 我们应该特殊处理抵消情况吗？
		pm2, c = bits.Sub64(pm2, zm2, 0)
		pm1, _ = bits.Sub64(pm1, zm1, c)
		nz := lz(pm1, pm2)
		pe -= nz
		m, pm2 = shl(pm1, pm2, uint(nz-1))
		m |= nonzero(pm2)
	}

	// 舍入并将平局舍入到偶数
	if pe > 1022+bias || pe == 1022+bias && (m+1<<9)>>63 == 1 {
		// 舍入后的值溢出指数范围
		return Float64frombits(uint64(ps)<<63 | uvinf)
	}
	if pe < 0 {
		n := uint(-pe)
		m = m>>n | nonzero(m&(1<<n-1))
		pe = 0
	}
	m = ((m + 1<<9) >> 10) & ^zero((m&(1<<10-1))^1<<9)
	pe &= -int32(nonzero(m))
	return Float64frombits(uint64(ps)<<63 + uint64(pe)<<52 + m)
}
