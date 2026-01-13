// Copyright 2013 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package bufio

import (
	"bytes"
	"errors"
	"io"
	"unicode/utf8"
)

// Scanner 为读取数据提供了一个方便的接口，例如
// 换行符分隔的文本行文件。对 [Scanner.Scan] 方法的连续调用
// 将逐步通过文件的 'token'，跳过
// token 之间的字节。token 的规范由
// [SplitFunc] 类型的分割函数定义；默认分割
// 函数将输入分解为去除行终止符的行。[Scanner.Split]
// 函数在此包中定义，用于将文件扫描为
// 行、字节、UTF-8 编码的 rune 和空格分隔的单词。
// 客户端可以改为提供自定义分割函数。
//
// 扫描在 EOF、第一个 I/O 错误或太大而无法
// 适应 [Scanner.Buffer] 的 token 处无法恢复地停止。
// 扫描停止时，读取器可能已
// 远远超过最后一个 token。需要对
// 错误处理或大 token 进行更多控制，或必须在
// 读取器上运行顺序扫描的程序应该使用 [bufio.Reader]。
type Scanner struct {
	r            io.Reader // 客户端提供的读取器。
	split        SplitFunc // 用于分割 token 的函数。
	maxTokenSize int       // token 的最大大小；由测试修改。
	token        []byte    // split 返回的最后一个 token。
	buf          []byte    // 用作 split 参数的缓冲区。
	start        int       // buf 中第一个未处理的字节。
	end          int       // buf 中数据的结尾。
	err          error     // 粘性错误。
	empties      int       // 连续空 token 的计数。
	scanCalled   bool      // Scan 已被调用；缓冲区正在使用中。
	done         bool      // Scan 已完成。
}

// SplitFunc 是用于标记化输入的分割函数的签名。
// 参数是剩余未处理数据的初始子字符串
// 和一个标志 atEOF，报告 [Reader] 是否没有更多数据
// 可以提供。返回值是推进输入的字节数
// 以及要返回给用户的下一个 token（如果有），加上错误（如果有）。
//
// 如果函数返回错误，扫描停止，在这种情况下某些
// 输入可能会被丢弃。如果该错误是 [ErrFinalToken]，扫描
// 无错误停止。与 [ErrFinalToken] 一起传送的非 nil token
// 将是最后一个 token，而与 [ErrFinalToken] 一起的 nil token
// 立即停止扫描。
//
// 否则，[Scanner] 推进输入。如果 token 不是 nil，
// [Scanner] 将其返回给用户。如果 token 是 nil，
// Scanner 读取更多数据并继续扫描；如果没有更多
// 数据--如果 atEOF 为 true--[Scanner] 返回。如果数据
// 还没有保存完整 token，例如在扫描行时没有换行符，
// [SplitFunc] 可以返回 (0, nil, nil) 来通知 [Scanner]
// 将更多数据读入切片并在输入中的同一点开始
// 更长的切片重试。
//
// 除非 atEOF 为 true，否则永远不会使用空数据切片调用该函数。
// 但是，如果 atEOF 为 true，数据可能非空，
// 并且照例保存未处理的文本。
type SplitFunc func(data []byte, atEOF bool) (advance int, token []byte, err error)

// Scanner 返回的错误。
var (
	ErrTooLong         = errors.New("bufio.Scanner: token too long")
	ErrNegativeAdvance = errors.New("bufio.Scanner: SplitFunc returns negative advance count")
	ErrAdvanceTooFar   = errors.New("bufio.Scanner: SplitFunc returns advance count beyond input")
	ErrBadReadCount    = errors.New("bufio.Scanner: Read returned impossible count")
)

const (
	// MaxScanTokenSize 是用于缓冲 token 的最大大小
	// 除非用户使用 [Scanner.Buffer] 提供显式缓冲区。
	// 实际的最大 token 大小可能较小，因为缓冲区
	// 可能需要包括例如换行符。
	MaxScanTokenSize = 64 * 1024

	startBufSize = 4096 // 缓冲区初始分配的大小。
)

// NewScanner 返回一个新的 [Scanner] 来从 r 读取。
// 分割函数默认为 [ScanLines]。
func NewScanner(r io.Reader) *Scanner {
	return &Scanner{
		r:            r,
		split:        ScanLines,
		maxTokenSize: MaxScanTokenSize,
	}
}

// Err 返回 [Scanner] 遇到的第一个非 EOF 错误。
func (s *Scanner) Err() error {
	if s.err == io.EOF {
		return nil
	}
	return s.err
}

// Bytes 返回通过调用 [Scanner.Scan] 生成的最新 token。
// 底层数组可能指向将被随后的
// Scan 调用覆盖的数据。它不进行任何分配。
func (s *Scanner) Bytes() []byte {
	return s.token
}

// Text 返回通过调用 [Scanner.Scan] 生成的最新 token
// 作为新分配的保存其字节的字符串。
func (s *Scanner) Text() string {
	return string(s.token)
}

// ErrFinalToken 是一个特殊的哨兵错误值。它旨在
// 由分割函数返回以指示扫描应该
// 无错误停止。如果与此错误一起传送的 token 不是 nil，
// 该 token 是最后一个 token。
//
// 该值有助于提前停止处理或者当需要
// 传送最终空 token 时（它不同于 nil token）。
// 可以用自定义错误值实现相同的行为，但
// 在这里提供它更整洁。
// 有关此值的用法，请参见 emptyFinalToken 示例。
var ErrFinalToken = errors.New("final token")

// Scan 将 [Scanner] 推进到下一个 token，然后
// 可通过 [Scanner.Bytes] 或 [Scanner.Text] 方法使用。
// 当没有更多 token 时返回 false，
// 通过到达输入的末尾或错误。
// Scan 返回 false 后，[Scanner.Err] 方法将返回任何
// 扫描期间发生的错误，除了如果它是 [io.EOF]，[Scanner.Err]
// 将返回 nil。
// 如果分割函数返回太多空
// token 而不推进输入，Scan 会 panic。
// 这是扫描器的常见错误模式。
func (s *Scanner) Scan() bool {
	if s.done {
		return false
	}
	s.scanCalled = true
	// 循环直到我们有一个 token。
	for {
		// 看看我们能否使用我们已有的内容获得一个 token。
		// 如果我们已经用尽了数据但有错误，给分割函数
		// 一个机会来恢复任何剩余的、可能为空的 token。
		if s.end > s.start || s.err != nil {
			advance, token, err := s.split(s.buf[s.start:s.end], s.err != nil)
			if err != nil {
				if err == ErrFinalToken {
					s.token = token
					s.done = true
					// 当 token 不是 nil 时，这意味着扫描停止
					// 带有尾部 token，因此返回值
					// 应该为 true 以指示 token 的存在。
					return token != nil
				}
				s.setErr(err)
				return false
			}
			if !s.advance(advance) {
				return false
			}
			s.token = token
			if token != nil {
				if s.err == nil || advance > 0 {
					s.empties = 0
				} else {
					// 在 EOF 时返回 token 但不推进输入。
					s.empties++
					if s.empties > maxConsecutiveEmptyReads {
						panic("bufio.Scan: too many empty tokens without progressing")
					}
				}
				return true
			}
		}
		// 我们无法使用我们拥有的东西生成 token。
		// 如果我们已经遇到 EOF 或 I/O 错误，我们完成了。
		if s.err != nil {
			// 关闭它。
			s.start = 0
			s.end = 0
			return false
		}
		// 必须读取更多数据。
		// 首先，如果有大量空闲空间或需要空间，
		// 将数据移动到缓冲区的开头。
		if s.start > 0 && (s.end == len(s.buf) || s.start > len(s.buf)/2) {
			copy(s.buf, s.buf[s.start:s.end])
			s.end -= s.start
			s.start = 0
		}
		// 缓冲区满吗？如果是，调整大小。
		if s.end == len(s.buf) {
			// 保证下面乘法没有溢出。
			const maxInt = int(^uint(0) >> 1)
			if len(s.buf) >= s.maxTokenSize || len(s.buf) > maxInt/2 {
				s.setErr(ErrTooLong)
				return false
			}
			newSize := len(s.buf) * 2
			if newSize == 0 {
				newSize = startBufSize
			}
			newSize = min(newSize, s.maxTokenSize)
			newBuf := make([]byte, newSize)
			copy(newBuf, s.buf[s.start:s.end])
			s.buf = newBuf
			s.end -= s.start
			s.start = 0
		}
		// 最后我们可以读取一些输入。确保我们不会陷入
		// 一个表现不良的读取器。官方上我们不需要这样做，但让
		// 我们更加谨慎：Scanner 用于安全、简单的任务。
		for loop := 0; ; {
			n, err := s.r.Read(s.buf[s.end:len(s.buf)])
			if n < 0 || len(s.buf)-s.end < n {
				s.setErr(ErrBadReadCount)
				break
			}
			s.end += n
			if err != nil {
				s.setErr(err)
				break
			}
			if n > 0 {
				s.empties = 0
				break
			}
			loop++
			if loop > maxConsecutiveEmptyReads {
				s.setErr(io.ErrNoProgress)
				break
			}
		}
	}
}

// advance 消耗缓冲区的 n 个字节。报告前进是否合法。
func (s *Scanner) advance(n int) bool {
	if n < 0 {
		s.setErr(ErrNegativeAdvance)
		return false
	}
	if n > s.end-s.start {
		s.setErr(ErrAdvanceTooFar)
		return false
	}
	s.start += n
	return true
}

// setErr 记录遇到的第一个错误。
func (s *Scanner) setErr(err error) {
	if s.err == nil || s.err == io.EOF {
		s.err = err
	}
}

// Buffer 控制 Scanner 的内存分配。
// 它设置扫描时要使用的初始缓冲区
// 和扫描期间可能分配的缓冲区的最大大小。
// 缓冲区的内容被忽略。
//
// 最大 token 大小必须小于 max 和 cap(buf) 中的较大值。
// 如果 max <= cap(buf)，[Scanner.Scan] 将仅使用此缓冲区且不进行分配。
//
// 默认情况下，[Scanner.Scan] 使用内部缓冲区并设置
// 最大 token 大小为 [MaxScanTokenSize]。
//
// 如果在扫描开始后调用 Buffer，会 panic。
func (s *Scanner) Buffer(buf []byte, max int) {
	if s.scanCalled {
		panic("Buffer called after Scan")
	}
	s.buf = buf[0:cap(buf)]
	s.maxTokenSize = max
}

// Split 设置 [Scanner] 的分割函数。
// 默认分割函数是 [ScanLines]。
//
// 如果在扫描开始后调用 Split，会 panic。
func (s *Scanner) Split(split SplitFunc) {
	if s.scanCalled {
		panic("Split called after Scan")
	}
	s.split = split
}

// 分割函数

// ScanBytes 是 [Scanner] 的分割函数，返回每个字节作为 token。
func ScanBytes(data []byte, atEOF bool) (advance int, token []byte, err error) {
	if atEOF && len(data) == 0 {
		return 0, nil, nil
	}
	return 1, data[0:1], nil
}

var errorRune = []byte(string(utf8.RuneError))

// ScanRunes 是 [Scanner] 的分割函数，返回每个
// UTF-8 编码的 rune 作为 token。返回的 rune 序列
// 等同于在输入上进行的范围循环（作为字符串），这
// 意味着错误的 UTF-8 编码转换为 U+FFFD = "\xef\xbf\xbd"。
// 由于 Scan 接口，这使得客户端无法
// 区分正确编码的替换 rune 和编码错误。
func ScanRunes(data []byte, atEOF bool) (advance int, token []byte, err error) {
	if atEOF && len(data) == 0 {
		return 0, nil, nil
	}

	// 快速路径 1：ASCII。
	if data[0] < utf8.RuneSelf {
		return 1, data[0:1], nil
	}

	// 快速路径 2：正确的 UTF-8 解码，无错误。
	_, width := utf8.DecodeRune(data)
	if width > 1 {
		// 这是一个有效的编码。对于正确编码的
		// 非 ASCII rune，宽度不能为 1。
		return width, data[0:width], nil
	}

	// 我们知道这是一个错误：我们有 width==1 且隐含 r==utf8.RuneError。
	// 错误是因为没有完整的 rune 要解码吗？
	// FullRune 正确区分错误和不完整的编码。
	if !atEOF && !utf8.FullRune(data) {
		// 不完整；获取更多字节。
		return 0, nil, nil
	}

	// 我们有一个真正的 UTF-8 编码错误。返回一个正确编码的错误 rune
	// 但仅前进一个字节。这匹配在错误编码的字符串上
	// 进行范围循环的行为。
	return 1, errorRune, nil
}

// dropCR 从数据中删除末尾 \r。
func dropCR(data []byte) []byte {
	if len(data) > 0 && data[len(data)-1] == '\r' {
		return data[0 : len(data)-1]
	}
	return data
}

// ScanLines 是 [Scanner] 的分割函数，返回每行
// 文本，去除任何尾部的行尾标记。返回的行可能
// 为空。行尾标记是一个可选的回车符，后面跟
// 一个强制换行符。用正则表达式表示，它是 `\r?\n`。
// 即使输入的最后一个非空行没有
// 换行符，也会返回它。
func ScanLines(data []byte, atEOF bool) (advance int, token []byte, err error) {
	if atEOF && len(data) == 0 {
		return 0, nil, nil
	}
	if i := bytes.IndexByte(data, '\n'); i >= 0 {
		// 我们有一个完整的换行符终止的行。
		return i + 1, dropCR(data[0:i]), nil
	}
	// 如果我们在 EOF，我们有一个最终的、未终止的行。返回它。
	if atEOF {
		return len(data), dropCR(data), nil
	}
	// 请求更多数据。
	return 0, nil, nil
}

// isSpace 报告字符是否是 Unicode 空白字符。
// 我们避免依赖 unicode 包，但在测试中检查实现的有效性。
func isSpace(r rune) bool {
	if r <= '\u00FF' {
		// 明显的 ASCII 字符：\t 到 \r 加上空格。加上两个 Latin-1 的怪异字符。
		switch r {
		case ' ', '\t', '\n', '\v', '\f', '\r':
			return true
		case '\u0085', '\u00A0':
			return true
		}
		return false
	}
	// 高值字符。
	if '\u2000' <= r && r <= '\u200a' {
		return true
	}
	switch r {
	case '\u1680', '\u2028', '\u2029', '\u202f', '\u205f', '\u3000':
		return true
	}
	return false
}

// ScanWords 是 [Scanner] 的分割函数，返回每个
// 以空格分隔的文本单词，删除周围的空格。它将
// 永远不会返回空字符串。空格的定义由
// unicode.IsSpace 设置。
func ScanWords(data []byte, atEOF bool) (advance int, token []byte, err error) {
	// 跳过前导空格。
	start := 0
	for width := 0; start < len(data); start += width {
		var r rune
		r, width = utf8.DecodeRune(data[start:])
		if !isSpace(r) {
			break
		}
	}
	// 扫描直到空格，标记单词的结尾。
	for width, i := 0, start; i < len(data); i += width {
		var r rune
		r, width = utf8.DecodeRune(data[i:])
		if isSpace(r) {
			return i + width, data[start:i], nil
		}
	}
	// 如果我们在 EOF，我们有一个最终的、非空的、未终止的单词。返回它。
	if atEOF && len(data) > start {
		return len(data), data[start:], nil
	}
	// 请求更多数据。
	return start, nil, nil
}
