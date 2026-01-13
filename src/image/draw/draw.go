// 版权所有 2009 The Go Authors。保留所有权利。
// 本源代码的使用受 BSD 风格许可证约束，
// 该许可证可在 LICENSE 文件中找到。

// draw 包提供图像合成函数。
//
// 有关此包的介绍，请参阅 "The Go image/draw package"：
// https://golang.org/doc/articles/image_draw.html
package draw

import (
	"image"
	"image/color"
	"image/internal/imageutil"
)

// m 是 image.Color.RGBA 返回的最大颜色值。
const m = 1<<16 - 1

// Image 是带有 Set 方法来更改单个像素的 image.Image。
type Image interface {
	image.Image
	Set(x, y int, c color.Color)
}

// RGBA64Image 通过 SetRGBA64 方法扩展了 [Image] 和 [image.RGBA64Image] 接口，
// 用于更改单个像素。SetRGBA64 等效于调用 Set，但它可以避免
// 将具体颜色类型转换为 [color.Color] 接口类型时的内存分配。
type RGBA64Image interface {
	image.RGBA64Image
	Set(x, y int, c color.Color)
	SetRGBA64(x, y int, c color.RGBA64)
}

// Quantizer 为图像生成调色板。
type Quantizer interface {
	// Quantize 向 p 追加最多 cap(p) - len(p) 个颜色，并返回
	// 适合将 m 转换为调色板图像的更新后的调色板。
	Quantize(p color.Palette, m image.Image) color.Palette
}

// Op 是 Porter-Duff 合成运算符。
type Op int

const (
	// Over 指定 ``(src in mask) over dst''。
	Over Op = iota
	// Src 指定 ``src in mask''。
	Src
)

// Draw 通过使用此 [Op] 调用 Draw 函数来实现 [Drawer] 接口。
func (op Op) Draw(dst Image, r image.Rectangle, src image.Image, sp image.Point) {
	DrawMask(dst, r, src, sp, nil, image.Point{}, op)
}

// Drawer 包含 [Draw] 方法。
type Drawer interface {
	// Draw 将 dst 中的 r.Min 与 src 中的 sp 对齐，然后用在 dst 上绘制 src 的
	// 结果替换 dst 中的矩形 r。
	Draw(dst Image, r image.Rectangle, src image.Image, sp image.Point)
}

// FloydSteinberg 是一个使用 Floyd-Steinberg 误差扩散的 [Src] [Op] [Drawer]。
var FloydSteinberg Drawer = floydSteinberg{}

type floydSteinberg struct{}

func (floydSteinberg) Draw(dst Image, r image.Rectangle, src image.Image, sp image.Point) {
	clip(dst, &r, src, &sp, nil, nil)
	if r.Empty() {
		return
	}
	drawPaletted(dst, r, src, sp, true)
}

// clip 将 r 裁剪到每个图像的边界（在转换到目标图像的坐标空间之后），
// 并将点 sp 和 mp 移动与 r.Min 变化相同的量。
func clip(dst Image, r *image.Rectangle, src image.Image, sp *image.Point, mask image.Image, mp *image.Point) {
	orig := r.Min
	*r = r.Intersect(dst.Bounds())
	*r = r.Intersect(src.Bounds().Add(orig.Sub(*sp)))
	if mask != nil {
		*r = r.Intersect(mask.Bounds().Add(orig.Sub(*mp)))
	}
	dx := r.Min.X - orig.X
	dy := r.Min.Y - orig.Y
	if dx == 0 && dy == 0 {
		return
	}
	sp.X += dx
	sp.Y += dy
	if mp != nil {
		mp.X += dx
		mp.Y += dy
	}
}

func processBackward(dst image.Image, r image.Rectangle, src image.Image, sp image.Point) bool {
	return dst == src &&
		r.Overlaps(r.Add(sp.Sub(r.Min))) &&
		(sp.Y < r.Min.Y || (sp.Y == r.Min.Y && sp.X < r.Min.X))
}

// Draw 使用 nil 掩码调用 [DrawMask]。
func Draw(dst Image, r image.Rectangle, src image.Image, sp image.Point, op Op) {
	DrawMask(dst, r, src, sp, nil, image.Point{}, op)
}

// DrawMask 将 dst 中的 r.Min 与 src 中的 sp 和 mask 中的 mp 对齐，然后用
// Porter-Duff 合成的结果替换 dst 中的矩形 r。nil 掩码被视为不透明。
func DrawMask(dst Image, r image.Rectangle, src image.Image, sp image.Point, mask image.Image, mp image.Point, op Op) {
	clip(dst, &r, src, &sp, mask, &mp)
	if r.Empty() {
		return
	}

	// 特殊情况的快速路径。如果都不适用，则回退到
	// 通用但较慢的实现。
	//
	// 对于 NRGBA 和 NRGBA64 图像类型，代码路径不仅更快。
	// 它们还避免了将非预乘 alpha 颜色与预乘 alpha 颜色
	// 相互转换时会发生的信息丢失。参见 TestDrawSrcNonpremultiplied。
	switch dst0 := dst.(type) {
	case *image.RGBA:
		if op == Over {
			if mask == nil {
				switch src0 := src.(type) {
				case *image.Uniform:
					sr, sg, sb, sa := src0.RGBA()
					if sa == 0xffff {
						drawFillSrc(dst0, r, sr, sg, sb, sa)
					} else {
						drawFillOver(dst0, r, sr, sg, sb, sa)
					}
					return
				case *image.RGBA:
					drawCopyOver(dst0, r, src0, sp)
					return
				case *image.NRGBA:
					drawNRGBAOver(dst0, r, src0, sp)
					return
				case *image.YCbCr:
					// image.YCbCr 始终是完全不透明的，因此如果
					// mask 为 nil（即完全不透明），则 op 实际上
					// 始终是 Src。image.Gray 和 image.CMYK 同理。
					if imageutil.DrawYCbCr(dst0, r, src0, sp) {
						return
					}
				case *image.Gray:
					drawGray(dst0, r, src0, sp)
					return
				case *image.CMYK:
					drawCMYK(dst0, r, src0, sp)
					return
				}
			} else if mask0, ok := mask.(*image.Alpha); ok {
				switch src0 := src.(type) {
				case *image.Uniform:
					drawGlyphOver(dst0, r, src0, mask0, mp)
					return
				case *image.RGBA:
					drawRGBAMaskOver(dst0, r, src0, sp, mask0, mp)
					return
				case *image.Gray:
					drawGrayMaskOver(dst0, r, src0, sp, mask0, mp)
					return
				// case 顺序很重要。下一个 case（image.RGBA64Image）是
				// 上述具体类型也实现的接口类型。
				case image.RGBA64Image:
					drawRGBA64ImageMaskOver(dst0, r, src0, sp, mask0, mp)
					return
				}
			}
		} else {
			if mask == nil {
				switch src0 := src.(type) {
				case *image.Uniform:
					sr, sg, sb, sa := src0.RGBA()
					drawFillSrc(dst0, r, sr, sg, sb, sa)
					return
				case *image.RGBA:
					d0 := dst0.PixOffset(r.Min.X, r.Min.Y)
					s0 := src0.PixOffset(sp.X, sp.Y)
					drawCopySrc(
						dst0.Pix[d0:], dst0.Stride, r, src0.Pix[s0:], src0.Stride, sp, 4*r.Dx())
					return
				case *image.NRGBA:
					drawNRGBASrc(dst0, r, src0, sp)
					return
				case *image.YCbCr:
					if imageutil.DrawYCbCr(dst0, r, src0, sp) {
						return
					}
				case *image.Gray:
					drawGray(dst0, r, src0, sp)
					return
				case *image.CMYK:
					drawCMYK(dst0, r, src0, sp)
					return
				}
			}
		}
		drawRGBA(dst0, r, src, sp, mask, mp, op)
		return
	case *image.Paletted:
		if op == Src && mask == nil {
			if src0, ok := src.(*image.Uniform); ok {
				colorIndex := uint8(dst0.Palette.Index(src0.C))
				i0 := dst0.PixOffset(r.Min.X, r.Min.Y)
				i1 := i0 + r.Dx()
				for i := i0; i < i1; i++ {
					dst0.Pix[i] = colorIndex
				}
				firstRow := dst0.Pix[i0:i1]
				for y := r.Min.Y + 1; y < r.Max.Y; y++ {
					i0 += dst0.Stride
					i1 += dst0.Stride
					copy(dst0.Pix[i0:i1], firstRow)
				}
				return
			} else if !processBackward(dst, r, src, sp) {
				drawPaletted(dst0, r, src, sp, false)
				return
			}
		}
	case *image.NRGBA:
		if op == Src && mask == nil {
			if src0, ok := src.(*image.NRGBA); ok {
				d0 := dst0.PixOffset(r.Min.X, r.Min.Y)
				s0 := src0.PixOffset(sp.X, sp.Y)
				drawCopySrc(
					dst0.Pix[d0:], dst0.Stride, r, src0.Pix[s0:], src0.Stride, sp, 4*r.Dx())
				return
			}
		}
	case *image.NRGBA64:
		if op == Src && mask == nil {
			if src0, ok := src.(*image.NRGBA64); ok {
				d0 := dst0.PixOffset(r.Min.X, r.Min.Y)
				s0 := src0.PixOffset(sp.X, sp.Y)
				drawCopySrc(
					dst0.Pix[d0:], dst0.Stride, r, src0.Pix[s0:], src0.Stride, sp, 8*r.Dx())
				return
			}
		}
	}

	x0, x1, dx := r.Min.X, r.Max.X, 1
	y0, y1, dy := r.Min.Y, r.Max.Y, 1
	if processBackward(dst, r, src, sp) {
		x0, x1, dx = x1-1, x0-1, -1
		y0, y1, dy = y1-1, y0-1, -1
	}

	// FALLBACK1.17
	//
	// 尝试 draw.RGBA64Image 和 image.RGBA64Image 接口，这些接口从 Go 1.17 起
	// 成为标准库的一部分。它们类似于 draw.Image 和 image.Image 接口，
	// 但可以避免将具体颜色类型转换为 color.Color 接口类型时的内存分配。

	if dst0, _ := dst.(RGBA64Image); dst0 != nil {
		if src0, _ := src.(image.RGBA64Image); src0 != nil {
			if mask == nil {
				sy := sp.Y + y0 - r.Min.Y
				my := mp.Y + y0 - r.Min.Y
				for y := y0; y != y1; y, sy, my = y+dy, sy+dy, my+dy {
					sx := sp.X + x0 - r.Min.X
					mx := mp.X + x0 - r.Min.X
					for x := x0; x != x1; x, sx, mx = x+dx, sx+dx, mx+dx {
						if op == Src {
							dst0.SetRGBA64(x, y, src0.RGBA64At(sx, sy))
						} else {
							srgba := src0.RGBA64At(sx, sy)
							a := m - uint32(srgba.A)
							drgba := dst0.RGBA64At(x, y)
							dst0.SetRGBA64(x, y, color.RGBA64{
								R: uint16((uint32(drgba.R)*a)/m) + srgba.R,
								G: uint16((uint32(drgba.G)*a)/m) + srgba.G,
								B: uint16((uint32(drgba.B)*a)/m) + srgba.B,
								A: uint16((uint32(drgba.A)*a)/m) + srgba.A,
							})
						}
					}
				}
				return

			} else if mask0, _ := mask.(image.RGBA64Image); mask0 != nil {
				sy := sp.Y + y0 - r.Min.Y
				my := mp.Y + y0 - r.Min.Y
				for y := y0; y != y1; y, sy, my = y+dy, sy+dy, my+dy {
					sx := sp.X + x0 - r.Min.X
					mx := mp.X + x0 - r.Min.X
					for x := x0; x != x1; x, sx, mx = x+dx, sx+dx, mx+dx {
						ma := uint32(mask0.RGBA64At(mx, my).A)
						switch {
						case ma == 0:
							if op == Over {
								// 无操作。
							} else {
								dst0.SetRGBA64(x, y, color.RGBA64{})
							}
						case ma == m && op == Src:
							dst0.SetRGBA64(x, y, src0.RGBA64At(sx, sy))
						default:
							srgba := src0.RGBA64At(sx, sy)
							if op == Over {
								drgba := dst0.RGBA64At(x, y)
								a := m - (uint32(srgba.A) * ma / m)
								dst0.SetRGBA64(x, y, color.RGBA64{
									R: uint16((uint32(drgba.R)*a + uint32(srgba.R)*ma) / m),
									G: uint16((uint32(drgba.G)*a + uint32(srgba.G)*ma) / m),
									B: uint16((uint32(drgba.B)*a + uint32(srgba.B)*ma) / m),
									A: uint16((uint32(drgba.A)*a + uint32(srgba.A)*ma) / m),
								})
							} else {
								dst0.SetRGBA64(x, y, color.RGBA64{
									R: uint16(uint32(srgba.R) * ma / m),
									G: uint16(uint32(srgba.G) * ma / m),
									B: uint16(uint32(srgba.B) * ma / m),
									A: uint16(uint32(srgba.A) * ma / m),
								})
							}
						}
					}
				}
				return
			}
		}
	}

	// FALLBACK1.0
	//
	// 如果上面的快速代码路径都不适用，则使用 draw.Image 和
	// image.Image 接口，这些接口从 Go 1.0 起成为标准库的一部分。

	var out color.RGBA64
	sy := sp.Y + y0 - r.Min.Y
	my := mp.Y + y0 - r.Min.Y
	for y := y0; y != y1; y, sy, my = y+dy, sy+dy, my+dy {
		sx := sp.X + x0 - r.Min.X
		mx := mp.X + x0 - r.Min.X
		for x := x0; x != x1; x, sx, mx = x+dx, sx+dx, mx+dx {
			ma := uint32(m)
			if mask != nil {
				_, _, _, ma = mask.At(mx, my).RGBA()
			}
			switch {
			case ma == 0:
				if op == Over {
					// 无操作。
				} else {
					dst.Set(x, y, color.Transparent)
				}
			case ma == m && op == Src:
				dst.Set(x, y, src.At(sx, sy))
			default:
				sr, sg, sb, sa := src.At(sx, sy).RGBA()
				if op == Over {
					dr, dg, db, da := dst.At(x, y).RGBA()
					a := m - (sa * ma / m)
					out.R = uint16((dr*a + sr*ma) / m)
					out.G = uint16((dg*a + sg*ma) / m)
					out.B = uint16((db*a + sb*ma) / m)
					out.A = uint16((da*a + sa*ma) / m)
				} else {
					out.R = uint16(sr * ma / m)
					out.G = uint16(sg * ma / m)
					out.B = uint16(sb * ma / m)
					out.A = uint16(sa * ma / m)
				}
				// 第三个参数是 &out 而不是 out（且 out 在内层循环外声明），
				// 以避免在 sizeof(color.RGBA64) > sizeof(uintptr) 时，
				// 此处到 color.Color 的隐式转换在内层循环中分配内存。
				dst.Set(x, y, &out)
			}
		}
	}
}

func drawFillOver(dst *image.RGBA, r image.Rectangle, sr, sg, sb, sa uint32) {
	// 这里的 0x101 与 drawRGBA 中的原因相同。
	a := (m - sa) * 0x101
	i0 := dst.PixOffset(r.Min.X, r.Min.Y)
	i1 := i0 + r.Dx()*4
	for y := r.Min.Y; y != r.Max.Y; y++ {
		for i := i0; i < i1; i += 4 {
			dr := &dst.Pix[i+0]
			dg := &dst.Pix[i+1]
			db := &dst.Pix[i+2]
			da := &dst.Pix[i+3]

			*dr = uint8((uint32(*dr)*a/m + sr) >> 8)
			*dg = uint8((uint32(*dg)*a/m + sg) >> 8)
			*db = uint8((uint32(*db)*a/m + sb) >> 8)
			*da = uint8((uint32(*da)*a/m + sa) >> 8)
		}
		i0 += dst.Stride
		i1 += dst.Stride
	}
}

func drawFillSrc(dst *image.RGBA, r image.Rectangle, sr, sg, sb, sa uint32) {
	sr8 := uint8(sr >> 8)
	sg8 := uint8(sg >> 8)
	sb8 := uint8(sb >> 8)
	sa8 := uint8(sa >> 8)
	// 内置的 copy 函数比直接使用 for 循环填充颜色更快，
	// 但 copy 需要一个切片源。因此我们使用 for 循环填充第一行，
	// 然后将第一行用作剩余行的切片源。
	i0 := dst.PixOffset(r.Min.X, r.Min.Y)
	i1 := i0 + r.Dx()*4
	for i := i0; i < i1; i += 4 {
		dst.Pix[i+0] = sr8
		dst.Pix[i+1] = sg8
		dst.Pix[i+2] = sb8
		dst.Pix[i+3] = sa8
	}
	firstRow := dst.Pix[i0:i1]
	for y := r.Min.Y + 1; y < r.Max.Y; y++ {
		i0 += dst.Stride
		i1 += dst.Stride
		copy(dst.Pix[i0:i1], firstRow)
	}
}

func drawCopyOver(dst *image.RGBA, r image.Rectangle, src *image.RGBA, sp image.Point) {
	dx, dy := r.Dx(), r.Dy()
	d0 := dst.PixOffset(r.Min.X, r.Min.Y)
	s0 := src.PixOffset(sp.X, sp.Y)
	var (
		ddelta, sdelta int
		i0, i1, idelta int
	)
	if r.Min.Y < sp.Y || r.Min.Y == sp.Y && r.Min.X <= sp.X {
		ddelta = dst.Stride
		sdelta = src.Stride
		i0, i1, idelta = 0, dx*4, +4
	} else {
		// 如果源起始点高于目标起始点，或高度相同但在左侧，
		// 则我们按从右到左、从下到上的顺序合成行，而不是从左到右、从上到下。
		d0 += (dy - 1) * dst.Stride
		s0 += (dy - 1) * src.Stride
		ddelta = -dst.Stride
		sdelta = -src.Stride
		i0, i1, idelta = (dx-1)*4, -4, -4
	}
	for ; dy > 0; dy-- {
		dpix := dst.Pix[d0:]
		spix := src.Pix[s0:]
		for i := i0; i != i1; i += idelta {
			s := spix[i : i+4 : i+4] // 小容量可提高性能，参见 https://golang.org/issue/27857
			sr := uint32(s[0]) * 0x101
			sg := uint32(s[1]) * 0x101
			sb := uint32(s[2]) * 0x101
			sa := uint32(s[3]) * 0x101

			// 这里的 0x101 与 drawRGBA 中的原因相同。
			a := (m - sa) * 0x101

			d := dpix[i : i+4 : i+4] // 小容量可提高性能，参见 https://golang.org/issue/27857
			d[0] = uint8((uint32(d[0])*a/m + sr) >> 8)
			d[1] = uint8((uint32(d[1])*a/m + sg) >> 8)
			d[2] = uint8((uint32(d[2])*a/m + sb) >> 8)
			d[3] = uint8((uint32(d[3])*a/m + sa) >> 8)
		}
		d0 += ddelta
		s0 += sdelta
	}
}

// drawCopySrc 从 srcPix 复制字节到 dstPix。这些参数大致对应于
// image 包的具体 image.Image 实现的 Pix 字段，
// 但是有偏移（dstPix 是 dst.Pix[dpOffset:] 而不是 dst.Pix）。
func drawCopySrc(
	dstPix []byte, dstStride int, r image.Rectangle,
	srcPix []byte, srcStride int, sp image.Point,
	bytesPerRow int) {

	d0, s0, ddelta, sdelta, dy := 0, 0, dstStride, srcStride, r.Dy()
	if r.Min.Y > sp.Y {
		// 如果源起始点高于目标起始点，则我们按从下到上的顺序
		// 合成行，而不是从上到下。与 drawCopyOver 函数不同，
		// 我们不必检查 x 坐标，因为内置的 copy 函数可以处理
		// 重叠的切片。
		d0 = (dy - 1) * dstStride
		s0 = (dy - 1) * srcStride
		ddelta = -dstStride
		sdelta = -srcStride
	}
	for ; dy > 0; dy-- {
		copy(dstPix[d0:d0+bytesPerRow], srcPix[s0:s0+bytesPerRow])
		d0 += ddelta
		s0 += sdelta
	}
}

func drawNRGBAOver(dst *image.RGBA, r image.Rectangle, src *image.NRGBA, sp image.Point) {
	i0 := (r.Min.X - dst.Rect.Min.X) * 4
	i1 := (r.Max.X - dst.Rect.Min.X) * 4
	si0 := (sp.X - src.Rect.Min.X) * 4
	yMax := r.Max.Y - dst.Rect.Min.Y

	y := r.Min.Y - dst.Rect.Min.Y
	sy := sp.Y - src.Rect.Min.Y
	for ; y != yMax; y, sy = y+1, sy+1 {
		dpix := dst.Pix[y*dst.Stride:]
		spix := src.Pix[sy*src.Stride:]

		for i, si := i0, si0; i < i1; i, si = i+4, si+4 {
			// 从非预乘颜色转换为预乘颜色。
			s := spix[si : si+4 : si+4] // 小容量可提高性能，参见 https://golang.org/issue/27857
			sa := uint32(s[3]) * 0x101
			sr := uint32(s[0]) * sa / 0xff
			sg := uint32(s[1]) * sa / 0xff
			sb := uint32(s[2]) * sa / 0xff

			d := dpix[i : i+4 : i+4] // 小容量可提高性能，参见 https://golang.org/issue/27857
			dr := uint32(d[0])
			dg := uint32(d[1])
			db := uint32(d[2])
			da := uint32(d[3])

			// 这里的 0x101 与 drawRGBA 中的原因相同。
			a := (m - sa) * 0x101

			d[0] = uint8((dr*a/m + sr) >> 8)
			d[1] = uint8((dg*a/m + sg) >> 8)
			d[2] = uint8((db*a/m + sb) >> 8)
			d[3] = uint8((da*a/m + sa) >> 8)
		}
	}
}

func drawNRGBASrc(dst *image.RGBA, r image.Rectangle, src *image.NRGBA, sp image.Point) {
	i0 := (r.Min.X - dst.Rect.Min.X) * 4
	i1 := (r.Max.X - dst.Rect.Min.X) * 4
	si0 := (sp.X - src.Rect.Min.X) * 4
	yMax := r.Max.Y - dst.Rect.Min.Y

	y := r.Min.Y - dst.Rect.Min.Y
	sy := sp.Y - src.Rect.Min.Y
	for ; y != yMax; y, sy = y+1, sy+1 {
		dpix := dst.Pix[y*dst.Stride:]
		spix := src.Pix[sy*src.Stride:]

		for i, si := i0, si0; i < i1; i, si = i+4, si+4 {
			// 从非预乘颜色转换为预乘颜色。
			s := spix[si : si+4 : si+4] // 小容量可提高性能，参见 https://golang.org/issue/27857
			sa := uint32(s[3]) * 0x101
			sr := uint32(s[0]) * sa / 0xff
			sg := uint32(s[1]) * sa / 0xff
			sb := uint32(s[2]) * sa / 0xff

			d := dpix[i : i+4 : i+4] // 小容量可提高性能，参见 https://golang.org/issue/27857
			d[0] = uint8(sr >> 8)
			d[1] = uint8(sg >> 8)
			d[2] = uint8(sb >> 8)
			d[3] = uint8(sa >> 8)
		}
	}
}

func drawGray(dst *image.RGBA, r image.Rectangle, src *image.Gray, sp image.Point) {
	i0 := (r.Min.X - dst.Rect.Min.X) * 4
	i1 := (r.Max.X - dst.Rect.Min.X) * 4
	si0 := (sp.X - src.Rect.Min.X) * 1
	yMax := r.Max.Y - dst.Rect.Min.Y

	y := r.Min.Y - dst.Rect.Min.Y
	sy := sp.Y - src.Rect.Min.Y
	for ; y != yMax; y, sy = y+1, sy+1 {
		dpix := dst.Pix[y*dst.Stride:]
		spix := src.Pix[sy*src.Stride:]

		for i, si := i0, si0; i < i1; i, si = i+4, si+1 {
			p := spix[si]
			d := dpix[i : i+4 : i+4] // 小容量可提高性能，参见 https://golang.org/issue/27857
			d[0] = p
			d[1] = p
			d[2] = p
			d[3] = 255
		}
	}
}

func drawCMYK(dst *image.RGBA, r image.Rectangle, src *image.CMYK, sp image.Point) {
	i0 := (r.Min.X - dst.Rect.Min.X) * 4
	i1 := (r.Max.X - dst.Rect.Min.X) * 4
	si0 := (sp.X - src.Rect.Min.X) * 4
	yMax := r.Max.Y - dst.Rect.Min.Y

	y := r.Min.Y - dst.Rect.Min.Y
	sy := sp.Y - src.Rect.Min.Y
	for ; y != yMax; y, sy = y+1, sy+1 {
		dpix := dst.Pix[y*dst.Stride:]
		spix := src.Pix[sy*src.Stride:]

		for i, si := i0, si0; i < i1; i, si = i+4, si+4 {
			s := spix[si : si+4 : si+4] // 小容量可提高性能，参见 https://golang.org/issue/27857
			d := dpix[i : i+4 : i+4]
			d[0], d[1], d[2] = color.CMYKToRGB(s[0], s[1], s[2], s[3])
			d[3] = 255
		}
	}
}

func drawGlyphOver(dst *image.RGBA, r image.Rectangle, src *image.Uniform, mask *image.Alpha, mp image.Point) {
	i0 := dst.PixOffset(r.Min.X, r.Min.Y)
	i1 := i0 + r.Dx()*4
	mi0 := mask.PixOffset(mp.X, mp.Y)
	sr, sg, sb, sa := src.RGBA()
	for y, my := r.Min.Y, mp.Y; y != r.Max.Y; y, my = y+1, my+1 {
		for i, mi := i0, mi0; i < i1; i, mi = i+4, mi+1 {
			ma := uint32(mask.Pix[mi])
			if ma == 0 {
				continue
			}
			ma |= ma << 8

			// 这里的 0x101 与 drawRGBA 中的原因相同。
			a := (m - (sa * ma / m)) * 0x101

			d := dst.Pix[i : i+4 : i+4] // 小容量可提高性能，参见 https://golang.org/issue/27857
			d[0] = uint8((uint32(d[0])*a + sr*ma) / m >> 8)
			d[1] = uint8((uint32(d[1])*a + sg*ma) / m >> 8)
			d[2] = uint8((uint32(d[2])*a + sb*ma) / m >> 8)
			d[3] = uint8((uint32(d[3])*a + sa*ma) / m >> 8)
		}
		i0 += dst.Stride
		i1 += dst.Stride
		mi0 += mask.Stride
	}
}

func drawGrayMaskOver(dst *image.RGBA, r image.Rectangle, src *image.Gray, sp image.Point, mask *image.Alpha, mp image.Point) {
	x0, x1, dx := r.Min.X, r.Max.X, 1
	y0, y1, dy := r.Min.Y, r.Max.Y, 1
	if r.Overlaps(r.Add(sp.Sub(r.Min))) {
		if sp.Y < r.Min.Y || sp.Y == r.Min.Y && sp.X < r.Min.X {
			x0, x1, dx = x1-1, x0-1, -1
			y0, y1, dy = y1-1, y0-1, -1
		}
	}

	sy := sp.Y + y0 - r.Min.Y
	my := mp.Y + y0 - r.Min.Y
	sx0 := sp.X + x0 - r.Min.X
	mx0 := mp.X + x0 - r.Min.X
	sx1 := sx0 + (x1 - x0)
	i0 := dst.PixOffset(x0, y0)
	di := dx * 4
	for y := y0; y != y1; y, sy, my = y+dy, sy+dy, my+dy {
		for i, sx, mx := i0, sx0, mx0; sx != sx1; i, sx, mx = i+di, sx+dx, mx+dx {
			mi := mask.PixOffset(mx, my)
			ma := uint32(mask.Pix[mi])
			ma |= ma << 8
			si := src.PixOffset(sx, sy)
			sy := uint32(src.Pix[si])
			sy |= sy << 8
			sa := uint32(0xffff)

			d := dst.Pix[i : i+4 : i+4] // 小容量可提高性能，参见 https://golang.org/issue/27857
			dr := uint32(d[0])
			dg := uint32(d[1])
			db := uint32(d[2])
			da := uint32(d[3])

			// dr、dg、db 和 da 目前都是 8 位颜色，范围在 [0,255]。
			// 我们使用 16 位颜色，因此通常会这样做：
			// dr |= dr << 8
			// dg、db 和 da 也类似，但我们改为将 a
			// （16 位颜色，范围在 [0,65535]）乘以 0x101。
			// 这会产生相同的结果，但算术运算更少。
			a := (m - (sa * ma / m)) * 0x101

			d[0] = uint8((dr*a + sy*ma) / m >> 8)
			d[1] = uint8((dg*a + sy*ma) / m >> 8)
			d[2] = uint8((db*a + sy*ma) / m >> 8)
			d[3] = uint8((da*a + sa*ma) / m >> 8)
		}
		i0 += dy * dst.Stride
	}
}

func drawRGBAMaskOver(dst *image.RGBA, r image.Rectangle, src *image.RGBA, sp image.Point, mask *image.Alpha, mp image.Point) {
	x0, x1, dx := r.Min.X, r.Max.X, 1
	y0, y1, dy := r.Min.Y, r.Max.Y, 1
	if dst == src && r.Overlaps(r.Add(sp.Sub(r.Min))) {
		if sp.Y < r.Min.Y || sp.Y == r.Min.Y && sp.X < r.Min.X {
			x0, x1, dx = x1-1, x0-1, -1
			y0, y1, dy = y1-1, y0-1, -1
		}
	}

	sy := sp.Y + y0 - r.Min.Y
	my := mp.Y + y0 - r.Min.Y
	sx0 := sp.X + x0 - r.Min.X
	mx0 := mp.X + x0 - r.Min.X
	sx1 := sx0 + (x1 - x0)
	i0 := dst.PixOffset(x0, y0)
	di := dx * 4
	for y := y0; y != y1; y, sy, my = y+dy, sy+dy, my+dy {
		for i, sx, mx := i0, sx0, mx0; sx != sx1; i, sx, mx = i+di, sx+dx, mx+dx {
			mi := mask.PixOffset(mx, my)
			ma := uint32(mask.Pix[mi])
			ma |= ma << 8
			si := src.PixOffset(sx, sy)
			sr := uint32(src.Pix[si+0])
			sg := uint32(src.Pix[si+1])
			sb := uint32(src.Pix[si+2])
			sa := uint32(src.Pix[si+3])
			sr |= sr << 8
			sg |= sg << 8
			sb |= sb << 8
			sa |= sa << 8
			d := dst.Pix[i : i+4 : i+4] // 小容量可提高性能，参见 https://golang.org/issue/27857
			dr := uint32(d[0])
			dg := uint32(d[1])
			db := uint32(d[2])
			da := uint32(d[3])

			// dr、dg、db 和 da 目前都是 8 位颜色，范围在 [0,255]。
			// 我们使用 16 位颜色，因此通常会这样做：
			// dr |= dr << 8
			// dg、db 和 da 也类似，但我们改为将 a
			// （16 位颜色，范围在 [0,65535]）乘以 0x101。
			// 这会产生相同的结果，但算术运算更少。
			a := (m - (sa * ma / m)) * 0x101

			d[0] = uint8((dr*a + sr*ma) / m >> 8)
			d[1] = uint8((dg*a + sg*ma) / m >> 8)
			d[2] = uint8((db*a + sb*ma) / m >> 8)
			d[3] = uint8((da*a + sa*ma) / m >> 8)
		}
		i0 += dy * dst.Stride
	}
}

func drawRGBA64ImageMaskOver(dst *image.RGBA, r image.Rectangle, src image.RGBA64Image, sp image.Point, mask *image.Alpha, mp image.Point) {
	x0, x1, dx := r.Min.X, r.Max.X, 1
	y0, y1, dy := r.Min.Y, r.Max.Y, 1
	if image.Image(dst) == src && r.Overlaps(r.Add(sp.Sub(r.Min))) {
		if sp.Y < r.Min.Y || sp.Y == r.Min.Y && sp.X < r.Min.X {
			x0, x1, dx = x1-1, x0-1, -1
			y0, y1, dy = y1-1, y0-1, -1
		}
	}

	sy := sp.Y + y0 - r.Min.Y
	my := mp.Y + y0 - r.Min.Y
	sx0 := sp.X + x0 - r.Min.X
	mx0 := mp.X + x0 - r.Min.X
	sx1 := sx0 + (x1 - x0)
	i0 := dst.PixOffset(x0, y0)
	di := dx * 4
	for y := y0; y != y1; y, sy, my = y+dy, sy+dy, my+dy {
		for i, sx, mx := i0, sx0, mx0; sx != sx1; i, sx, mx = i+di, sx+dx, mx+dx {
			mi := mask.PixOffset(mx, my)
			ma := uint32(mask.Pix[mi])
			ma |= ma << 8
			srgba := src.RGBA64At(sx, sy)
			d := dst.Pix[i : i+4 : i+4] // 小容量可提高性能，参见 https://golang.org/issue/27857
			dr := uint32(d[0])
			dg := uint32(d[1])
			db := uint32(d[2])
			da := uint32(d[3])

			// dr、dg、db 和 da 目前都是 8 位颜色，范围在 [0,255]。
			// 我们使用 16 位颜色，因此通常会这样做：
			// dr |= dr << 8
			// dg、db 和 da 也类似，但我们改为将 a
			// （16 位颜色，范围在 [0,65535]）乘以 0x101。
			// 这会产生相同的结果，但算术运算更少。
			a := (m - (uint32(srgba.A) * ma / m)) * 0x101

			d[0] = uint8((dr*a + uint32(srgba.R)*ma) / m >> 8)
			d[1] = uint8((dg*a + uint32(srgba.G)*ma) / m >> 8)
			d[2] = uint8((db*a + uint32(srgba.B)*ma) / m >> 8)
			d[3] = uint8((da*a + uint32(srgba.A)*ma) / m >> 8)
		}
		i0 += dy * dst.Stride
	}
}

func drawRGBA(dst *image.RGBA, r image.Rectangle, src image.Image, sp image.Point, mask image.Image, mp image.Point, op Op) {
	x0, x1, dx := r.Min.X, r.Max.X, 1
	y0, y1, dy := r.Min.Y, r.Max.Y, 1
	if image.Image(dst) == src && r.Overlaps(r.Add(sp.Sub(r.Min))) {
		if sp.Y < r.Min.Y || sp.Y == r.Min.Y && sp.X < r.Min.X {
			x0, x1, dx = x1-1, x0-1, -1
			y0, y1, dy = y1-1, y0-1, -1
		}
	}

	sy := sp.Y + y0 - r.Min.Y
	my := mp.Y + y0 - r.Min.Y
	sx0 := sp.X + x0 - r.Min.X
	mx0 := mp.X + x0 - r.Min.X
	sx1 := sx0 + (x1 - x0)
	i0 := dst.PixOffset(x0, y0)
	di := dx * 4

	// 尝试 image.RGBA64Image 接口，该接口从 Go 1.17 起成为标准库的一部分。
	//
	// 此优化类似于 FALLBACK1.17 在 DrawMask 中优化 FALLBACK1.0 的方式，
	// 只是这里 dst 的具体类型已知为 *image.RGBA。
	if src0, _ := src.(image.RGBA64Image); src0 != nil {
		if mask == nil {
			if op == Over {
				for y := y0; y != y1; y, sy, my = y+dy, sy+dy, my+dy {
					for i, sx, mx := i0, sx0, mx0; sx != sx1; i, sx, mx = i+di, sx+dx, mx+dx {
						srgba := src0.RGBA64At(sx, sy)
						d := dst.Pix[i : i+4 : i+4]
						dr := uint32(d[0])
						dg := uint32(d[1])
						db := uint32(d[2])
						da := uint32(d[3])
						a := (m - uint32(srgba.A)) * 0x101
						d[0] = uint8((dr*a/m + uint32(srgba.R)) >> 8)
						d[1] = uint8((dg*a/m + uint32(srgba.G)) >> 8)
						d[2] = uint8((db*a/m + uint32(srgba.B)) >> 8)
						d[3] = uint8((da*a/m + uint32(srgba.A)) >> 8)
					}
					i0 += dy * dst.Stride
				}
			} else {
				for y := y0; y != y1; y, sy, my = y+dy, sy+dy, my+dy {
					for i, sx, mx := i0, sx0, mx0; sx != sx1; i, sx, mx = i+di, sx+dx, mx+dx {
						srgba := src0.RGBA64At(sx, sy)
						d := dst.Pix[i : i+4 : i+4]
						d[0] = uint8(srgba.R >> 8)
						d[1] = uint8(srgba.G >> 8)
						d[2] = uint8(srgba.B >> 8)
						d[3] = uint8(srgba.A >> 8)
					}
					i0 += dy * dst.Stride
				}
			}
			return

		} else if mask0, _ := mask.(image.RGBA64Image); mask0 != nil {
			if op == Over {
				for y := y0; y != y1; y, sy, my = y+dy, sy+dy, my+dy {
					for i, sx, mx := i0, sx0, mx0; sx != sx1; i, sx, mx = i+di, sx+dx, mx+dx {
						ma := uint32(mask0.RGBA64At(mx, my).A)
						srgba := src0.RGBA64At(sx, sy)
						d := dst.Pix[i : i+4 : i+4]
						dr := uint32(d[0])
						dg := uint32(d[1])
						db := uint32(d[2])
						da := uint32(d[3])
						a := (m - (uint32(srgba.A) * ma / m)) * 0x101
						d[0] = uint8((dr*a + uint32(srgba.R)*ma) / m >> 8)
						d[1] = uint8((dg*a + uint32(srgba.G)*ma) / m >> 8)
						d[2] = uint8((db*a + uint32(srgba.B)*ma) / m >> 8)
						d[3] = uint8((da*a + uint32(srgba.A)*ma) / m >> 8)
					}
					i0 += dy * dst.Stride
				}
			} else {
				for y := y0; y != y1; y, sy, my = y+dy, sy+dy, my+dy {
					for i, sx, mx := i0, sx0, mx0; sx != sx1; i, sx, mx = i+di, sx+dx, mx+dx {
						ma := uint32(mask0.RGBA64At(mx, my).A)
						srgba := src0.RGBA64At(sx, sy)
						d := dst.Pix[i : i+4 : i+4]
						d[0] = uint8(uint32(srgba.R) * ma / m >> 8)
						d[1] = uint8(uint32(srgba.G) * ma / m >> 8)
						d[2] = uint8(uint32(srgba.B) * ma / m >> 8)
						d[3] = uint8(uint32(srgba.A) * ma / m >> 8)
					}
					i0 += dy * dst.Stride
				}
			}
			return
		}
	}

	// 使用 image.Image 接口，该接口从 Go 1.0 起成为标准库的一部分。
	//
	// 这类似于 DrawMask 中的 FALLBACK1.0，
	// 只是这里 dst 的具体类型已知为 *image.RGBA。
	for y := y0; y != y1; y, sy, my = y+dy, sy+dy, my+dy {
		for i, sx, mx := i0, sx0, mx0; sx != sx1; i, sx, mx = i+di, sx+dx, mx+dx {
			ma := uint32(m)
			if mask != nil {
				_, _, _, ma = mask.At(mx, my).RGBA()
			}
			sr, sg, sb, sa := src.At(sx, sy).RGBA()
			d := dst.Pix[i : i+4 : i+4] // 小容量可提高性能，参见 https://golang.org/issue/27857
			if op == Over {
				dr := uint32(d[0])
				dg := uint32(d[1])
				db := uint32(d[2])
				da := uint32(d[3])

				// dr, dg, db and da are all 8-bit color at the moment, ranging in [0,255].
				// We work in 16-bit color, and so would normally do:
				// dr |= dr << 8
				// and similarly for dg, db and da, but instead we multiply a
				// (which is a 16-bit color, ranging in [0,65535]) by 0x101.
				// This yields the same result, but is fewer arithmetic operations.
				a := (m - (sa * ma / m)) * 0x101

				d[0] = uint8((dr*a + sr*ma) / m >> 8)
				d[1] = uint8((dg*a + sg*ma) / m >> 8)
				d[2] = uint8((db*a + sb*ma) / m >> 8)
				d[3] = uint8((da*a + sa*ma) / m >> 8)

			} else {
				d[0] = uint8(sr * ma / m >> 8)
				d[1] = uint8(sg * ma / m >> 8)
				d[2] = uint8(sb * ma / m >> 8)
				d[3] = uint8(sa * ma / m >> 8)
			}
		}
		i0 += dy * dst.Stride
	}
}

// clamp 将 i 限制在区间 [0, 0xffff] 内。
func clamp(i int32) int32 {
	if i < 0 {
		return 0
	}
	if i > 0xffff {
		return 0xffff
	}
	return i
}

// sqDiff 返回 x 和 y 的差的平方，右移 2 位，使得
// 四个这样的值相加不会溢出 uint32。
//
// x 和 y 都假定在 [0, 0xffff] 范围内。
func sqDiff(x, y int32) uint32 {
	// 这是依赖于语言规范保证的无符号整数运算的溢出/回绕属性的
	// 优化代码。有关更多详细信息，请参阅 image/color 包中的 sqDiff。
	d := uint32(x - y)
	return (d * d) >> 2
}

func drawPaletted(dst Image, r image.Rectangle, src image.Image, sp image.Point, floydSteinberg bool) {
	// TODO(nigeltao): 处理 dst 和 src 重叠的情况。
	// 在向后遍历图像（从右到左、从下到上）时尝试 Floyd-Steinberg
	// 是否有意义？

	// 如果 dst 是 *image.Paletted，我们对 dst.Set 和 dst.At 有快速路径。
	// dst.Set 的等效版本是 image/color/color.go 中 color.Palette 的 Index
	// 方法使用的算法的批处理版本，加上可选的 Floyd-Steinberg 误差扩散。
	palette, pix, stride := [][4]int32(nil), []byte(nil), 0
	if p, ok := dst.(*image.Paletted); ok {
		palette = make([][4]int32, len(p.Palette))
		for i, col := range p.Palette {
			r, g, b, a := col.RGBA()
			palette[i][0] = int32(r)
			palette[i][1] = int32(g)
			palette[i][2] = int32(b)
			palette[i][3] = int32(a)
		}
		pix, stride = p.Pix[p.PixOffset(r.Min.X, r.Min.Y):], p.Stride
	}

	// quantErrorCurr 和 quantErrorNext 是已传播到当前行和下一行像素的
	// Floyd-Steinberg 量化误差。+2 简化了边缘附近的计算。
	var quantErrorCurr, quantErrorNext [][4]int32
	if floydSteinberg {
		quantErrorCurr = make([][4]int32, r.Dx()+2)
		quantErrorNext = make([][4]int32, r.Dx()+2)
	}
	pxRGBA := func(x, y int) (r, g, b, a uint32) { return src.At(x, y).RGBA() }
	// 特殊情况的快速路径，以避免过度使用 color.Color 接口，
	// 该接口会逃逸到堆上，但需要为 r 上的每个像素发现。
	// 另请参见 https://golang.org/issues/15759。
	switch src0 := src.(type) {
	case *image.RGBA:
		pxRGBA = func(x, y int) (r, g, b, a uint32) { return src0.RGBAAt(x, y).RGBA() }
	case *image.NRGBA:
		pxRGBA = func(x, y int) (r, g, b, a uint32) { return src0.NRGBAAt(x, y).RGBA() }
	case *image.YCbCr:
		pxRGBA = func(x, y int) (r, g, b, a uint32) { return src0.YCbCrAt(x, y).RGBA() }
	}

	// 遍历每个源像素。
	out := color.RGBA64{A: 0xffff}
	for y := 0; y != r.Dy(); y++ {
		for x := 0; x != r.Dx(); x++ {
			// er、eg 和 eb 是像素的 R、G、B 值加上
			// 可选的 Floyd-Steinberg 误差。
			sr, sg, sb, sa := pxRGBA(sp.X+x, sp.Y+y)
			er, eg, eb, ea := int32(sr), int32(sg), int32(sb), int32(sa)
			if floydSteinberg {
				er = clamp(er + quantErrorCurr[x+1][0]/16)
				eg = clamp(eg + quantErrorCurr[x+1][1]/16)
				eb = clamp(eb + quantErrorCurr[x+1][2]/16)
				ea = clamp(ea + quantErrorCurr[x+1][3]/16)
			}

			if palette != nil {
				// 在欧几里得 R、G、B、A 空间中找到最接近的调色板颜色：
				// 使差的平方和最小化的那个。
				// TODO(nigeltao): 考虑更智能的算法。
				bestIndex, bestSum := 0, uint32(1<<32-1)
				for index, p := range palette {
					sum := sqDiff(er, p[0]) + sqDiff(eg, p[1]) + sqDiff(eb, p[2]) + sqDiff(ea, p[3])
					if sum < bestSum {
						bestIndex, bestSum = index, sum
						if sum == 0 {
							break
						}
					}
				}
				pix[y*stride+x] = byte(bestIndex)

				if !floydSteinberg {
					continue
				}
				er -= palette[bestIndex][0]
				eg -= palette[bestIndex][1]
				eb -= palette[bestIndex][2]
				ea -= palette[bestIndex][3]

			} else {
				out.R = uint16(er)
				out.G = uint16(eg)
				out.B = uint16(eb)
				out.A = uint16(ea)
				// 第三个参数是 &out 而不是 out（且 out 在内层循环外声明），
				// 以避免在 sizeof(color.RGBA64) > sizeof(uintptr) 时，
				// 此处到 color.Color 的隐式转换在内层循环中分配内存。
				dst.Set(r.Min.X+x, r.Min.Y+y, &out)

				if !floydSteinberg {
					continue
				}
				sr, sg, sb, sa = dst.At(r.Min.X+x, r.Min.Y+y).RGBA()
				er -= int32(sr)
				eg -= int32(sg)
				eb -= int32(sb)
				ea -= int32(sa)
			}

			// 传播 Floyd-Steinberg 量化误差。
			quantErrorNext[x+0][0] += er * 3
			quantErrorNext[x+0][1] += eg * 3
			quantErrorNext[x+0][2] += eb * 3
			quantErrorNext[x+0][3] += ea * 3
			quantErrorNext[x+1][0] += er * 5
			quantErrorNext[x+1][1] += eg * 5
			quantErrorNext[x+1][2] += eb * 5
			quantErrorNext[x+1][3] += ea * 5
			quantErrorNext[x+2][0] += er * 1
			quantErrorNext[x+2][1] += eg * 1
			quantErrorNext[x+2][2] += eb * 1
			quantErrorNext[x+2][3] += ea * 1
			quantErrorCurr[x+2][0] += er * 7
			quantErrorCurr[x+2][1] += eg * 7
			quantErrorCurr[x+2][2] += eb * 7
			quantErrorCurr[x+2][3] += ea * 7
		}

		// 回收量化误差缓冲区。
		if floydSteinberg {
			quantErrorCurr, quantErrorNext = quantErrorNext, quantErrorCurr
			clear(quantErrorNext)
		}
	}
}
