// 版权所有 2011 The Go Authors。保留所有权利。
// 本源代码的使用受 BSD 风格许可证约束，
// 该许可证可在 LICENSE 文件中找到。

// color 包实现了一个基本的颜色库。
package color

// Color 可以将自身转换为每通道 16 位的预乘 alpha RGBA。
// 转换可能是有损的。
type Color interface {
	// RGBA 返回颜色的预乘 alpha 红色、绿色、蓝色和 alpha 值。
	// 每个值的范围在 [0, 0xffff] 内，但用 uint32 表示，
	// 这样乘以高达 0xffff 的混合因子时不会溢出。
	//
	// 预乘 alpha 的颜色分量 c 已按 alpha (a) 缩放，
	// 因此有效值为 0 <= c <= a。
	RGBA() (r, g, b, a uint32)
}

// RGBA 表示传统的 32 位预乘 alpha 颜色，红色、绿色、蓝色和 alpha 各占 8 位。
//
// 预乘 alpha 的颜色分量 C 已按 alpha (A) 缩放，因此有效值为 0 <= C <= A。
type RGBA struct {
	R, G, B, A uint8
}

func (c RGBA) RGBA() (r, g, b, a uint32) {
	r = uint32(c.R)
	r |= r << 8
	g = uint32(c.G)
	g |= g << 8
	b = uint32(c.B)
	b |= b << 8
	a = uint32(c.A)
	a |= a << 8
	return
}

// RGBA64 表示 64 位预乘 alpha 颜色，红色、绿色、蓝色和 alpha 各占 16 位。
//
// 预乘 alpha 的颜色分量 C 已按 alpha (A) 缩放，因此有效值为 0 <= C <= A。
type RGBA64 struct {
	R, G, B, A uint16
}

func (c RGBA64) RGBA() (r, g, b, a uint32) {
	return uint32(c.R), uint32(c.G), uint32(c.B), uint32(c.A)
}

// NRGBA 表示非预乘 alpha 的 32 位颜色。
type NRGBA struct {
	R, G, B, A uint8
}

func (c NRGBA) RGBA() (r, g, b, a uint32) {
	r = uint32(c.R)
	r |= r << 8
	r *= uint32(c.A)
	r /= 0xff
	g = uint32(c.G)
	g |= g << 8
	g *= uint32(c.A)
	g /= 0xff
	b = uint32(c.B)
	b |= b << 8
	b *= uint32(c.A)
	b /= 0xff
	a = uint32(c.A)
	a |= a << 8
	return
}

// NRGBA64 表示非预乘 alpha 的 64 位颜色，
// 红色、绿色、蓝色和 alpha 各占 16 位。
type NRGBA64 struct {
	R, G, B, A uint16
}

func (c NRGBA64) RGBA() (r, g, b, a uint32) {
	r = uint32(c.R)
	r *= uint32(c.A)
	r /= 0xffff
	g = uint32(c.G)
	g *= uint32(c.A)
	g /= 0xffff
	b = uint32(c.B)
	b *= uint32(c.A)
	b /= 0xffff
	a = uint32(c.A)
	return
}

// Alpha 表示 8 位 alpha 颜色。
type Alpha struct {
	A uint8
}

func (c Alpha) RGBA() (r, g, b, a uint32) {
	a = uint32(c.A)
	a |= a << 8
	return a, a, a, a
}

// Alpha16 表示 16 位 alpha 颜色。
type Alpha16 struct {
	A uint16
}

func (c Alpha16) RGBA() (r, g, b, a uint32) {
	a = uint32(c.A)
	return a, a, a, a
}

// Gray 表示 8 位灰度颜色。
type Gray struct {
	Y uint8
}

func (c Gray) RGBA() (r, g, b, a uint32) {
	y := uint32(c.Y)
	y |= y << 8
	return y, y, y, 0xffff
}

// Gray16 表示 16 位灰度颜色。
type Gray16 struct {
	Y uint16
}

func (c Gray16) RGBA() (r, g, b, a uint32) {
	y := uint32(c.Y)
	return y, y, y, 0xffff
}

// Model 可以将任何 [Color] 转换为其自身颜色模型中的一个。转换可能是有损的。
type Model interface {
	Convert(c Color) Color
}

// ModelFunc 返回一个调用 f 来实现转换的 [Model]。
func ModelFunc(f func(Color) Color) Model {
	// 注意：使用 *modelFunc 作为实现意味着调用者仍然可以使用
	// 类似 m == RGBAModel 的比较。如果我们直接使用 func 值，
	// 这是不可能的，因为函数不再可比较。
	return &modelFunc{f}
}

type modelFunc struct {
	f func(Color) Color
}

func (m *modelFunc) Convert(c Color) Color {
	return m.f(c)
}

// 标准颜色类型的模型。
var (
	RGBAModel    Model = ModelFunc(rgbaModel)
	RGBA64Model  Model = ModelFunc(rgba64Model)
	NRGBAModel   Model = ModelFunc(nrgbaModel)
	NRGBA64Model Model = ModelFunc(nrgba64Model)
	AlphaModel   Model = ModelFunc(alphaModel)
	Alpha16Model Model = ModelFunc(alpha16Model)
	GrayModel    Model = ModelFunc(grayModel)
	Gray16Model  Model = ModelFunc(gray16Model)
)

func rgbaModel(c Color) Color {
	if _, ok := c.(RGBA); ok {
		return c
	}
	r, g, b, a := c.RGBA()
	return RGBA{uint8(r >> 8), uint8(g >> 8), uint8(b >> 8), uint8(a >> 8)}
}

func rgba64Model(c Color) Color {
	if _, ok := c.(RGBA64); ok {
		return c
	}
	r, g, b, a := c.RGBA()
	return RGBA64{uint16(r), uint16(g), uint16(b), uint16(a)}
}

func nrgbaModel(c Color) Color {
	if _, ok := c.(NRGBA); ok {
		return c
	}
	r, g, b, a := c.RGBA()
	if a == 0xffff {
		return NRGBA{uint8(r >> 8), uint8(g >> 8), uint8(b >> 8), 0xff}
	}
	if a == 0 {
		return NRGBA{0, 0, 0, 0}
	}
	// 由于 Color.RGBA 返回预乘 alpha 的颜色，我们应该有 r <= a && g <= a && b <= a。
	r = (r * 0xffff) / a
	g = (g * 0xffff) / a
	b = (b * 0xffff) / a
	return NRGBA{uint8(r >> 8), uint8(g >> 8), uint8(b >> 8), uint8(a >> 8)}
}

func nrgba64Model(c Color) Color {
	if _, ok := c.(NRGBA64); ok {
		return c
	}
	r, g, b, a := c.RGBA()
	if a == 0xffff {
		return NRGBA64{uint16(r), uint16(g), uint16(b), 0xffff}
	}
	if a == 0 {
		return NRGBA64{0, 0, 0, 0}
	}
	// 由于 Color.RGBA 返回预乘 alpha 的颜色，我们应该有 r <= a && g <= a && b <= a。
	r = (r * 0xffff) / a
	g = (g * 0xffff) / a
	b = (b * 0xffff) / a
	return NRGBA64{uint16(r), uint16(g), uint16(b), uint16(a)}
}

func alphaModel(c Color) Color {
	if _, ok := c.(Alpha); ok {
		return c
	}
	_, _, _, a := c.RGBA()
	return Alpha{uint8(a >> 8)}
}

func alpha16Model(c Color) Color {
	if _, ok := c.(Alpha16); ok {
		return c
	}
	_, _, _, a := c.RGBA()
	return Alpha16{uint16(a)}
}

func grayModel(c Color) Color {
	if _, ok := c.(Gray); ok {
		return c
	}
	r, g, b, _ := c.RGBA()

	// 这些系数（分数 0.299、0.587 和 0.114）与 JFIF 规范给出的
	// 以及 ycbcr.go 中 func RGBToYCbCr 使用的相同。
	//
	// 注意 19595 + 38470 + 7471 等于 65536。
	//
	// 24 是 16 + 8。16 与 RGBToYCbCr 中使用的相同。8 是因为
	// 返回值是 8 位颜色，而不是 16 位颜色。
	y := (19595*r + 38470*g + 7471*b + 1<<15) >> 24

	return Gray{uint8(y)}
}

func gray16Model(c Color) Color {
	if _, ok := c.(Gray16); ok {
		return c
	}
	r, g, b, _ := c.RGBA()

	// 这些系数（分数 0.299、0.587 和 0.114）与 JFIF 规范给出的
	// 以及 ycbcr.go 中 func RGBToYCbCr 使用的相同。
	//
	// 注意 19595 + 38470 + 7471 等于 65536。
	y := (19595*r + 38470*g + 7471*b + 1<<15) >> 16

	return Gray16{uint16(y)}
}

// Palette 是颜色的调色板。
type Palette []Color

// Convert 返回在欧几里得 R,G,B 空间中最接近 c 的调色板颜色。
func (p Palette) Convert(c Color) Color {
	if len(p) == 0 {
		return nil
	}
	return p[p.Index(c)]
}

// Index 返回在欧几里得 R,G,B,A 空间中最接近 c 的调色板颜色的索引。
func (p Palette) Index(c Color) int {
	// 此计算的批处理版本在 image/draw/draw.go 中。

	cr, cg, cb, ca := c.RGBA()
	ret, bestSum := 0, uint32(1<<32-1)
	for i, v := range p {
		vr, vg, vb, va := v.RGBA()
		sum := sqDiff(cr, vr) + sqDiff(cg, vg) + sqDiff(cb, vb) + sqDiff(ca, va)
		if sum < bestSum {
			if sum == 0 {
				return i
			}
			ret, bestSum = i, sum
		}
	}
	return ret
}

// sqDiff 返回 x 和 y 的差的平方，右移 2 位，使得
// 四个这样的值相加不会溢出 uint32。
//
// x 和 y 都假定在 [0, 0xffff] 范围内。
func sqDiff(x, y uint32) uint32 {
	// 此函数的规范代码如下：
	//
	//	var d uint32
	//	if x > y {
	//		d = x - y
	//	} else {
	//		d = y - x
	//	}
	//	return (d * d) >> 2
	//
	// 语言规范保证无符号整数值操作在溢出/回绕方面具有以下属性：
	//
	// > 对于无符号整数值，操作 +、-、* 和 << 以 2^n 为模计算，
	// > 其中 n 是无符号整数类型的位宽。简单地说，这些无符号整数操作
	// > 在溢出时丢弃高位，程序可以依赖"回绕"行为。
	//
	// 考虑到这些属性以及此函数在热路径（x,y 循环）中被调用的事实，
	// 它被简化为下面的代码，这稍微快一些。请参阅 TestSqDiff 进行正确性检查。
	d := x - y
	return (d * d) >> 2
}

// 标准颜色。
var (
	Black       = Gray16{0}
	White       = Gray16{0xffff}
	Transparent = Alpha16{0}
	Opaque      = Alpha16{0xffff}
)
