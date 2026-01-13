// Copyright 2010 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package fmt

import (
	"errors"
	"io"
	"math"
	"os"
	"reflect"
	"strconv"
	"sync"
	"unicode/utf8"
)

// ScanState 代表传递给自定义扫描器的扫描器状态。
// 扫描器可以一次一个符文地扫描或要求 ScanState
// 发现下一个空格分隔的令牌。
type ScanState interface {
	// ReadRune 从输入读取下一个符文（Unicode 码点）。
	// 如果在 Scanln、Fscanln 或 Sscanln 期间调用，ReadRune() 将
	// 在返回第一个 '\n' 或读取超过
	// 指定宽度时返回 EOF。
	ReadRune() (r rune, size int, err error)
	// UnreadRune 导致对 ReadRune 的下一个调用返回相同的符文。
	UnreadRune() error
	// SkipSpace 跳过输入中的空格。根据
	// 正在执行的操作适当处理换行符；详见包文档
	// 了解更多信息。
	SkipSpace()
	// Token 如果 skipSpace 为 true 则跳过输入中的空格，然后返回
	// 满足 f(c) 的 Unicode 码点 c 的运行。如果 f 为 nil，
	// 则使用 !unicode.IsSpace(c)；即令牌将保持非空格
	// 字符。根据正在执行的操作适当处理换行符；
	// 详见包文档了解更多信息。
	// 返回的切片指向可能被覆盖的共享数据
	// 通过对 Token 的下一个调用、使用 ScanState 作为输入的
	// Scan 函数调用，或当调用的 Scan 方法返回时。
	Token(skipSpace bool, f func(rune) bool) (token []byte, err error)
	// Width 返回宽度选项的值以及它是否已设置。
	// 单位是 Unicode 码点。
	Width() (wid int, ok bool)
	// 因为 ReadRune 由接口实现，Read 不应该被
	// 扫描例程调用，ScanState 的有效实现
	// 可能会选择总是从 Read 返回错误。
	Read(buf []byte) (n int, err error)
}

// Scanner 由任何具有 Scan 方法的值实现，该方法扫描
// 输入以查找值的表示并将结果存储在
// 接收者中，接收者必须是指针才有用。对于
// [Scan]、[Scanf] 或 [Scanln] 的任何实现此接口的参数，
// Scan 方法被调用。
type Scanner interface {
	Scan(state ScanState, verb rune) error
}

// Scan 扫描从标准输入读取的文本，将连续的
// 空格分隔的值存储到连续的参数中。换行符计算为
// 空格。返回成功扫描的项数。
// 如果少于参数数，err 将说明原因。
func Scan(a ...any) (n int, err error) {
	return Fscan(os.Stdin, a...)
}

// Scanln 类似于 [Scan]，但在换行符处停止扫描，
// 最后一项之后必须有换行符或 EOF。
func Scanln(a ...any) (n int, err error) {
	return Fscanln(os.Stdin, a...)
}

// Scanf 扫描从标准输入读取的文本，将连续的
// 空格分隔的值存储到连续的参数中，由
// 格式确定。返回成功扫描的项数。
// 如果少于参数数，err 将说明原因。
// 输入中的换行符必须与格式中的换行符匹配。
// 一个例外：动词 %c 总是扫描输入中的下一个符文，
// 即使它是空格（或制表符等）或换行符。
func Scanf(format string, a ...any) (n int, err error) {
	return Fscanf(os.Stdin, format, a...)
}

type stringReader string

func (r *stringReader) Read(b []byte) (n int, err error) {
	n = copy(b, *r)
	*r = (*r)[n:]
	if n == 0 {
		err = io.EOF
	}
	return
}

// Sscan 扫描参数字符串，将连续的空格分隔的
// 值存储到连续的参数中。换行符计为空格。它
// 返回成功扫描的项数。如果少于
// 参数数，err 将说明原因。
func Sscan(str string, a ...any) (n int, err error) {
	return Fscan((*stringReader)(&str), a...)
}

// Sscanln 类似于 [Sscan]，但在换行符处停止扫描，
// 最后一项之后必须有换行符或 EOF。
func Sscanln(str string, a ...any) (n int, err error) {
	return Fscanln((*stringReader)(&str), a...)
}

// Sscanf 扫描参数字符串，将连续的空格分隔的
// 值存储到连续的参数中，由格式确定。它
// 返回成功解析的项数。
// 输入中的换行符必须与格式中的换行符匹配。
func Sscanf(str string, format string, a ...any) (n int, err error) {
	return Fscanf((*stringReader)(&str), format, a...)
}

// Fscan 扫描从 r 读取的文本，将连续的空格分隔的
// 值存储到连续的参数中。换行符计为空格。它
// 返回成功扫描的项数。如果少于
// 参数数，err 将说明原因。
func Fscan(r io.Reader, a ...any) (n int, err error) {
	s, old := newScanState(r, true, false)
	n, err = s.doScan(a)
	s.free(old)
	return
}

// Fscanln 类似于 [Fscan]，但在换行符处停止扫描，
// 最后一项之后必须有换行符或 EOF。
func Fscanln(r io.Reader, a ...any) (n int, err error) {
	s, old := newScanState(r, false, true)
	n, err = s.doScan(a)
	s.free(old)
	return
}

// Fscanf 扫描从 r 读取的文本，将连续的空格分隔的
// 值存储到连续的参数中，由格式确定。它
// 返回成功解析的项数。
// 输入中的换行符必须与格式中的换行符匹配。
func Fscanf(r io.Reader, format string, a ...any) (n int, err error) {
	s, old := newScanState(r, false, false)
	n, err = s.doScanf(format, a)
	s.free(old)
	return
}

// scanError 代表由扫描软件生成的错误。
// 它被用作唯一的签名来识别恢复时的这类错误。
type scanError struct {
	err error
}

const eof = -1

// ss 是 ScanState 的内部实现。
type ss struct {
	rs    io.RuneScanner // 从哪里读取输入
	buf   buffer         // 令牌累加器
	count int            // 迄今为止消耗的符文。
	atEOF bool           // 已读取 EOF
	ssave
}

// ssave 保持 ss 的需要被保存和恢复的部分
// 在递归扫描中。
type ssave struct {
	validSave bool // 是或曾是实际 ss 的一部分。
	nlIsEnd   bool // 换行符是否终止扫描
	nlIsSpace bool // 换行符是否计为空白
	argLimit  int  // ss.count 对此参数的最大值；argLimit <= limit
	limit     int  // ss.count 的最大值。
	maxWid    int  // 此参数的宽度。
}

// Read 方法仅在 ScanState 中，以便 ScanState
// 满足 io.Reader。它在按预期使用时不会被调用，
// 所以不需要使其实际工作。
func (s *ss) Read(buf []byte) (n int, err error) {
	return 0, errors.New("ScanState's Read should not be called. Use ReadRune")
}

func (s *ss) ReadRune() (r rune, size int, err error) {
	if s.atEOF || s.count >= s.argLimit {
		err = io.EOF
		return
	}

	r, size, err = s.rs.ReadRune()
	if err == nil {
		s.count++
		if s.nlIsEnd && r == '\n' {
			s.atEOF = true
		}
	} else if err == io.EOF {
		s.atEOF = true
	}
	return
}

func (s *ss) Width() (wid int, ok bool) {
	if s.maxWid == hugeWid {
		return 0, false
	}
	return s.maxWid, true
}

// 公共方法返回错误；此私有方法恐慌。
// 如果 getRune 到达 EOF，返回值是 EOF (-1)。
func (s *ss) getRune() (r rune) {
	r, _, err := s.ReadRune()
	if err != nil {
		if err == io.EOF {
			return eof
		}
		s.error(err)
	}
	return
}

// mustReadRune 将 io.EOF 转换为 panic(io.ErrUnexpectedEOF)。
// 它在诸如字符串扫描之类的情况下调用，其中 EOF 是
// 语法错误。
func (s *ss) mustReadRune() (r rune) {
	r = s.getRune()
	if r == eof {
		s.error(io.ErrUnexpectedEOF)
	}
	return
}

func (s *ss) UnreadRune() error {
	s.rs.UnreadRune()
	s.atEOF = false
	s.count--
	return nil
}

func (s *ss) error(err error) {
	panic(scanError{err})
}

func (s *ss) errorString(err string) {
	panic(scanError{errors.New(err)})
}

func (s *ss) Token(skipSpace bool, f func(rune) bool) (tok []byte, err error) {
	defer func() {
		if e := recover(); e != nil {
			if se, ok := e.(scanError); ok {
				err = se.err
			} else {
				panic(e)
			}
		}
	}()
	if f == nil {
		f = notSpace
	}
	s.buf = s.buf[:0]
	tok = s.token(skipSpace, f)
	return
}

// space 是 unicode.White_Space 范围的副本，
// 以避免依赖包 unicode。
var space = [][2]uint16{
	{0x0009, 0x000d},
	{0x0020, 0x0020},
	{0x0085, 0x0085},
	{0x00a0, 0x00a0},
	{0x1680, 0x1680},
	{0x2000, 0x200a},
	{0x2028, 0x2029},
	{0x202f, 0x202f},
	{0x205f, 0x205f},
	{0x3000, 0x3000},
}

func isSpace(r rune) bool {
	if r >= 1<<16 {
		return false
	}
	rx := uint16(r)
	for _, rng := range space {
		if rx < rng[0] {
			return false
		}
		if rx <= rng[1] {
			return true
		}
	}
	return false
}

// notSpace 是 Token 中使用的默认扫描函数。
func notSpace(r rune) bool {
	return !isSpace(r)
}

// readRune 是一个结构，用于从 io.Reader 读取
// UTF-8 编码的码点。如果给扫描器的 Reader
// 还未实现 io.RuneScanner，则使用此结构。
type readRune struct {
	reader   io.Reader
	buf      [utf8.UTFMax]byte // 仅在 ReadRune 内部使用
	pending  int               // pendBuf 中的字节数；仅对坏 UTF-8 >0
	pendBuf  [utf8.UTFMax]byte // 剩余字节
	peekRune rune              // 如果 >=0 下一个符文；当 <0 时是 ^(previous Rune)
}

// readByte 从输入返回下一个字节，可能
// 来自前一个读取的剩余部分（如果 UTF-8 格式不正确）。
func (r *readRune) readByte() (b byte, err error) {
	if r.pending > 0 {
		b = r.pendBuf[0]
		copy(r.pendBuf[0:], r.pendBuf[1:])
		r.pending--
		return
	}
	n, err := io.ReadFull(r.reader, r.pendBuf[:1])
	if n != 1 {
		return 0, err
	}
	return r.pendBuf[0], err
}

// ReadRune 从 r 内的 io.Reader 返回下一个 UTF-8 编码的码点。
func (r *readRune) ReadRune() (rr rune, size int, err error) {
	if r.peekRune >= 0 {
		rr = r.peekRune
		r.peekRune = ^r.peekRune
		size = utf8.RuneLen(rr)
		return
	}
	r.buf[0], err = r.readByte()
	if err != nil {
		return
	}
	if r.buf[0] < utf8.RuneSelf { // 常见 ASCII 情况的快速检查
		rr = rune(r.buf[0])
		size = 1 // 已知为 1。
		// 翻转符文的位，以便 UnreadRune 可用。
		r.peekRune = ^rr
		return
	}
	var n int
	for n = 1; !utf8.FullRune(r.buf[:n]); n++ {
		r.buf[n], err = r.readByte()
		if err != nil {
			if err == io.EOF {
				err = nil
				break
			}
			return
		}
	}
	rr, size = utf8.DecodeRune(r.buf[:n])
	if size < n { // 一个错误，为下一个读取保存字节
		copy(r.pendBuf[r.pending:], r.buf[size:n])
		r.pending += n - size
	}
	// 翻转符文的位，以便 UnreadRune 可用。
	r.peekRune = ^rr
	return
}

func (r *readRune) UnreadRune() error {
	if r.peekRune >= 0 {
		return errors.New("fmt: scanning called UnreadRune with no rune available")
	}
	// Reverse bit flip of previously read rune to obtain valid >=0 state.
	r.peekRune = ^r.peekRune
	return nil
}

var ssFree = sync.Pool{
	New: func() any { return new(ss) },
}

// newScanState 分配一个新的 ss 结构或获取一个缓存的。
func newScanState(r io.Reader, nlIsSpace, nlIsEnd bool) (s *ss, old ssave) {
	s = ssFree.Get().(*ss)
	if rs, ok := r.(io.RuneScanner); ok {
		s.rs = rs
	} else {
		s.rs = &readRune{reader: r, peekRune: -1}
	}
	s.nlIsSpace = nlIsSpace
	s.nlIsEnd = nlIsEnd
	s.atEOF = false
	s.limit = hugeWid
	s.argLimit = hugeWid
	s.maxWid = hugeWid
	s.validSave = true
	s.count = 0
	return
}

// free 在 ssFree 中保存使用过的 ss 结构；避免每次调用分配。
func (s *ss) free(old ssave) {
	// 如果它被递归使用，只需恢复旧状态。
	if old.validSave {
		s.ssave = old
		return
	}
	// 不要保留具有大缓冲区的 ss 结构。
	if cap(s.buf) > 1024 {
		return
	}
	s.buf = s.buf[:0]
	s.rs = nil
	ssFree.Put(s)
}

// SkipSpace 为 Scan 方法提供了跳过空格和换行符
// 字符的能力，与由格式字符串和
// [Scan]/[Scanln] 设置的当前扫描模式保持一致。
func (s *ss) SkipSpace() {
	for {
		r := s.getRune()
		if r == eof {
			return
		}
		if r == '\r' && s.peek("\n") {
			continue
		}
		if r == '\n' {
			if s.nlIsSpace {
				continue
			}
			s.errorString("unexpected newline")
			return
		}
		if !isSpace(r) {
			s.UnreadRune()
			break
		}
	}
}

// token 返回输入中的下一个空格分隔的字符串。它
// 跳过空白。对于 Scanln，它在换行符处停止。对于 Scan，
// 换行符被视为空格。
func (s *ss) token(skipSpace bool, f func(rune) bool) []byte {
	if skipSpace {
		s.SkipSpace()
	}
	// 读取直到空白或换行符
	for {
		r := s.getRune()
		if r == eof {
			break
		}
		if !f(r) {
			s.UnreadRune()
			break
		}
		s.buf.writeRune(r)
	}
	return s.buf
}

var errComplex = errors.New("syntax error scanning complex number")
var errBool = errors.New("syntax error scanning boolean")

func indexRune(s string, r rune) int {
	for i, c := range s {
		if c == r {
			return i
		}
	}
	return -1
}

// consume 读取输入中的下一个符文并报告它是否在 ok 字符串中。
// 如果 accept 为 true，它将字符放入输入令牌中。
func (s *ss) consume(ok string, accept bool) bool {
	r := s.getRune()
	if r == eof {
		return false
	}
	if indexRune(ok, r) >= 0 {
		if accept {
			s.buf.writeRune(r)
		}
		return true
	}
	if r != eof && accept {
		s.UnreadRune()
	}
	return false
}

// peek 报告下一个字符是否在 ok 字符串中，而不消耗它。
func (s *ss) peek(ok string) bool {
	r := s.getRune()
	if r != eof {
		s.UnreadRune()
	}
	return indexRune(ok, r) >= 0
}

func (s *ss) notEOF() {
	// 保证有数据可读。
	if r := s.getRune(); r == eof {
		panic(io.EOF)
	}
	s.UnreadRune()
}

// accept 检查输入中的下一个符文。如果它在字符串中，
// 它将其放入缓冲区并返回 true。否则返回 false。
func (s *ss) accept(ok string) bool {
	return s.consume(ok, true)
}

// okVerb 验证动词是否在列表中，如果不在则适当设置 s.err。
func (s *ss) okVerb(verb rune, okVerbs, typ string) bool {
	for _, v := range okVerbs {
		if v == verb {
			return true
		}
	}
	s.errorString("bad verb '%" + string(verb) + "' for " + typ)
	return false
}

// scanBool returns the value of the boolean represented by the next token.
func (s *ss) scanBool(verb rune) bool {
	s.SkipSpace()
	s.notEOF()
	if !s.okVerb(verb, "tv", "boolean") {
		return false
	}
	// Syntax-checking a boolean is annoying. We're not fastidious about case.
	switch s.getRune() {
	case '0':
		return false
	case '1':
		return true
	case 't', 'T':
		if s.accept("rR") && (!s.accept("uU") || !s.accept("eE")) {
			s.error(errBool)
		}
		return true
	case 'f', 'F':
		if s.accept("aA") && (!s.accept("lL") || !s.accept("sS") || !s.accept("eE")) {
			s.error(errBool)
		}
		return false
	}
	return false
}

// Numerical elements
const (
	binaryDigits      = "01"
	octalDigits       = "01234567"
	decimalDigits     = "0123456789"
	hexadecimalDigits = "0123456789aAbBcCdDeEfF"
	sign              = "+-"
	period            = "."
	exponent          = "eEpP"
)

// getBase returns the numeric base represented by the verb and its digit string.
func (s *ss) getBase(verb rune) (base int, digits string) {
	s.okVerb(verb, "bdoUxXv", "integer") // sets s.err
	base = 10
	digits = decimalDigits
	switch verb {
	case 'b':
		base = 2
		digits = binaryDigits
	case 'o':
		base = 8
		digits = octalDigits
	case 'x', 'X', 'U':
		base = 16
		digits = hexadecimalDigits
	}
	return
}

// scanNumber returns the numerical string with specified digits starting here.
func (s *ss) scanNumber(digits string, haveDigits bool) string {
	if !haveDigits {
		s.notEOF()
		if !s.accept(digits) {
			s.errorString("expected integer")
		}
	}
	for s.accept(digits) {
	}
	return string(s.buf)
}

// scanRune returns the next rune value in the input.
func (s *ss) scanRune(bitSize int) int64 {
	s.notEOF()
	r := s.getRune()
	n := uint(bitSize)
	x := (int64(r) << (64 - n)) >> (64 - n)
	if x != int64(r) {
		s.errorString("overflow on character value " + string(r))
	}
	return int64(r)
}

// scanBasePrefix reports whether the integer begins with a base prefix
// and returns the base, digit string, and whether a zero was found.
// It is called only if the verb is %v.
func (s *ss) scanBasePrefix() (base int, digits string, zeroFound bool) {
	if !s.peek("0") {
		return 0, decimalDigits + "_", false
	}
	s.accept("0")
	// Special cases for 0, 0b, 0o, 0x.
	switch {
	case s.peek("bB"):
		s.consume("bB", true)
		return 0, binaryDigits + "_", true
	case s.peek("oO"):
		s.consume("oO", true)
		return 0, octalDigits + "_", true
	case s.peek("xX"):
		s.consume("xX", true)
		return 0, hexadecimalDigits + "_", true
	default:
		return 0, octalDigits + "_", true
	}
}

// scanInt returns the value of the integer represented by the next
// token, checking for overflow. Any error is stored in s.err.
func (s *ss) scanInt(verb rune, bitSize int) int64 {
	if verb == 'c' {
		return s.scanRune(bitSize)
	}
	s.SkipSpace()
	s.notEOF()
	base, digits := s.getBase(verb)
	haveDigits := false
	if verb == 'U' {
		if !s.consume("U", false) || !s.consume("+", false) {
			s.errorString("bad unicode format ")
		}
	} else {
		s.accept(sign) // If there's a sign, it will be left in the token buffer.
		if verb == 'v' {
			base, digits, haveDigits = s.scanBasePrefix()
		}
	}
	tok := s.scanNumber(digits, haveDigits)
	i, err := strconv.ParseInt(tok, base, 64)
	if err != nil {
		s.error(err)
	}
	n := uint(bitSize)
	x := (i << (64 - n)) >> (64 - n)
	if x != i {
		s.errorString("integer overflow on token " + tok)
	}
	return i
}

// scanUint returns the value of the unsigned integer represented
// by the next token, checking for overflow. Any error is stored in s.err.
func (s *ss) scanUint(verb rune, bitSize int) uint64 {
	if verb == 'c' {
		return uint64(s.scanRune(bitSize))
	}
	s.SkipSpace()
	s.notEOF()
	base, digits := s.getBase(verb)
	haveDigits := false
	if verb == 'U' {
		if !s.consume("U", false) || !s.consume("+", false) {
			s.errorString("bad unicode format ")
		}
	} else if verb == 'v' {
		base, digits, haveDigits = s.scanBasePrefix()
	}
	tok := s.scanNumber(digits, haveDigits)
	i, err := strconv.ParseUint(tok, base, 64)
	if err != nil {
		s.error(err)
	}
	n := uint(bitSize)
	x := (i << (64 - n)) >> (64 - n)
	if x != i {
		s.errorString("unsigned integer overflow on token " + tok)
	}
	return i
}

// floatToken returns the floating-point number starting here, no longer than swid
// if the width is specified. It's not rigorous about syntax because it doesn't check that
// we have at least some digits, but Atof will do that.
func (s *ss) floatToken() string {
	s.buf = s.buf[:0]
	// NaN?
	if s.accept("nN") && s.accept("aA") && s.accept("nN") {
		return string(s.buf)
	}
	// leading sign?
	s.accept(sign)
	// Inf?
	if s.accept("iI") && s.accept("nN") && s.accept("fF") {
		return string(s.buf)
	}
	digits := decimalDigits + "_"
	exp := exponent
	if s.accept("0") && s.accept("xX") {
		digits = hexadecimalDigits + "_"
		exp = "pP"
	}
	// digits?
	for s.accept(digits) {
	}
	// decimal point?
	if s.accept(period) {
		// fraction?
		for s.accept(digits) {
		}
	}
	// exponent?
	if s.accept(exp) {
		// leading sign?
		s.accept(sign)
		// digits?
		for s.accept(decimalDigits + "_") {
		}
	}
	return string(s.buf)
}

// complexTokens returns the real and imaginary parts of the complex number starting here.
// The number might be parenthesized and has the format (N+Ni) where N is a floating-point
// number and there are no spaces within.
func (s *ss) complexTokens() (real, imag string) {
	// TODO: accept N and Ni independently?
	parens := s.accept("(")
	real = s.floatToken()
	s.buf = s.buf[:0]
	// Must now have a sign.
	if !s.accept("+-") {
		s.error(errComplex)
	}
	// Sign is now in buffer
	imagSign := string(s.buf)
	imag = s.floatToken()
	if !s.accept("i") {
		s.error(errComplex)
	}
	if parens && !s.accept(")") {
		s.error(errComplex)
	}
	return real, imagSign + imag
}

func hasX(s string) bool {
	for i := 0; i < len(s); i++ {
		if s[i] == 'x' || s[i] == 'X' {
			return true
		}
	}
	return false
}

// convertFloat converts the string to a float64value.
func (s *ss) convertFloat(str string, n int) float64 {
	// strconv.ParseFloat will handle "+0x1.fp+2",
	// but we have to implement our non-standard
	// decimal+binary exponent mix (1.2p4) ourselves.
	if p := indexRune(str, 'p'); p >= 0 && !hasX(str) {
		// Atof doesn't handle power-of-2 exponents,
		// but they're easy to evaluate.
		f, err := strconv.ParseFloat(str[:p], n)
		if err != nil {
			// Put full string into error.
			if e, ok := err.(*strconv.NumError); ok {
				e.Num = str
			}
			s.error(err)
		}
		m, err := strconv.Atoi(str[p+1:])
		if err != nil {
			// Put full string into error.
			if e, ok := err.(*strconv.NumError); ok {
				e.Num = str
			}
			s.error(err)
		}
		return math.Ldexp(f, m)
	}
	f, err := strconv.ParseFloat(str, n)
	if err != nil {
		s.error(err)
	}
	return f
}

// scanComplex converts the next token to a complex128 value.
// The atof argument is a type-specific reader for the underlying type.
// If we're reading complex64, atof will parse float32s and convert them
// to float64's to avoid reproducing this code for each complex type.
func (s *ss) scanComplex(verb rune, n int) complex128 {
	if !s.okVerb(verb, floatVerbs, "complex") {
		return 0
	}
	s.SkipSpace()
	s.notEOF()
	sreal, simag := s.complexTokens()
	real := s.convertFloat(sreal, n/2)
	imag := s.convertFloat(simag, n/2)
	return complex(real, imag)
}

// convertString returns the string represented by the next input characters.
// The format of the input is determined by the verb.
func (s *ss) convertString(verb rune) (str string) {
	if !s.okVerb(verb, "svqxX", "string") {
		return ""
	}
	s.SkipSpace()
	s.notEOF()
	switch verb {
	case 'q':
		str = s.quotedString()
	case 'x', 'X':
		str = s.hexString()
	default:
		str = string(s.token(true, notSpace)) // %s and %v just return the next word
	}
	return
}

// quotedString returns the double- or back-quoted string represented by the next input characters.
func (s *ss) quotedString() string {
	s.notEOF()
	quote := s.getRune()
	switch quote {
	case '`':
		// Back-quoted: Anything goes until EOF or back quote.
		for {
			r := s.mustReadRune()
			if r == quote {
				break
			}
			s.buf.writeRune(r)
		}
		return string(s.buf)
	case '"':
		// Double-quoted: Include the quotes and let strconv.Unquote do the backslash escapes.
		s.buf.writeByte('"')
		for {
			r := s.mustReadRune()
			s.buf.writeRune(r)
			if r == '\\' {
				// In a legal backslash escape, no matter how long, only the character
				// immediately after the escape can itself be a backslash or quote.
				// Thus we only need to protect the first character after the backslash.
				s.buf.writeRune(s.mustReadRune())
			} else if r == '"' {
				break
			}
		}
		result, err := strconv.Unquote(string(s.buf))
		if err != nil {
			s.error(err)
		}
		return result
	default:
		s.errorString("expected quoted string")
	}
	return ""
}

// hexDigit returns the value of the hexadecimal digit.
func hexDigit(d rune) (int, bool) {
	digit := int(d)
	switch digit {
	case '0', '1', '2', '3', '4', '5', '6', '7', '8', '9':
		return digit - '0', true
	case 'a', 'b', 'c', 'd', 'e', 'f':
		return 10 + digit - 'a', true
	case 'A', 'B', 'C', 'D', 'E', 'F':
		return 10 + digit - 'A', true
	}
	return -1, false
}

// hexByte returns the next hex-encoded (two-character) byte from the input.
// It returns ok==false if the next bytes in the input do not encode a hex byte.
// If the first byte is hex and the second is not, processing stops.
func (s *ss) hexByte() (b byte, ok bool) {
	rune1 := s.getRune()
	if rune1 == eof {
		return
	}
	value1, ok := hexDigit(rune1)
	if !ok {
		s.UnreadRune()
		return
	}
	value2, ok := hexDigit(s.mustReadRune())
	if !ok {
		s.errorString("illegal hex digit")
		return
	}
	return byte(value1<<4 | value2), true
}

// hexString returns the space-delimited hexpair-encoded string.
func (s *ss) hexString() string {
	s.notEOF()
	for {
		b, ok := s.hexByte()
		if !ok {
			break
		}
		s.buf.writeByte(b)
	}
	if len(s.buf) == 0 {
		s.errorString("no hex data for %x string")
		return ""
	}
	return string(s.buf)
}

const (
	floatVerbs = "beEfFgGv"

	hugeWid = 1 << 30

	intBits     = 32 << (^uint(0) >> 63)
	uintptrBits = 32 << (^uintptr(0) >> 63)
)

// scanPercent scans a literal percent character.
func (s *ss) scanPercent() {
	s.SkipSpace()
	s.notEOF()
	if !s.accept("%") {
		s.errorString("missing literal %")
	}
}

// scanOne scans a single value, deriving the scanner from the type of the argument.
func (s *ss) scanOne(verb rune, arg any) {
	s.buf = s.buf[:0]
	var err error
	// If the parameter has its own Scan method, use that.
	if v, ok := arg.(Scanner); ok {
		err = v.Scan(s, verb)
		if err != nil {
			if err == io.EOF {
				err = io.ErrUnexpectedEOF
			}
			s.error(err)
		}
		return
	}

	switch v := arg.(type) {
	case *bool:
		*v = s.scanBool(verb)
	case *complex64:
		*v = complex64(s.scanComplex(verb, 64))
	case *complex128:
		*v = s.scanComplex(verb, 128)
	case *int:
		*v = int(s.scanInt(verb, intBits))
	case *int8:
		*v = int8(s.scanInt(verb, 8))
	case *int16:
		*v = int16(s.scanInt(verb, 16))
	case *int32:
		*v = int32(s.scanInt(verb, 32))
	case *int64:
		*v = s.scanInt(verb, 64)
	case *uint:
		*v = uint(s.scanUint(verb, intBits))
	case *uint8:
		*v = uint8(s.scanUint(verb, 8))
	case *uint16:
		*v = uint16(s.scanUint(verb, 16))
	case *uint32:
		*v = uint32(s.scanUint(verb, 32))
	case *uint64:
		*v = s.scanUint(verb, 64)
	case *uintptr:
		*v = uintptr(s.scanUint(verb, uintptrBits))
	// Floats are tricky because you want to scan in the precision of the result, not
	// scan in high precision and convert, in order to preserve the correct error condition.
	case *float32:
		if s.okVerb(verb, floatVerbs, "float32") {
			s.SkipSpace()
			s.notEOF()
			*v = float32(s.convertFloat(s.floatToken(), 32))
		}
	case *float64:
		if s.okVerb(verb, floatVerbs, "float64") {
			s.SkipSpace()
			s.notEOF()
			*v = s.convertFloat(s.floatToken(), 64)
		}
	case *string:
		*v = s.convertString(verb)
	case *[]byte:
		// We scan to string and convert so we get a copy of the data.
		// If we scanned to bytes, the slice would point at the buffer.
		*v = []byte(s.convertString(verb))
	default:
		val := reflect.ValueOf(v)
		ptr := val
		if ptr.Kind() != reflect.Pointer {
			s.errorString("type not a pointer: " + val.Type().String())
			return
		}
		switch v := ptr.Elem(); v.Kind() {
		case reflect.Bool:
			v.SetBool(s.scanBool(verb))
		case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
			v.SetInt(s.scanInt(verb, v.Type().Bits()))
		case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64, reflect.Uintptr:
			v.SetUint(s.scanUint(verb, v.Type().Bits()))
		case reflect.String:
			v.SetString(s.convertString(verb))
		case reflect.Slice:
			// For now, can only handle (renamed) []byte.
			typ := v.Type()
			if typ.Elem().Kind() != reflect.Uint8 {
				s.errorString("can't scan type: " + val.Type().String())
			}
			str := s.convertString(verb)
			v.Set(reflect.MakeSlice(typ, len(str), len(str)))
			for i := 0; i < len(str); i++ {
				v.Index(i).SetUint(uint64(str[i]))
			}
		case reflect.Float32, reflect.Float64:
			s.SkipSpace()
			s.notEOF()
			v.SetFloat(s.convertFloat(s.floatToken(), v.Type().Bits()))
		case reflect.Complex64, reflect.Complex128:
			v.SetComplex(s.scanComplex(verb, v.Type().Bits()))
		default:
			s.errorString("can't scan type: " + val.Type().String())
		}
	}
}

// errorHandler turns local panics into error returns.
func errorHandler(errp *error) {
	if e := recover(); e != nil {
		if se, ok := e.(scanError); ok { // catch local error
			*errp = se.err
		} else if eof, ok := e.(error); ok && eof == io.EOF { // out of input
			*errp = eof
		} else {
			panic(e)
		}
	}
}

// doScan does the real work for scanning without a format string.
func (s *ss) doScan(a []any) (numProcessed int, err error) {
	defer errorHandler(&err)
	for _, arg := range a {
		s.scanOne('v', arg)
		numProcessed++
	}
	// Check for newline (or EOF) if required (Scanln etc.).
	if s.nlIsEnd {
		for {
			r := s.getRune()
			if r == '\n' || r == eof {
				break
			}
			if !isSpace(r) {
				s.errorString("expected newline")
				break
			}
		}
	}
	return
}

// advance determines whether the next characters in the input match
// those of the format. It returns the number of bytes (sic) consumed
// in the format. All runs of space characters in either input or
// format behave as a single space. Newlines are special, though:
// newlines in the format must match those in the input and vice versa.
// This routine also handles the %% case. If the return value is zero,
// either format starts with a % (with no following %) or the input
// is empty. If it is negative, the input did not match the string.
func (s *ss) advance(format string) (i int) {
	for i < len(format) {
		fmtc, w := utf8.DecodeRuneInString(format[i:])

		// Space processing.
		// In the rest of this comment "space" means spaces other than newline.
		// Newline in the format matches input of zero or more spaces and then newline or end-of-input.
		// Spaces in the format before the newline are collapsed into the newline.
		// Spaces in the format after the newline match zero or more spaces after the corresponding input newline.
		// Other spaces in the format match input of one or more spaces or end-of-input.
		if isSpace(fmtc) {
			newlines := 0
			trailingSpace := false
			for isSpace(fmtc) && i < len(format) {
				if fmtc == '\n' {
					newlines++
					trailingSpace = false
				} else {
					trailingSpace = true
				}
				i += w
				fmtc, w = utf8.DecodeRuneInString(format[i:])
			}
			for j := 0; j < newlines; j++ {
				inputc := s.getRune()
				for isSpace(inputc) && inputc != '\n' {
					inputc = s.getRune()
				}
				if inputc != '\n' && inputc != eof {
					s.errorString("newline in format does not match input")
				}
			}
			if trailingSpace {
				inputc := s.getRune()
				if newlines == 0 {
					// If the trailing space stood alone (did not follow a newline),
					// it must find at least one space to consume.
					if !isSpace(inputc) && inputc != eof {
						s.errorString("expected space in input to match format")
					}
					if inputc == '\n' {
						s.errorString("newline in input does not match format")
					}
				}
				for isSpace(inputc) && inputc != '\n' {
					inputc = s.getRune()
				}
				if inputc != eof {
					s.UnreadRune()
				}
			}
			continue
		}

		// Verbs.
		if fmtc == '%' {
			// % at end of string is an error.
			if i+w == len(format) {
				s.errorString("missing verb: % at end of format string")
			}
			// %% acts like a real percent
			nextc, _ := utf8.DecodeRuneInString(format[i+w:]) // will not match % if string is empty
			if nextc != '%' {
				return
			}
			i += w // skip the first %
		}

		// Literals.
		inputc := s.mustReadRune()
		if fmtc != inputc {
			s.UnreadRune()
			return -1
		}
		i += w
	}
	return
}

// doScanf does the real work when scanning with a format string.
// At the moment, it handles only pointers to basic types.
func (s *ss) doScanf(format string, a []any) (numProcessed int, err error) {
	defer errorHandler(&err)
	end := len(format) - 1
	// We process one item per non-trivial format
	for i := 0; i <= end; {
		w := s.advance(format[i:])
		if w > 0 {
			i += w
			continue
		}
		// Either we failed to advance, we have a percent character, or we ran out of input.
		if format[i] != '%' {
			// Can't advance format. Why not?
			if w < 0 {
				s.errorString("input does not match format")
			}
			// Otherwise at EOF; "too many operands" error handled below
			break
		}
		i++ // % is one byte

		// do we have 20 (width)?
		var widPresent bool
		s.maxWid, widPresent, i = parsenum(format, i, end)
		if !widPresent {
			s.maxWid = hugeWid
		}

		c, w := utf8.DecodeRuneInString(format[i:])
		i += w

		if c != 'c' {
			s.SkipSpace()
		}
		if c == '%' {
			s.scanPercent()
			continue // Do not consume an argument.
		}
		s.argLimit = s.limit
		if f := s.count + s.maxWid; f < s.argLimit {
			s.argLimit = f
		}

		if numProcessed >= len(a) { // out of operands
			s.errorString("too few operands for format '%" + format[i-w:] + "'")
			break
		}
		arg := a[numProcessed]

		s.scanOne(c, arg)
		numProcessed++
		s.argLimit = s.limit
	}
	if numProcessed < len(a) {
		s.errorString("too many operands")
	}
	return
}
