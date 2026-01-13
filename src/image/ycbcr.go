// 版权所有 2011 The Go Authors。保留所有权利。
// 本源代码的使用受 BSD 风格许可证约束，
// 该许可证可在 LICENSE 文件中找到。

package image

import (
	"image/color"
)

// YCbCrSubsampleRatio 是 YCbCr 图像中使用的色度子采样比。
type YCbCrSubsampleRatio int

const (
	YCbCrSubsampleRatio444 YCbCrSubsampleRatio = iota
	YCbCrSubsampleRatio422
	YCbCrSubsampleRatio420
	YCbCrSubsampleRatio440
	YCbCrSubsampleRatio411
	YCbCrSubsampleRatio410
)

func (s YCbCrSubsampleRatio) String() string {
	switch s {
	case YCbCrSubsampleRatio444:
		return "YCbCrSubsampleRatio444"
	case YCbCrSubsampleRatio422:
		return "YCbCrSubsampleRatio422"
	case YCbCrSubsampleRatio420:
		return "YCbCrSubsampleRatio420"
	case YCbCrSubsampleRatio440:
		return "YCbCrSubsampleRatio440"
	case YCbCrSubsampleRatio411:
		return "YCbCrSubsampleRatio411"
	case YCbCrSubsampleRatio410:
		return "YCbCrSubsampleRatio410"
	}
	return "YCbCrSubsampleRatioUnknown"
}

// YCbCr 是一个内存中的 Y'CbCr 颜色图像。每个像素有一个 Y 样本，
// 但每个 Cb 和 Cr 样本可以跨越一个或多个像素。
// YStride 是垂直相邻像素之间 Y 切片索引的差值。
// CStride 是映射到不同色度样本的垂直相邻像素之间 Cb 和 Cr 切片索引的差值。
// 这不是绝对要求，但 YStride 和 len(Y) 通常是 8 的倍数，并且：
//
//	对于 4:4:4，CStride == YStride/1 && len(Cb) == len(Cr) == len(Y)/1。
//	对于 4:2:2，CStride == YStride/2 && len(Cb) == len(Cr) == len(Y)/2。
//	对于 4:2:0，CStride == YStride/2 && len(Cb) == len(Cr) == len(Y)/4。
//	对于 4:4:0，CStride == YStride/1 && len(Cb) == len(Cr) == len(Y)/2。
//	对于 4:1:1，CStride == YStride/4 && len(Cb) == len(Cr) == len(Y)/4。
//	对于 4:1:0，CStride == YStride/4 && len(Cb) == len(Cr) == len(Y)/8。
type YCbCr struct {
	Y, Cb, Cr      []uint8
	YStride        int
	CStride        int
	SubsampleRatio YCbCrSubsampleRatio
	Rect           Rectangle
}

func (p *YCbCr) ColorModel() color.Model {
	return color.YCbCrModel
}

func (p *YCbCr) Bounds() Rectangle {
	return p.Rect
}

func (p *YCbCr) At(x, y int) color.Color {
	return p.YCbCrAt(x, y)
}

func (p *YCbCr) RGBA64At(x, y int) color.RGBA64 {
	r, g, b, a := p.YCbCrAt(x, y).RGBA()
	return color.RGBA64{uint16(r), uint16(g), uint16(b), uint16(a)}
}

func (p *YCbCr) YCbCrAt(x, y int) color.YCbCr {
	if !(Point{x, y}.In(p.Rect)) {
		return color.YCbCr{}
	}
	yi := p.YOffset(x, y)
	ci := p.COffset(x, y)
	return color.YCbCr{
		p.Y[yi],
		p.Cb[ci],
		p.Cr[ci],
	}
}

// YOffset 返回 Y 中对应于 (x, y) 处像素的第一个元素的索引。
func (p *YCbCr) YOffset(x, y int) int {
	return (y-p.Rect.Min.Y)*p.YStride + (x - p.Rect.Min.X)
}

// COffset 返回 Cb 或 Cr 中对应于 (x, y) 处像素的第一个元素的索引。
func (p *YCbCr) COffset(x, y int) int {
	switch p.SubsampleRatio {
	case YCbCrSubsampleRatio422:
		return (y-p.Rect.Min.Y)*p.CStride + (x/2 - p.Rect.Min.X/2)
	case YCbCrSubsampleRatio420:
		return (y/2-p.Rect.Min.Y/2)*p.CStride + (x/2 - p.Rect.Min.X/2)
	case YCbCrSubsampleRatio440:
		return (y/2-p.Rect.Min.Y/2)*p.CStride + (x - p.Rect.Min.X)
	case YCbCrSubsampleRatio411:
		return (y-p.Rect.Min.Y)*p.CStride + (x/4 - p.Rect.Min.X/4)
	case YCbCrSubsampleRatio410:
		return (y/2-p.Rect.Min.Y/2)*p.CStride + (x/4 - p.Rect.Min.X/4)
	}
	// 默认为 4:4:4 子采样。
	return (y-p.Rect.Min.Y)*p.CStride + (x - p.Rect.Min.X)
}

// SubImage 返回一个表示通过 r 可见的图像 p 部分的图像。
// 返回的值与原始图像共享像素。
func (p *YCbCr) SubImage(r Rectangle) Image {
	r = r.Intersect(p.Rect)
	// 如果 r1 和 r2 是 Rectangle，当交集为空时，r1.Intersect(r2) 不保证
	// 在 r1 或 r2 内部。如果不显式检查这一点，下面的 Pix[i:] 表达式可能会 panic。
	if r.Empty() {
		return &YCbCr{
			SubsampleRatio: p.SubsampleRatio,
		}
	}
	yi := p.YOffset(r.Min.X, r.Min.Y)
	ci := p.COffset(r.Min.X, r.Min.Y)
	return &YCbCr{
		Y:              p.Y[yi:],
		Cb:             p.Cb[ci:],
		Cr:             p.Cr[ci:],
		SubsampleRatio: p.SubsampleRatio,
		YStride:        p.YStride,
		CStride:        p.CStride,
		Rect:           r,
	}
}

func (p *YCbCr) Opaque() bool {
	return true
}

func yCbCrSize(r Rectangle, subsampleRatio YCbCrSubsampleRatio) (w, h, cw, ch int) {
	w, h = r.Dx(), r.Dy()
	switch subsampleRatio {
	case YCbCrSubsampleRatio422:
		cw = (r.Max.X+1)/2 - r.Min.X/2
		ch = h
	case YCbCrSubsampleRatio420:
		cw = (r.Max.X+1)/2 - r.Min.X/2
		ch = (r.Max.Y+1)/2 - r.Min.Y/2
	case YCbCrSubsampleRatio440:
		cw = w
		ch = (r.Max.Y+1)/2 - r.Min.Y/2
	case YCbCrSubsampleRatio411:
		cw = (r.Max.X+3)/4 - r.Min.X/4
		ch = h
	case YCbCrSubsampleRatio410:
		cw = (r.Max.X+3)/4 - r.Min.X/4
		ch = (r.Max.Y+1)/2 - r.Min.Y/2
	default:
		// 默认为 4:4:4 子采样。
		cw = w
		ch = h
	}
	return
}

// NewYCbCr 返回具有给定边界和子采样比的新 YCbCr 图像。
func NewYCbCr(r Rectangle, subsampleRatio YCbCrSubsampleRatio) *YCbCr {
	w, h, cw, ch := yCbCrSize(r, subsampleRatio)

	// 对于有效的 Rectangle r，totalLength 应与下面的 i2 相同。
	totalLength := add2NonNeg(
		mul3NonNeg(1, w, h),
		mul3NonNeg(2, cw, ch),
	)
	if totalLength < 0 {
		panic("image: NewYCbCr Rectangle has huge or negative dimensions")
	}

	i0 := w*h + 0*cw*ch
	i1 := w*h + 1*cw*ch
	i2 := w*h + 2*cw*ch
	b := make([]byte, i2)
	return &YCbCr{
		Y:              b[:i0:i0],
		Cb:             b[i0:i1:i1],
		Cr:             b[i1:i2:i2],
		SubsampleRatio: subsampleRatio,
		YStride:        w,
		CStride:        cw,
		Rect:           r,
	}
}

// NYCbCrA 是一个内存中的非预乘 alpha 的 Y'CbCr-带-alpha 颜色图像。
// A 和 AStride 类似于嵌入的 YCbCr 的 Y 和 YStride 字段。
type NYCbCrA struct {
	YCbCr
	A       []uint8
	AStride int
}

func (p *NYCbCrA) ColorModel() color.Model {
	return color.NYCbCrAModel
}

func (p *NYCbCrA) At(x, y int) color.Color {
	return p.NYCbCrAAt(x, y)
}

func (p *NYCbCrA) RGBA64At(x, y int) color.RGBA64 {
	r, g, b, a := p.NYCbCrAAt(x, y).RGBA()
	return color.RGBA64{uint16(r), uint16(g), uint16(b), uint16(a)}
}

func (p *NYCbCrA) NYCbCrAAt(x, y int) color.NYCbCrA {
	if !(Point{X: x, Y: y}.In(p.Rect)) {
		return color.NYCbCrA{}
	}
	yi := p.YOffset(x, y)
	ci := p.COffset(x, y)
	ai := p.AOffset(x, y)
	return color.NYCbCrA{
		color.YCbCr{
			Y:  p.Y[yi],
			Cb: p.Cb[ci],
			Cr: p.Cr[ci],
		},
		p.A[ai],
	}
}

// AOffset 返回 A 中对应于 (x, y) 处像素的第一个元素的索引。
func (p *NYCbCrA) AOffset(x, y int) int {
	return (y-p.Rect.Min.Y)*p.AStride + (x - p.Rect.Min.X)
}

// SubImage 返回一个表示通过 r 可见的图像 p 部分的图像。
// 返回的值与原始图像共享像素。
func (p *NYCbCrA) SubImage(r Rectangle) Image {
	r = r.Intersect(p.Rect)
	// 如果 r1 和 r2 是 Rectangle，当交集为空时，r1.Intersect(r2) 不保证
	// 在 r1 或 r2 内部。如果不显式检查这一点，下面的 Pix[i:] 表达式可能会 panic。
	if r.Empty() {
		return &NYCbCrA{
			YCbCr: YCbCr{
				SubsampleRatio: p.SubsampleRatio,
			},
		}
	}
	yi := p.YOffset(r.Min.X, r.Min.Y)
	ci := p.COffset(r.Min.X, r.Min.Y)
	ai := p.AOffset(r.Min.X, r.Min.Y)
	return &NYCbCrA{
		YCbCr: YCbCr{
			Y:              p.Y[yi:],
			Cb:             p.Cb[ci:],
			Cr:             p.Cr[ci:],
			SubsampleRatio: p.SubsampleRatio,
			YStride:        p.YStride,
			CStride:        p.CStride,
			Rect:           r,
		},
		A:       p.A[ai:],
		AStride: p.AStride,
	}
}

// Opaque 扫描整个图像并报告它是否完全不透明。
func (p *NYCbCrA) Opaque() bool {
	if p.Rect.Empty() {
		return true
	}
	i0, i1 := 0, p.Rect.Dx()
	for y := p.Rect.Min.Y; y < p.Rect.Max.Y; y++ {
		for _, a := range p.A[i0:i1] {
			if a != 0xff {
				return false
			}
		}
		i0 += p.AStride
		i1 += p.AStride
	}
	return true
}

// NewNYCbCrA 返回具有给定边界和子采样比的新 [NYCbCrA] 图像。
func NewNYCbCrA(r Rectangle, subsampleRatio YCbCrSubsampleRatio) *NYCbCrA {
	w, h, cw, ch := yCbCrSize(r, subsampleRatio)

	// 对于有效的 Rectangle r，totalLength 应与下面的 i3 相同。
	totalLength := add2NonNeg(
		mul3NonNeg(2, w, h),
		mul3NonNeg(2, cw, ch),
	)
	if totalLength < 0 {
		panic("image: NewNYCbCrA Rectangle has huge or negative dimension")
	}

	i0 := 1*w*h + 0*cw*ch
	i1 := 1*w*h + 1*cw*ch
	i2 := 1*w*h + 2*cw*ch
	i3 := 2*w*h + 2*cw*ch
	b := make([]byte, i3)
	return &NYCbCrA{
		YCbCr: YCbCr{
			Y:              b[:i0:i0],
			Cb:             b[i0:i1:i1],
			Cr:             b[i1:i2:i2],
			SubsampleRatio: subsampleRatio,
			YStride:        w,
			CStride:        cw,
			Rect:           r,
		},
		A:       b[i2:],
		AStride: w,
	}
}
