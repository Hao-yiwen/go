// 版权所有 2010 The Go Authors。保留所有权利。
// 本源代码的使用受 BSD 风格许可证约束，
// 该许可证可在 LICENSE 文件中找到。

package gzip

import (
	"compress/flate"
	"errors"
	"fmt"
	"hash/crc32"
	"io"
	"time"
)

// 这些常数从 [flate] 包复制，以便导入 [compress/gzip] 的代码
// 也不必导入 [compress/flate]。
const (
	NoCompression      = flate.NoCompression
	BestSpeed          = flate.BestSpeed
	BestCompression    = flate.BestCompression
	DefaultCompression = flate.DefaultCompression
	HuffmanOnly        = flate.HuffmanOnly
)

// Writer 是 [io.WriteCloser]。
// 对 Writer 的写入被压缩并写入 w。
type Writer struct {
	Header      // 在第一次调用 Write、Flush 或 Close 时写入
	w           io.Writer
	level       int
	wroteHeader bool
	closed      bool
	buf         [10]byte
	compressor  *flate.Writer
	digest      uint32 // CRC-32，IEEE 多项式（第 8 节）
	size        uint32 // 未压缩大小（第 2.3.1 节）
	err         error
}

// NewWriter 返回一个新的 [Writer]。
// 对返回的 writer 的写入被压缩并写入 w。
//
// 调用者有责任在完成时调用 [Writer] 上的 Close。
// 写入可能会被缓冲，直到 Close 才刷新。
//
// 希望在 Writer.[Header] 中设置字段的调用者必须在第一次调用 Write、Flush 或 Close 之前进行。
func NewWriter(w io.Writer) *Writer {
	z, _ := NewWriterLevel(w, DefaultCompression)
	return z
}

// NewWriterLevel 类似于 [NewWriter]，但指定压缩级别而不是假设 [DefaultCompression]。
//
// 压缩级别可以是 [DefaultCompression]、[NoCompression]、[HuffmanOnly]
// 或 [BestSpeed] 和 [BestCompression] 之间的任何整数值（含）。
// 如果级别有效，返回的错误将为 nil。
func NewWriterLevel(w io.Writer, level int) (*Writer, error) {
	if level < HuffmanOnly || level > BestCompression {
		return nil, fmt.Errorf("gzip: invalid compression level: %d", level)
	}
	z := new(Writer)
	z.init(w, level)
	return z, nil
}

func (z *Writer) init(w io.Writer, level int) {
	compressor := z.compressor
	if compressor != nil {
		compressor.Reset(w)
	}
	*z = Writer{
		Header: Header{
			OS: 255, // unknown
		},
		w:          w,
		level:      level,
		compressor: compressor,
	}
}

// Reset 丢弃 [Writer] z 的状态，使其等同于从 [NewWriter] 或 [NewWriterLevel]
// 的原始状态，但改为写入 w。这允许重用 [Writer] 而不是分配新的。
func (z *Writer) Reset(w io.Writer) {
	z.init(w, z.level)
}

// writeBytes 将长度前缀的字节切片写入 z.w。
func (z *Writer) writeBytes(b []byte) error {
	if len(b) > 0xffff {
		return errors.New("gzip.Write: Extra data is too large")
	}
	le.PutUint16(z.buf[:2], uint16(len(b)))
	_, err := z.w.Write(z.buf[:2])
	if err != nil {
		return err
	}
	_, err = z.w.Write(b)
	return err
}

// writeString 将 UTF-8 字符串 s 以 GZIP 格式写入 z.w。
// GZIP（RFC 1952）指定字符串为以 NUL 结尾的 ISO 8859-1（Latin-1）。
func (z *Writer) writeString(s string) (err error) {
	// GZIP 存储 Latin-1 字符串；如果非 Latin-1 则错误；如果非 ASCII 则转换。
	needconv := false
	for _, v := range s {
		if v == 0 || v > 0xff {
			return errors.New("gzip.Write: non-Latin-1 header string")
		}
		if v > 0x7f {
			needconv = true
		}
	}
	if needconv {
		b := make([]byte, 0, len(s))
		for _, v := range s {
			b = append(b, byte(v))
		}
		_, err = z.w.Write(b)
	} else {
		_, err = io.WriteString(z.w, s)
	}
	if err != nil {
		return err
	}
	// GZIP 字符串以 NUL 结尾。
	z.buf[0] = 0
	_, err = z.w.Write(z.buf[:1])
	return err
}

// Write 将 p 的压缩形式写入底层 [io.Writer]。
// 压缩的字节不一定会刷新，除非 [Writer] 被关闭。
func (z *Writer) Write(p []byte) (int, error) {
	if z.err != nil {
		return 0, z.err
	}
	var n int
	// 延迟写入 GZIP 头。
	if !z.wroteHeader {
		z.wroteHeader = true
		z.buf = [10]byte{0: gzipID1, 1: gzipID2, 2: gzipDeflate}
		if z.Extra != nil {
			z.buf[3] |= 0x04
		}
		if z.Name != "" {
			z.buf[3] |= 0x08
		}
		if z.Comment != "" {
			z.buf[3] |= 0x10
		}
		if z.ModTime.After(time.Unix(0, 0)) {
			// 第 2.3.1 节，MTIME 的零值表示修改时间未设置。
			le.PutUint32(z.buf[4:8], uint32(z.ModTime.Unix()))
		}
		if z.level == BestCompression {
			z.buf[8] = 2
		} else if z.level == BestSpeed {
			z.buf[8] = 4
		}
		z.buf[9] = z.OS
		_, z.err = z.w.Write(z.buf[:10])
		if z.err != nil {
			return 0, z.err
		}
		if z.Extra != nil {
			z.err = z.writeBytes(z.Extra)
			if z.err != nil {
				return 0, z.err
			}
		}
		if z.Name != "" {
			z.err = z.writeString(z.Name)
			if z.err != nil {
				return 0, z.err
			}
		}
		if z.Comment != "" {
			z.err = z.writeString(z.Comment)
			if z.err != nil {
				return 0, z.err
			}
		}
		if z.compressor == nil {
			z.compressor, _ = flate.NewWriter(z.w, z.level)
		}
	}
	z.size += uint32(len(p))
	z.digest = crc32.Update(z.digest, crc32.IEEETable, p)
	n, z.err = z.compressor.Write(p)
	return n, z.err
}

// Flush 刷新任何待处理的压缩数据到底层 writer。
//
// 这主要在压缩网络协议中有用，以确保
// 远程读者有足够的数据来重建数据包。
// Flush 直到数据被写入后才返回。
// 如果底层 writer 返回错误，Flush 返回该错误。
//
// 在 zlib 库的术语中，Flush 等同于 Z_SYNC_FLUSH。
func (z *Writer) Flush() error {
	if z.err != nil {
		return z.err
	}
	if z.closed {
		return nil
	}
	if !z.wroteHeader {
		z.Write(nil)
		if z.err != nil {
			return z.err
		}
	}
	z.err = z.compressor.Flush()
	return z.err
}

// Close 通过刷新任何未写入的数据到底层 [io.Writer] 并写入 GZIP 页脚来关闭 [Writer]。
// 它不关闭底层 [io.Writer]。
func (z *Writer) Close() error {
	if z.err != nil {
		return z.err
	}
	if z.closed {
		return nil
	}
	z.closed = true
	if !z.wroteHeader {
		z.Write(nil)
		if z.err != nil {
			return z.err
		}
	}
	z.err = z.compressor.Close()
	if z.err != nil {
		return z.err
	}
	le.PutUint32(z.buf[:4], z.digest)
	le.PutUint32(z.buf[4:8], z.size)
	_, z.err = z.w.Write(z.buf[:8])
	return z.err
}
