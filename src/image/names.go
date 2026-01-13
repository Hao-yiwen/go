// 版权所有 2010 The Go Authors。保留所有权利。
// 本源代码的使用受 BSD 风格许可证约束，
// 该许可证可在 LICENSE 文件中找到。

package image

import (
	"image/color"
)

var (
	// Black 是不透明的黑色均匀图像。
	Black = NewUniform(color.Black)
	// White 是不透明的白色均匀图像。
	White = NewUniform(color.White)
	// Transparent 是完全透明的均匀图像。
	Transparent = NewUniform(color.Transparent)
	// Opaque 是完全不透明的均匀图像。
	Opaque = NewUniform(color.Opaque)
)

// Uniform 是一个具有均匀颜色的无限大小的 [Image]。
// 它实现了 [color.Color]、[color.Model] 和 [Image] 接口。
type Uniform struct {
	C color.Color
}

func (c *Uniform) RGBA() (r, g, b, a uint32) {
	return c.C.RGBA()
}

func (c *Uniform) ColorModel() color.Model {
	return c
}

func (c *Uniform) Convert(color.Color) color.Color {
	return c.C
}

func (c *Uniform) Bounds() Rectangle { return Rectangle{Point{-1e9, -1e9}, Point{1e9, 1e9}} }

func (c *Uniform) At(x, y int) color.Color { return c.C }

func (c *Uniform) RGBA64At(x, y int) color.RGBA64 {
	r, g, b, a := c.C.RGBA()
	return color.RGBA64{uint16(r), uint16(g), uint16(b), uint16(a)}
}

// Opaque 扫描整个图像并报告它是否完全不透明。
func (c *Uniform) Opaque() bool {
	_, _, _, a := c.C.RGBA()
	return a == 0xffff
}

// NewUniform 返回具有给定颜色的新 [Uniform] 图像。
func NewUniform(c color.Color) *Uniform {
	return &Uniform{c}
}
