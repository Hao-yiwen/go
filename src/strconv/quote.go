// Copyright 2009 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

//go:generate go run makeisprint.go -output isprint.go

package strconv

import (
	"unicode/utf8"
)

const (
	lowerhex = "0123456789abcdef"
	upperhex = "0123456789ABCDEF"
)

// contains 报告字符串是否包含字节 c.
func contains(s string, c byte) bool {
	return index(s, c) != -1
}

func quoteWith(s string, quote byte, ASCIIonly, graphicOnly bool) string {
	return string(appendQuotedWith(make([]byte, 0, 3*len(s)/2), s, quote, ASCIIonly, graphicOnly))
}

func quoteRuneWith(r rune, quote byte, ASCIIonly, graphicOnly bool) string {
	return string(appendQuotedRuneWith(nil, r, quote, ASCIIonly, graphicOnly))
}

func appendQuotedWith(buf []byte, s string, quote byte, ASCIIonly, graphicOnly bool) []byte {
	// 经常被调用来处理大字符串，所以预分配。如果有引号，
	// 这是保守的但仍然有很大帮助。
	if cap(buf)-len(buf) < len(s) {
		nBuf := make([]byte, len(buf), len(buf)+1+len(s)+1)
		copy(nBuf, buf)
		buf = nBuf
	}
	buf = append(buf, quote)
	for r, width := rune(0), 0; len(s) > 0; s = s[width:] {
		r, width = utf8.DecodeRuneInString(s)
		if width == 1 && r == utf8.RuneError {
			buf = append(buf, `\x`...)
			buf = append(buf, lowerhex[s[0]>>4])
			buf = append(buf, lowerhex[s[0]&0xF])
			continue
		}
		buf = appendEscapedRune(buf, r, quote, ASCIIonly, graphicOnly)
	}
	buf = append(buf, quote)
	return buf
}

func appendQuotedRuneWith(buf []byte, r rune, quote byte, ASCIIonly, graphicOnly bool) []byte {
	buf = append(buf, quote)
	if !utf8.ValidRune(r) {
		r = utf8.RuneError
	}
	buf = appendEscapedRune(buf, r, quote, ASCIIonly, graphicOnly)
	buf = append(buf, quote)
	return buf
}

func appendEscapedRune(buf []byte, r rune, quote byte, ASCIIonly, graphicOnly bool) []byte {
	if r == rune(quote) || r == '\\' { // 总是转义反斜杠
		buf = append(buf, '\\')
		buf = append(buf, byte(r))
		return buf
	}
	if ASCIIonly {
		if r < utf8.RuneSelf && IsPrint(r) {
			buf = append(buf, byte(r))
			return buf
		}
	} else if IsPrint(r) || graphicOnly && isInGraphicList(r) {
		return utf8.AppendRune(buf, r)
	}
	switch r {
	case '\a':
		buf = append(buf, `\a`...)
	case '\b':
		buf = append(buf, `\b`...)
	case '\f':
		buf = append(buf, `\f`...)
	case '\n':
		buf = append(buf, `\n`...)
	case '\r':
		buf = append(buf, `\r`...)
	case '\t':
		buf = append(buf, `\t`...)
	case '\v':
		buf = append(buf, `\v`...)
	default:
		switch {
		case r < ' ' || r == 0x7f:
			buf = append(buf, `\x`...)
			buf = append(buf, lowerhex[byte(r)>>4])
			buf = append(buf, lowerhex[byte(r)&0xF])
		case !utf8.ValidRune(r):
			r = 0xFFFD
			fallthrough
		case r < 0x10000:
			buf = append(buf, `\u`...)
			for s := 12; s >= 0; s -= 4 {
				buf = append(buf, lowerhex[r>>uint(s)&0xF])
			}
		default:
			buf = append(buf, `\U`...)
			for s := 28; s >= 0; s -= 4 {
				buf = append(buf, lowerhex[r>>uint(s)&0xF])
			}
		}
	}
	return buf
}

// Quote 返回表示 s 的双引号 Go 字符串字面量。
// 返回的字符串使用 Go 转义序列 (\t, \n, \xFF, \u0100) 表示控制字符和
// 由 [IsPrint] 定义的不可打印字符。
func Quote(s string) string {
	return quoteWith(s, '"', false, false)
}

// AppendQuote 将由 [Quote] 生成的表示 s 的双引号 Go 字符串字面量追加到 dst，
// 并返回扩展后的缓冲区。
func AppendQuote(dst []byte, s string) []byte {
	return appendQuotedWith(dst, s, '"', false, false)
}

// QuoteToASCII 返回表示 s 的双引号 Go 字符串字面量。
// 返回的字符串使用 Go 转义序列 (\t, \n, \xFF, \u0100) 表示非 ASCII 字符
// 和由 [IsPrint] 定义的不可打印字符。
func QuoteToASCII(s string) string {
	return quoteWith(s, '"', true, false)
}

// AppendQuoteToASCII 将由 [QuoteToASCII] 生成的表示 s 的双引号 Go 字符串字面量
// 追加到 dst，并返回扩展后的缓冲区。
func AppendQuoteToASCII(dst []byte, s string) []byte {
	return appendQuotedWith(dst, s, '"', true, false)
}

// QuoteToGraphic 返回表示 s 的双引号 Go 字符串字面量。
// 返回的字符串保留由 [IsGraphic] 定义的 Unicode 图形字符不变，
// 并对非图形字符使用 Go 转义序列 (\t, \n, \xFF, \u0100)。
func QuoteToGraphic(s string) string {
	return quoteWith(s, '"', false, true)
}

// AppendQuoteToGraphic 将由 [QuoteToGraphic] 生成的表示 s 的双引号 Go 字符串字面量
// 追加到 dst，并返回扩展后的缓冲区。
func AppendQuoteToGraphic(dst []byte, s string) []byte {
	return appendQuotedWith(dst, s, '"', false, true)
}

// QuoteRune 返回表示 rune 的单引号 Go 字符字面量。
// 返回的字符串使用 Go 转义序列 (\t, \n, \xFF, \u0100) 表示控制字符
// 和由 [IsPrint] 定义的不可打印字符。
// 如果 r 不是有效的 Unicode 码位，它被解释为 Unicode 替换字符 U+FFFD。
func QuoteRune(r rune) string {
	return quoteRuneWith(r, '\'', false, false)
}

// AppendQuoteRune 将由 [QuoteRune] 生成的表示 rune 的单引号 Go 字符字面量
// 追加到 dst，并返回扩展后的缓冲区。
func AppendQuoteRune(dst []byte, r rune) []byte {
	return appendQuotedRuneWith(dst, r, '\'', false, false)
}

// QuoteRuneToASCII 返回表示 rune 的单引号 Go 字符字面量。
// 返回的字符串使用 Go 转义序列 (\t, \n, \xFF, \u0100) 表示非 ASCII 字符
// 和由 [IsPrint] 定义的不可打印字符。
// 如果 r 不是有效的 Unicode 码位，它被解释为 Unicode 替换字符 U+FFFD。
func QuoteRuneToASCII(r rune) string {
	return quoteRuneWith(r, '\'', true, false)
}

// AppendQuoteRuneToASCII 将由 [QuoteRuneToASCII] 生成的表示 rune 的单引号 Go 字符字面量
// 追加到 dst，并返回扩展后的缓冲区。
func AppendQuoteRuneToASCII(dst []byte, r rune) []byte {
	return appendQuotedRuneWith(dst, r, '\'', true, false)
}

// QuoteRuneToGraphic 返回表示 rune 的单引号 Go 字符字面量。
// 如果 rune 不是由 [IsGraphic] 定义的 Unicode 图形字符，
// 返回的字符串将使用 Go 转义序列 (\t, \n, \xFF, \u0100)。
// 如果 r 不是有效的 Unicode 码位，它被解释为 Unicode 替换字符 U+FFFD。
func QuoteRuneToGraphic(r rune) string {
	return quoteRuneWith(r, '\'', false, true)
}

// AppendQuoteRuneToGraphic 将由 [QuoteRuneToGraphic] 生成的表示 rune 的单引号 Go 字符字面量
// 追加到 dst，并返回扩展后的缓冲区。
func AppendQuoteRuneToGraphic(dst []byte, r rune) []byte {
	return appendQuotedRuneWith(dst, r, '\'', false, true)
}

// CanBackquote 报告字符串 s 是否可以不变地表示为单行反引号字符串，
// 除制表符外不包含控制字符。
func CanBackquote(s string) bool {
	for len(s) > 0 {
		r, wid := utf8.DecodeRuneInString(s)
		s = s[wid:]
		if wid > 1 {
			if r == '\ufeff' {
				return false // BOM 是不可见的，不应被引用。
			}
			continue // 所有其他多字节 rune 正确编码并假设为可打印的。
		}
		if r == utf8.RuneError {
			return false
		}
		if (r < ' ' && r != '\t') || r == '`' || r == '\u007F' {
			return false
		}
	}
	return true
}

func unhex(b byte) (v rune, ok bool) {
	c := rune(b)
	switch {
	case '0' <= c && c <= '9':
		return c - '0', true
	case 'a' <= c && c <= 'f':
		return c - 'a' + 10, true
	case 'A' <= c && c <= 'F':
		return c - 'A' + 10, true
	}
	return
}

// UnquoteChar 解码由字符串 s 表示的转义字符串或字符字面量中的第一个字符或字节。
// 它返回四个值:
//
//  1. value，解码的 Unicode 码位或字节值;
//  2. multibyte，一个布尔值，指示解码的字符是否需要多字节 UTF-8 表示;
//  3. tail，字符之后的字符串的剩余部分; 和
//  4. 如果字符在语法上有效，错误将为 nil。
//
// 第二个参数 quote 指定要解析的字面量类型，
// 因此指定了允许的转义引号字符。
// 如果设置为单引号，它允许序列 \' 并禁止未转义的 '。
// 如果设置为双引号，它允许 \" 并禁止未转义的 "。
// 如果设置为零，它不允许任何一种转义，并允许两个引号字符不转义出现。
func UnquoteChar(s string, quote byte) (value rune, multibyte bool, tail string, err error) {
	// 简单的情况
	if len(s) == 0 {
		err = ErrSyntax
		return
	}
	switch c := s[0]; {
	case c == quote && (quote == '\'' || quote == '"'):
		err = ErrSyntax
		return
	case c >= utf8.RuneSelf:
		r, size := utf8.DecodeRuneInString(s)
		return r, true, s[size:], nil
	case c != '\\':
		return rune(s[0]), false, s[1:], nil
	}

	// 困难的情况: c 是反斜杠
	if len(s) <= 1 {
		err = ErrSyntax
		return
	}
	c := s[1]
	s = s[2:]

	switch c {
	case 'a':
		value = '\a'
	case 'b':
		value = '\b'
	case 'f':
		value = '\f'
	case 'n':
		value = '\n'
	case 'r':
		value = '\r'
	case 't':
		value = '\t'
	case 'v':
		value = '\v'
	case 'x', 'u', 'U':
		n := 0
		switch c {
		case 'x':
			n = 2
		case 'u':
			n = 4
		case 'U':
			n = 8
		}
		var v rune
		if len(s) < n {
			err = ErrSyntax
			return
		}
		for j := 0; j < n; j++ {
			x, ok := unhex(s[j])
			if !ok {
				err = ErrSyntax
				return
			}
			v = v<<4 | x
		}
		s = s[n:]
		if c == 'x' {
			// 单字节字符串，可能不是 UTF-8
			value = v
			break
		}
		if !utf8.ValidRune(v) {
			err = ErrSyntax
			return
		}
		value = v
		multibyte = true
	case '0', '1', '2', '3', '4', '5', '6', '7':
		v := rune(c) - '0'
		if len(s) < 2 {
			err = ErrSyntax
			return
		}
		for j := 0; j < 2; j++ { // 已有一个数字；再加两个
			x := rune(s[j]) - '0'
			if x < 0 || x > 7 {
				err = ErrSyntax
				return
			}
			v = (v << 3) | x
		}
		s = s[2:]
		if v > 255 {
			err = ErrSyntax
			return
		}
		value = v
	case '\\':
		value = '\\'
	case '\'', '"':
		if c != quote {
			err = ErrSyntax
			return
		}
		value = rune(c)
	default:
		err = ErrSyntax
		return
	}
	tail = s
	return
}

// QuotedPrefix 返回 s 前缀处的引用字符串（如 [Unquote] 所理解的）。
// 如果 s 不以有效的引用字符串开始，QuotedPrefix 返回错误。
func QuotedPrefix(s string) (string, error) {
	out, _, err := unquote(s, false)
	return out, err
}

// Unquote 将 s 解释为单引号、双引号或反引号 Go 字符串字面量，
// 返回 s 引用的字符串值。(如果 s 是单引号，它将是 Go 字符字面量;
// Unquote 返回对应的单字符字符串。对于空字符字面量，
// Unquote 返回空字符串。)
func Unquote(s string) (string, error) {
	out, rem, err := unquote(s, true)
	if len(rem) > 0 {
		return "", ErrSyntax
	}
	return out, err
}

// unquote 解析输入开始处的引用字符串，
// 返回解析后的前缀、剩余的后缀和任何解析错误。
// 如果 unescape 为 true，解析后的前缀被取消转义，
// 否则输入前缀按原样提供。
func unquote(in string, unescape bool) (out, rem string, err error) {
	// 确定引号形式并乐观地找到终止引号。
	if len(in) < 2 {
		return "", in, ErrSyntax
	}
	quote := in[0]
	end := index(in[1:], quote)
	if end < 0 {
		return "", in, ErrSyntax
	}
	end += 2 // 终止引号之后的位置；如果存在转义序列，可能是错的

	switch quote {
	case '`':
		switch {
		case !unescape:
			out = in[:end] // 包括引号
		case !contains(in[:end], '\r'):
			out = in[len("`") : end-len("`")] // 不包括引号
		default:
			// 原始字符串字面量内的回车字符 ('\r')
			// 从原始字符串值中被丢弃。
			buf := make([]byte, 0, end-len("`")-len("\r")-len("`"))
			for i := len("`"); i < end-len("`"); i++ {
				if in[i] != '\r' {
					buf = append(buf, in[i])
				}
			}
			out = string(buf)
		}
		// 注: 先前的实现没有验证原始字符串是否由有效的 UTF-8 字符组成，
		// 我们也继续不进行此验证。
		// Go 规范并未明确要求有效的 UTF-8，
		// 但只提及对于 Go 源代码（必须是有效 UTF-8）隐式有效。
		return out, in[end:], nil
	case '"', '\'':
		// 处理没有任何转义序列的引用字符串。
		if !contains(in[:end], '\\') && !contains(in[:end], '\n') {
			var valid bool
			switch quote {
			case '"':
				valid = utf8.ValidString(in[len(`"`) : end-len(`"`)])
			case '\'':
				r, n := utf8.DecodeRuneInString(in[len("'") : end-len("'")])
				valid = len("'")+n+len("'") == end && (r != utf8.RuneError || n != 1)
			}
			if valid {
				out = in[:end]
				if unescape {
					out = out[1 : end-1] // 不包括引号
				}
				return out, in[end:], nil
			}
		}

		// 处理包含转义序列的引用字符串。
		var buf []byte
		in0 := in
		in = in[1:] // 跳过开始引号
		if unescape {
			buf = make([]byte, 0, 3*end/2) // 尝试避免更多分配
		}
		for len(in) > 0 && in[0] != quote {
			// 处理下一个字符，
			// 拒绝任何无效的未转义换行符。
			r, multibyte, rem, err := UnquoteChar(in, quote)
			if in[0] == '\n' || err != nil {
				return "", in0, ErrSyntax
			}
			in = rem

			// 如果取消转义输入，则追加字符。
			if unescape {
				if r < utf8.RuneSelf || !multibyte {
					buf = append(buf, byte(r))
				} else {
					buf = utf8.AppendRune(buf, r)
				}
			}

			// 单引号字符串必须是单个字符。
			if quote == '\'' {
				break
			}
		}

		// 验证字符串以终止引号结束。
		if !(len(in) > 0 && in[0] == quote) {
			return "", in0, ErrSyntax
		}
		in = in[1:] // 跳过终止引号

		if unescape {
			return string(buf), in, nil
		}
		return in0[:len(in0)-len(in)], in, nil
	default:
		return "", in, ErrSyntax
	}
}

// bsearch 在语义上与 [slices.BinarySearch] 相同 (不进行 NaN 检查)
// 我们复制了这个函数，因为我们无法在这里导入 "slices"。
func bsearch[S ~[]E, E ~uint16 | ~uint32](s S, v E) (int, bool) {
	n := len(s)
	i, j := 0, n
	for i < j {
		h := i + (j-i)>>1
		if s[h] < v {
			i = h + 1
		} else {
			j = h
		}
	}
	return i, i < n && s[i] == v
}

// TODO: IsPrint 是 unicode.IsPrint 的本地实现，已由测试验证给出
// 相同的答案。它允许此包不依赖于 unicode，
// 因此不需要导入所有 Unicode 表。如果链接器
// 更善于丢弃未使用的表，我们可以摆脱这个实现。
// 那样会更好。

// IsPrint 报告 rune 是否由 Go 定义为可打印的，
// 定义与 [unicode.IsPrint] 相同: 字母、数字、标点符号、
// 符号和 ASCII 空格。
func IsPrint(r rune) bool {
	// 对 Latin-1 的快速检查
	if r <= 0xFF {
		if 0x20 <= r && r <= 0x7E {
			// 所有 ASCII 从空格到 DEL-1 都是可打印的。
			return true
		}
		if 0xA1 <= r && r <= 0xFF {
			// 类似地对于 ¡ 到 ÿ...
			return r != 0xAD // ...除了奇怪的软连字符。
		}
		return false
	}

	// 相同的算法，适用于 uint16 或 uint32 值。
	// 首先，找到第一个 i 使得 isPrint[i] >= x。
	// 这是可能跨越 x 的一对的起点或终点的索引。
	// 起点是偶数 (isPrint[i&^1])，终点是奇数 (isPrint[i|1])。
	// 如果我们在一个范围内找到 x，确保 x 不在 isNotPrint 列表中。

	if 0 <= r && r < 1<<16 {
		rr, isPrint, isNotPrint := uint16(r), isPrint16, isNotPrint16
		i, _ := bsearch(isPrint, rr)
		if i >= len(isPrint) || rr < isPrint[i&^1] || isPrint[i|1] < rr {
			return false
		}
		_, found := bsearch(isNotPrint, rr)
		return !found
	}

	rr, isPrint, isNotPrint := uint32(r), isPrint32, isNotPrint32
	i, _ := bsearch(isPrint, rr)
	if i >= len(isPrint) || rr < isPrint[i&^1] || isPrint[i|1] < rr {
		return false
	}
	if r >= 0x20000 {
		return true
	}
	r -= 0x10000
	_, found := bsearch(isNotPrint, uint16(r))
	return !found
}

// IsGraphic 报告 rune 是否由 Unicode 定义为图形。这样的
// 字符包括来自 L、M、N、P、S 和 Zs 类别的字母、标记、数字、标点符号、符号和
// 空格。
func IsGraphic(r rune) bool {
	if IsPrint(r) {
		return true
	}
	return isInGraphicList(r)
}

// isInGraphicList 报告 rune 是否在 isGraphic 列表中。这与
// IsGraphic 的分离允许 quoteWith 避免两次调用 IsPrint。
// 仅当 IsPrint 失败时才应调用。
func isInGraphicList(r rune) bool {
	// 我们知道 r 必须符合 16 位 - 见 makeisprint.go。
	if r > 0xFFFF {
		return false
	}
	_, found := bsearch(isGraphic, uint16(r))
	return found
}
