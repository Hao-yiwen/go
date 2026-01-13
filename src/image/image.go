// 版权所有 2009 The Go Authors。保留所有权利。
// 本源代码的使用受 BSD 风格许可证约束，
// 该许可证可在 LICENSE 文件中找到。

// image 包实现了一个基本的二维图像库。
//
// 基础接口称为 [Image]。[Image] 包含颜色，颜色在 image/color 包中描述。
//
// [Image] 接口的值可以通过调用 [NewRGBA] 和 [NewPaletted] 等函数创建，
// 也可以通过对包含 GIF、JPEG 或 PNG 等格式图像数据的 [io.Reader] 调用 [Decode] 来创建。
// 解码任何特定的图像格式都需要预先注册一个解码器函数。
// 注册通常作为初始化该格式包的副作用自动完成，因此要解码 PNG 图像，
// 只需在程序的 main 包中添加
//
//	import _ "image/png"
//
// 即可。_ 表示导入一个包纯粹是为了其初始化副作用。
//
// 更多详情请参见 "The Go image package"：
// https://golang.org/doc/articles/image_package.html
//
// # 安全注意事项
//
// image 包可用于解析任意大小的图像，这可能会导致内存不足的机器资源耗尽。
// 在处理任意图像时，应在调用 [Decode] 之前先调用 [DecodeConfig]，
// 以便程序可以判断返回的头信息中定义的图像是否可以用可用资源安全解码。
// 调用 [Decode] 产生的超大图像（如 [DecodeConfig] 返回的头信息中所定义的），
// 无论图像本身是否格式错误，都不被视为安全问题。
// 如果调用 [DecodeConfig] 返回的头信息与 [Decode] 返回的图像不匹配，
// 这可能被视为安全问题，应按照 [Go 安全策略] 报告。
//
// [Go Security Policy]: https://go.dev/security/policy
package image

import (
	"image/color"
)

// Config 保存图像的颜色模型和尺寸。
type Config struct {
	ColorModel    color.Model
	Width, Height int
}

// Image 是一个有限的矩形网格，包含来自颜色模型的 [color.Color] 值。
type Image interface {
	// ColorModel 返回 Image 的颜色模型。
	ColorModel() color.Model
	// Bounds 返回 At 方法可以返回非零颜色的区域。
	// 边界不一定包含点 (0, 0)。
	Bounds() Rectangle
	// At 返回位于 (x, y) 处像素的颜色。
	// At(Bounds().Min.X, Bounds().Min.Y) 返回网格的左上角像素。
	// At(Bounds().Max.X-1, Bounds().Max.Y-1) 返回右下角像素。
	At(x, y int) color.Color
}

// RGBA64Image 是一个 [Image]，其像素可以直接转换为 color.RGBA64。
type RGBA64Image interface {
	// RGBA64At 返回位于 (x, y) 处像素的 RGBA64 颜色。它等同于调用
	// At(x, y).RGBA() 并将返回的 32 位值转换为 color.RGBA64，
	// 但它可以避免将具体颜色类型转换为 color.Color 接口类型时的内存分配。
	RGBA64At(x, y int) color.RGBA64
	Image
}

// PalettedImage 是一个颜色可能来自有限调色板的图像。
// 如果 m 是 PalettedImage 且 m.ColorModel() 返回 [color.Palette] p，
// 则 m.At(x, y) 应等同于 p[m.ColorIndexAt(x, y)]。如果 m 的
// 颜色模型不是 color.Palette，则 ColorIndexAt 的行为是未定义的。
type PalettedImage interface {
	// ColorIndexAt 返回位于 (x, y) 处像素的调色板索引。
	ColorIndexAt(x, y int) uint8
	Image
}

// pixelBufferLength 返回 NewXxx 函数中 []uint8 类型 Pix 切片字段的长度。
// 概念上，这只是 (bpp * width * height)，但如果其中任何一个为负数
// 或计算会导致 int 类型溢出，此函数会 panic。
//
// 由于向后兼容性原因，此函数会 panic 而不是返回错误。
// NewXxx 函数不返回错误。
func pixelBufferLength(bytesPerPixel int, r Rectangle, imageTypeName string) int {
	totalLength := mul3NonNeg(bytesPerPixel, r.Dx(), r.Dy())
	if totalLength < 0 {
		panic("image: New" + imageTypeName + " Rectangle has huge or negative dimensions")
	}
	return totalLength
}

// RGBA 是一个内存中的图像，其 At 方法返回 [color.RGBA] 值。
type RGBA struct {
	// Pix 保存图像的像素，按 R、G、B、A 顺序排列。位于
	// (x, y) 处的像素从 Pix[(y-Rect.Min.Y)*Stride + (x-Rect.Min.X)*4] 开始。
	Pix []uint8
	// Stride 是 Pix 中垂直相邻像素之间的步幅（以字节为单位）。
	Stride int
	// Rect 是图像的边界。
	Rect Rectangle
}

func (p *RGBA) ColorModel() color.Model { return color.RGBAModel }

func (p *RGBA) Bounds() Rectangle { return p.Rect }

func (p *RGBA) At(x, y int) color.Color {
	return p.RGBAAt(x, y)
}

func (p *RGBA) RGBA64At(x, y int) color.RGBA64 {
	if !(Point{x, y}.In(p.Rect)) {
		return color.RGBA64{}
	}
	i := p.PixOffset(x, y)
	s := p.Pix[i : i+4 : i+4] // 小容量可提高性能，参见 https://golang.org/issue/27857
	r := uint16(s[0])
	g := uint16(s[1])
	b := uint16(s[2])
	a := uint16(s[3])
	return color.RGBA64{
		(r << 8) | r,
		(g << 8) | g,
		(b << 8) | b,
		(a << 8) | a,
	}
}

func (p *RGBA) RGBAAt(x, y int) color.RGBA {
	if !(Point{x, y}.In(p.Rect)) {
		return color.RGBA{}
	}
	i := p.PixOffset(x, y)
	s := p.Pix[i : i+4 : i+4] // 小容量可提高性能，参见 https://golang.org/issue/27857
	return color.RGBA{s[0], s[1], s[2], s[3]}
}

// PixOffset 返回 Pix 中对应于 (x, y) 处像素的第一个元素的索引。
func (p *RGBA) PixOffset(x, y int) int {
	return (y-p.Rect.Min.Y)*p.Stride + (x-p.Rect.Min.X)*4
}

func (p *RGBA) Set(x, y int, c color.Color) {
	if !(Point{x, y}.In(p.Rect)) {
		return
	}
	i := p.PixOffset(x, y)
	c1 := color.RGBAModel.Convert(c).(color.RGBA)
	s := p.Pix[i : i+4 : i+4] // 小容量可提高性能，参见 https://golang.org/issue/27857
	s[0] = c1.R
	s[1] = c1.G
	s[2] = c1.B
	s[3] = c1.A
}

func (p *RGBA) SetRGBA64(x, y int, c color.RGBA64) {
	if !(Point{x, y}.In(p.Rect)) {
		return
	}
	i := p.PixOffset(x, y)
	s := p.Pix[i : i+4 : i+4] // 小容量可提高性能，参见 https://golang.org/issue/27857
	s[0] = uint8(c.R >> 8)
	s[1] = uint8(c.G >> 8)
	s[2] = uint8(c.B >> 8)
	s[3] = uint8(c.A >> 8)
}

func (p *RGBA) SetRGBA(x, y int, c color.RGBA) {
	if !(Point{x, y}.In(p.Rect)) {
		return
	}
	i := p.PixOffset(x, y)
	s := p.Pix[i : i+4 : i+4] // 小容量可提高性能，参见 https://golang.org/issue/27857
	s[0] = c.R
	s[1] = c.G
	s[2] = c.B
	s[3] = c.A
}

// SubImage 返回一个表示通过 r 可见的图像 p 部分的图像。
// 返回的值与原始图像共享像素。
func (p *RGBA) SubImage(r Rectangle) Image {
	r = r.Intersect(p.Rect)
	// 如果 r1 和 r2 是 Rectangle，当交集为空时，r1.Intersect(r2) 不保证
	// 在 r1 或 r2 内部。如果不显式检查这一点，下面的 Pix[i:] 表达式可能会 panic。
	if r.Empty() {
		return &RGBA{}
	}
	i := p.PixOffset(r.Min.X, r.Min.Y)
	return &RGBA{
		Pix:    p.Pix[i:],
		Stride: p.Stride,
		Rect:   r,
	}
}

// Opaque 扫描整个图像并报告它是否完全不透明。
func (p *RGBA) Opaque() bool {
	if p.Rect.Empty() {
		return true
	}
	i0, i1 := 3, p.Rect.Dx()*4
	for y := p.Rect.Min.Y; y < p.Rect.Max.Y; y++ {
		for i := i0; i < i1; i += 4 {
			if p.Pix[i] != 0xff {
				return false
			}
		}
		i0 += p.Stride
		i1 += p.Stride
	}
	return true
}

// NewRGBA 返回具有给定边界的新 [RGBA] 图像。
func NewRGBA(r Rectangle) *RGBA {
	return &RGBA{
		Pix:    make([]uint8, pixelBufferLength(4, r, "RGBA")),
		Stride: 4 * r.Dx(),
		Rect:   r,
	}
}

// RGBA64 是一个内存中的图像，其 At 方法返回 [color.RGBA64] 值。
type RGBA64 struct {
	// Pix 保存图像的像素，按 R、G、B、A 顺序和大端格式排列。位于
	// (x, y) 处的像素从 Pix[(y-Rect.Min.Y)*Stride + (x-Rect.Min.X)*8] 开始。
	Pix []uint8
	// Stride 是 Pix 中垂直相邻像素之间的步幅（以字节为单位）。
	Stride int
	// Rect 是图像的边界。
	Rect Rectangle
}

func (p *RGBA64) ColorModel() color.Model { return color.RGBA64Model }

func (p *RGBA64) Bounds() Rectangle { return p.Rect }

func (p *RGBA64) At(x, y int) color.Color {
	return p.RGBA64At(x, y)
}

func (p *RGBA64) RGBA64At(x, y int) color.RGBA64 {
	if !(Point{x, y}.In(p.Rect)) {
		return color.RGBA64{}
	}
	i := p.PixOffset(x, y)
	s := p.Pix[i : i+8 : i+8] // 小容量可提高性能，参见 https://golang.org/issue/27857
	return color.RGBA64{
		uint16(s[0])<<8 | uint16(s[1]),
		uint16(s[2])<<8 | uint16(s[3]),
		uint16(s[4])<<8 | uint16(s[5]),
		uint16(s[6])<<8 | uint16(s[7]),
	}
}

// PixOffset 返回 Pix 中对应于 (x, y) 处像素的第一个元素的索引。
func (p *RGBA64) PixOffset(x, y int) int {
	return (y-p.Rect.Min.Y)*p.Stride + (x-p.Rect.Min.X)*8
}

func (p *RGBA64) Set(x, y int, c color.Color) {
	if !(Point{x, y}.In(p.Rect)) {
		return
	}
	i := p.PixOffset(x, y)
	c1 := color.RGBA64Model.Convert(c).(color.RGBA64)
	s := p.Pix[i : i+8 : i+8] // 小容量可提高性能，参见 https://golang.org/issue/27857
	s[0] = uint8(c1.R >> 8)
	s[1] = uint8(c1.R)
	s[2] = uint8(c1.G >> 8)
	s[3] = uint8(c1.G)
	s[4] = uint8(c1.B >> 8)
	s[5] = uint8(c1.B)
	s[6] = uint8(c1.A >> 8)
	s[7] = uint8(c1.A)
}

func (p *RGBA64) SetRGBA64(x, y int, c color.RGBA64) {
	if !(Point{x, y}.In(p.Rect)) {
		return
	}
	i := p.PixOffset(x, y)
	s := p.Pix[i : i+8 : i+8] // 小容量可提高性能，参见 https://golang.org/issue/27857
	s[0] = uint8(c.R >> 8)
	s[1] = uint8(c.R)
	s[2] = uint8(c.G >> 8)
	s[3] = uint8(c.G)
	s[4] = uint8(c.B >> 8)
	s[5] = uint8(c.B)
	s[6] = uint8(c.A >> 8)
	s[7] = uint8(c.A)
}

// SubImage 返回一个表示通过 r 可见的图像 p 部分的图像。
// 返回的值与原始图像共享像素。
func (p *RGBA64) SubImage(r Rectangle) Image {
	r = r.Intersect(p.Rect)
	// 如果 r1 和 r2 是 Rectangle，当交集为空时，r1.Intersect(r2) 不保证
	// 在 r1 或 r2 内部。如果不显式检查这一点，下面的 Pix[i:] 表达式可能会 panic。
	if r.Empty() {
		return &RGBA64{}
	}
	i := p.PixOffset(r.Min.X, r.Min.Y)
	return &RGBA64{
		Pix:    p.Pix[i:],
		Stride: p.Stride,
		Rect:   r,
	}
}

// Opaque 扫描整个图像并报告它是否完全不透明。
func (p *RGBA64) Opaque() bool {
	if p.Rect.Empty() {
		return true
	}
	i0, i1 := 6, p.Rect.Dx()*8
	for y := p.Rect.Min.Y; y < p.Rect.Max.Y; y++ {
		for i := i0; i < i1; i += 8 {
			if p.Pix[i+0] != 0xff || p.Pix[i+1] != 0xff {
				return false
			}
		}
		i0 += p.Stride
		i1 += p.Stride
	}
	return true
}

// NewRGBA64 返回具有给定边界的新 [RGBA64] 图像。
func NewRGBA64(r Rectangle) *RGBA64 {
	return &RGBA64{
		Pix:    make([]uint8, pixelBufferLength(8, r, "RGBA64")),
		Stride: 8 * r.Dx(),
		Rect:   r,
	}
}

// NRGBA 是一个内存中的图像，其 At 方法返回 [color.NRGBA] 值。
type NRGBA struct {
	// Pix 保存图像的像素，按 R、G、B、A 顺序排列。位于
	// (x, y) 处的像素从 Pix[(y-Rect.Min.Y)*Stride + (x-Rect.Min.X)*4] 开始。
	Pix []uint8
	// Stride 是 Pix 中垂直相邻像素之间的步幅（以字节为单位）。
	Stride int
	// Rect 是图像的边界。
	Rect Rectangle
}

func (p *NRGBA) ColorModel() color.Model { return color.NRGBAModel }

func (p *NRGBA) Bounds() Rectangle { return p.Rect }

func (p *NRGBA) At(x, y int) color.Color {
	return p.NRGBAAt(x, y)
}

func (p *NRGBA) RGBA64At(x, y int) color.RGBA64 {
	r, g, b, a := p.NRGBAAt(x, y).RGBA()
	return color.RGBA64{uint16(r), uint16(g), uint16(b), uint16(a)}
}

func (p *NRGBA) NRGBAAt(x, y int) color.NRGBA {
	if !(Point{x, y}.In(p.Rect)) {
		return color.NRGBA{}
	}
	i := p.PixOffset(x, y)
	s := p.Pix[i : i+4 : i+4] // 小容量可提高性能，参见 https://golang.org/issue/27857
	return color.NRGBA{s[0], s[1], s[2], s[3]}
}

// PixOffset 返回 Pix 中对应于 (x, y) 处像素的第一个元素的索引。
func (p *NRGBA) PixOffset(x, y int) int {
	return (y-p.Rect.Min.Y)*p.Stride + (x-p.Rect.Min.X)*4
}

func (p *NRGBA) Set(x, y int, c color.Color) {
	if !(Point{x, y}.In(p.Rect)) {
		return
	}
	i := p.PixOffset(x, y)
	c1 := color.NRGBAModel.Convert(c).(color.NRGBA)
	s := p.Pix[i : i+4 : i+4] // 小容量可提高性能，参见 https://golang.org/issue/27857
	s[0] = c1.R
	s[1] = c1.G
	s[2] = c1.B
	s[3] = c1.A
}

func (p *NRGBA) SetRGBA64(x, y int, c color.RGBA64) {
	if !(Point{x, y}.In(p.Rect)) {
		return
	}
	r, g, b, a := uint32(c.R), uint32(c.G), uint32(c.B), uint32(c.A)
	if (a != 0) && (a != 0xffff) {
		r = (r * 0xffff) / a
		g = (g * 0xffff) / a
		b = (b * 0xffff) / a
	}
	i := p.PixOffset(x, y)
	s := p.Pix[i : i+4 : i+4] // 小容量可提高性能，参见 https://golang.org/issue/27857
	s[0] = uint8(r >> 8)
	s[1] = uint8(g >> 8)
	s[2] = uint8(b >> 8)
	s[3] = uint8(a >> 8)
}

func (p *NRGBA) SetNRGBA(x, y int, c color.NRGBA) {
	if !(Point{x, y}.In(p.Rect)) {
		return
	}
	i := p.PixOffset(x, y)
	s := p.Pix[i : i+4 : i+4] // 小容量可提高性能，参见 https://golang.org/issue/27857
	s[0] = c.R
	s[1] = c.G
	s[2] = c.B
	s[3] = c.A
}

// SubImage 返回一个表示通过 r 可见的图像 p 部分的图像。
// 返回的值与原始图像共享像素。
func (p *NRGBA) SubImage(r Rectangle) Image {
	r = r.Intersect(p.Rect)
	// 如果 r1 和 r2 是 Rectangle，当交集为空时，r1.Intersect(r2) 不保证
	// 在 r1 或 r2 内部。如果不显式检查这一点，下面的 Pix[i:] 表达式可能会 panic。
	if r.Empty() {
		return &NRGBA{}
	}
	i := p.PixOffset(r.Min.X, r.Min.Y)
	return &NRGBA{
		Pix:    p.Pix[i:],
		Stride: p.Stride,
		Rect:   r,
	}
}

// Opaque 扫描整个图像并报告它是否完全不透明。
func (p *NRGBA) Opaque() bool {
	if p.Rect.Empty() {
		return true
	}
	i0, i1 := 3, p.Rect.Dx()*4
	for y := p.Rect.Min.Y; y < p.Rect.Max.Y; y++ {
		for i := i0; i < i1; i += 4 {
			if p.Pix[i] != 0xff {
				return false
			}
		}
		i0 += p.Stride
		i1 += p.Stride
	}
	return true
}

// NewNRGBA 返回具有给定边界的新 [NRGBA] 图像。
func NewNRGBA(r Rectangle) *NRGBA {
	return &NRGBA{
		Pix:    make([]uint8, pixelBufferLength(4, r, "NRGBA")),
		Stride: 4 * r.Dx(),
		Rect:   r,
	}
}

// NRGBA64 是一个内存中的图像，其 At 方法返回 [color.NRGBA64] 值。
type NRGBA64 struct {
	// Pix 保存图像的像素，按 R、G、B、A 顺序和大端格式排列。位于
	// (x, y) 处的像素从 Pix[(y-Rect.Min.Y)*Stride + (x-Rect.Min.X)*8] 开始。
	Pix []uint8
	// Stride 是 Pix 中垂直相邻像素之间的步幅（以字节为单位）。
	Stride int
	// Rect 是图像的边界。
	Rect Rectangle
}

func (p *NRGBA64) ColorModel() color.Model { return color.NRGBA64Model }

func (p *NRGBA64) Bounds() Rectangle { return p.Rect }

func (p *NRGBA64) At(x, y int) color.Color {
	return p.NRGBA64At(x, y)
}

func (p *NRGBA64) RGBA64At(x, y int) color.RGBA64 {
	r, g, b, a := p.NRGBA64At(x, y).RGBA()
	return color.RGBA64{uint16(r), uint16(g), uint16(b), uint16(a)}
}

func (p *NRGBA64) NRGBA64At(x, y int) color.NRGBA64 {
	if !(Point{x, y}.In(p.Rect)) {
		return color.NRGBA64{}
	}
	i := p.PixOffset(x, y)
	s := p.Pix[i : i+8 : i+8] // 小容量可提高性能，参见 https://golang.org/issue/27857
	return color.NRGBA64{
		uint16(s[0])<<8 | uint16(s[1]),
		uint16(s[2])<<8 | uint16(s[3]),
		uint16(s[4])<<8 | uint16(s[5]),
		uint16(s[6])<<8 | uint16(s[7]),
	}
}

// PixOffset 返回 Pix 中对应于 (x, y) 处像素的第一个元素的索引。
func (p *NRGBA64) PixOffset(x, y int) int {
	return (y-p.Rect.Min.Y)*p.Stride + (x-p.Rect.Min.X)*8
}

func (p *NRGBA64) Set(x, y int, c color.Color) {
	if !(Point{x, y}.In(p.Rect)) {
		return
	}
	i := p.PixOffset(x, y)
	c1 := color.NRGBA64Model.Convert(c).(color.NRGBA64)
	s := p.Pix[i : i+8 : i+8] // 小容量可提高性能，参见 https://golang.org/issue/27857
	s[0] = uint8(c1.R >> 8)
	s[1] = uint8(c1.R)
	s[2] = uint8(c1.G >> 8)
	s[3] = uint8(c1.G)
	s[4] = uint8(c1.B >> 8)
	s[5] = uint8(c1.B)
	s[6] = uint8(c1.A >> 8)
	s[7] = uint8(c1.A)
}

func (p *NRGBA64) SetRGBA64(x, y int, c color.RGBA64) {
	if !(Point{x, y}.In(p.Rect)) {
		return
	}
	r, g, b, a := uint32(c.R), uint32(c.G), uint32(c.B), uint32(c.A)
	if (a != 0) && (a != 0xffff) {
		r = (r * 0xffff) / a
		g = (g * 0xffff) / a
		b = (b * 0xffff) / a
	}
	i := p.PixOffset(x, y)
	s := p.Pix[i : i+8 : i+8] // 小容量可提高性能，参见 https://golang.org/issue/27857
	s[0] = uint8(r >> 8)
	s[1] = uint8(r)
	s[2] = uint8(g >> 8)
	s[3] = uint8(g)
	s[4] = uint8(b >> 8)
	s[5] = uint8(b)
	s[6] = uint8(a >> 8)
	s[7] = uint8(a)
}

func (p *NRGBA64) SetNRGBA64(x, y int, c color.NRGBA64) {
	if !(Point{x, y}.In(p.Rect)) {
		return
	}
	i := p.PixOffset(x, y)
	s := p.Pix[i : i+8 : i+8] // 小容量可提高性能，参见 https://golang.org/issue/27857
	s[0] = uint8(c.R >> 8)
	s[1] = uint8(c.R)
	s[2] = uint8(c.G >> 8)
	s[3] = uint8(c.G)
	s[4] = uint8(c.B >> 8)
	s[5] = uint8(c.B)
	s[6] = uint8(c.A >> 8)
	s[7] = uint8(c.A)
}

// SubImage 返回一个表示通过 r 可见的图像 p 部分的图像。
// 返回的值与原始图像共享像素。
func (p *NRGBA64) SubImage(r Rectangle) Image {
	r = r.Intersect(p.Rect)
	// 如果 r1 和 r2 是 Rectangle，当交集为空时，r1.Intersect(r2) 不保证
	// 在 r1 或 r2 内部。如果不显式检查这一点，下面的 Pix[i:] 表达式可能会 panic。
	if r.Empty() {
		return &NRGBA64{}
	}
	i := p.PixOffset(r.Min.X, r.Min.Y)
	return &NRGBA64{
		Pix:    p.Pix[i:],
		Stride: p.Stride,
		Rect:   r,
	}
}

// Opaque 扫描整个图像并报告它是否完全不透明。
func (p *NRGBA64) Opaque() bool {
	if p.Rect.Empty() {
		return true
	}
	i0, i1 := 6, p.Rect.Dx()*8
	for y := p.Rect.Min.Y; y < p.Rect.Max.Y; y++ {
		for i := i0; i < i1; i += 8 {
			if p.Pix[i+0] != 0xff || p.Pix[i+1] != 0xff {
				return false
			}
		}
		i0 += p.Stride
		i1 += p.Stride
	}
	return true
}

// NewNRGBA64 返回具有给定边界的新 [NRGBA64] 图像。
func NewNRGBA64(r Rectangle) *NRGBA64 {
	return &NRGBA64{
		Pix:    make([]uint8, pixelBufferLength(8, r, "NRGBA64")),
		Stride: 8 * r.Dx(),
		Rect:   r,
	}
}

// Alpha 是一个内存中的图像，其 At 方法返回 [color.Alpha] 值。
type Alpha struct {
	// Pix 保存图像的像素，作为 alpha 值。位于
	// (x, y) 处的像素从 Pix[(y-Rect.Min.Y)*Stride + (x-Rect.Min.X)*1] 开始。
	Pix []uint8
	// Stride 是 Pix 中垂直相邻像素之间的步幅（以字节为单位）。
	Stride int
	// Rect 是图像的边界。
	Rect Rectangle
}

func (p *Alpha) ColorModel() color.Model { return color.AlphaModel }

func (p *Alpha) Bounds() Rectangle { return p.Rect }

func (p *Alpha) At(x, y int) color.Color {
	return p.AlphaAt(x, y)
}

func (p *Alpha) RGBA64At(x, y int) color.RGBA64 {
	a := uint16(p.AlphaAt(x, y).A)
	a |= a << 8
	return color.RGBA64{a, a, a, a}
}

func (p *Alpha) AlphaAt(x, y int) color.Alpha {
	if !(Point{x, y}.In(p.Rect)) {
		return color.Alpha{}
	}
	i := p.PixOffset(x, y)
	return color.Alpha{p.Pix[i]}
}

// PixOffset 返回 Pix 中对应于 (x, y) 处像素的第一个元素的索引。
func (p *Alpha) PixOffset(x, y int) int {
	return (y-p.Rect.Min.Y)*p.Stride + (x-p.Rect.Min.X)*1
}

func (p *Alpha) Set(x, y int, c color.Color) {
	if !(Point{x, y}.In(p.Rect)) {
		return
	}
	i := p.PixOffset(x, y)
	p.Pix[i] = color.AlphaModel.Convert(c).(color.Alpha).A
}

func (p *Alpha) SetRGBA64(x, y int, c color.RGBA64) {
	if !(Point{x, y}.In(p.Rect)) {
		return
	}
	i := p.PixOffset(x, y)
	p.Pix[i] = uint8(c.A >> 8)
}

func (p *Alpha) SetAlpha(x, y int, c color.Alpha) {
	if !(Point{x, y}.In(p.Rect)) {
		return
	}
	i := p.PixOffset(x, y)
	p.Pix[i] = c.A
}

// SubImage 返回一个表示通过 r 可见的图像 p 部分的图像。
// 返回的值与原始图像共享像素。
func (p *Alpha) SubImage(r Rectangle) Image {
	r = r.Intersect(p.Rect)
	// 如果 r1 和 r2 是 Rectangle，当交集为空时，r1.Intersect(r2) 不保证
	// 在 r1 或 r2 内部。如果不显式检查这一点，下面的 Pix[i:] 表达式可能会 panic。
	if r.Empty() {
		return &Alpha{}
	}
	i := p.PixOffset(r.Min.X, r.Min.Y)
	return &Alpha{
		Pix:    p.Pix[i:],
		Stride: p.Stride,
		Rect:   r,
	}
}

// Opaque 扫描整个图像并报告它是否完全不透明。
func (p *Alpha) Opaque() bool {
	if p.Rect.Empty() {
		return true
	}
	i0, i1 := 0, p.Rect.Dx()
	for y := p.Rect.Min.Y; y < p.Rect.Max.Y; y++ {
		for i := i0; i < i1; i++ {
			if p.Pix[i] != 0xff {
				return false
			}
		}
		i0 += p.Stride
		i1 += p.Stride
	}
	return true
}

// NewAlpha 返回具有给定边界的新 [Alpha] 图像。
func NewAlpha(r Rectangle) *Alpha {
	return &Alpha{
		Pix:    make([]uint8, pixelBufferLength(1, r, "Alpha")),
		Stride: 1 * r.Dx(),
		Rect:   r,
	}
}

// Alpha16 是一个内存中的图像，其 At 方法返回 [color.Alpha16] 值。
type Alpha16 struct {
	// Pix 保存图像的像素，以大端格式存储 alpha 值。位于
	// (x, y) 处的像素从 Pix[(y-Rect.Min.Y)*Stride + (x-Rect.Min.X)*2] 开始。
	Pix []uint8
	// Stride 是 Pix 中垂直相邻像素之间的步幅（以字节为单位）。
	Stride int
	// Rect 是图像的边界。
	Rect Rectangle
}

func (p *Alpha16) ColorModel() color.Model { return color.Alpha16Model }

func (p *Alpha16) Bounds() Rectangle { return p.Rect }

func (p *Alpha16) At(x, y int) color.Color {
	return p.Alpha16At(x, y)
}

func (p *Alpha16) RGBA64At(x, y int) color.RGBA64 {
	a := p.Alpha16At(x, y).A
	return color.RGBA64{a, a, a, a}
}

func (p *Alpha16) Alpha16At(x, y int) color.Alpha16 {
	if !(Point{x, y}.In(p.Rect)) {
		return color.Alpha16{}
	}
	i := p.PixOffset(x, y)
	return color.Alpha16{uint16(p.Pix[i+0])<<8 | uint16(p.Pix[i+1])}
}

// PixOffset 返回 Pix 中对应于 (x, y) 处像素的第一个元素的索引。
func (p *Alpha16) PixOffset(x, y int) int {
	return (y-p.Rect.Min.Y)*p.Stride + (x-p.Rect.Min.X)*2
}

func (p *Alpha16) Set(x, y int, c color.Color) {
	if !(Point{x, y}.In(p.Rect)) {
		return
	}
	i := p.PixOffset(x, y)
	c1 := color.Alpha16Model.Convert(c).(color.Alpha16)
	p.Pix[i+0] = uint8(c1.A >> 8)
	p.Pix[i+1] = uint8(c1.A)
}

func (p *Alpha16) SetRGBA64(x, y int, c color.RGBA64) {
	if !(Point{x, y}.In(p.Rect)) {
		return
	}
	i := p.PixOffset(x, y)
	p.Pix[i+0] = uint8(c.A >> 8)
	p.Pix[i+1] = uint8(c.A)
}

func (p *Alpha16) SetAlpha16(x, y int, c color.Alpha16) {
	if !(Point{x, y}.In(p.Rect)) {
		return
	}
	i := p.PixOffset(x, y)
	p.Pix[i+0] = uint8(c.A >> 8)
	p.Pix[i+1] = uint8(c.A)
}

// SubImage 返回一个表示通过 r 可见的图像 p 部分的图像。
// 返回的值与原始图像共享像素。
func (p *Alpha16) SubImage(r Rectangle) Image {
	r = r.Intersect(p.Rect)
	// 如果 r1 和 r2 是 Rectangle，当交集为空时，r1.Intersect(r2) 不保证
	// 在 r1 或 r2 内部。如果不显式检查这一点，下面的 Pix[i:] 表达式可能会 panic。
	if r.Empty() {
		return &Alpha16{}
	}
	i := p.PixOffset(r.Min.X, r.Min.Y)
	return &Alpha16{
		Pix:    p.Pix[i:],
		Stride: p.Stride,
		Rect:   r,
	}
}

// Opaque 扫描整个图像并报告它是否完全不透明。
func (p *Alpha16) Opaque() bool {
	if p.Rect.Empty() {
		return true
	}
	i0, i1 := 0, p.Rect.Dx()*2
	for y := p.Rect.Min.Y; y < p.Rect.Max.Y; y++ {
		for i := i0; i < i1; i += 2 {
			if p.Pix[i+0] != 0xff || p.Pix[i+1] != 0xff {
				return false
			}
		}
		i0 += p.Stride
		i1 += p.Stride
	}
	return true
}

// NewAlpha16 返回具有给定边界的新 [Alpha16] 图像。
func NewAlpha16(r Rectangle) *Alpha16 {
	return &Alpha16{
		Pix:    make([]uint8, pixelBufferLength(2, r, "Alpha16")),
		Stride: 2 * r.Dx(),
		Rect:   r,
	}
}

// Gray 是一个内存中的图像，其 At 方法返回 [color.Gray] 值。
type Gray struct {
	// Pix 保存图像的像素，作为灰度值。位于
	// (x, y) 处的像素从 Pix[(y-Rect.Min.Y)*Stride + (x-Rect.Min.X)*1] 开始。
	Pix []uint8
	// Stride 是 Pix 中垂直相邻像素之间的步幅（以字节为单位）。
	Stride int
	// Rect 是图像的边界。
	Rect Rectangle
}

func (p *Gray) ColorModel() color.Model { return color.GrayModel }

func (p *Gray) Bounds() Rectangle { return p.Rect }

func (p *Gray) At(x, y int) color.Color {
	return p.GrayAt(x, y)
}

func (p *Gray) RGBA64At(x, y int) color.RGBA64 {
	gray := uint16(p.GrayAt(x, y).Y)
	gray |= gray << 8
	return color.RGBA64{gray, gray, gray, 0xffff}
}

func (p *Gray) GrayAt(x, y int) color.Gray {
	if !(Point{x, y}.In(p.Rect)) {
		return color.Gray{}
	}
	i := p.PixOffset(x, y)
	return color.Gray{p.Pix[i]}
}

// PixOffset 返回 Pix 中对应于 (x, y) 处像素的第一个元素的索引。
func (p *Gray) PixOffset(x, y int) int {
	return (y-p.Rect.Min.Y)*p.Stride + (x-p.Rect.Min.X)*1
}

func (p *Gray) Set(x, y int, c color.Color) {
	if !(Point{x, y}.In(p.Rect)) {
		return
	}
	i := p.PixOffset(x, y)
	p.Pix[i] = color.GrayModel.Convert(c).(color.Gray).Y
}

func (p *Gray) SetRGBA64(x, y int, c color.RGBA64) {
	if !(Point{x, y}.In(p.Rect)) {
		return
	}
	// 此公式与 color.grayModel 中的相同。
	gray := (19595*uint32(c.R) + 38470*uint32(c.G) + 7471*uint32(c.B) + 1<<15) >> 24
	i := p.PixOffset(x, y)
	p.Pix[i] = uint8(gray)
}

func (p *Gray) SetGray(x, y int, c color.Gray) {
	if !(Point{x, y}.In(p.Rect)) {
		return
	}
	i := p.PixOffset(x, y)
	p.Pix[i] = c.Y
}

// SubImage 返回一个表示通过 r 可见的图像 p 部分的图像。
// 返回的值与原始图像共享像素。
func (p *Gray) SubImage(r Rectangle) Image {
	r = r.Intersect(p.Rect)
	// 如果 r1 和 r2 是 Rectangle，当交集为空时，r1.Intersect(r2) 不保证
	// 在 r1 或 r2 内部。如果不显式检查这一点，下面的 Pix[i:] 表达式可能会 panic。
	if r.Empty() {
		return &Gray{}
	}
	i := p.PixOffset(r.Min.X, r.Min.Y)
	return &Gray{
		Pix:    p.Pix[i:],
		Stride: p.Stride,
		Rect:   r,
	}
}

// Opaque 扫描整个图像并报告它是否完全不透明。
func (p *Gray) Opaque() bool {
	return true
}

// NewGray 返回具有给定边界的新 [Gray] 图像。
func NewGray(r Rectangle) *Gray {
	return &Gray{
		Pix:    make([]uint8, pixelBufferLength(1, r, "Gray")),
		Stride: 1 * r.Dx(),
		Rect:   r,
	}
}

// Gray16 是一个内存中的图像，其 At 方法返回 [color.Gray16] 值。
type Gray16 struct {
	// Pix 保存图像的像素，以大端格式存储灰度值。位于
	// (x, y) 处的像素从 Pix[(y-Rect.Min.Y)*Stride + (x-Rect.Min.X)*2] 开始。
	Pix []uint8
	// Stride 是 Pix 中垂直相邻像素之间的步幅（以字节为单位）。
	Stride int
	// Rect 是图像的边界。
	Rect Rectangle
}

func (p *Gray16) ColorModel() color.Model { return color.Gray16Model }

func (p *Gray16) Bounds() Rectangle { return p.Rect }

func (p *Gray16) At(x, y int) color.Color {
	return p.Gray16At(x, y)
}

func (p *Gray16) RGBA64At(x, y int) color.RGBA64 {
	gray := p.Gray16At(x, y).Y
	return color.RGBA64{gray, gray, gray, 0xffff}
}

func (p *Gray16) Gray16At(x, y int) color.Gray16 {
	if !(Point{x, y}.In(p.Rect)) {
		return color.Gray16{}
	}
	i := p.PixOffset(x, y)
	return color.Gray16{uint16(p.Pix[i+0])<<8 | uint16(p.Pix[i+1])}
}

// PixOffset 返回 Pix 中对应于 (x, y) 处像素的第一个元素的索引。
func (p *Gray16) PixOffset(x, y int) int {
	return (y-p.Rect.Min.Y)*p.Stride + (x-p.Rect.Min.X)*2
}

func (p *Gray16) Set(x, y int, c color.Color) {
	if !(Point{x, y}.In(p.Rect)) {
		return
	}
	i := p.PixOffset(x, y)
	c1 := color.Gray16Model.Convert(c).(color.Gray16)
	p.Pix[i+0] = uint8(c1.Y >> 8)
	p.Pix[i+1] = uint8(c1.Y)
}

func (p *Gray16) SetRGBA64(x, y int, c color.RGBA64) {
	if !(Point{x, y}.In(p.Rect)) {
		return
	}
	// 此公式与 color.gray16Model 中的相同。
	gray := (19595*uint32(c.R) + 38470*uint32(c.G) + 7471*uint32(c.B) + 1<<15) >> 16
	i := p.PixOffset(x, y)
	p.Pix[i+0] = uint8(gray >> 8)
	p.Pix[i+1] = uint8(gray)
}

func (p *Gray16) SetGray16(x, y int, c color.Gray16) {
	if !(Point{x, y}.In(p.Rect)) {
		return
	}
	i := p.PixOffset(x, y)
	p.Pix[i+0] = uint8(c.Y >> 8)
	p.Pix[i+1] = uint8(c.Y)
}

// SubImage 返回一个表示通过 r 可见的图像 p 部分的图像。
// 返回的值与原始图像共享像素。
func (p *Gray16) SubImage(r Rectangle) Image {
	r = r.Intersect(p.Rect)
	// 如果 r1 和 r2 是 Rectangle，当交集为空时，r1.Intersect(r2) 不保证
	// 在 r1 或 r2 内部。如果不显式检查这一点，下面的 Pix[i:] 表达式可能会 panic。
	if r.Empty() {
		return &Gray16{}
	}
	i := p.PixOffset(r.Min.X, r.Min.Y)
	return &Gray16{
		Pix:    p.Pix[i:],
		Stride: p.Stride,
		Rect:   r,
	}
}

// Opaque 扫描整个图像并报告它是否完全不透明。
func (p *Gray16) Opaque() bool {
	return true
}

// NewGray16 返回具有给定边界的新 [Gray16] 图像。
func NewGray16(r Rectangle) *Gray16 {
	return &Gray16{
		Pix:    make([]uint8, pixelBufferLength(2, r, "Gray16")),
		Stride: 2 * r.Dx(),
		Rect:   r,
	}
}

// CMYK 是一个内存中的图像，其 At 方法返回 [color.CMYK] 值。
type CMYK struct {
	// Pix 保存图像的像素，按 C、M、Y、K 顺序排列。位于
	// (x, y) 处的像素从 Pix[(y-Rect.Min.Y)*Stride + (x-Rect.Min.X)*4] 开始。
	Pix []uint8
	// Stride 是 Pix 中垂直相邻像素之间的步幅（以字节为单位）。
	Stride int
	// Rect 是图像的边界。
	Rect Rectangle
}

func (p *CMYK) ColorModel() color.Model { return color.CMYKModel }

func (p *CMYK) Bounds() Rectangle { return p.Rect }

func (p *CMYK) At(x, y int) color.Color {
	return p.CMYKAt(x, y)
}

func (p *CMYK) RGBA64At(x, y int) color.RGBA64 {
	r, g, b, a := p.CMYKAt(x, y).RGBA()
	return color.RGBA64{uint16(r), uint16(g), uint16(b), uint16(a)}
}

func (p *CMYK) CMYKAt(x, y int) color.CMYK {
	if !(Point{x, y}.In(p.Rect)) {
		return color.CMYK{}
	}
	i := p.PixOffset(x, y)
	s := p.Pix[i : i+4 : i+4] // 小容量可提高性能，参见 https://golang.org/issue/27857
	return color.CMYK{s[0], s[1], s[2], s[3]}
}

// PixOffset 返回 Pix 中对应于 (x, y) 处像素的第一个元素的索引。
func (p *CMYK) PixOffset(x, y int) int {
	return (y-p.Rect.Min.Y)*p.Stride + (x-p.Rect.Min.X)*4
}

func (p *CMYK) Set(x, y int, c color.Color) {
	if !(Point{x, y}.In(p.Rect)) {
		return
	}
	i := p.PixOffset(x, y)
	c1 := color.CMYKModel.Convert(c).(color.CMYK)
	s := p.Pix[i : i+4 : i+4] // 小容量可提高性能，参见 https://golang.org/issue/27857
	s[0] = c1.C
	s[1] = c1.M
	s[2] = c1.Y
	s[3] = c1.K
}

func (p *CMYK) SetRGBA64(x, y int, c color.RGBA64) {
	if !(Point{x, y}.In(p.Rect)) {
		return
	}
	cc, mm, yy, kk := color.RGBToCMYK(uint8(c.R>>8), uint8(c.G>>8), uint8(c.B>>8))
	i := p.PixOffset(x, y)
	s := p.Pix[i : i+4 : i+4] // 小容量可提高性能，参见 https://golang.org/issue/27857
	s[0] = cc
	s[1] = mm
	s[2] = yy
	s[3] = kk
}

func (p *CMYK) SetCMYK(x, y int, c color.CMYK) {
	if !(Point{x, y}.In(p.Rect)) {
		return
	}
	i := p.PixOffset(x, y)
	s := p.Pix[i : i+4 : i+4] // 小容量可提高性能，参见 https://golang.org/issue/27857
	s[0] = c.C
	s[1] = c.M
	s[2] = c.Y
	s[3] = c.K
}

// SubImage 返回一个表示通过 r 可见的图像 p 部分的图像。
// 返回的值与原始图像共享像素。
func (p *CMYK) SubImage(r Rectangle) Image {
	r = r.Intersect(p.Rect)
	// 如果 r1 和 r2 是 Rectangle，当交集为空时，r1.Intersect(r2) 不保证
	// 在 r1 或 r2 内部。如果不显式检查这一点，下面的 Pix[i:] 表达式可能会 panic。
	if r.Empty() {
		return &CMYK{}
	}
	i := p.PixOffset(r.Min.X, r.Min.Y)
	return &CMYK{
		Pix:    p.Pix[i:],
		Stride: p.Stride,
		Rect:   r,
	}
}

// Opaque 扫描整个图像并报告它是否完全不透明。
func (p *CMYK) Opaque() bool {
	return true
}

// NewCMYK 返回具有给定边界的新 CMYK 图像。
func NewCMYK(r Rectangle) *CMYK {
	return &CMYK{
		Pix:    make([]uint8, pixelBufferLength(4, r, "CMYK")),
		Stride: 4 * r.Dx(),
		Rect:   r,
	}
}

// Paletted 是一个内存中的图像，使用 uint8 索引指向给定的调色板。
type Paletted struct {
	// Pix 保存图像的像素，作为调色板索引。位于
	// (x, y) 处的像素从 Pix[(y-Rect.Min.Y)*Stride + (x-Rect.Min.X)*1] 开始。
	Pix []uint8
	// Stride 是 Pix 中垂直相邻像素之间的步幅（以字节为单位）。
	Stride int
	// Rect 是图像的边界。
	Rect Rectangle
	// Palette 是图像的调色板。
	Palette color.Palette
}

func (p *Paletted) ColorModel() color.Model { return p.Palette }

func (p *Paletted) Bounds() Rectangle { return p.Rect }

func (p *Paletted) At(x, y int) color.Color {
	if len(p.Palette) == 0 {
		return nil
	}
	if !(Point{x, y}.In(p.Rect)) {
		return p.Palette[0]
	}
	i := p.PixOffset(x, y)
	return p.Palette[p.Pix[i]]
}

func (p *Paletted) RGBA64At(x, y int) color.RGBA64 {
	if len(p.Palette) == 0 {
		return color.RGBA64{}
	}
	c := color.Color(nil)
	if !(Point{x, y}.In(p.Rect)) {
		c = p.Palette[0]
	} else {
		i := p.PixOffset(x, y)
		c = p.Palette[p.Pix[i]]
	}
	r, g, b, a := c.RGBA()
	return color.RGBA64{
		uint16(r),
		uint16(g),
		uint16(b),
		uint16(a),
	}
}

// PixOffset 返回 Pix 中对应于 (x, y) 处像素的第一个元素的索引。
func (p *Paletted) PixOffset(x, y int) int {
	return (y-p.Rect.Min.Y)*p.Stride + (x-p.Rect.Min.X)*1
}

func (p *Paletted) Set(x, y int, c color.Color) {
	if !(Point{x, y}.In(p.Rect)) {
		return
	}
	i := p.PixOffset(x, y)
	p.Pix[i] = uint8(p.Palette.Index(c))
}

func (p *Paletted) SetRGBA64(x, y int, c color.RGBA64) {
	if !(Point{x, y}.In(p.Rect)) {
		return
	}
	i := p.PixOffset(x, y)
	p.Pix[i] = uint8(p.Palette.Index(c))
}

func (p *Paletted) ColorIndexAt(x, y int) uint8 {
	if !(Point{x, y}.In(p.Rect)) {
		return 0
	}
	i := p.PixOffset(x, y)
	return p.Pix[i]
}

func (p *Paletted) SetColorIndex(x, y int, index uint8) {
	if !(Point{x, y}.In(p.Rect)) {
		return
	}
	i := p.PixOffset(x, y)
	p.Pix[i] = index
}

// SubImage 返回一个表示通过 r 可见的图像 p 部分的图像。
// 返回的值与原始图像共享像素。
func (p *Paletted) SubImage(r Rectangle) Image {
	r = r.Intersect(p.Rect)
	// 如果 r1 和 r2 是 Rectangle，当交集为空时，r1.Intersect(r2) 不保证
	// 在 r1 或 r2 内部。如果不显式检查这一点，下面的 Pix[i:] 表达式可能会 panic。
	if r.Empty() {
		return &Paletted{
			Palette: p.Palette,
		}
	}
	i := p.PixOffset(r.Min.X, r.Min.Y)
	return &Paletted{
		Pix:     p.Pix[i:],
		Stride:  p.Stride,
		Rect:    p.Rect.Intersect(r),
		Palette: p.Palette,
	}
}

// Opaque 扫描整个图像并报告它是否完全不透明。
func (p *Paletted) Opaque() bool {
	var present [256]bool
	i0, i1 := 0, p.Rect.Dx()
	for y := p.Rect.Min.Y; y < p.Rect.Max.Y; y++ {
		for _, c := range p.Pix[i0:i1] {
			present[c] = true
		}
		i0 += p.Stride
		i1 += p.Stride
	}
	for i, c := range p.Palette {
		if !present[i] {
			continue
		}
		_, _, _, a := c.RGBA()
		if a != 0xffff {
			return false
		}
	}
	return true
}

// NewPaletted 返回具有给定宽度、高度和调色板的新 [Paletted] 图像。
func NewPaletted(r Rectangle, p color.Palette) *Paletted {
	return &Paletted{
		Pix:     make([]uint8, pixelBufferLength(1, r, "Paletted")),
		Stride:  1 * r.Dx(),
		Rect:    r,
		Palette: p,
	}
}
