// 版权所有 2009 The Go Authors。保留所有权利。
// 本源代码的使用受 BSD 风格许可证约束，
// 该许可证可在 LICENSE 文件中找到。

// Package scanner 提供了一个用于 UTF-8 编码文本的扫描器和分词器。
// 它接收一个提供源代码的 io.Reader，然后可以通过重复调用 Scan 函数进行分词。
// 为了与现有工具兼容，不允许使用 NUL 字符。如果源代码的第一个字符是
// UTF-8 编码的字节顺序标记（BOM），则会被丢弃。
//
// 默认情况下，[Scanner] 会跳过空白字符和 Go 注释，并识别 Go 语言规范
// 定义的所有字面量。可以对其进行自定义，使其仅识别这些字面量的子集，
// 并识别不同的标识符和空白字符。
package scanner

import (
	"bytes"
	"fmt"
	"io"
	"os"
	"unicode"
	"unicode/utf8"
)

// Position 是一个表示源代码位置的值。
// 如果 Line > 0，则位置有效。
type Position struct {
	Filename string // 文件名（如果有）
	Offset   int    // 字节偏移量，从 0 开始
	Line     int    // 行号，从 1 开始
	Column   int    // 列号，从 1 开始（每行的字符计数）
}

// IsValid 报告该位置是否有效。
func (pos *Position) IsValid() bool { return pos.Line > 0 }

func (pos Position) String() string {
	s := pos.Filename
	if s == "" {
		s = "<input>"
	}
	if pos.IsValid() {
		s += fmt.Sprintf(":%d:%d", pos.Line, pos.Column)
	}
	return s
}

// 预定义的模式位，用于控制 token 的识别。例如，
// 要配置一个 [Scanner] 使其仅识别（Go）标识符、
// 整数并跳过注释，请将 Scanner 的 Mode 字段设置为：
//
//	ScanIdents | ScanInts | ScanComments | SkipComments
//
// 除了注释（如果设置了 SkipComments 则会被跳过）之外，
// 未识别的 token 不会被忽略。相反，扫描器只是
// 返回各个单独的字符（或可能的子 token）。
// 例如，如果模式是 ScanIdents（而非 ScanStrings），则字符串
// "foo" 会被扫描为 token 序列 '"' [Ident] '"'。
//
// 使用 GoTokens 来配置 Scanner，使其接受所有 Go
// 字面量 token，包括 Go 标识符。注释将被跳过。
const (
	ScanIdents     = 1 << -Ident
	ScanInts       = 1 << -Int
	ScanFloats     = 1 << -Float // 包含整数和十六进制浮点数
	ScanChars      = 1 << -Char
	ScanStrings    = 1 << -String
	ScanRawStrings = 1 << -RawString
	ScanComments   = 1 << -Comment
	SkipComments   = 1 << -skipComment // 如果与 ScanComments 一起设置，注释会被当作空白处理
	GoTokens       = ScanIdents | ScanFloats | ScanChars | ScanStrings | ScanRawStrings | ScanComments | SkipComments
)

// Scan 的结果是这些 token 之一或一个 Unicode 字符。
const (
	EOF = -(iota + 1)
	Ident
	Int
	Float
	Char
	String
	RawString
	Comment

	// 仅供内部使用
	skipComment
)

var tokenString = map[rune]string{
	EOF:       "EOF",
	Ident:     "Ident",
	Int:       "Int",
	Float:     "Float",
	Char:      "Char",
	String:    "String",
	RawString: "RawString",
	Comment:   "Comment",
}

// TokenString 返回一个 token 或 Unicode 字符的可打印字符串。
func TokenString(tok rune) string {
	if s, found := tokenString[tok]; found {
		return s
	}
	return fmt.Sprintf("%q", string(tok))
}

// GoWhitespace 是 [Scanner] 的 Whitespace 字段的默认值。
// 其值选择 Go 的空白字符。
const GoWhitespace = 1<<'\t' | 1<<'\n' | 1<<'\r' | 1<<' '

const bufLen = 1024 // 至少为 utf8.UTFMax

// Scanner 实现从 [io.Reader] 读取 Unicode 字符和 token。
type Scanner struct {
	// 输入
	src io.Reader

	// 源缓冲区
	srcBuf [bufLen + 1]byte // +1 用于 s.next() 常见情况的哨兵
	srcPos int              // 读取位置（srcBuf 索引）
	srcEnd int              // 源结束位置（srcBuf 索引）

	// 源位置
	srcBufOffset int // srcBuf[0] 在源中的字节偏移量
	line         int // 行计数
	column       int // 字符计数
	lastLineLen  int // 最后一行的字符长度（用于正确报告列号）
	lastCharLen  int // 最后一个字符的字节长度

	// Token 文本缓冲区
	// 通常，token 文本完全存储在 srcBuf 中，但一般情况下
	// token 文本的头部可能缓冲在 tokBuf 中，而 token 文本的
	// 尾部存储在 srcBuf 中。
	tokBuf bytes.Buffer // 不再在 srcBuf 中的 token 文本头部
	tokPos int          // token 文本尾部位置（srcBuf 索引）；如果 >= 0 则有效
	tokEnd int          // token 文本尾部结束位置（srcBuf 索引）

	// 单字符前瞻
	ch rune // 当前 srcPos 之前的字符

	// Error 在遇到每个错误时被调用。如果没有设置 Error
	// 函数，则错误会报告到 os.Stderr。
	Error func(s *Scanner, msg string)

	// ErrorCount 在遇到每个错误时加一。
	ErrorCount int

	// Mode 字段控制识别哪些 token。例如，
	// 要识别整数，请在 Mode 中设置 ScanInts 位。该字段可以
	// 随时更改。
	Mode uint

	// Whitespace 字段控制哪些字符被识别为空白。
	// 要将字符 ch <= ' ' 识别为空白，请在 Whitespace 中设置
	// 第 ch 位（对于 ch > ' ' 的值，Scanner 的行为未定义）。
	// 该字段可以随时更改。
	Whitespace uint64

	// IsIdentRune 是一个谓词，控制在标识符中接受哪些字符
	// 作为第 i 个 rune。有效字符集不得与空白字符集相交。
	// 如果没有设置 IsIdentRune 函数，则接受常规 Go 标识符。
	// 该字段可以随时更改。
	IsIdentRune func(ch rune, i int) bool

	// 最近扫描的 token 的起始位置；由 Scan 设置。
	// 调用 Init 或 Next 会使位置无效（Line == 0）。
	// Filename 字段始终不会被 Scanner 修改。
	// 如果报告了错误（通过 Error）且 Position 无效，
	// 则扫描器不在 token 内。在这种情况下调用 Pos 来获取
	// 错误位置，或获取最近扫描的 token 之后的位置。
	Position
}

// Init 使用新的源初始化 [Scanner] 并返回 s。
// [Scanner.Error] 设置为 nil，[Scanner.ErrorCount] 设置为 0，[Scanner.Mode] 设置为 [GoTokens]，
// [Scanner.Whitespace] 设置为 [GoWhitespace]。
func (s *Scanner) Init(src io.Reader) *Scanner {
	s.src = src

	// 初始化源缓冲区
	// （第一次调用 next() 会通过调用 src.Read 来填充它）
	s.srcBuf[0] = utf8.RuneSelf // 哨兵
	s.srcPos = 0
	s.srcEnd = 0

	// 初始化源位置
	s.srcBufOffset = 0
	s.line = 1
	s.column = 0
	s.lastLineLen = 0
	s.lastCharLen = 0

	// 初始化 token 文本缓冲区
	// （第一次调用 next() 时需要）。
	s.tokPos = -1

	// 初始化单字符前瞻
	s.ch = -2 // 尚未读取字符，不是 EOF

	// 初始化公共字段
	s.Error = nil
	s.ErrorCount = 0
	s.Mode = GoTokens
	s.Whitespace = GoWhitespace
	s.Line = 0 // 使 token 位置无效

	return s
}

// next 读取并返回下一个 Unicode 字符。它的设计使得
// 在常见的 ASCII 情况下只需要做最少的工作
// （一次检测同时检查 ASCII 和缓冲区末尾，以及一次
// 检查换行符）。
func (s *Scanner) next() rune {
	ch, width := rune(s.srcBuf[s.srcPos]), 1

	if ch >= utf8.RuneSelf {
		// 非常见情况：不是 ASCII 或字节不足
		for s.srcPos+utf8.UTFMax > s.srcEnd && !utf8.FullRune(s.srcBuf[s.srcPos:s.srcEnd]) {
			// 字节不足：读取更多，但首先
			// 保存 token 文本（如果有）
			if s.tokPos >= 0 {
				s.tokBuf.Write(s.srcBuf[s.tokPos:s.srcPos])
				s.tokPos = 0
				// s.tokEnd 由 Scan() 设置
			}
			// 将未读字节移动到缓冲区开头
			copy(s.srcBuf[0:], s.srcBuf[s.srcPos:s.srcEnd])
			s.srcBufOffset += s.srcPos
			// 读取更多字节
			// （io.Reader 在到达读取内容的末尾时必须返回 io.EOF -
			// 简单地返回 n == 0 会使此循环永远重试；但在这种情况下
			// 错误在于 reader 的实现）
			i := s.srcEnd - s.srcPos
			n, err := s.src.Read(s.srcBuf[i:bufLen])
			s.srcPos = 0
			s.srcEnd = i + n
			s.srcBuf[s.srcEnd] = utf8.RuneSelf // 哨兵
			if err != nil {
				if err != io.EOF {
					s.error(err.Error())
				}
				if s.srcEnd == 0 {
					if s.lastCharLen > 0 {
						// 前一个字符不是 EOF
						s.column++
					}
					s.lastCharLen = 0
					return EOF
				}
				// 如果 err == EOF，我们不会获得更多
				// 字节；中断以避免无限循环。如果
				// err 是其他错误，我们不知道是否
				// 能获得更多字节；因此也中断。
				break
			}
		}
		// 至少一个字节
		ch = rune(s.srcBuf[s.srcPos])
		if ch >= utf8.RuneSelf {
			// 非常见情况：不是 ASCII
			ch, width = utf8.DecodeRune(s.srcBuf[s.srcPos:s.srcEnd])
			if ch == utf8.RuneError && width == 1 {
				// 前进以获得正确的错误位置
				s.srcPos += width
				s.lastCharLen = width
				s.column++
				s.error("invalid UTF-8 encoding")
				return ch
			}
		}
	}

	// 前进
	s.srcPos += width
	s.lastCharLen = width
	s.column++

	// 特殊情况
	switch ch {
	case 0:
		// 为了与其他工具兼容
		s.error("invalid character NUL")
	case '\n':
		s.line++
		s.lastLineLen = s.column
		s.column = 0
	}

	return ch
}

// Next 读取并返回下一个 Unicode 字符。
// 它在源的末尾返回 [EOF]。它通过调用 s.Error（如果非 nil）
// 报告读取错误；否则它会将错误消息打印到 [os.Stderr]。
// Next 不更新 [Scanner.Position] 字段；使用 [Scanner.Pos]()
// 来获取当前位置。
func (s *Scanner) Next() rune {
	s.tokPos = -1 // 不收集 token 文本
	s.Line = 0    // 使 token 位置无效
	ch := s.Peek()
	if ch != EOF {
		s.ch = s.next()
	}
	return ch
}

// Peek 返回源中的下一个 Unicode 字符，但不推进
// 扫描器。如果扫描器的位置在源的最后一个字符处，
// 则返回 [EOF]。
func (s *Scanner) Peek() rune {
	if s.ch == -2 {
		// 此代码仅在第一个字符时运行
		s.ch = s.next()
		if s.ch == '\uFEFF' {
			s.ch = s.next() // 忽略 BOM
		}
	}
	return s.ch
}

func (s *Scanner) error(msg string) {
	s.tokEnd = s.srcPos - s.lastCharLen // 确保 token 文本已终止
	s.ErrorCount++
	if s.Error != nil {
		s.Error(s, msg)
		return
	}
	pos := s.Position
	if !pos.IsValid() {
		pos = s.Pos()
	}
	fmt.Fprintf(os.Stderr, "%s: %s\n", pos, msg)
}

func (s *Scanner) errorf(format string, args ...any) {
	s.error(fmt.Sprintf(format, args...))
}

func (s *Scanner) isIdentRune(ch rune, i int) bool {
	if s.IsIdentRune != nil {
		return ch != EOF && s.IsIdentRune(ch, i)
	}
	return ch == '_' || unicode.IsLetter(ch) || unicode.IsDigit(ch) && i > 0
}

func (s *Scanner) scanIdentifier() rune {
	// 我们知道第零个 rune 是正确的；从下一个开始扫描
	ch := s.next()
	for i := 1; s.isIdentRune(ch, i); i++ {
		ch = s.next()
	}
	return ch
}

func lower(ch rune) rune     { return ('a' - 'A') | ch } // 如果 ch 是 ASCII 字母，则返回小写的 ch
func isDecimal(ch rune) bool { return '0' <= ch && ch <= '9' }
func isHex(ch rune) bool     { return '0' <= ch && ch <= '9' || 'a' <= lower(ch) && lower(ch) <= 'f' }

// digits 接受从 ch0 开始的序列 { digit | '_' }。
// 如果 base <= 10，digits 接受任何十进制数字，但如果
// *invalid == 0，则在 *invalid 中记录第一个无效数字 >= base。
// digits 返回不再属于序列的第一个 rune，以及一个位集，
// 描述序列是否包含数字（设置位 0）或分隔符 '_'（设置位 1）。
func (s *Scanner) digits(ch0 rune, base int, invalid *rune) (ch rune, digsep int) {
	ch = ch0
	if base <= 10 {
		max := rune('0' + base)
		for isDecimal(ch) || ch == '_' {
			ds := 1
			if ch == '_' {
				ds = 2
			} else if ch >= max && *invalid == 0 {
				*invalid = ch
			}
			digsep |= ds
			ch = s.next()
		}
	} else {
		for isHex(ch) || ch == '_' {
			ds := 1
			if ch == '_' {
				ds = 2
			}
			digsep |= ds
			ch = s.next()
		}
	}
	return
}

func (s *Scanner) scanNumber(ch rune, seenDot bool) (rune, rune) {
	base := 10         // 数字基数
	prefix := rune(0)  // 0（十进制）、'0'（0-八进制）、'x'、'o' 或 'b' 之一
	digsep := 0        // 位 0：存在数字，位 1：存在 '_'
	invalid := rune(0) // 字面量中的无效数字，或 0

	// 整数部分
	var tok rune
	var ds int
	if !seenDot {
		tok = Int
		if ch == '0' {
			ch = s.next()
			switch lower(ch) {
			case 'x':
				ch = s.next()
				base, prefix = 16, 'x'
			case 'o':
				ch = s.next()
				base, prefix = 8, 'o'
			case 'b':
				ch = s.next()
				base, prefix = 2, 'b'
			default:
				base, prefix = 8, '0'
				digsep = 1 // 前导 0
			}
		}
		ch, ds = s.digits(ch, base, &invalid)
		digsep |= ds
		if ch == '.' && s.Mode&ScanFloats != 0 {
			ch = s.next()
			seenDot = true
		}
	}

	// 小数部分
	if seenDot {
		tok = Float
		if prefix == 'o' || prefix == 'b' {
			s.error("invalid radix point in " + litname(prefix))
		}
		ch, ds = s.digits(ch, base, &invalid)
		digsep |= ds
	}

	if digsep&1 == 0 {
		s.error(litname(prefix) + " has no digits")
	}

	// 指数部分
	if e := lower(ch); (e == 'e' || e == 'p') && s.Mode&ScanFloats != 0 {
		switch {
		case e == 'e' && prefix != 0 && prefix != '0':
			s.errorf("%q exponent requires decimal mantissa", ch)
		case e == 'p' && prefix != 'x':
			s.errorf("%q exponent requires hexadecimal mantissa", ch)
		}
		ch = s.next()
		tok = Float
		if ch == '+' || ch == '-' {
			ch = s.next()
		}
		ch, ds = s.digits(ch, 10, nil)
		digsep |= ds
		if ds&1 == 0 {
			s.error("exponent has no digits")
		}
	} else if prefix == 'x' && tok == Float {
		s.error("hexadecimal mantissa requires a 'p' exponent")
	}

	if tok == Int && invalid != 0 {
		s.errorf("invalid digit %q in %s", invalid, litname(prefix))
	}

	if digsep&2 != 0 {
		s.tokEnd = s.srcPos - s.lastCharLen // 确保 token 文本已终止
		if i := invalidSep(s.TokenText()); i >= 0 {
			s.error("'_' must separate successive digits")
		}
	}

	return tok, ch
}

func litname(prefix rune) string {
	switch prefix {
	default:
		return "decimal literal"
	case 'x':
		return "hexadecimal literal"
	case 'o', '0':
		return "octal literal"
	case 'b':
		return "binary literal"
	}
}

// invalidSep 返回 x 中第一个无效分隔符的索引，或返回 -1。
func invalidSep(x string) int {
	x1 := ' ' // 前缀字符，我们只关心它是否是 'x'
	d := '.'  // 数字，'_'、'0'（数字）或 '.'（其他任何字符）之一
	i := 0

	// 前缀算作一个数字
	if len(x) >= 2 && x[0] == '0' {
		x1 = lower(rune(x[1]))
		if x1 == 'x' || x1 == 'o' || x1 == 'b' {
			d = '0'
			i = 2
		}
	}

	// 尾数和指数
	for ; i < len(x); i++ {
		p := d // 前一个数字
		d = rune(x[i])
		switch {
		case d == '_':
			if p != '0' {
				return i
			}
		case isDecimal(d) || x1 == 'x' && isHex(d):
			d = '0'
		default:
			if p == '_' {
				return i - 1
			}
			d = '.'
		}
	}
	if d == '_' {
		return len(x) - 1
	}

	return -1
}

func digitVal(ch rune) int {
	switch {
	case '0' <= ch && ch <= '9':
		return int(ch - '0')
	case 'a' <= lower(ch) && lower(ch) <= 'f':
		return int(lower(ch) - 'a' + 10)
	}
	return 16 // 大于任何合法数字值
}

func (s *Scanner) scanDigits(ch rune, base, n int) rune {
	for n > 0 && digitVal(ch) < base {
		ch = s.next()
		n--
	}
	if n > 0 {
		s.error("invalid char escape")
	}
	return ch
}

func (s *Scanner) scanEscape(quote rune) rune {
	ch := s.next() // 读取 '/' 之后的字符
	switch ch {
	case 'a', 'b', 'f', 'n', 'r', 't', 'v', '\\', quote:
		// 无需操作
		ch = s.next()
	case '0', '1', '2', '3', '4', '5', '6', '7':
		ch = s.scanDigits(ch, 8, 3)
	case 'x':
		ch = s.scanDigits(s.next(), 16, 2)
	case 'u':
		ch = s.scanDigits(s.next(), 16, 4)
	case 'U':
		ch = s.scanDigits(s.next(), 16, 8)
	default:
		s.error("invalid char escape")
	}
	return ch
}

func (s *Scanner) scanString(quote rune) (n int) {
	ch := s.next() // 读取引号之后的字符
	for ch != quote {
		if ch == '\n' || ch < 0 {
			s.error("literal not terminated")
			return
		}
		if ch == '\\' {
			ch = s.scanEscape(quote)
		} else {
			ch = s.next()
		}
		n++
	}
	return
}

func (s *Scanner) scanRawString() {
	ch := s.next() // 读取 '`' 之后的字符
	for ch != '`' {
		if ch < 0 {
			s.error("literal not terminated")
			return
		}
		ch = s.next()
	}
}

func (s *Scanner) scanChar() {
	if s.scanString('\'') != 1 {
		s.error("invalid char literal")
	}
}

func (s *Scanner) scanComment(ch rune) rune {
	// ch == '/' || ch == '*'
	if ch == '/' {
		// 行注释
		ch = s.next() // 读取 "//" 之后的字符
		for ch != '\n' && ch >= 0 {
			ch = s.next()
		}
		return ch
	}

	// 通用注释
	ch = s.next() // 读取 "/*" 之后的字符
	for {
		if ch < 0 {
			s.error("comment not terminated")
			break
		}
		ch0 := ch
		ch = s.next()
		if ch0 == '*' && ch == '/' {
			ch = s.next()
			break
		}
	}
	return ch
}

// Scan 从源读取下一个 token 或 Unicode 字符并返回它。
// 它仅识别相应的 [Scanner.Mode] 位（1<<-t）被设置的 token t。
// 它在源的末尾返回 [EOF]。它通过调用 s.Error（如果非 nil）
// 报告扫描器错误（读取和 token 错误）；否则它会将错误消息
// 打印到 [os.Stderr]。
func (s *Scanner) Scan() rune {
	ch := s.Peek()

	// 重置 token 文本位置
	s.tokPos = -1
	s.Line = 0

redo:
	// 跳过空白
	for s.Whitespace&(1<<uint(ch)) != 0 {
		ch = s.next()
	}

	// 开始收集 token 文本
	s.tokBuf.Reset()
	s.tokPos = s.srcPos - s.lastCharLen

	// 设置 token 位置
	// （这是 Pos() 中代码的稍微优化版本）
	s.Offset = s.srcBufOffset + s.tokPos
	if s.column > 0 {
		// 常见情况：最后一个字符不是 '\n'
		s.Line = s.line
		s.Column = s.column
	} else {
		// 最后一个字符是 '\n'
		// （我们不可能在源的开头，
		// 因为我们至少调用了一次 next()）
		s.Line = s.line - 1
		s.Column = s.lastLineLen
	}

	// 确定 token 值
	tok := ch
	switch {
	case s.isIdentRune(ch, 0):
		if s.Mode&ScanIdents != 0 {
			tok = Ident
			ch = s.scanIdentifier()
		} else {
			ch = s.next()
		}
	case isDecimal(ch):
		if s.Mode&(ScanInts|ScanFloats) != 0 {
			tok, ch = s.scanNumber(ch, false)
		} else {
			ch = s.next()
		}
	default:
		switch ch {
		case EOF:
			break
		case '"':
			if s.Mode&ScanStrings != 0 {
				s.scanString('"')
				tok = String
			}
			ch = s.next()
		case '\'':
			if s.Mode&ScanChars != 0 {
				s.scanChar()
				tok = Char
			}
			ch = s.next()
		case '.':
			ch = s.next()
			if isDecimal(ch) && s.Mode&ScanFloats != 0 {
				tok, ch = s.scanNumber(ch, true)
			}
		case '/':
			ch = s.next()
			if (ch == '/' || ch == '*') && s.Mode&ScanComments != 0 {
				if s.Mode&SkipComments != 0 {
					s.tokPos = -1 // 不收集 token 文本
					ch = s.scanComment(ch)
					goto redo
				}
				ch = s.scanComment(ch)
				tok = Comment
			}
		case '`':
			if s.Mode&ScanRawStrings != 0 {
				s.scanRawString()
				tok = RawString
			}
			ch = s.next()
		default:
			ch = s.next()
		}
	}

	// token 文本结束
	s.tokEnd = s.srcPos - s.lastCharLen

	s.ch = ch
	return tok
}

// Pos 返回紧随 [Scanner.Next] 或 [Scanner.Scan] 最后一次调用
// 返回的字符或 token 之后的字符的位置。
// 使用 [Scanner.Position] 字段获取最近扫描的 token 的起始位置。
func (s *Scanner) Pos() (pos Position) {
	pos.Filename = s.Filename
	pos.Offset = s.srcBufOffset + s.srcPos - s.lastCharLen
	switch {
	case s.column > 0:
		// 常见情况：最后一个字符不是 '\n'
		pos.Line = s.line
		pos.Column = s.column
	case s.lastLineLen > 0:
		// 最后一个字符是 '\n'
		pos.Line = s.line - 1
		pos.Column = s.lastLineLen
	default:
		// 在源的开头
		pos.Line = 1
		pos.Column = 1
	}
	return
}

// TokenText 返回与最近扫描的 token 对应的字符串。
// 在调用 [Scanner.Scan] 之后以及在 [Scanner.Error] 调用中有效。
func (s *Scanner) TokenText() string {
	if s.tokPos < 0 {
		// 没有 token 文本
		return ""
	}

	if s.tokEnd < s.tokPos {
		// 如果到达 EOF，s.tokEnd 设置为 -1（s.srcPos == 0）
		s.tokEnd = s.tokPos
	}
	// s.tokEnd >= s.tokPos

	if s.tokBuf.Len() == 0 {
		// 常见情况：整个 token 文本仍在 srcBuf 中
		return string(s.srcBuf[s.tokPos:s.tokEnd])
	}

	// 部分 token 文本已保存在 tokBuf 中：将其余部分也保存在
	// tokBuf 中并返回其内容
	s.tokBuf.Write(s.srcBuf[s.tokPos:s.tokEnd])
	s.tokPos = s.tokEnd // 确保 TokenText() 调用的幂等性
	return s.tokBuf.String()
}
