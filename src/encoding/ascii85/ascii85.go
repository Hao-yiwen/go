// 版权所有 2009 The Go Authors。保留所有权利。
// 本源代码的使用受 BSD 风格许可证约束，
// 该许可证可在 LICENSE 文件中找到。

// ascii85 包实现了 ascii85 数据编码，
// 该编码用于 btoa 工具以及 Adobe 的 PostScript 和 PDF 文档格式。
package ascii85

import (
	"io"
	"strconv"
)

/*
 * 编码器
 */

// Encode 将 src 编码到 dst 中，最多写入 [MaxEncodedLen](len(src)) 个字节，
// 返回实际写入的字节数。
//
// 编码按 4 字节块处理，对最后的片段使用特殊编码，
// 因此 Encode 不适合用于大数据流的单个块。请改用 [NewEncoder]。
//
// 通常，ascii85 编码的数据会用 <~ 和 ~> 符号包装。
// Encode 不会添加这些符号。
func Encode(dst, src []byte) int {
	if len(src) == 0 {
		return 0
	}

	n := 0
	for len(src) > 0 {
		dst[0] = 0
		dst[1] = 0
		dst[2] = 0
		dst[3] = 0
		dst[4] = 0

		// 将 4 字节解包为 uint32，然后重新打包为 base 85 的 5 字节。
		var v uint32
		switch len(src) {
		default:
			v |= uint32(src[3])
			fallthrough
		case 3:
			v |= uint32(src[2]) << 8
			fallthrough
		case 2:
			v |= uint32(src[1]) << 16
			fallthrough
		case 1:
			v |= uint32(src[0]) << 24
		}

		// 特殊情况：零（!!!!!）缩写为 z。
		if v == 0 && len(src) >= 4 {
			dst[0] = 'z'
			dst = dst[1:]
			src = src[4:]
			n++
			continue
		}

		// 否则，从 ! 开始的 5 个 base 85 数字。
		for i := 4; i >= 0; i-- {
			dst[i] = '!' + byte(v%85)
			v /= 85
		}

		// 如果 src 较短，丢弃低位目标字节。
		m := 5
		if len(src) < 4 {
			m -= 4 - len(src)
			src = nil
		} else {
			src = src[4:]
		}
		dst = dst[m:]
		n += m
	}
	return n
}

// MaxEncodedLen 返回 n 个源字节编码后的最大长度。
func MaxEncodedLen(n int) int { return (n + 3) / 4 * 5 }

// NewEncoder 返回一个新的 ascii85 流编码器。写入返回的 writer 的数据
// 将被编码后写入 w。
// Ascii85 编码按 32 位块操作；写入完成后，
// 调用者必须 Close 返回的编码器以刷新任何尾部的不完整块。
func NewEncoder(w io.Writer) io.WriteCloser { return &encoder{w: w} }

type encoder struct {
	err  error
	w    io.Writer
	buf  [4]byte    // 等待编码的缓冲数据
	nbuf int        // buf 中的字节数
	out  [1024]byte // 输出缓冲区
}

func (e *encoder) Write(p []byte) (n int, err error) {
	if e.err != nil {
		return 0, e.err
	}

	// 前导碎片。
	if e.nbuf > 0 {
		var i int
		for i = 0; i < len(p) && e.nbuf < 4; i++ {
			e.buf[e.nbuf] = p[i]
			e.nbuf++
		}
		n += i
		p = p[i:]
		if e.nbuf < 4 {
			return
		}
		nout := Encode(e.out[0:], e.buf[0:])
		if _, e.err = e.w.Write(e.out[0:nout]); e.err != nil {
			return n, e.err
		}
		e.nbuf = 0
	}

	// 大的内部数据块。
	for len(p) >= 4 {
		nn := len(e.out) / 5 * 4
		if nn > len(p) {
			nn = len(p)
		}
		nn -= nn % 4
		if nn > 0 {
			nout := Encode(e.out[0:], p[0:nn])
			if _, e.err = e.w.Write(e.out[0:nout]); e.err != nil {
				return n, e.err
			}
		}
		n += nn
		p = p[nn:]
	}

	// 尾部碎片。
	copy(e.buf[:], p)
	e.nbuf = len(p)
	n += len(p)
	return
}

// Close 刷新编码器中任何待处理的输出。
// 在调用 Close 后调用 Write 是错误的。
func (e *encoder) Close() error {
	// 如果缓冲区中还有剩余内容，将其刷新输出
	if e.err == nil && e.nbuf > 0 {
		nout := Encode(e.out[0:], e.buf[0:e.nbuf])
		e.nbuf = 0
		_, e.err = e.w.Write(e.out[0:nout])
	}
	return e.err
}

/*
 * 解码器
 */

type CorruptInputError int64

func (e CorruptInputError) Error() string {
	return "illegal ascii85 data at input byte " + strconv.FormatInt(int64(e), 10)
}

// Decode 将 src 解码到 dst 中，返回写入 dst 的字节数和从 src 消耗的字节数。
// 如果 src 包含无效的 ascii85 数据，Decode 将返回成功写入的字节数
// 和一个 [CorruptInputError]。
// Decode 忽略 src 中的空格和控制字符。
// 通常，ascii85 编码的数据会用 <~ 和 ~> 符号包装。
// Decode 期望调用者已经去除了这些符号。
//
// 如果 flush 为 true，Decode 假定 src 表示输入流的末尾，
// 并完全处理它，而不是等待另一个 32 位块的完成。
//
// [NewDecoder] 在 Decode 周围包装了一个 [io.Reader] 接口。
func Decode(dst, src []byte, flush bool) (ndst, nsrc int, err error) {
	var v uint32
	var nb int
	for i, b := range src {
		if len(dst)-ndst < 4 {
			return
		}
		switch {
		case b <= ' ':
			continue
		case b == 'z' && nb == 0:
			nb = 5
			v = 0
		case '!' <= b && b <= 'u':
			v = v*85 + uint32(b-'!')
			nb++
		default:
			return 0, 0, CorruptInputError(i)
		}
		if nb == 5 {
			nsrc = i + 1
			dst[ndst] = byte(v >> 24)
			dst[ndst+1] = byte(v >> 16)
			dst[ndst+2] = byte(v >> 8)
			dst[ndst+3] = byte(v)
			ndst += 4
			nb = 0
			v = 0
		}
	}
	if flush {
		nsrc = len(src)
		if nb > 0 {
			// 最后片段中的输出字节数
			// 是剩余输入字节数减 1：
			// 额外的字节提供了足够的位来覆盖
			// 该块编码的低效性。
			if nb == 1 {
				return 0, 0, CorruptInputError(len(src))
			}
			for i := nb; i < 5; i++ {
				// 短编码截断了输出值。
				// 我们必须假设最坏情况的值（数字 84）
				// 以确保高位是正确的。
				v = v*85 + 84
			}
			for i := 0; i < nb-1; i++ {
				dst[ndst] = byte(v >> 24)
				v <<= 8
				ndst++
			}
		}
	}
	return
}

// NewDecoder 构造一个新的 ascii85 流解码器。
func NewDecoder(r io.Reader) io.Reader { return &decoder{r: r} }

type decoder struct {
	err     error
	readErr error
	r       io.Reader
	buf     [1024]byte // 剩余输入
	nbuf    int
	out     []byte // 剩余解码输出
	outbuf  [1024]byte
}

func (d *decoder) Read(p []byte) (n int, err error) {
	if len(p) == 0 {
		return 0, nil
	}
	if d.err != nil {
		return 0, d.err
	}

	for {
		// 复制上次解码的剩余输出。
		if len(d.out) > 0 {
			n = copy(p, d.out)
			d.out = d.out[n:]
			return
		}

		// 解码上次读取的剩余输入。
		var nn, nsrc, ndst int
		if d.nbuf > 0 {
			ndst, nsrc, d.err = Decode(d.outbuf[0:], d.buf[0:d.nbuf], d.readErr != nil)
			if ndst > 0 {
				d.out = d.outbuf[0:ndst]
				d.nbuf = copy(d.buf[0:], d.buf[nsrc:d.nbuf])
				continue // 复制输出并返回
			}
			if ndst == 0 && d.err == nil {
				// 特殊情况：输入缓冲区大部分被非数据字节填充。
				// 过滤掉这些字节以腾出空间容纳更多输入。
				off := 0
				for i := 0; i < d.nbuf; i++ {
					if d.buf[i] > ' ' {
						d.buf[off] = d.buf[i]
						off++
					}
				}
				d.nbuf = off
			}
		}

		// 输入用尽，解码输出用尽。检查错误。
		if d.err != nil {
			return 0, d.err
		}
		if d.readErr != nil {
			d.err = d.readErr
			return 0, d.err
		}

		// 读取更多数据。
		nn, d.readErr = d.r.Read(d.buf[d.nbuf:])
		d.nbuf += nn
	}
}
