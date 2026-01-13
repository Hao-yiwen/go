// 版权所有 2009 The Go Authors。保留所有权利。
// 本源代码的使用受 BSD 风格许可证约束，
// 该许可证可在 LICENSE 文件中找到。

package png

import (
	"bufio"
	"compress/zlib"
	"encoding/binary"
	"hash/crc32"
	"image"
	"image/color"
	"io"
	"strconv"
)

// Encoder 配置编码 PNG 图像。
type Encoder struct {
	CompressionLevel CompressionLevel

	// BufferPool 可选择指定一个缓冲池以在编码图像时获取临时的
	// EncoderBuffers。
	BufferPool EncoderBufferPool
}

// EncoderBufferPool 是一个用于获取和返回 [EncoderBuffer] 结构体的临时
// 实例的接口。这可以用于在编码多个图像时重用缓冲区。
type EncoderBufferPool interface {
	Get() *EncoderBuffer
	Put(*EncoderBuffer)
}

// EncoderBuffer 保存用于编码 PNG 图像的缓冲区。
type EncoderBuffer encoder

type encoder struct {
	enc     *Encoder
	w       io.Writer
	m       image.Image
	cb      int
	err     error
	header  [8]byte
	footer  [4]byte
	tmp     [4 * 256]byte
	cr      [nFilter][]uint8
	pr      []uint8
	zw      *zlib.Writer
	zwLevel int
	bw      *bufio.Writer
}

// CompressionLevel 指示压缩级别。
type CompressionLevel int

const (
	DefaultCompression CompressionLevel = 0
	NoCompression      CompressionLevel = -1
	BestSpeed          CompressionLevel = -2
	BestCompression    CompressionLevel = -3

	// 正的 CompressionLevel 值保留用于表示数字 zlib
	// 压缩级别，尽管目前尚未实现。
)

type opaquer interface {
	Opaque() bool
}

// 返回图像是否完全不透明。
func opaque(m image.Image) bool {
	if o, ok := m.(opaquer); ok {
		return o.Opaque()
	}
	b := m.Bounds()
	for y := b.Min.Y; y < b.Max.Y; y++ {
		for x := b.Min.X; x < b.Max.X; x++ {
			_, _, _, a := m.At(x, y).RGBA()
			if a != 0xffff {
				return false
			}
		}
	}
	return true
}

// 将字节解释为有符号 int8 的绝对值。
func abs8(d uint8) int {
	if d < 128 {
		return int(d)
	}
	return 256 - int(d)
}

func (e *encoder) writeChunk(b []byte, name string) {
	if e.err != nil {
		return
	}
	n := uint32(len(b))
	if int(n) != len(b) {
		e.err = UnsupportedError(name + " chunk is too large: " + strconv.Itoa(len(b)))
		return
	}
	binary.BigEndian.PutUint32(e.header[:4], n)
	e.header[4] = name[0]
	e.header[5] = name[1]
	e.header[6] = name[2]
	e.header[7] = name[3]
	crc := crc32.NewIEEE()
	crc.Write(e.header[4:8])
	crc.Write(b)
	binary.BigEndian.PutUint32(e.footer[:4], crc.Sum32())

	_, e.err = e.w.Write(e.header[:8])
	if e.err != nil {
		return
	}
	_, e.err = e.w.Write(b)
	if e.err != nil {
		return
	}
	_, e.err = e.w.Write(e.footer[:4])
}

func (e *encoder) writeIHDR() {
	b := e.m.Bounds()
	binary.BigEndian.PutUint32(e.tmp[0:4], uint32(b.Dx()))
	binary.BigEndian.PutUint32(e.tmp[4:8], uint32(b.Dy()))
	// 设置位深度和颜色类型。
	switch e.cb {
	case cbG8:
		e.tmp[8] = 8
		e.tmp[9] = ctGrayscale
	case cbTC8:
		e.tmp[8] = 8
		e.tmp[9] = ctTrueColor
	case cbP8:
		e.tmp[8] = 8
		e.tmp[9] = ctPaletted
	case cbP4:
		e.tmp[8] = 4
		e.tmp[9] = ctPaletted
	case cbP2:
		e.tmp[8] = 2
		e.tmp[9] = ctPaletted
	case cbP1:
		e.tmp[8] = 1
		e.tmp[9] = ctPaletted
	case cbTCA8:
		e.tmp[8] = 8
		e.tmp[9] = ctTrueColorAlpha
	case cbG16:
		e.tmp[8] = 16
		e.tmp[9] = ctGrayscale
	case cbTC16:
		e.tmp[8] = 16
		e.tmp[9] = ctTrueColor
	case cbTCA16:
		e.tmp[8] = 16
		e.tmp[9] = ctTrueColorAlpha
	}
	e.tmp[10] = 0 // 默认压缩方法
	e.tmp[11] = 0 // 默认过滤方法
	e.tmp[12] = 0 // 非交错
	e.writeChunk(e.tmp[:13], "IHDR")
}

func (e *encoder) writePLTEAndTRNS(p color.Palette) {
	if len(p) < 1 || len(p) > 256 {
		e.err = FormatError("bad palette length: " + strconv.Itoa(len(p)))
		return
	}
	last := -1
	for i, c := range p {
		c1 := color.NRGBAModel.Convert(c).(color.NRGBA)
		e.tmp[3*i+0] = c1.R
		e.tmp[3*i+1] = c1.G
		e.tmp[3*i+2] = c1.B
		if c1.A != 0xff {
			last = i
		}
		e.tmp[3*256+i] = c1.A
	}
	e.writeChunk(e.tmp[:3*len(p)], "PLTE")
	if last != -1 {
		e.writeChunk(e.tmp[3*256:3*256+1+last], "tRNS")
	}
}

// encoder 是一个 io.Writer，通过写入 PNG IDAT 块来满足写入，
// 包括每个 Write 调用的 8 字节头和 4 字节 CRC 校验和。这样的调用
// 应该相对不频繁，因为 writeIDATs 使用了 [bufio.Writer]。
//
// 这个方法应该只从 writeIDATs（通过 writeImage）调用。
// 其他代码不应该将 encoder 视为 io.Writer。
func (e *encoder) Write(b []byte) (int, error) {
	e.writeChunk(b, "IDAT")
	if e.err != nil {
		return 0, e.err
	}
	return len(b), nil
}

// 选择用于编码当前行的过滤器，并应用它。
// 返回值是过滤器的索引，也是 cr 中已应用过滤器的行的索引。
func filter(cr *[nFilter][]byte, pr []byte, bpp int) int {
	// 我们尝试所有五种过滤器类型，并选择能最小化绝对差值之和的类型。
	// 这与 libpng 使用的启发式方法相同，尽管尝试过滤器的顺序是
	// 根据估计最有可能最小化（ftUp、ftPaeth、ftNone、ftSub、ftAverage）
	// 的顺序，而不是按它们的枚举顺序（ftNone、ftSub、ftUp、ftAverage、ftPaeth）。
	cdat0 := cr[0][1:]
	cdat1 := cr[1][1:]
	cdat2 := cr[2][1:]
	cdat3 := cr[3][1:]
	cdat4 := cr[4][1:]
	pdat := pr[1:]
	n := len(cdat0)

	// 向上过滤器。
	sum := 0
	for i := 0; i < n; i++ {
		cdat2[i] = cdat0[i] - pdat[i]
		sum += abs8(cdat2[i])
	}
	best := sum
	filter := ftUp

	// Paeth 过滤器。
	sum = 0
	for i := 0; i < bpp; i++ {
		cdat4[i] = cdat0[i] - pdat[i]
		sum += abs8(cdat4[i])
	}
	for i := bpp; i < n; i++ {
		cdat4[i] = cdat0[i] - paeth(cdat0[i-bpp], pdat[i], pdat[i-bpp])
		sum += abs8(cdat4[i])
		if sum >= best {
			break
		}
	}
	if sum < best {
		best = sum
		filter = ftPaeth
	}

	// 无过滤器。
	sum = 0
	for i := 0; i < n; i++ {
		sum += abs8(cdat0[i])
		if sum >= best {
			break
		}
	}
	if sum < best {
		best = sum
		filter = ftNone
	}

	// 子过滤器。
	sum = 0
	for i := 0; i < bpp; i++ {
		cdat1[i] = cdat0[i]
		sum += abs8(cdat1[i])
	}
	for i := bpp; i < n; i++ {
		cdat1[i] = cdat0[i] - cdat0[i-bpp]
		sum += abs8(cdat1[i])
		if sum >= best {
			break
		}
	}
	if sum < best {
		best = sum
		filter = ftSub
	}

	// 平均过滤器。
	sum = 0
	for i := 0; i < bpp; i++ {
		cdat3[i] = cdat0[i] - pdat[i]/2
		sum += abs8(cdat3[i])
	}
	for i := bpp; i < n; i++ {
		cdat3[i] = cdat0[i] - uint8((int(cdat0[i-bpp])+int(pdat[i]))/2)
		sum += abs8(cdat3[i])
		if sum >= best {
			break
		}
	}
	if sum < best {
		filter = ftAverage
	}

	return filter
}

func (e *encoder) writeImage(w io.Writer, m image.Image, cb int, level int) error {
	if e.zw == nil || e.zwLevel != level {
		zw, err := zlib.NewWriterLevel(w, level)
		if err != nil {
			return err
		}
		e.zw = zw
		e.zwLevel = level
	} else {
		e.zw.Reset(w)
	}
	defer e.zw.Close()

	bitsPerPixel := 0

	switch cb {
	case cbG8:
		bitsPerPixel = 8
	case cbTC8:
		bitsPerPixel = 24
	case cbP8:
		bitsPerPixel = 8
	case cbP4:
		bitsPerPixel = 4
	case cbP2:
		bitsPerPixel = 2
	case cbP1:
		bitsPerPixel = 1
	case cbTCA8:
		bitsPerPixel = 32
	case cbTC16:
		bitsPerPixel = 48
	case cbTCA16:
		bitsPerPixel = 64
	case cbG16:
		bitsPerPixel = 16
	}

	// cr[*] 和 pr 是当前和前一行的字节。
	// cr[0] 是未过滤的（或等效地，用 ftNone 过滤器过滤）。
	// cr[ft]，对于非零过滤器类型 ft，是用于在其他 PNG 过滤器类型下转换 cr[0] 的缓冲区。
	// 这些缓冲区分配一次并为每一行重新使用。
	// +1 是为了每行过滤器类型，它在 cr[*][0]。
	b := m.Bounds()
	sz := 1 + (bitsPerPixel*b.Dx()+7)/8
	for i := range e.cr {
		if cap(e.cr[i]) < sz {
			e.cr[i] = make([]uint8, sz)
		} else {
			e.cr[i] = e.cr[i][:sz]
		}
		e.cr[i][0] = uint8(i)
	}
	cr := e.cr
	if cap(e.pr) < sz {
		e.pr = make([]uint8, sz)
	} else {
		e.pr = e.pr[:sz]
		clear(e.pr)
	}
	pr := e.pr

	gray, _ := m.(*image.Gray)
	rgba, _ := m.(*image.RGBA)
	paletted, _ := m.(*image.Paletted)
	nrgba, _ := m.(*image.NRGBA)

	for y := b.Min.Y; y < b.Max.Y; y++ {
		// 从颜色转换为字节。
		i := 1
		switch cb {
		case cbG8:
			if gray != nil {
				offset := (y - b.Min.Y) * gray.Stride
				copy(cr[0][1:], gray.Pix[offset:offset+b.Dx()])
			} else {
				for x := b.Min.X; x < b.Max.X; x++ {
					c := color.GrayModel.Convert(m.At(x, y)).(color.Gray)
					cr[0][i] = c.Y
					i++
				}
			}
		case cbTC8:
			// 我们之前已验证 alpha 值完全不透明。
			cr0 := cr[0]
			stride, pix := 0, []byte(nil)
			if rgba != nil {
				stride, pix = rgba.Stride, rgba.Pix
			} else if nrgba != nil {
				stride, pix = nrgba.Stride, nrgba.Pix
			}
			if stride != 0 {
				j0 := (y - b.Min.Y) * stride
				j1 := j0 + b.Dx()*4
				for j := j0; j < j1; j += 4 {
					cr0[i+0] = pix[j+0]
					cr0[i+1] = pix[j+1]
					cr0[i+2] = pix[j+2]
					i += 3
				}
			} else {
				for x := b.Min.X; x < b.Max.X; x++ {
					r, g, b, _ := m.At(x, y).RGBA()
					cr0[i+0] = uint8(r >> 8)
					cr0[i+1] = uint8(g >> 8)
					cr0[i+2] = uint8(b >> 8)
					i += 3
				}
			}
		case cbP8:
			if paletted != nil {
				offset := (y - b.Min.Y) * paletted.Stride
				copy(cr[0][1:], paletted.Pix[offset:offset+b.Dx()])
			} else {
				pi := m.(image.PalettedImage)
				for x := b.Min.X; x < b.Max.X; x++ {
					cr[0][i] = pi.ColorIndexAt(x, y)
					i += 1
				}
			}

		case cbP4, cbP2, cbP1:
			pi := m.(image.PalettedImage)

			var a uint8
			var c int
			pixelsPerByte := 8 / bitsPerPixel
			for x := b.Min.X; x < b.Max.X; x++ {
				a = a<<uint(bitsPerPixel) | pi.ColorIndexAt(x, y)
				c++
				if c == pixelsPerByte {
					cr[0][i] = a
					i += 1
					a = 0
					c = 0
				}
			}
			if c != 0 {
				for c != pixelsPerByte {
					a = a << uint(bitsPerPixel)
					c++
				}
				cr[0][i] = a
			}

		case cbTCA8:
			if nrgba != nil {
				offset := (y - b.Min.Y) * nrgba.Stride
				copy(cr[0][1:], nrgba.Pix[offset:offset+b.Dx()*4])
			} else if rgba != nil {
				dst := cr[0][1:]
				src := rgba.Pix[rgba.PixOffset(b.Min.X, y):rgba.PixOffset(b.Max.X, y)]
				for ; len(src) >= 4; dst, src = dst[4:], src[4:] {
					d := (*[4]byte)(dst)
					s := (*[4]byte)(src)
					if s[3] == 0x00 {
						d[0] = 0
						d[1] = 0
						d[2] = 0
						d[3] = 0
					} else if s[3] == 0xff {
						copy(d[:], s[:])
					} else {
						// 这段代码做的事情与 color.NRGBAModel.Convert(
						// rgba.At(x, y)).(color.NRGBA) 相同，但没有额外的内存
						// 分配或接口/函数调用开销。
						//
						// 乘数 m 结合了 0x101（将 8 位颜色转换为 16 位颜色）
						// 和 0xffff（当与除以 a 结合时，从 alpha 预乘
						// 转换为非 alpha 预乘）。
						const m = 0x101 * 0xffff
						a := uint32(s[3]) * 0x101
						d[0] = uint8((uint32(s[0]) * m / a) >> 8)
						d[1] = uint8((uint32(s[1]) * m / a) >> 8)
						d[2] = uint8((uint32(s[2]) * m / a) >> 8)
						d[3] = s[3]
					}
				}
			} else {
				// 从 image.Image（alpha 预乘）转换为 PNG 的非 alpha 预乘。
				for x := b.Min.X; x < b.Max.X; x++ {
					c := color.NRGBAModel.Convert(m.At(x, y)).(color.NRGBA)
					cr[0][i+0] = c.R
					cr[0][i+1] = c.G
					cr[0][i+2] = c.B
					cr[0][i+3] = c.A
					i += 4
				}
			}
		case cbG16:
			for x := b.Min.X; x < b.Max.X; x++ {
				c := color.Gray16Model.Convert(m.At(x, y)).(color.Gray16)
				cr[0][i+0] = uint8(c.Y >> 8)
				cr[0][i+1] = uint8(c.Y)
				i += 2
			}
		case cbTC16:
			// 我们之前已验证 alpha 值完全不透明。
			for x := b.Min.X; x < b.Max.X; x++ {
				r, g, b, _ := m.At(x, y).RGBA()
				cr[0][i+0] = uint8(r >> 8)
				cr[0][i+1] = uint8(r)
				cr[0][i+2] = uint8(g >> 8)
				cr[0][i+3] = uint8(g)
				cr[0][i+4] = uint8(b >> 8)
				cr[0][i+5] = uint8(b)
				i += 6
			}
		case cbTCA16:
			// 从 image.Image（alpha 预乘）转换为 PNG 的非 alpha 预乘。
			for x := b.Min.X; x < b.Max.X; x++ {
				c := color.NRGBA64Model.Convert(m.At(x, y)).(color.NRGBA64)
				cr[0][i+0] = uint8(c.R >> 8)
				cr[0][i+1] = uint8(c.R)
				cr[0][i+2] = uint8(c.G >> 8)
				cr[0][i+3] = uint8(c.G)
				cr[0][i+4] = uint8(c.B >> 8)
				cr[0][i+5] = uint8(c.B)
				cr[0][i+6] = uint8(c.A >> 8)
				cr[0][i+7] = uint8(c.A)
				i += 8
			}
		}

		// 应用过滤器。
		// 跳过 NoCompression 和调色板图像（cbP8）的过滤，因为
		// "过滤器很少对调色板图像有用"，并且会导致
		// 文件变大（见 http://www.libpng.org/pub/png/book/chapter09.html）。
		f := ftNone
		if level != zlib.NoCompression && cb != cbP8 && cb != cbP4 && cb != cbP2 && cb != cbP1 {
			// 由于我们跳过调色板图像，我们不必担心
			// bitsPerPixel 不是 8 的倍数
			bpp := bitsPerPixel / 8
			f = filter(&cr, pr, bpp)
		}

		// 写入压缩字节。
		if _, err := e.zw.Write(cr[f]); err != nil {
			return err
		}

		// y 的当前行是 y+1 的前一行。
		pr, cr[0] = cr[0], pr
	}
	return nil
}

// 将实际的图像数据写入一个或多个 IDAT 块。
func (e *encoder) writeIDATs() {
	if e.err != nil {
		return
	}
	if e.bw == nil {
		e.bw = bufio.NewWriterSize(e, 1<<15)
	} else {
		e.bw.Reset(e)
	}
	e.err = e.writeImage(e.bw, e.m, e.cb, levelToZlib(e.enc.CompressionLevel))
	if e.err != nil {
		return
	}
	e.err = e.bw.Flush()
}

// 需要此函数是因为我们希望
// Encoder.CompressionLevel 的零值映射到 zlib.DefaultCompression。
func levelToZlib(l CompressionLevel) int {
	switch l {
	case DefaultCompression:
		return zlib.DefaultCompression
	case NoCompression:
		return zlib.NoCompression
	case BestSpeed:
		return zlib.BestSpeed
	case BestCompression:
		return zlib.BestCompression
	default:
		return zlib.DefaultCompression
	}
}

func (e *encoder) writeIEND() { e.writeChunk(nil, "IEND") }

// Encode 将图像 m 以 PNG 格式写入 w。任何图像都可以
// 编码，但不是 [image.NRGBA] 的图像可能会有损编码。
func Encode(w io.Writer, m image.Image) error {
	var e Encoder
	return e.Encode(w, m)
}

// Encode 将图像 m 以 PNG 格式写入 w。
func (enc *Encoder) Encode(w io.Writer, m image.Image) error {
	// 显然，负的宽度和高度是无效的。此外，PNG
	// 规范第 11.2.2 节说零是无效的。过大的图像也会被
	// 拒绝。
	mw, mh := int64(m.Bounds().Dx()), int64(m.Bounds().Dy())
	if mw <= 0 || mh <= 0 || mw >= 1<<32 || mh >= 1<<32 {
		return FormatError("invalid image size: " + strconv.FormatInt(mw, 10) + "x" + strconv.FormatInt(mh, 10))
	}

	var e *encoder
	if enc.BufferPool != nil {
		buffer := enc.BufferPool.Get()
		e = (*encoder)(buffer)

	}
	if e == nil {
		e = &encoder{}
	}
	if enc.BufferPool != nil {
		defer enc.BufferPool.Put((*EncoderBuffer)(e))
	}

	e.enc = enc
	e.w = w
	e.m = m

	var pal color.Palette
	// cbP8 编码需要 PalettedImage 的 ColorIndexAt 方法。
	if _, ok := m.(image.PalettedImage); ok {
		pal, _ = m.ColorModel().(color.Palette)
	}
	if pal != nil {
		if len(pal) <= 2 {
			e.cb = cbP1
		} else if len(pal) <= 4 {
			e.cb = cbP2
		} else if len(pal) <= 16 {
			e.cb = cbP4
		} else {
			e.cb = cbP8
		}
	} else {
		switch m.ColorModel() {
		case color.GrayModel:
			e.cb = cbG8
		case color.Gray16Model:
			e.cb = cbG16
		case color.RGBAModel, color.NRGBAModel, color.AlphaModel:
			if opaque(m) {
				e.cb = cbTC8
			} else {
				e.cb = cbTCA8
			}
		default:
			if opaque(m) {
				e.cb = cbTC16
			} else {
				e.cb = cbTCA16
			}
		}
	}

	_, e.err = io.WriteString(w, pngHeader)
	e.writeIHDR()
	if pal != nil {
		e.writePLTEAndTRNS(pal)
	}
	e.writeIDATs()
	e.writeIEND()
	return e.err
}
