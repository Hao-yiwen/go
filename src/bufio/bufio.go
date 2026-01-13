// Copyright 2009 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// Package bufio implements buffered I/O. It wraps an io.Reader or io.Writer
// object, creating another object (Reader or Writer) that also implements
// the interface but provides buffering and some help for textual I/O.
package bufio

import (
	"bytes"
	"errors"
	"io"
	"strings"
	"unicode/utf8"
)

const (
	defaultBufSize = 4096
)

var (
	ErrInvalidUnreadByte = errors.New("bufio: invalid use of UnreadByte")
	ErrInvalidUnreadRune = errors.New("bufio: invalid use of UnreadRune")
	ErrBufferFull        = errors.New("bufio: buffer full")
	ErrNegativeCount     = errors.New("bufio: negative count")
)

// 缓冲输入。

// Reader 为 io.Reader 对象实现缓冲。
// 新的 Reader 通过调用 [NewReader] 或 [NewReaderSize] 创建；
// 或者可以使用 Reader 的零值，然后对其调用 [Reset]。
type Reader struct {
	buf          []byte
	rd           io.Reader // 客户端提供的读取器
	r, w         int       // buf 读和写位置
	err          error
	lastByte     int // 为 UnreadByte 读取的最后一个字节；-1 表示无效
	lastRuneSize int // 为 UnreadRune 读取的最后一个 rune 的大小；-1 表示无效
}

const minReadBufferSize = 16
const maxConsecutiveEmptyReads = 100

// NewReaderSize 返回一个新的 [Reader]，其缓冲区至少有指定大小。
// 如果参数 io.Reader 已经是一个缓冲区足够大的 [Reader]，
// 它将返回底层的 [Reader]。
func NewReaderSize(rd io.Reader, size int) *Reader {
	// 它已经是一个 Reader 了吗？
	b, ok := rd.(*Reader)
	if ok && len(b.buf) >= size {
		return b
	}
	r := new(Reader)
	r.reset(make([]byte, max(size, minReadBufferSize)), rd)
	return r
}

// NewReader 返回一个新的 [Reader]，其缓冲区具有默认大小。
func NewReader(rd io.Reader) *Reader {
	return NewReaderSize(rd, defaultBufSize)
}

// Size 返回底层缓冲区的大小（以字节为单位）。
func (b *Reader) Size() int { return len(b.buf) }

// Reset 丢弃所有缓冲数据，重置所有状态，并将
// 缓冲读取器切换为从 r 读取。
// 对 [Reader] 的零值调用 Reset 会将内部缓冲区
// 初始化为默认大小。
// 调用 b.Reset(b)（即将 [Reader] 重置为自身）不执行任何操作。
func (b *Reader) Reset(r io.Reader) {
	// 如果 Reader r 被传递给 NewReader，NewReader 将返回 r。
	// 代码的不同层可能会这样做，然后稍后将 r
	// 传递给 Reset。在这种情况下避免无限递归。
	if b == r {
		return
	}
	if b.buf == nil {
		b.buf = make([]byte, defaultBufSize)
	}
	b.reset(b.buf, r)
}

func (b *Reader) reset(buf []byte, r io.Reader) {
	*b = Reader{
		buf:          buf,
		rd:           r,
		lastByte:     -1,
		lastRuneSize: -1,
	}
}

var errNegativeRead = errors.New("bufio: reader returned negative count from Read")

// fill 将新的块读入缓冲区。
func (b *Reader) fill() {
	// 将现有数据滑动到开头。
	if b.r > 0 {
		copy(b.buf, b.buf[b.r:b.w])
		b.w -= b.r
		b.r = 0
	}

	if b.w >= len(b.buf) {
		panic("bufio: tried to fill full buffer")
	}

	// 读取新数据：尝试有限次数。
	for i := maxConsecutiveEmptyReads; i > 0; i-- {
		n, err := b.rd.Read(b.buf[b.w:])
		if n < 0 {
			panic(errNegativeRead)
		}
		b.w += n
		if err != nil {
			b.err = err
			return
		}
		if n > 0 {
			return
		}
	}
	b.err = io.ErrNoProgress
}

func (b *Reader) readErr() error {
	err := b.err
	b.err = nil
	return err
}

// Peek 返回接下来的 n 个字节而不推进读取器。这些字节在
// 下一次读取调用时停止有效。如果必要，Peek 将读取更多字节
// 到缓冲区中以使 n 个字节可用。如果 Peek 返回少于
// n 个字节，它还返回一个解释为什么读取过短的错误。
// 如果 n 大于 b 的缓冲区大小，错误是 [ErrBufferFull]。
//
// 调用 Peek 会阻止 [Reader.UnreadByte] 或 [Reader.UnreadRune] 调用成功
// 直到下一个读取操作。
func (b *Reader) Peek(n int) ([]byte, error) {
	if n < 0 {
		return nil, ErrNegativeCount
	}

	b.lastByte = -1
	b.lastRuneSize = -1

	for b.w-b.r < n && b.w-b.r < len(b.buf) && b.err == nil {
		b.fill() // b.w-b.r < len(b.buf) => 缓冲区未满
	}

	if n > len(b.buf) {
		return b.buf[b.r:b.w], ErrBufferFull
	}

	// 0 <= n <= len(b.buf)
	var err error
	if avail := b.w - b.r; avail < n {
		// 缓冲区中数据不足
		n = avail
		err = b.readErr()
		if err == nil {
			err = ErrBufferFull
		}
	}
	return b.buf[b.r : b.r+n], err
}

// Discard 跳过接下来的 n 个字节，返回丢弃的字节数。
//
// 如果 Discard 跳过少于 n 个字节，它也返回一个错误。
// 如果 0 <= n <= b.Buffered()，Discard 保证无需
// 从底层 io.Reader 读取就能成功。
func (b *Reader) Discard(n int) (discarded int, err error) {
	if n < 0 {
		return 0, ErrNegativeCount
	}
	if n == 0 {
		return
	}

	b.lastByte = -1
	b.lastRuneSize = -1

	remain := n
	for {
		skip := b.Buffered()
		if skip == 0 {
			b.fill()
			skip = b.Buffered()
		}
		if skip > remain {
			skip = remain
		}
		b.r += skip
		remain -= skip
		if remain == 0 {
			return n, nil
		}
		if b.err != nil {
			return n - remain, b.readErr()
		}
	}
}

// Read 读取数据到 p 中。
// 它返回读取到 p 中的字节数。
// 这些字节最多来自底层 [Reader] 的一次 Read，
// 因此 n 可能小于 len(p)。
// 要精确读取 len(p) 个字节，使用 io.ReadFull(b, p)。
// 如果底层 [Reader] 可以使用 io.EOF 返回非零计数，
// 那么此 Read 方法也可以这样做；参见 [io.Reader] 文档。
func (b *Reader) Read(p []byte) (n int, err error) {
	n = len(p)
	if n == 0 {
		if b.Buffered() > 0 {
			return 0, nil
		}
		return 0, b.readErr()
	}
	if b.r == b.w {
		if b.err != nil {
			return 0, b.readErr()
		}
		if len(p) >= len(b.buf) {
			// 大读取，缓冲区为空。
			// 直接读取到 p 以避免复制。
			n, b.err = b.rd.Read(p)
			if n < 0 {
				panic(errNegativeRead)
			}
			if n > 0 {
				b.lastByte = int(p[n-1])
				b.lastRuneSize = -1
			}
			return n, b.readErr()
		}
		// 一次读取。
		// 不要使用 b.fill，它会循环。
		b.r = 0
		b.w = 0
		n, b.err = b.rd.Read(b.buf)
		if n < 0 {
			panic(errNegativeRead)
		}
		if n == 0 {
			return 0, b.readErr()
		}
		b.w += n
	}

	// 尽可能多地复制
	// 注意：如果切片在这里发生 panic，可能是因为
	// 底层读取器返回了错误的计数。参见问题 49795。
	n = copy(p, b.buf[b.r:b.w])
	b.r += n
	b.lastByte = int(b.buf[b.r-1])
	b.lastRuneSize = -1
	return n, nil
}

// ReadByte 读取并返回单个字节。
// 如果没有可用的字节，返回错误。
func (b *Reader) ReadByte() (byte, error) {
	b.lastRuneSize = -1
	for b.r == b.w {
		if b.err != nil {
			return 0, b.readErr()
		}
		b.fill() // 缓冲区为空
	}
	c := b.buf[b.r]
	b.r++
	b.lastByte = int(c)
	return c, nil
}

// UnreadByte 将最后一个字节取消读取。只有最近读取的字节可以被取消读取。
//
// 如果在 [Reader] 上调用的最后一个方法不是读取操作，
// UnreadByte 返回错误。值得注意的是，[Reader.Peek]、[Reader.Discard] 和 [Reader.WriteTo]
// 不被视为读取操作。
func (b *Reader) UnreadByte() error {
	if b.lastByte < 0 || b.r == 0 && b.w > 0 {
		return ErrInvalidUnreadByte
	}
	// b.r > 0 || b.w == 0
	if b.r > 0 {
		b.r--
	} else {
		// b.r == 0 && b.w == 0
		b.w = 1
	}
	b.buf[b.r] = byte(b.lastByte)
	b.lastByte = -1
	b.lastRuneSize = -1
	return nil
}

// ReadRune 读取单个 UTF-8 编码的 Unicode 字符并返回
// 该 rune 及其大小（字节）。如果编码的 rune 无效，它消耗一个字节
// 并返回 unicode.ReplacementChar (U+FFFD)，大小为 1。
func (b *Reader) ReadRune() (r rune, size int, err error) {
	for b.r+utf8.UTFMax > b.w && !utf8.FullRune(b.buf[b.r:b.w]) && b.err == nil && b.w-b.r < len(b.buf) {
		b.fill() // b.w-b.r < len(buf) => 缓冲区未满
	}
	b.lastRuneSize = -1
	if b.r == b.w {
		return 0, 0, b.readErr()
	}
	r, size = utf8.DecodeRune(b.buf[b.r:b.w])
	b.r += size
	b.lastByte = int(b.buf[b.r-1])
	b.lastRuneSize = size
	return r, size, nil
}

// UnreadRune 将最后一个 rune 取消读取。如果在
// [Reader] 上调用的最近方法不是 [Reader.ReadRune]，[Reader.UnreadRune] 返回错误。（在这一点上
// 它比 [Reader.UnreadByte] 更严格，后者会从任何读取操作中取消读取最后一个字节。）
func (b *Reader) UnreadRune() error {
	if b.lastRuneSize < 0 || b.r < b.lastRuneSize {
		return ErrInvalidUnreadRune
	}
	b.r -= b.lastRuneSize
	b.lastByte = -1
	b.lastRuneSize = -1
	return nil
}

// Buffered 返回可从当前缓冲区读取的字节数。
func (b *Reader) Buffered() int { return b.w - b.r }

// ReadSlice 读取直到输入中首次出现分隔符，
// 返回指向缓冲区中字节的切片。
// 这些字节在下一次读取时停止有效。
// 如果 ReadSlice 在找到分隔符之前遇到错误，
// 它返回缓冲区中的所有数据和错误本身（通常是 io.EOF）。
// 如果缓冲区填满但没有分隔符，ReadSlice 失败并返回错误 [ErrBufferFull]。
// 因为从 ReadSlice 返回的数据将被
// 下一个 I/O 操作覆盖，大多数客户端应该使用
// [Reader.ReadBytes] 或 ReadString。
// 当且仅当行不以 delim 结尾时，ReadSlice 返回 err != nil。
func (b *Reader) ReadSlice(delim byte) (line []byte, err error) {
	s := 0 // 搜索开始索引
	for {
		// 搜索缓冲区。
		if i := bytes.IndexByte(b.buf[b.r+s:b.w], delim); i >= 0 {
			i += s
			line = b.buf[b.r : b.r+i+1]
			b.r += i + 1
			break
		}

		// 待处理错误？
		if b.err != nil {
			line = b.buf[b.r:b.w]
			b.r = b.w
			err = b.readErr()
			break
		}

		// 缓冲区满？
		if b.Buffered() >= len(b.buf) {
			b.r = b.w
			line = b.buf
			err = ErrBufferFull
			break
		}

		s = b.w - b.r // 不重新扫描之前扫描过的区域

		b.fill() // 缓冲区未满
	}

	// 处理最后一个字节（如果有）。
	if i := len(line) - 1; i >= 0 {
		b.lastByte = int(line[i])
		b.lastRuneSize = -1
	}

	return
}

// ReadLine 是一个低级行读取原语。大多数调用者应该使用
// [Reader.ReadBytes]('\n') 或 [Reader.ReadString]('\n') 或使用 [Scanner]。
//
// ReadLine 尝试返回单行，不包括行尾字节。
// 如果该行对于缓冲区来说太长，则设置 isPrefix 并返回
// 行的开头。行的其余部分将从
// 后续调用返回。当返回行的最后一个片段时，isPrefix 将为 false。
// 返回的缓冲区仅在下一次调用 ReadLine 之前有效。
// ReadLine 要么返回非 nil 行，要么返回错误，永远不会两者都有。
//
// 从 ReadLine 返回的文本不包括行尾（"\r\n" 或 "\n"）。
// 如果输入在没有最后行尾的情况下结束，则不给出任何提示或错误。
// 在 ReadLine 之后调用 [Reader.UnreadByte] 将始终取消读取最后读取的字节
// （可能是属于行尾的字符），即使该字节不是
// ReadLine 返回的行的一部分。
func (b *Reader) ReadLine() (line []byte, isPrefix bool, err error) {
	line, err = b.ReadSlice('\n')
	if err == ErrBufferFull {
		// 处理 "\r\n" 跨越缓冲区的情况。
		if len(line) > 0 && line[len(line)-1] == '\r' {
			// 将 '\r' 放回 buf 并从行中删除它。
			// 让下一次 ReadLine 调用检查 "\r\n"。
			if b.r == 0 {
				// 应该无法到达
				panic("bufio: tried to rewind past start of buffer")
			}
			b.r--
			line = line[:len(line)-1]
		}
		return line, true, nil
	}

	if len(line) == 0 {
		if err != nil {
			line = nil
		}
		return
	}
	err = nil

	if line[len(line)-1] == '\n' {
		drop := 1
		if len(line) > 1 && line[len(line)-2] == '\r' {
			drop = 2
		}
		line = line[:len(line)-drop]
	}
	return
}

// collectFragments 读取直到输入中首次出现分隔符。它
// 返回（完整缓冲区的切片、分隔符前的剩余字节、组合的前两个元素中的字节总数、错误）。
// 完整结果等于
// `bytes.Join(append(fullBuffers, finalFragment), nil)`，其长度为 `totalLen`。
// 结果以这种方式构造，以允许调用者
// 最小化分配和复制。
func (b *Reader) collectFragments(delim byte) (fullBuffers [][]byte, finalFragment []byte, totalLen int, err error) {
	var frag []byte
	// 使用 ReadSlice 寻找分隔符，累积完整缓冲区。
	for {
		var e error
		frag, e = b.ReadSlice(delim)
		if e == nil { // 得到最终片段
			break
		}
		if e != ErrBufferFull { // 意外错误
			err = e
			break
		}

		// 制作缓冲区的副本。
		buf := bytes.Clone(frag)
		fullBuffers = append(fullBuffers, buf)
		totalLen += len(buf)
	}

	totalLen += len(frag)
	return fullBuffers, frag, totalLen, err
}

// ReadBytes 读取直到输入中首次出现分隔符，
// 返回包含数据直到和包括分隔符的切片。
// 如果 ReadBytes 在找到分隔符之前遇到错误，
// 它返回在错误之前读取的数据和错误本身（通常是 io.EOF）。
// 当且仅当返回的数据不以 delim 结尾时，ReadBytes 返回 err != nil。
// 对于简单的用途，Scanner 可能更方便。
func (b *Reader) ReadBytes(delim byte) ([]byte, error) {
	full, frag, n, err := b.collectFragments(delim)
	// 分配新缓冲区以容纳完整片段和片段。
	buf := make([]byte, n)
	n = 0
	// 复制完整片段和片段。
	for i := range full {
		n += copy(buf[n:], full[i])
	}
	copy(buf[n:], frag)
	return buf, err
}

// ReadString 读取直到输入中首次出现分隔符，
// 返回包含数据直到和包括分隔符的字符串。
// 如果 ReadString 在找到分隔符之前遇到错误，
// 它返回在错误之前读取的数据和错误本身（通常是 io.EOF）。
// 当且仅当返回的数据不以 delim 结尾时，ReadString 返回 err != nil。
// 对于简单的用途，Scanner 可能更方便。
func (b *Reader) ReadString(delim byte) (string, error) {
	full, frag, n, err := b.collectFragments(delim)
	// 分配新缓冲区以容纳完整片段和片段。
	var buf strings.Builder
	buf.Grow(n)
	// 复制完整片段和片段。
	for _, fb := range full {
		buf.Write(fb)
	}
	buf.Write(frag)
	return buf.String(), err
}

// WriteTo 实现 io.WriterTo。
// 这可能多次调用底层 [Reader] 的 [Reader.Read] 方法。
// 如果底层读取器支持 [Reader.WriteTo] 方法，
// 这将调用底层 [Reader.WriteTo] 而不进行缓冲。
func (b *Reader) WriteTo(w io.Writer) (n int64, err error) {
	b.lastByte = -1
	b.lastRuneSize = -1

	if b.r < b.w {
		n, err = b.writeBuf(w)
		if err != nil {
			return
		}
	}

	if r, ok := b.rd.(io.WriterTo); ok {
		m, err := r.WriteTo(w)
		n += m
		return n, err
	}

	if w, ok := w.(io.ReaderFrom); ok {
		m, err := w.ReadFrom(b.rd)
		n += m
		return n, err
	}

	if b.w-b.r < len(b.buf) {
		b.fill() // 缓冲区未满
	}

	for b.r < b.w {
		// b.r < b.w => 缓冲区不为空
		m, err := b.writeBuf(w)
		n += m
		if err != nil {
			return n, err
		}
		b.fill() // 缓冲区为空
	}

	if b.err == io.EOF {
		b.err = nil
	}

	return n, b.readErr()
}

var errNegativeWrite = errors.New("bufio: writer returned negative count from Write")

// writeBuf 将 [Reader] 的缓冲区写入写入器。
func (b *Reader) writeBuf(w io.Writer) (int64, error) {
	n, err := w.Write(b.buf[b.r:b.w])
	if n < 0 {
		panic(errNegativeWrite)
	}
	b.r += n
	return int64(n), err
}

// 缓冲输出

// Writer 为 [io.Writer] 对象实现缓冲。
// 如果在写入 [Writer] 时发生错误，将不再
// 接受数据，所有后续的写入和 [Writer.Flush] 都将返回该错误。
// 写入所有数据后，客户端应调用
// [Writer.Flush] 方法以保证所有数据已转发到
// 底层 [io.Writer]。
type Writer struct {
	err error
	buf []byte
	n   int
	wr  io.Writer
}

// NewWriterSize 返回一个新的 [Writer]，其缓冲区至少有指定大小。
// 如果参数 io.Writer 已经是一个缓冲区足够大的 [Writer]，
// 它将返回底层的 [Writer]。
func NewWriterSize(w io.Writer, size int) *Writer {
	// 它已经是一个 Writer 了吗？
	b, ok := w.(*Writer)
	if ok && len(b.buf) >= size {
		return b
	}
	if size <= 0 {
		size = defaultBufSize
	}
	return &Writer{
		buf: make([]byte, size),
		wr:  w,
	}
}

// NewWriter 返回一个新的 [Writer]，其缓冲区具有默认大小。
// 如果参数 io.Writer 已经是一个缓冲区足够大的 [Writer]，
// 它将返回底层的 [Writer]。
func NewWriter(w io.Writer) *Writer {
	return NewWriterSize(w, defaultBufSize)
}

// Size 返回底层缓冲区的大小（以字节为单位）。
func (b *Writer) Size() int { return len(b.buf) }

// Reset 丢弃任何未刷新的缓冲数据，清除任何错误，并
// 将 b 重置为将其输出写入 w。
// 对 [Writer] 的零值调用 Reset 会将内部缓冲区
// 初始化为默认大小。
// 调用 w.Reset(w)（即将 [Writer] 重置为自身）不执行任何操作。
func (b *Writer) Reset(w io.Writer) {
	// 如果 Writer w 被传递给 NewWriter，NewWriter 将返回 w。
	// 代码的不同层可能会这样做，然后稍后将 w
	// 传递给 Reset。在这种情况下避免无限递归。
	if b == w {
		return
	}
	if b.buf == nil {
		b.buf = make([]byte, defaultBufSize)
	}
	b.err = nil
	b.n = 0
	b.wr = w
}

// Flush 将任何缓冲数据写入底层 [io.Writer]。
func (b *Writer) Flush() error {
	if b.err != nil {
		return b.err
	}
	if b.n == 0 {
		return nil
	}
	n, err := b.wr.Write(b.buf[0:b.n])
	if n < b.n && err == nil {
		err = io.ErrShortWrite
	}
	if err != nil {
		if n > 0 && n < b.n {
			copy(b.buf[0:b.n-n], b.buf[n:b.n])
		}
		b.n -= n
		b.err = err
		return err
	}
	b.n = 0
	return nil
}

// Available 返回缓冲区中有多少字节未使用。
func (b *Writer) Available() int { return len(b.buf) - b.n }

// AvailableBuffer 返回一个具有 b.Available() 容量的空缓冲区。
// 这个缓冲区旨在被附加到并
// 传递给紧随其后的 [Writer.Write] 调用。
// 缓冲区仅在 b 上的下一个写入操作之前有效。
func (b *Writer) AvailableBuffer() []byte {
	return b.buf[b.n:][:0]
}

// Buffered 返回已写入当前缓冲区的字节数。
func (b *Writer) Buffered() int { return b.n }

// Write 将 p 的内容写入缓冲区。
// 它返回写入的字节数。
// 如果 nn < len(p)，它也返回一个解释
// 为什么写入过短的错误。
func (b *Writer) Write(p []byte) (nn int, err error) {
	for len(p) > b.Available() && b.err == nil {
		var n int
		if b.Buffered() == 0 {
			// 大写入，缓冲区为空。
			// 直接从 p 写入以避免复制。
			n, b.err = b.wr.Write(p)
		} else {
			n = copy(b.buf[b.n:], p)
			b.n += n
			b.Flush()
		}
		nn += n
		p = p[n:]
	}
	if b.err != nil {
		return nn, b.err
	}
	n := copy(b.buf[b.n:], p)
	b.n += n
	nn += n
	return nn, nil
}

// WriteByte 写入单个字节。
func (b *Writer) WriteByte(c byte) error {
	if b.err != nil {
		return b.err
	}
	if b.Available() <= 0 && b.Flush() != nil {
		return b.err
	}
	b.buf[b.n] = c
	b.n++
	return nil
}

// WriteRune 写入单个 Unicode 代码点，返回
// 写入的字节数和任何错误。
func (b *Writer) WriteRune(r rune) (size int, err error) {
	// 作为 uint32 比较以正确处理负 rune。
	if uint32(r) < utf8.RuneSelf {
		err = b.WriteByte(byte(r))
		if err != nil {
			return 0, err
		}
		return 1, nil
	}
	if b.err != nil {
		return 0, b.err
	}
	n := b.Available()
	if n < utf8.UTFMax {
		if b.Flush(); b.err != nil {
			return 0, b.err
		}
		n = b.Available()
		if n < utf8.UTFMax {
			// 仅当缓冲区太小时才会发生。
			return b.WriteString(string(r))
		}
	}
	size = utf8.EncodeRune(b.buf[b.n:], r)
	b.n += size
	return size, nil
}

// WriteString 写入字符串。
// 它返回写入的字节数。
// 如果计数小于 len(s)，它也返回一个解释
// 为什么写入过短的错误。
func (b *Writer) WriteString(s string) (int, error) {
	var sw io.StringWriter
	tryStringWriter := true

	nn := 0
	for len(s) > b.Available() && b.err == nil {
		var n int
		if b.Buffered() == 0 && sw == nil && tryStringWriter {
			// 最多检查一次 b.wr 是否为 StringWriter。
			sw, tryStringWriter = b.wr.(io.StringWriter)
		}
		if b.Buffered() == 0 && tryStringWriter {
			// 大写入，缓冲区为空，且底层写入器支持
			// WriteString：将写入转发给底层 StringWriter。
			// 这避免了额外的复制。
			n, b.err = sw.WriteString(s)
		} else {
			n = copy(b.buf[b.n:], s)
			b.n += n
			b.Flush()
		}
		nn += n
		s = s[n:]
	}
	if b.err != nil {
		return nn, b.err
	}
	n := copy(b.buf[b.n:], s)
	b.n += n
	nn += n
	return nn, nil
}

// ReadFrom 实现 [io.ReaderFrom]。如果底层写入器
// 支持 ReadFrom 方法，这将调用底层 ReadFrom。
// 如果有缓冲数据和底层 ReadFrom，这将填充
// 缓冲区并在调用 ReadFrom 之前写入它。
func (b *Writer) ReadFrom(r io.Reader) (n int64, err error) {
	if b.err != nil {
		return 0, b.err
	}
	readerFrom, readerFromOK := b.wr.(io.ReaderFrom)
	var m int
	for {
		if b.Available() == 0 {
			if err1 := b.Flush(); err1 != nil {
				return n, err1
			}
		}
		if readerFromOK && b.Buffered() == 0 {
			nn, err := readerFrom.ReadFrom(r)
			b.err = err
			n += nn
			return n, err
		}
		nr := 0
		for nr < maxConsecutiveEmptyReads {
			m, err = r.Read(b.buf[b.n:])
			if m != 0 || err != nil {
				break
			}
			nr++
		}
		if nr == maxConsecutiveEmptyReads {
			return n, io.ErrNoProgress
		}
		b.n += m
		n += int64(m)
		if err != nil {
			break
		}
	}
	if err == io.EOF {
		// 如果我们恰好填充了缓冲区，主动刷新。
		if b.Available() == 0 {
			err = b.Flush()
		} else {
			err = nil
		}
	}
	return n, err
}

// 缓冲输入和输出

// ReadWriter 存储指向 [Reader] 和 [Writer] 的指针。
// 它实现了 [io.ReadWriter]。
type ReadWriter struct {
	*Reader
	*Writer
}

// NewReadWriter 分配一个新的 [ReadWriter]，将请求分派给 r 和 w。
func NewReadWriter(r *Reader, w *Writer) *ReadWriter {
	return &ReadWriter{r, w}
}
