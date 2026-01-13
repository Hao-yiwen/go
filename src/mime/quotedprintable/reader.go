// 版权所有 2012 The Go Authors。保留所有权利。
// 本源代码的使用受 BSD 风格许可证约束，
// 该许可证可在 LICENSE 文件中找到。

// Package quotedprintable 实现 RFC 2045 规定的 quoted-printable 编码。
package quotedprintable

import (
	"bufio"
	"bytes"
	"fmt"
	"io"
)

// Reader 是一个 quoted-printable 解码器。
type Reader struct {
	br   *bufio.Reader
	rerr error  // 上次读取错误
	line []byte // 在从 br 读取更多数据之前要消费的字节
}

// NewReader 返回一个 quoted-printable 读取器，从 r 进行解码。
func NewReader(r io.Reader) *Reader {
	return &Reader{
		br: bufio.NewReader(r),
	}
}

func fromHex(b byte) (byte, error) {
	switch {
	case b >= '0' && b <= '9':
		return b - '0', nil
	case b >= 'A' && b <= 'F':
		return b - 'A' + 10, nil
	// 接受格式不规范的字节。
	case b >= 'a' && b <= 'f':
		return b - 'a' + 10, nil
	}
	return 0, fmt.Errorf("quotedprintable: 无效的十六进制字节 0x%02x", b)
}

func readHexByte(v []byte) (b byte, err error) {
	if len(v) < 2 {
		return 0, io.ErrUnexpectedEOF
	}
	var hb, lb byte
	if hb, err = fromHex(v[0]); err != nil {
		return 0, err
	}
	if lb, err = fromHex(v[1]); err != nil {
		return 0, err
	}
	return hb<<4 | lb, nil
}

func isQPDiscardWhitespace(r rune) bool {
	switch r {
	case '\n', '\r', ' ', '\t':
		return true
	}
	return false
}

var (
	crlf       = []byte("\r\n")
	lf         = []byte("\n")
	softSuffix = []byte("=")
	lwspChar   = " \t"
)

// Read 从底层读取器读取和解码 quoted-printable 数据。
func (r *Reader) Read(p []byte) (n int, err error) {
	// RFC 2045 的偏差：
	// 1. 除了 "=\r\n"，"=\n" 也被视为软行中断。
	// 2. 它将传递未被 '=' 前缀的 '\r' 或 '\n'，这与
	//    其他不规范的 QP 编码器和解码器一致。
	// 3. 它接受消息末尾的软行中断（=）（issue 15486）；即
	//    从底层读取器读取的最后一个字节允许是 '='，
	//    并将被静默忽略。
	// 4. 如果 = 后面没有跟两个十六进制数字（但不在行尾），
	//    则将其作为字面 = 对待（issue 13219）。
	for len(p) > 0 {
		if len(r.line) == 0 {
			if r.rerr != nil {
				return n, r.rerr
			}
			r.line, r.rerr = r.br.ReadSlice('\n')

			// 该行是否以 CRLF 结尾而不是仅以 LF 结尾？
			hasLF := bytes.HasSuffix(r.line, lf)
			hasCR := bytes.HasSuffix(r.line, crlf)
			wholeLine := r.line
			r.line = bytes.TrimRightFunc(wholeLine, isQPDiscardWhitespace)
			if bytes.HasSuffix(r.line, softSuffix) {
				rightStripped := bytes.TrimLeft(wholeLine[len(r.line):], lwspChar)
				r.line = r.line[:len(r.line)-1]
				if !bytes.HasPrefix(rightStripped, lf) && !bytes.HasPrefix(rightStripped, crlf) &&
					!(len(rightStripped) == 0 && len(r.line) > 0 && r.rerr == io.EOF) {
					r.rerr = fmt.Errorf("quotedprintable: = 后面的无效字节：%q", rightStripped)
				}
			} else if hasLF {
				if hasCR {
					r.line = append(r.line, '\r', '\n')
				} else {
					r.line = append(r.line, '\n')
				}
			}
			continue
		}
		b := r.line[0]

		switch {
		case b == '=':
			b, err = readHexByte(r.line[1:])
			if err != nil {
				if len(r.line) >= 2 && r.line[1] != '\r' && r.line[1] != '\n' {
					// 将 = 作为字面 = 处理。
					b = '='
					break
				}
				return n, err
			}
			r.line = r.line[2:] // 3 中的 2；其他 1 在下面完成
		case b == '\t' || b == '\r' || b == '\n':
			break
		case b >= 0x80:
			// 作为 RFC 2045 的扩展，我们接受
			// 值 >= 0x80 而不提出抱怨。Issue 22597。
			break
		case b < ' ' || b > '~':
			return n, fmt.Errorf("quotedprintable: 主体中的无效未转义字节 0x%02x", b)
		}
		p[0] = b
		p = p[1:]
		r.line = r.line[1:]
		n++
	}
	return n, nil
}
