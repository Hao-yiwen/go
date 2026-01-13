// 版权所有 2011 The Go Authors。保留所有权利。
// 本源代码的使用受 BSD 风格许可证约束，
// 该许可证可在 LICENSE 文件中找到。

// Package lzw 实现 Lempel-Ziv-Welch 压缩数据格式，
// 如 T. A. Welch 所述，"高性能数据压缩技术"，Computer, 17(6)（1984 年 6 月），pp 8-19。
//
// 特别是，它实现了 GIF 和 PDF 文件格式使用的 LZW，
// 这意味着可变宽度码最多 12 位，前两个非字面码是清除码和 EOF 码。
//
// TIFF 文件格式使用 LZW 算法的类似但不兼容的版本。
// 参见 [golang.org/x/image/tiff/lzw] 包以获得实现。
package lzw

// TODO(nigeltao): 检查 PDF 是否以与 GIF 相同的方式使用 LZW，
// 修正 LSB/MSB 打包顺序。

import (
	"bufio"
	"errors"
	"fmt"
	"io"
)

// Order 指定 LZW 数据流中的位顺序。
type Order int

const (
	// LSB 表示最低有效位优先，用于 GIF 文件格式。
	LSB Order = iota
	// MSB 表示最高有效位优先，用于 TIFF 和 PDF 文件格式。
	MSB
)

const (
	maxWidth           = 12
	decoderInvalidCode = 0xffff
	flushBuffer        = 1 << maxWidth
)

// Reader 是一个 [io.Reader]，可用于读取 LZW 格式的压缩数据。
type Reader struct {
	r        io.ByteReader
	bits     uint32
	nBits    uint
	width    uint
	read     func(*Reader) (uint16, error) // readLSB 或 readMSB
	litWidth int                           // 字面码的位宽
	err      error

	// 前 1<<litWidth 个码是字面码。
	// 接下来的两个码表示清除和 EOF。
	// 其他有效码在范围 [lo, hi] 内，其中 lo := clear + 2，
	// 每次看到代码时上界递增。
	//
	// overflow 是 hi 溢出代码宽度的码。
	// 它总是等于 1 << width。
	//
	// last 是最近看到的码，或 decoderInvalidCode。
	//
	// 一个不变量是 hi < overflow。
	clear, eof, hi, overflow, last uint16

	// [lo, hi] 中的每个码 c 扩展为两个或更多字节。对于 c != hi：
	//   suffix[c] 是这些字节中的最后一个。
	//   prefix[c] 是除最后一个字节外所有字节的码。
	//   此码可以是字面码或 [lo, c) 中的另一个码。
	// c == hi 的情况是特殊情况。
	suffix [1 << maxWidth]uint8
	prefix [1 << maxWidth]uint16

	// output 是临时输出缓冲区。
	// 字面码从缓冲区的开始累积。
	// 非字面码解码为后缀序列，首先从缓冲区末尾向左向右书写，然后被复制
	// to the start of the buffer.
	// It is flushed when it contains >= 1<<maxWidth bytes,
	// so that there is always room to decode an entire code.
	output [2 * 1 << maxWidth]byte
	o      int    // write index into output
	toRead []byte // bytes to return from Read
}

// readLSB returns the next code for "Least Significant Bits first" data.
func (r *Reader) readLSB() (uint16, error) {
	for r.nBits < r.width {
		x, err := r.r.ReadByte()
		if err != nil {
			return 0, err
		}
		r.bits |= uint32(x) << r.nBits
		r.nBits += 8
	}
	code := uint16(r.bits & (1<<r.width - 1))
	r.bits >>= r.width
	r.nBits -= r.width
	return code, nil
}

// readMSB returns the next code for "Most Significant Bits first" data.
func (r *Reader) readMSB() (uint16, error) {
	for r.nBits < r.width {
		x, err := r.r.ReadByte()
		if err != nil {
			return 0, err
		}
		r.bits |= uint32(x) << (24 - r.nBits)
		r.nBits += 8
	}
	code := uint16(r.bits >> (32 - r.width))
	r.bits <<= r.width
	r.nBits -= r.width
	return code, nil
}

// Read implements io.Reader, reading uncompressed bytes from its underlying reader.
func (r *Reader) Read(b []byte) (int, error) {
	for {
		if len(r.toRead) > 0 {
			n := copy(b, r.toRead)
			r.toRead = r.toRead[n:]
			return n, nil
		}
		if r.err != nil {
			return 0, r.err
		}
		r.decode()
	}
}

// decode decompresses bytes from r and leaves them in d.toRead.
// read specifies how to decode bytes into codes.
// litWidth is the width in bits of literal codes.
func (r *Reader) decode() {
	// Loop over the code stream, converting codes into decompressed bytes.
loop:
	for {
		code, err := r.read(r)
		if err != nil {
			if err == io.EOF {
				err = io.ErrUnexpectedEOF
			}
			r.err = err
			break
		}
		switch {
		case code < r.clear:
			// We have a literal code.
			r.output[r.o] = uint8(code)
			r.o++
			if r.last != decoderInvalidCode {
				// Save what the hi code expands to.
				r.suffix[r.hi] = uint8(code)
				r.prefix[r.hi] = r.last
			}
		case code == r.clear:
			r.width = 1 + uint(r.litWidth)
			r.hi = r.eof
			r.overflow = 1 << r.width
			r.last = decoderInvalidCode
			continue
		case code == r.eof:
			r.err = io.EOF
			break loop
		case code <= r.hi:
			c, i := code, len(r.output)-1
			if code == r.hi && r.last != decoderInvalidCode {
				// code == hi is a special case which expands to the last expansion
				// followed by the head of the last expansion. To find the head, we walk
				// the prefix chain until we find a literal code.
				c = r.last
				for c >= r.clear {
					c = r.prefix[c]
				}
				r.output[i] = uint8(c)
				i--
				c = r.last
			}
			// Copy the suffix chain into output and then write that to w.
			for c >= r.clear {
				r.output[i] = r.suffix[c]
				i--
				c = r.prefix[c]
			}
			r.output[i] = uint8(c)
			r.o += copy(r.output[r.o:], r.output[i:])
			if r.last != decoderInvalidCode {
				// Save what the hi code expands to.
				r.suffix[r.hi] = uint8(c)
				r.prefix[r.hi] = r.last
			}
		default:
			r.err = errors.New("lzw: invalid code")
			break loop
		}
		r.last, r.hi = code, r.hi+1
		if r.hi >= r.overflow {
			if r.hi > r.overflow {
				panic("unreachable")
			}
			if r.width == maxWidth {
				r.last = decoderInvalidCode
				// Undo the d.hi++ a few lines above, so that (1) we maintain
				// the invariant that d.hi < d.overflow, and (2) d.hi does not
				// eventually overflow a uint16.
				r.hi--
			} else {
				r.width++
				r.overflow = 1 << r.width
			}
		}
		if r.o >= flushBuffer {
			break
		}
	}
	// Flush pending output.
	r.toRead = r.output[:r.o]
	r.o = 0
}

var errClosed = errors.New("lzw: reader/writer is closed")

// Close closes the [Reader] and returns an error for any future read operation.
// It does not close the underlying [io.Reader].
func (r *Reader) Close() error {
	r.err = errClosed // in case any Reads come along
	return nil
}

// Reset clears the [Reader]'s state and allows it to be reused again
// as a new [Reader].
func (r *Reader) Reset(src io.Reader, order Order, litWidth int) {
	*r = Reader{}
	r.init(src, order, litWidth)
}

// NewReader creates a new [io.ReadCloser].
// Reads from the returned [io.ReadCloser] read and decompress data from r.
// If r does not also implement [io.ByteReader],
// the decompressor may read more data than necessary from r.
// It is the caller's responsibility to call Close on the ReadCloser when
// finished reading.
// The number of bits to use for literal codes, litWidth, must be in the
// range [2,8] and is typically 8. It must equal the litWidth
// used during compression.
//
// It is guaranteed that the underlying type of the returned [io.ReadCloser]
// is a *[Reader].
func NewReader(r io.Reader, order Order, litWidth int) io.ReadCloser {
	return newReader(r, order, litWidth)
}

func newReader(src io.Reader, order Order, litWidth int) *Reader {
	r := new(Reader)
	r.init(src, order, litWidth)
	return r
}

func (r *Reader) init(src io.Reader, order Order, litWidth int) {
	switch order {
	case LSB:
		r.read = (*Reader).readLSB
	case MSB:
		r.read = (*Reader).readMSB
	default:
		r.err = errors.New("lzw: unknown order")
		return
	}
	if litWidth < 2 || 8 < litWidth {
		r.err = fmt.Errorf("lzw: litWidth %d out of range", litWidth)
		return
	}

	br, ok := src.(io.ByteReader)
	if !ok && src != nil {
		br = bufio.NewReader(src)
	}
	r.r = br
	r.litWidth = litWidth
	r.width = 1 + uint(litWidth)
	r.clear = uint16(1) << uint(litWidth)
	r.eof, r.hi = r.clear+1, r.clear+1
	r.overflow = uint16(1) << r.width
	r.last = decoderInvalidCode
}
