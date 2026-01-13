// Copyright 2009 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package fmt

import (
	"strconv"
	"unicode/utf8"
)

const (
	ldigits = "0123456789abcdefx"
	udigits = "0123456789ABCDEFX"
)

const (
	signed   = true
	unsigned = false
)

// 标志放在单独的结构体中以便轻松清空。
type fmtFlags struct {
	widPresent  bool
	precPresent bool
	minus       bool
	plus        bool
	sharp       bool
	space       bool
	zero        bool

	// 对于格式 %+v %#v，我们设置 plusV/sharpV 标志
	// 并清空 plus/sharp 标志，因为 %+v 和 %#v 实际上是
	// 在顶层设置的不同的无标志格式。
	plusV  bool
	sharpV bool
}

// fmt 是由 Printf 等使用的原始格式化程序。
// 它打印到一个必须单独设置的缓冲区。
type fmt struct {
	buf *buffer

	fmtFlags

	wid  int // 宽度
	prec int // 精度

	// intbuf 大到足以存储带符号的 int64 的 %b 并且
	// 避免在 32 位架构上在结构体末尾填充。
	intbuf [68]byte
}

func (f *fmt) clearflags() {
	f.fmtFlags = fmtFlags{}
	f.wid = 0
	f.prec = 0
}

func (f *fmt) init(buf *buffer) {
	f.buf = buf
	f.clearflags()
}

// writePadding 生成 n 字节的填充。
func (f *fmt) writePadding(n int) {
	if n <= 0 { // 不需要填充字节。
		return
	}
	buf := *f.buf
	oldLen := len(buf)
	newLen := oldLen + n
	// 为填充腾出足够的空间。
	if newLen > cap(buf) {
		buf = make(buffer, cap(buf)*2+n)
		copy(buf, *f.buf)
	}
	// 决定填充应使用哪个字节。
	padByte := byte(' ')
	// 零填充只允许在左边。
	if f.zero && !f.minus {
		padByte = byte('0')
	}
	// 用 padByte 填充。
	padding := buf[oldLen:newLen]
	for i := range padding {
		padding[i] = padByte
	}
	*f.buf = buf[:newLen]
}

// pad 将 b 追加到 f.buf，在左边(!f.minus)或右边(f.minus)填充。
func (f *fmt) pad(b []byte) {
	if !f.widPresent || f.wid == 0 {
		f.buf.write(b)
		return
	}
	width := f.wid - utf8.RuneCount(b)
	if !f.minus {
		// 左填充
		f.writePadding(width)
		f.buf.write(b)
	} else {
		// 右填充
		f.buf.write(b)
		f.writePadding(width)
	}
}

// padString 将 s 追加到 f.buf，在左边(!f.minus)或右边(f.minus)填充。
func (f *fmt) padString(s string) {
	if !f.widPresent || f.wid == 0 {
		f.buf.writeString(s)
		return
	}
	width := f.wid - utf8.RuneCountInString(s)
	if !f.minus {
		// 左填充
		f.writePadding(width)
		f.buf.writeString(s)
	} else {
		// 右填充
		f.buf.writeString(s)
		f.writePadding(width)
	}
}

// fmtBoolean 格式化一个布尔值。
func (f *fmt) fmtBoolean(v bool) {
	if v {
		f.padString("true")
	} else {
		f.padString("false")
	}
}

// fmtUnicode 将 uint64 格式化为 "U+0078" 或设置 f.sharp 时为 "U+0078 'x'"。
func (f *fmt) fmtUnicode(u uint64) {
	buf := f.intbuf[0:]

	// 设置默认精度时，最大需要的 buf 长度是 18
	// 对于格式化 %#U 的 -1（"U+FFFFFFFFFFFFFFFF"）
	// 这适合于已分配的容量为 68 字节的 intbuf。
	prec := 4
	if f.precPresent && f.prec > 4 {
		prec = f.prec
		// 计算 "U+"、数字、" '"、字符、"'" 所需的空间。
		width := 2 + prec + 2 + utf8.UTFMax + 1
		if width > len(buf) {
			buf = make([]byte, width)
		}
	}

	// 格式化为 buf，以 buf[i] 结束。从右到左格式化数字更容易。
	i := len(buf)

	// 对于 %#U，我们想在缓冲区末尾添加空格和引用的字符。
	if f.sharp && u <= utf8.MaxRune && strconv.IsPrint(rune(u)) {
		i--
		buf[i] = '\''
		i -= utf8.RuneLen(rune(u))
		utf8.EncodeRune(buf[i:], rune(u))
		i--
		buf[i] = '\''
		i--
		buf[i] = ' '
	}
	// 将 Unicode 码点 u 格式化为十六进制数。
	for u >= 16 {
		i--
		buf[i] = udigits[u&0xF]
		prec--
		u >>= 4
	}
	i--
	buf[i] = udigits[u]
	prec--
	// 在数字前添加零，直到达到请求的精度。
	for prec > 0 {
		i--
		buf[i] = '0'
		prec--
	}
	// 添加前导 "U+"。
	i--
	buf[i] = '+'
	i--
	buf[i] = 'U'

	oldZero := f.zero
	f.zero = false
	f.pad(buf[i:])
	f.zero = oldZero
}

// fmtInteger 格式化有符号和无符号整数。
func (f *fmt) fmtInteger(u uint64, base int, isSigned bool, verb rune, digits string) {
	negative := isSigned && int64(u) < 0
	if negative {
		u = -u
	}

	buf := f.intbuf[0:]
	// 已分配的容量为 68 字节的 f.intbuf
	// 在未设置精度或宽度时足够用于整数格式化。
	if f.widPresent || f.precPresent {
		// 为可能的符号和 "0x" 添加额外的 3 字节。
		width := 3 + f.wid + f.prec // wid 和 prec 总是正数。
		if width > len(buf) {
			// 我们需要一个更大的缓冲区。
			buf = make([]byte, width)
		}
	}

	// 请求额外前导零数字的两种方式：%.3d 或 %03d。
	// 如果两者都指定，f.zero 标志被忽略，
	// 改为使用空格填充。
	prec := 0
	if f.precPresent {
		prec = f.prec
		// 精度 0 且值 0 意味着"不打印任何内容"但要填充。
		if prec == 0 && u == 0 {
			oldZero := f.zero
			f.zero = false
			f.writePadding(f.wid)
			f.zero = oldZero
			return
		}
	} else if f.zero && !f.minus && f.widPresent { // 零填充只允许在左边。
		prec = f.wid
		if negative || f.plus || f.space {
			prec-- // 为符号留下空间
		}
	}

	// 因为从右到左打印更容易：将 u 格式化为 buf，以 buf[i] 结束。
	// 我们可以通过将 32 位情况分离到单独的块中来稍微加快速度，
	// 但这不值得重复，所以 u 是 64 位的。
	i := len(buf)
	// 对除法和取模使用常数以获得更高效的代码。
	// switch 情况按流行度排序。
	switch base {
	case 10:
		for u >= 10 {
			i--
			next := u / 10
			buf[i] = byte('0' + u - next*10)
			u = next
		}
	case 16:
		for u >= 16 {
			i--
			buf[i] = digits[u&0xF]
			u >>= 4
		}
	case 8:
		for u >= 8 {
			i--
			buf[i] = byte('0' + u&7)
			u >>= 3
		}
	case 2:
		for u >= 2 {
			i--
			buf[i] = byte('0' + u&1)
			u >>= 1
		}
	default:
		panic("fmt: unknown base; can't happen")
	}
	i--
	buf[i] = digits[u]
	for i > 0 && prec > len(buf)-i {
		i--
		buf[i] = '0'
	}

	// 各种前缀：0x、-、等。
	if f.sharp {
		switch base {
		case 2:
			// 添加前导 0b。
			i--
			buf[i] = 'b'
			i--
			buf[i] = '0'
		case 8:
			if buf[i] != '0' {
				i--
				buf[i] = '0'
			}
		case 16:
			// 添加前导 0x 或 0X。
			i--
			buf[i] = digits[16]
			i--
			buf[i] = '0'
		}
	}
	if verb == 'O' {
		i--
		buf[i] = 'o'
		i--
		buf[i] = '0'
	}

	if negative {
		i--
		buf[i] = '-'
	} else if f.plus {
		i--
		buf[i] = '+'
	} else if f.space {
		i--
		buf[i] = ' '
	}

	// 左填充零已经被像精度一样处理过了
	// 或者 f.zero 标志因为明确设置的精度而被忽略。
	oldZero := f.zero
	f.zero = false
	f.pad(buf[i:])
	f.zero = oldZero
}

// truncateString 根据指定的精度截断字符串 s（如果存在）。
func (f *fmt) truncateString(s string) string {
	if f.precPresent {
		n := f.prec
		for i := range s {
			n--
			if n < 0 {
				return s[:i]
			}
		}
	}
	return s
}

// truncate 根据指定的精度截断字节切片 b，视其为字符串（如果存在）。
func (f *fmt) truncate(b []byte) []byte {
	if f.precPresent {
		n := f.prec
		for i := 0; i < len(b); {
			n--
			if n < 0 {
				return b[:i]
			}
			_, wid := utf8.DecodeRune(b[i:])
			i += wid
		}
	}
	return b
}

// fmtS 格式化一个字符串。
func (f *fmt) fmtS(s string) {
	s = f.truncateString(s)
	f.padString(s)
}

// fmtBs 格式化字节切片 b，就像用 fmtS 格式化为字符串一样。
func (f *fmt) fmtBs(b []byte) {
	b = f.truncate(b)
	f.pad(b)
}

// fmtSbx 将字符串或字节切片格式化为其字节的十六进制编码。
func (f *fmt) fmtSbx(s string, b []byte, digits string) {
	length := len(b)
	if b == nil {
		// 不存在字节切片。假设字符串 s 应该被编码。
		length = len(s)
	}
	// 将长度设置为不处理超过精度要求的字节。
	if f.precPresent && f.prec < length {
		length = f.prec
	}
	// 计算编码的宽度，考虑 f.sharp 和 f.space 标志。
	width := 2 * length
	if width > 0 {
		if f.space {
			// 由两个十六进制数编码的每个元素都会得到前导 0x 或 0X。
			if f.sharp {
				width *= 2
			}
			// 元素将由空格分隔。
			width += length - 1
		} else if f.sharp {
			// 只有整个字符串会添加前导 0x 或 0X。
			width += 2
		}
	} else { // 应该被编码的字节切片或字符串为空。
		if f.widPresent {
			f.writePadding(f.wid)
		}
		return
	}
	// 处理左填充。
	if f.widPresent && f.wid > width && !f.minus {
		f.writePadding(f.wid - width)
	}
	// 直接将编码写入输出缓冲区。
	buf := *f.buf
	if f.sharp {
		// 添加前导 0x 或 0X。
		buf = append(buf, '0', digits[16])
	}
	var c byte
	for i := 0; i < length; i++ {
		if f.space && i > 0 {
			// 用空格分隔元素。
			buf = append(buf, ' ')
			if f.sharp {
				// 为每个元素添加前导 0x 或 0X。
				buf = append(buf, '0', digits[16])
			}
		}
		if b != nil {
			c = b[i] // 从输入字节切片中取一个字节。
		} else {
			c = s[i] // 从输入字符串中取一个字节。
		}
		// 将每个字节编码为两个十六进制数字。
		buf = append(buf, digits[c>>4], digits[c&0xF])
	}
	*f.buf = buf
	// 处理右填充。
	if f.widPresent && f.wid > width && f.minus {
		f.writePadding(f.wid - width)
	}
}

// fmtSx 将字符串格式化为其字节的十六进制编码。
func (f *fmt) fmtSx(s, digits string) {
	f.fmtSbx(s, nil, digits)
}

// fmtBx 将字节切片格式化为其字节的十六进制编码。
func (f *fmt) fmtBx(b []byte, digits string) {
	f.fmtSbx("", b, digits)
}

// fmtQ 将字符串格式化为双引号、转义的 Go 字符串常量。
// 如果设置了 f.sharp，如果字符串不包含除制表符外的任何控制字符，
// 可能会返回原始的（反引号）字符串。
func (f *fmt) fmtQ(s string) {
	s = f.truncateString(s)
	if f.sharp && strconv.CanBackquote(s) {
		f.padString("`" + s + "`")
		return
	}
	buf := f.intbuf[:0]
	if f.plus {
		f.pad(strconv.AppendQuoteToASCII(buf, s))
	} else {
		f.pad(strconv.AppendQuote(buf, s))
	}
}

// fmtC 将整数格式化为 Unicode 字符。
// 如果字符不是有效的 Unicode，它将打印 '\ufffd'。
func (f *fmt) fmtC(c uint64) {
	// 明确检查 c 是否超过 utf8.MaxRune，因为 uint64 到 rune 的转换
	// 可能会丢失指示溢出的精度。
	r := rune(c)
	if c > utf8.MaxRune {
		r = utf8.RuneError
	}
	buf := f.intbuf[:0]
	f.pad(utf8.AppendRune(buf, r))
}

// fmtQc 将整数格式化为单引号、转义的 Go 字符常量。
// 如果字符不是有效的 Unicode，它将打印 '\ufffd'。
func (f *fmt) fmtQc(c uint64) {
	r := rune(c)
	if c > utf8.MaxRune {
		r = utf8.RuneError
	}
	buf := f.intbuf[:0]
	if f.plus {
		f.pad(strconv.AppendQuoteRuneToASCII(buf, r))
	} else {
		f.pad(strconv.AppendQuoteRune(buf, r))
	}
}

// fmtFloat 格式化 float64。假设 verb 是 strconv.AppendFloat 的有效格式说明符
// 因此适合放在一个字节中。
func (f *fmt) fmtFloat(v float64, size int, verb rune, prec int) {
	// 格式说明符中的明确精度会覆盖默认精度。
	if f.precPresent {
		prec = f.prec
	}
	// 格式化数字，为前导 + 符号预留空间（如果需要）。
	num := strconv.AppendFloat(f.intbuf[:1], v, byte(verb), prec, size)
	if num[1] == '-' || num[1] == '+' {
		num = num[1:]
	} else {
		num[0] = '+'
	}
	// f.space 意味着添加前导空格而不是 "+" 符号，除非
	// f.plus 明确要求符号。
	if f.space && num[0] == '+' && !f.plus {
		num[0] = ' '
	}
	// 对无穷大和 NaN 的特殊处理，
	// 它们看起来不像数字，所以不应该用零填充。
	if num[1] == 'I' || num[1] == 'N' {
		oldZero := f.zero
		f.zero = false
		// 如果不要求，移除 NaN 前的符号。
		if num[1] == 'N' && !f.space && !f.plus {
			num = num[1:]
		}
		f.pad(num)
		f.zero = oldZero
		return
	}
	// sharp 标志强制为非二进制格式打印小数点
	// 并保留尾随零，我们可能需要恢复这些。
	if f.sharp && verb != 'b' {
		digits := 0
		switch verb {
		case 'v', 'g', 'G', 'x':
			digits = prec
			// 如果没有明确设置精度，使用精度 6。
			if digits == -1 {
				digits = 6
			}
		}

		// Buffer pre-allocated with enough room for
		// exponent notations of the form "e+123" or "p-1023".
		var tailBuf [6]byte
		tail := tailBuf[:0]

		hasDecimalPoint := false
		sawNonzeroDigit := false
		// Starting from i = 1 to skip sign at num[0].
		for i := 1; i < len(num); i++ {
			switch num[i] {
			case '.':
				hasDecimalPoint = true
			case 'p', 'P':
				tail = append(tail, num[i:]...)
				num = num[:i]
			case 'e', 'E':
				if verb != 'x' && verb != 'X' {
					tail = append(tail, num[i:]...)
					num = num[:i]
					break
				}
				fallthrough
			default:
				if num[i] != '0' {
					sawNonzeroDigit = true
				}
				// Count significant digits after the first non-zero digit.
				if sawNonzeroDigit {
					digits--
				}
			}
		}
		if !hasDecimalPoint {
			// Leading digit 0 should contribute once to digits.
			if len(num) == 2 && num[1] == '0' {
				digits--
			}
			num = append(num, '.')
		}
		for digits > 0 {
			num = append(num, '0')
			digits--
		}
		num = append(num, tail...)
	}
	// We want a sign if asked for and if the sign is not positive.
	if f.plus || num[0] != '+' {
		// If we're zero padding to the left we want the sign before the leading zeros.
		// Achieve this by writing the sign out and then padding the unsigned number.
		// Zero padding is allowed only to the left.
		if f.zero && !f.minus && f.widPresent && f.wid > len(num) {
			f.buf.writeByte(num[0])
			f.writePadding(f.wid - len(num))
			f.buf.write(num[1:])
			return
		}
		f.pad(num)
		return
	}
	// No sign to show and the number is positive; just print the unsigned number.
	f.pad(num[1:])
}
