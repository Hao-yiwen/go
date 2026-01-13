// 版权所有 2011 The Go Authors。保留所有权利。
// 本源代码的使用受 BSD 风格许可证约束，
// 该许可证可在 LICENSE 文件中找到。

package color

// RGBToYCbCr 将 RGB 三元组转换为 Y'CbCr 三元组。
func RGBToYCbCr(r, g, b uint8) (uint8, uint8, uint8) {
	// JFIF 规范说：
	//	Y' =  0.2990*R + 0.5870*G + 0.1140*B
	//	Cb = -0.1687*R - 0.3313*G + 0.5000*B + 128
	//	Cr =  0.5000*R - 0.4187*G - 0.0813*B + 128
	// https://www.w3.org/Graphics/JPEG/jfif3.pdf 说的是 Y 但实际指的是 Y'。

	r1 := int32(r)
	g1 := int32(g)
	b1 := int32(b)

	// yy 在 [0,0xff] 范围内。
	//
	// 注意 19595 + 38470 + 7471 等于 65536。
	yy := (19595*r1 + 38470*g1 + 7471*b1 + 1<<15) >> 16

	// 下面的位运算等同于
	//
	// cb := (-11056*r1 - 21712*g1 + 32768*b1 + 257<<15) >> 16
	// if cb < 0 {
	//     cb = 0
	// } else if cb > 0xff {
	//     cb = ^int32(0)
	// }
	//
	// 但使用更少的分支且更快。
	// 注意 return 语句中的 uint8 类型转换会将 ^int32(0) 转换为 0xff。
	// 下面计算 cr 的代码使用类似的模式。
	//
	// 注意 -11056 - 21712 + 32768 等于 0。
	cb := -11056*r1 - 21712*g1 + 32768*b1 + 257<<15
	if uint32(cb)&0xff000000 == 0 {
		cb >>= 16
	} else {
		cb = ^(cb >> 31)
	}

	// 注意 32768 - 27440 - 5328 等于 0。
	cr := 32768*r1 - 27440*g1 - 5328*b1 + 257<<15
	if uint32(cr)&0xff000000 == 0 {
		cr >>= 16
	} else {
		cr = ^(cr >> 31)
	}

	return uint8(yy), uint8(cb), uint8(cr)
}

// YCbCrToRGB 将 Y'CbCr 三元组转换为 RGB 三元组。
func YCbCrToRGB(y, cb, cr uint8) (uint8, uint8, uint8) {
	// JFIF 规范说：
	//	R = Y' + 1.40200*(Cr-128)
	//	G = Y' - 0.34414*(Cb-128) - 0.71414*(Cr-128)
	//	B = Y' + 1.77200*(Cb-128)
	// https://www.w3.org/Graphics/JPEG/jfif3.pdf 说的是 Y 但实际指的是 Y'。
	//
	// 这些公式使用非整数乘法因子。在计算时，整数数学通常比浮点数学更快。
	// 我们将所有这些因子乘以 1<<16 并四舍五入到最近的整数：
	//	 91881 = roundToNearestInteger(1.40200 * 65536)。
	//	 22554 = roundToNearestInteger(0.34414 * 65536)。
	//	 46802 = roundToNearestInteger(0.71414 * 65536)。
	//	116130 = roundToNearestInteger(1.77200 * 65536)。
	//
	// 添加 [0, 1<<16-1] 范围内的舍入调整，然后右移 16 位，
	// 得到原始公式的整数数学版本。
	//	R = (65536*Y' +  91881 *(Cr-128)                  + adjustment) >> 16
	//	G = (65536*Y' -  22554 *(Cb-128) - 46802*(Cr-128) + adjustment) >> 16
	//	B = (65536*Y' + 116130 *(Cb-128)                  + adjustment) >> 16
	// 1<<15 的常量舍入调整（1<<16 的一半）意味着除以 65536（右移 16）时四舍五入。
	// 类似地，0 的常量舍入调整意味着向下取整。
	//
	// 定义 YY1 = 65536*Y' + adjustment 简化了公式并需要更少的 CPU 操作：
	//	R = (YY1 +  91881 *(Cr-128)                 ) >> 16
	//	G = (YY1 -  22554 *(Cb-128) - 46802*(Cr-128)) >> 16
	//	B = (YY1 + 116130 *(Cb-128)                 ) >> 16
	//
	// 输入 (y, cb, cr) 是 8 位颜色，范围在 [0x00, 0xff]。在此函数中，
	// 输出也是 8 位颜色，但在下面相关的 YCbCr.RGBA 方法中，
	// 输出是 16 位颜色，范围在 [0x0000, 0xffff]。
	// 输出 16 位颜色只需将 "R = etc >> 16" 等式中的 16 改为 8，G 和 B 同理。
	//
	// 如上所述，1<<15 的常量舍入调整是自然的选择，但还有一个额外的约束：
	// 如果 c0 := YCbCr{Y: y, Cb: 0x80, Cr: 0x80} 且 c1 := Gray{Y: y}，
	// 则 c0.RGBA() 应该等于 c1.RGBA()。具体来说，如果 y == 0，
	// 则 "R = etc >> 8" 应该产生 0x0000，如果 y == 0xff，
	// 则 "R = etc >> 8" 应该产生 0xffff。如果我们使用 1<<15 的常量舍入调整，
	// 它将分别产生 0x0080 和 0xff80。
	//
	// 注意当 cb == 0x80 且 cr == 0x80 时，公式简化为：
	//	R = YY1 >> n
	//	G = YY1 >> n
	//	B = YY1 >> n
	// 其中 n 对于此函数（8 位颜色输出）为 16，对于 YCbCr.RGBA 方法
	//（16 位颜色输出）为 8。
	//
	// 解决方案是使舍入调整非常量，并等于 257*Y'，
	// 当 Y' 在 [0, 255] 范围内时，它在 [0, 1<<16-1] 范围内。
	// YY1 然后定义为：
	//	YY1 = 65536*Y' + 257*Y'
	// 或等价地：
	//	YY1 = Y' * 0x10101
	yy1 := int32(y) * 0x10101
	cb1 := int32(cb) - 128
	cr1 := int32(cr) - 128

	// 下面的位运算等同于
	//
	// r := (yy1 + 91881*cr1) >> 16
	// if r < 0 {
	//     r = 0
	// } else if r > 0xff {
	//     r = ^int32(0)
	// }
	//
	// 但使用更少的分支且更快。
	// 注意 return 语句中的 uint8 类型转换会将 ^int32(0) 转换为 0xff。
	// 下面计算 g 和 b 的代码使用类似的模式。
	r := yy1 + 91881*cr1
	if uint32(r)&0xff000000 == 0 {
		r >>= 16
	} else {
		r = ^(r >> 31)
	}

	g := yy1 - 22554*cb1 - 46802*cr1
	if uint32(g)&0xff000000 == 0 {
		g >>= 16
	} else {
		g = ^(g >> 31)
	}

	b := yy1 + 116130*cb1
	if uint32(b)&0xff000000 == 0 {
		b >>= 16
	} else {
		b = ^(b >> 31)
	}

	return uint8(r), uint8(g), uint8(b)
}

// YCbCr 表示完全不透明的 24 位 Y'CbCr 颜色，
// 一个亮度和两个色度分量各占 8 位。
//
// JPEG、VP8、MPEG 系列和其他编解码器使用此颜色模型。这些编解码器经常
// 交替使用 YUV 和 Y'CbCr 术语，但严格来说，YUV 术语仅适用于模拟视频信号，
// 而 Y'（亮度）是应用伽马校正后的 Y（光亮度）。
//
// RGB 和 Y'CbCr 之间的转换是有损的，并且有多种略有不同的公式用于两者之间的转换。
// 此包遵循 https://www.w3.org/Graphics/JPEG/jfif3.pdf 的 JFIF 规范。
type YCbCr struct {
	Y, Cb, Cr uint8
}

func (c YCbCr) RGBA() (uint32, uint32, uint32, uint32) {
	// 此代码是上面 YCbCrToRGB 函数的副本，只是它返回 [0, 0xffff] 范围内的值
	// 而不是 [0, 0xff]。这与让 YCbCr 通过先转换为 RGBA 来满足 Color 接口
	// 之间存在细微差别。后者通过来回转换每通道 8 位而丢失一些信息。
	//
	// 例如，此代码：
	//	const y, cb, cr = 0x7f, 0x7f, 0x7f
	//	r, g, b := color.YCbCrToRGB(y, cb, cr)
	//	r0, g0, b0, _ := color.YCbCr{y, cb, cr}.RGBA()
	//	r1, g1, b1, _ := color.RGBA{r, g, b, 0xff}.RGBA()
	//	fmt.Printf("0x%04x 0x%04x 0x%04x\n", r0, g0, b0)
	//	fmt.Printf("0x%04x 0x%04x 0x%04x\n", r1, g1, b1)
	// 打印：
	//	0x7e18 0x808d 0x7db9
	//	0x7e7e 0x8080 0x7d7d

	yy1 := int32(c.Y) * 0x10101
	cb1 := int32(c.Cb) - 128
	cr1 := int32(c.Cr) - 128

	// 下面的位运算等同于
	//
	// r := (yy1 + 91881*cr1) >> 8
	// if r < 0 {
	//     r = 0
	// } else if r > 0xff {
	//     r = 0xffff
	// }
	//
	// 但使用更少的分支且更快。
	// 下面计算 g 和 b 的代码使用类似的模式。
	r := yy1 + 91881*cr1
	if uint32(r)&0xff000000 == 0 {
		r >>= 8
	} else {
		r = ^(r >> 31) & 0xffff
	}

	g := yy1 - 22554*cb1 - 46802*cr1
	if uint32(g)&0xff000000 == 0 {
		g >>= 8
	} else {
		g = ^(g >> 31) & 0xffff
	}

	b := yy1 + 116130*cb1
	if uint32(b)&0xff000000 == 0 {
		b >>= 8
	} else {
		b = ^(b >> 31) & 0xffff
	}

	return uint32(r), uint32(g), uint32(b), 0xffff
}

// YCbCrModel 是 Y'CbCr 颜色的 [Model]。
var YCbCrModel Model = ModelFunc(yCbCrModel)

func yCbCrModel(c Color) Color {
	if _, ok := c.(YCbCr); ok {
		return c
	}
	r, g, b, _ := c.RGBA()
	y, u, v := RGBToYCbCr(uint8(r>>8), uint8(g>>8), uint8(b>>8))
	return YCbCr{y, u, v}
}

// NYCbCrA 表示非预乘 alpha 的 Y'CbCr-带-alpha 颜色，
// 一个亮度、两个色度和一个 alpha 分量各占 8 位。
type NYCbCrA struct {
	YCbCr
	A uint8
}

func (c NYCbCrA) RGBA() (uint32, uint32, uint32, uint32) {
	// 此方法的第一部分与 YCbCr.RGBA 相同。
	yy1 := int32(c.Y) * 0x10101
	cb1 := int32(c.Cb) - 128
	cr1 := int32(c.Cr) - 128

	// 下面的位运算等同于
	//
	// r := (yy1 + 91881*cr1) >> 8
	// if r < 0 {
	//     r = 0
	// } else if r > 0xff {
	//     r = 0xffff
	// }
	//
	// 但使用更少的分支且更快。
	// 下面计算 g 和 b 的代码使用类似的模式。
	r := yy1 + 91881*cr1
	if uint32(r)&0xff000000 == 0 {
		r >>= 8
	} else {
		r = ^(r >> 31) & 0xffff
	}

	g := yy1 - 22554*cb1 - 46802*cr1
	if uint32(g)&0xff000000 == 0 {
		g >>= 8
	} else {
		g = ^(g >> 31) & 0xffff
	}

	b := yy1 + 116130*cb1
	if uint32(b)&0xff000000 == 0 {
		b >>= 8
	} else {
		b = ^(b >> 31) & 0xffff
	}

	// 此方法的第二部分应用 alpha。
	a := uint32(c.A) * 0x101
	return uint32(r) * a / 0xffff, uint32(g) * a / 0xffff, uint32(b) * a / 0xffff, a
}

// NYCbCrAModel 是非预乘 alpha 的 Y'CbCr-带-alpha 颜色的 [Model]。
var NYCbCrAModel Model = ModelFunc(nYCbCrAModel)

func nYCbCrAModel(c Color) Color {
	switch c := c.(type) {
	case NYCbCrA:
		return c
	case YCbCr:
		return NYCbCrA{c, 0xff}
	}
	r, g, b, a := c.RGBA()

	// 从预乘 alpha 转换为非预乘 alpha。
	if a != 0 {
		r = (r * 0xffff) / a
		g = (g * 0xffff) / a
		b = (b * 0xffff) / a
	}

	y, u, v := RGBToYCbCr(uint8(r>>8), uint8(g>>8), uint8(b>>8))
	return NYCbCrA{YCbCr{Y: y, Cb: u, Cr: v}, uint8(a >> 8)}
}

// RGBToCMYK 将 RGB 三元组转换为 CMYK 四元组。
func RGBToCMYK(r, g, b uint8) (uint8, uint8, uint8, uint8) {
	rr := uint32(r)
	gg := uint32(g)
	bb := uint32(b)
	w := rr
	if w < gg {
		w = gg
	}
	if w < bb {
		w = bb
	}
	if w == 0 {
		return 0, 0, 0, 0xff
	}
	c := (w - rr) * 0xff / w
	m := (w - gg) * 0xff / w
	y := (w - bb) * 0xff / w
	return uint8(c), uint8(m), uint8(y), uint8(0xff - w)
}

// CMYKToRGB 将 [CMYK] 四元组转换为 RGB 三元组。
func CMYKToRGB(c, m, y, k uint8) (uint8, uint8, uint8) {
	w := 0xffff - uint32(k)*0x101
	r := (0xffff - uint32(c)*0x101) * w / 0xffff
	g := (0xffff - uint32(m)*0x101) * w / 0xffff
	b := (0xffff - uint32(y)*0x101) * w / 0xffff
	return uint8(r >> 8), uint8(g >> 8), uint8(b >> 8)
}

// CMYK 表示完全不透明的 CMYK 颜色，青色、品红色、黄色和黑色各占 8 位。
//
// 它不与任何特定的颜色配置文件关联。
type CMYK struct {
	C, M, Y, K uint8
}

func (c CMYK) RGBA() (uint32, uint32, uint32, uint32) {
	// 此代码是上面 CMYKToRGB 函数的副本，只是它返回 [0, 0xffff] 范围内的值
	// 而不是 [0, 0xff]。

	w := 0xffff - uint32(c.K)*0x101
	r := (0xffff - uint32(c.C)*0x101) * w / 0xffff
	g := (0xffff - uint32(c.M)*0x101) * w / 0xffff
	b := (0xffff - uint32(c.Y)*0x101) * w / 0xffff
	return r, g, b, 0xffff
}

// CMYKModel 是 CMYK 颜色的 [Model]。
var CMYKModel Model = ModelFunc(cmykModel)

func cmykModel(c Color) Color {
	if _, ok := c.(CMYK); ok {
		return c
	}
	r, g, b, _ := c.RGBA()
	cc, mm, yy, kk := RGBToCMYK(uint8(r>>8), uint8(g>>8), uint8(b>>8))
	return CMYK{cc, mm, yy, kk}
}
