// Copyright 2009 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package bytes

// 用于序列化数据的简单字节缓冲区。

import (
	"errors"
	"io"
	"unicode/utf8"
)

// smallBufferSize 是初始分配的最小容量。
const smallBufferSize = 64

// Buffer 是一个可变大小的字节缓冲区，具有 [Buffer.Read] 和 [Buffer.Write] 方法。
// Buffer 的零值是一个准备使用的空缓冲区。
type Buffer struct {
	buf      []byte // 内容是字节 buf[off : len(buf)]
	off      int    // 在 &buf[off] 处读取，在 &buf[len(buf)] 处写入
	lastRead readOp // 最后一次读操作，以便 Unread* 能正常工作。

	// 复制和修改非零 Buffer 容易出错，
	// 但我们不能采用 WaitGroup 和 Mutex 使用的 noCopy 技巧，
	// 这会导致 vet 的 copylocks 检查器报告滥用，因为 vet
	// 无法可靠地区分零和非零情况。
	// 有关历史，请参见 #26462、#25907、#47276、#48398。
}

// readOp 常量描述了在缓冲区上执行的最后一个操作，
// 以便 UnreadRune 和 UnreadByte 可以检查
// 无效的使用。opReadRuneX 常量的选择使得
// 转换为 int 时它们对应于读取的 rune 大小。
type readOp int8

// 不要为这些使用 iota，因为值需要与
// 名称和注释对应，显式时更容易看到。
const (
	opRead      readOp = -1 // 任何其他读操作。
	opInvalid   readOp = 0  // 非读操作。
	opReadRune1 readOp = 1  // 读大小为 1 的 rune。
	opReadRune2 readOp = 2  // 读大小为 2 的 rune。
	opReadRune3 readOp = 3  // 读大小为 3 的 rune。
	opReadRune4 readOp = 4  // 读大小为 4 的 rune。
)

// ErrTooLarge 在无法分配内存以在缓冲区中存储数据时传递给 panic。
var ErrTooLarge = errors.New("bytes.Buffer: too large")
var errNegativeRead = errors.New("bytes.Buffer: reader returned negative count from Read")

const maxInt = int(^uint(0) >> 1)

// Bytes 返回一个长度为 b.Len() 的切片，包含缓冲区的未读部分。
// 该切片仅在下一次缓冲区修改前有效（即，
// 仅在下一次调用诸如 [Buffer.Read]、[Buffer.Write]、[Buffer.Reset] 或 [Buffer.Truncate] 之类的方法前）。
// 该切片至少在下一次缓冲区修改前与缓冲区内容关联，
// 因此对切片的立即更改将影响将来读取的结果。
func (b *Buffer) Bytes() []byte { return b.buf[b.off:] }

// AvailableBuffer 返回一个具有 b.Available() 容量的空缓冲区。
// 该缓冲区旨在被附加到
// 并传递给紧随其后的 [Buffer.Write] 调用。
// 该缓冲区仅在下一个对 b 的写操作前有效。
func (b *Buffer) AvailableBuffer() []byte { return b.buf[len(b.buf):] }

// String 以字符串的形式返回缓冲区未读部分的内容。
// 如果 [Buffer] 是 nil 指针，它返回 "<nil>"。
//
// 要更有效地构建字符串，请参见 [strings.Builder] 类型。
func (b *Buffer) String() string {
	if b == nil {
		// 特殊情况，对调试有用。
		return "<nil>"
	}
	return string(b.buf[b.off:])
}

// Peek 在不推进缓冲区的情况下返回接下来的 n 个字节。
// 如果 Peek 返回少于 n 个字节，它也返回 [io.EOF]。
// 该切片仅在下一次调用读或写方法前有效。
// 该切片至少在下一次缓冲区修改前与缓冲区内容关联，
// 因此对切片的立即更改将影响将来读取的结果。
func (b *Buffer) Peek(n int) ([]byte, error) {
	if b.Len() < n {
		return b.buf[b.off:], io.EOF
	}
	return b.buf[b.off : b.off+n], nil
}

// empty 报告缓冲区的未读部分是否为空。
func (b *Buffer) empty() bool { return len(b.buf) <= b.off }

// Len 返回缓冲区未读部分的字节数；
// b.Len() == len(b.Bytes())。
func (b *Buffer) Len() int { return len(b.buf) - b.off }

// Cap 返回缓冲区底层字节切片的容量，即，
// 为缓冲区数据分配的总空间。
func (b *Buffer) Cap() int { return cap(b.buf) }

// Available 返回缓冲区中有多少字节未使用。
func (b *Buffer) Available() int { return cap(b.buf) - len(b.buf) }

// Truncate 从缓冲区丢弃除前 n 个未读字节外的所有内容，
// 但继续使用相同的分配存储。
// 如果 n 为负或大于缓冲区的长度，它会 panic。
func (b *Buffer) Truncate(n int) {
	if n == 0 {
		b.Reset()
		return
	}
	b.lastRead = opInvalid
	if n < 0 || n > b.Len() {
		panic("bytes.Buffer: truncation out of range")
	}
	b.buf = b.buf[:b.off+n]
}

// Reset 将缓冲区重置为空，
// 但它保留底层存储以供将来的写入使用。
// Reset 与 [Buffer.Truncate](0) 相同。
func (b *Buffer) Reset() {
	b.buf = b.buf[:0]
	b.off = 0
	b.lastRead = opInvalid
}

// tryGrowByReslice 是 grow 的一个可内联版本，用于仅需要重新切片内部缓冲区的快速情况。
// 它返回应该写入字节的索引以及是否成功。
func (b *Buffer) tryGrowByReslice(n int) (int, bool) {
	if l := len(b.buf); n <= cap(b.buf)-l {
		b.buf = b.buf[:l+n]
		return l, true
	}
	return 0, false
}

// grow 增大缓冲区以保证 n 个更多字节的空间。
// 它返回应该写入字节的索引。
// 如果缓冲区不能增大，它将以 ErrTooLarge 进行 panic。
func (b *Buffer) grow(n int) int {
	m := b.Len()
	// 如果缓冲区为空，重置以恢复空间。
	if m == 0 && b.off != 0 {
		b.Reset()
	}
	// 尝试通过重新切片来增大。
	if i, ok := b.tryGrowByReslice(n); ok {
		return i
	}
	if b.buf == nil && n <= smallBufferSize {
		b.buf = make([]byte, n, smallBufferSize)
		return 0
	}
	c := cap(b.buf)
	if n <= c/2-m {
		// 我们可以向下滑动内容而不是分配新的
		// 切片。我们只需要 m+n <= c 来滑动，但
		// 我们改为让容量翻倍，这样我们
		// 就不会花所有时间复制。
		copy(b.buf, b.buf[b.off:])
	} else if c > maxInt-c-n {
		panic(ErrTooLarge)
	} else {
		// 添加 b.off 以说明 b.buf[:b.off] 从前面被切片。
		b.buf = growSlice(b.buf[b.off:], b.off+n)
	}
	// 恢复 b.off 和 len(b.buf)。
	b.off = 0
	b.buf = b.buf[:m+n]
	return m
}

// Grow 在必要时增大缓冲区的容量，以保证
// 另外 n 个字节的空间。在 Grow(n) 之后，至少可以向
// 缓冲区写入 n 个字节而无需另外分配。
// 如果 n 为负，Grow 将 panic。
// 如果缓冲区不能增大，它将以 [ErrTooLarge] 进行 panic。
func (b *Buffer) Grow(n int) {
	if n < 0 {
		panic("bytes.Buffer.Grow: negative count")
	}
	m := b.grow(n)
	b.buf = b.buf[:m]
}

// Write 将 p 的内容附加到缓冲区，根据需要增大缓冲区。
// 返回值 n 是 p 的长度；err 总是 nil。如果
// 缓冲区变得太大，Write 将以 [ErrTooLarge] 进行 panic。
func (b *Buffer) Write(p []byte) (n int, err error) {
	b.lastRead = opInvalid
	m, ok := b.tryGrowByReslice(len(p))
	if !ok {
		m = b.grow(len(p))
	}
	return copy(b.buf[m:], p), nil
}

// WriteString 将 s 的内容附加到缓冲区，根据需要增大缓冲区。
// 返回值 n 是 s 的长度；err 总是 nil。如果
// 缓冲区变得太大，WriteString 将以 [ErrTooLarge] 进行 panic。
func (b *Buffer) WriteString(s string) (n int, err error) {
	b.lastRead = opInvalid
	m, ok := b.tryGrowByReslice(len(s))
	if !ok {
		m = b.grow(len(s))
	}
	return copy(b.buf[m:], s), nil
}

// MinRead 是传递给 [Buffer.ReadFrom] 调用的 [Buffer.Read] 的最小切片大小。
// 只要 [Buffer] 有至少 MinRead 个字节超过
// 保存 r 内容所需的量，[Buffer.ReadFrom] 就不会增大
// 底层缓冲区。
const MinRead = 512

// ReadFrom 从 r 读取数据直到 EOF，并将其附加到缓冲区，根据需要增大
// 缓冲区。返回值 n 是读取的字节数。读取期间遇到的除 io.EOF 外的任何
// 错误也会返回。如果缓冲区变得太大，ReadFrom 将以 [ErrTooLarge] 进行 panic。
func (b *Buffer) ReadFrom(r io.Reader) (n int64, err error) {
	b.lastRead = opInvalid
	for {
		i := b.grow(MinRead)
		b.buf = b.buf[:i]
		m, e := r.Read(b.buf[i:cap(b.buf)])
		if m < 0 {
			panic(errNegativeRead)
		}

		b.buf = b.buf[:i+m]
		n += int64(m)
		if e == io.EOF {
			return n, nil // e 是 EOF，所以显式返回 nil
		}
		if e != nil {
			return n, e
		}
	}
}

// growSlice 通过 n 增大 b，保留 b 的原始内容。
// 如果分配失败，它将以 ErrTooLarge 进行 panic。
func growSlice(b []byte, n int) []byte {
	defer func() {
		if recover() != nil {
			panic(ErrTooLarge)
		}
	}()
	// TODO(http://golang.org/issue/51462): 我们应该依赖 append-make
	// 模式，以便编译器可以调用 runtime.growslice。例如：
	//	return append(b, make([]byte, n)...)
	// 这避免了不必要的清零分配切片的前 len(b) 个字节，
	// 但这种模式导致 b 逃逸到堆上。
	//
	// 而是使用 append-make 模式与 nil 切片以确保
	// 我们分配向上舍入到最近大小类的缓冲区。
	c := len(b) + n // 确保为 n 个元素提供足够的空间
	if c < 2*cap(b) {
		// 增长率在历史上总是 2 倍。将来，
		// 我们可以纯粹依赖 append 来确定增长率。
		c = 2 * cap(b)
	}
	b2 := append([]byte(nil), make([]byte, c)...)
	i := copy(b2, b)
	return b2[:i]
}

// WriteTo 向 w 写入数据直到缓冲区被排空或发生错误。
// 返回值 n 是写入的字节数；它总是适合一个
// int，但它是 int64 以匹配 [io.WriterTo] 接口。写入期间遇到的任何
// 错误也会返回。
func (b *Buffer) WriteTo(w io.Writer) (n int64, err error) {
	b.lastRead = opInvalid
	if nBytes := b.Len(); nBytes > 0 {
		m, e := w.Write(b.buf[b.off:])
		if m > nBytes {
			panic("bytes.Buffer.WriteTo: invalid Write count")
		}
		b.off += m
		n = int64(m)
		if e != nil {
			return n, e
		}
		// 根据 io.Writer 中 Write 方法的定义，
		// 所有字节都应该已被写入
		if m != nBytes {
			return n, io.ErrShortWrite
		}
	}
	// 缓冲区现在为空；重置。
	b.Reset()
	return n, nil
}

// WriteByte 将字节 c 附加到缓冲区，根据需要增大缓冲区。
// 返回的错误总是 nil，但包括在内以匹配 [bufio.Writer] 的
// WriteByte。如果缓冲区变得太大，WriteByte 将以
// [ErrTooLarge] 进行 panic。
func (b *Buffer) WriteByte(c byte) error {
	b.lastRead = opInvalid
	m, ok := b.tryGrowByReslice(1)
	if !ok {
		m = b.grow(1)
	}
	b.buf[m] = c
	return nil
}

// WriteRune 将 Unicode 代码点 r 的 UTF-8 编码附加到
// 缓冲区，返回其长度和一个错误，该错误始终是 nil，但包括在内以
// 匹配 [bufio.Writer] 的 WriteRune。缓冲区根据需要增大；
// 如果它变得太大，WriteRune 将以 [ErrTooLarge] 进行 panic。
func (b *Buffer) WriteRune(r rune) (n int, err error) {
	// 作为 uint32 比较以正确处理负 rune。
	if uint32(r) < utf8.RuneSelf {
		b.WriteByte(byte(r))
		return 1, nil
	}
	b.lastRead = opInvalid
	m, ok := b.tryGrowByReslice(utf8.UTFMax)
	if !ok {
		m = b.grow(utf8.UTFMax)
	}
	b.buf = utf8.AppendRune(b.buf[:m], r)
	return len(b.buf) - m, nil
}

// Read 从缓冲区读取接下来的 len(p) 个字节或直到缓冲区
// 被排空。返回值 n 是读取的字节数。如果
// 缓冲区没有要返回的数据，err 是 [io.EOF]（除非 len(p) 为零）；
// 否则它是 nil。
func (b *Buffer) Read(p []byte) (n int, err error) {
	b.lastRead = opInvalid
	if b.empty() {
		// 缓冲区为空，重置以恢复空间。
		b.Reset()
		if len(p) == 0 {
			return 0, nil
		}
		return 0, io.EOF
	}
	n = copy(p, b.buf[b.off:])
	b.off += n
	if n > 0 {
		b.lastRead = opRead
	}
	return n, nil
}

// Next 返回一个切片，包含缓冲区中的接下来 n 个字节，
// 推进缓冲区，如同字节由 [Buffer.Read] 返回。
// 如果缓冲区中少于 n 个字节，Next 返回整个缓冲区。
// 该切片仅在下一次调用读或写方法前有效。
func (b *Buffer) Next(n int) []byte {
	b.lastRead = opInvalid
	m := b.Len()
	if n > m {
		n = m
	}
	data := b.buf[b.off : b.off+n]
	b.off += n
	if n > 0 {
		b.lastRead = opRead
	}
	return data
}

// ReadByte 读取并返回缓冲区中的下一个字节。
// 如果没有可用的字节，它返回错误 [io.EOF]。
func (b *Buffer) ReadByte() (byte, error) {
	if b.empty() {
		// 缓冲区为空，重置以恢复空间。
		b.Reset()
		return 0, io.EOF
	}
	c := b.buf[b.off]
	b.off++
	b.lastRead = opRead
	return c, nil
}

// ReadRune 读取并返回缓冲区中的下一个 UTF-8 编码的
// Unicode 代码点。
// 如果没有可用的字节，返回的错误是 io.EOF。
// 如果字节是错误的 UTF-8 编码，它
// 消费一个字节并返回 U+FFFD, 1。
func (b *Buffer) ReadRune() (r rune, size int, err error) {
	if b.empty() {
		// 缓冲区为空，重置以恢复空间。
		b.Reset()
		return 0, 0, io.EOF
	}
	c := b.buf[b.off]
	if c < utf8.RuneSelf {
		b.off++
		b.lastRead = opReadRune1
		return rune(c), 1, nil
	}
	r, n := utf8.DecodeRune(b.buf[b.off:])
	b.off += n
	b.lastRead = readOp(n)
	return r, n, nil
}

// UnreadRune unreads [Buffer.ReadRune] 返回的最后一个 rune。
// 如果缓冲区上最近的读或写操作
// 不是成功的 [Buffer.ReadRune]，UnreadRune 返回一个错误。（在这方面
// 它比 [Buffer.UnreadByte] 更严格，后者会 unread
// 来自任何读操作的最后一个字节。）
func (b *Buffer) UnreadRune() error {
	if b.lastRead <= opInvalid {
		return errors.New("bytes.Buffer: UnreadRune: previous operation was not a successful ReadRune")
	}
	if b.off >= int(b.lastRead) {
		b.off -= int(b.lastRead)
	}
	b.lastRead = opInvalid
	return nil
}

var errUnreadByte = errors.New("bytes.Buffer: UnreadByte: previous operation was not a successful read")

// UnreadByte unreads 最近成功读取操作返回的最后一个字节，
// 该操作至少读取一个字节。如果自上次读取以来发生了写入，
// 如果最后一次读取返回了错误，或者如果读取读取了零
// 字节，UnreadByte 返回一个错误。
func (b *Buffer) UnreadByte() error {
	if b.lastRead == opInvalid {
		return errUnreadByte
	}
	b.lastRead = opInvalid
	if b.off > 0 {
		b.off--
	}
	return nil
}

// ReadBytes 读取直到输入中 delim 的第一次出现，
// 返回一个切片，包含直到并包括分隔符的数据。
// 如果 ReadBytes 在找到分隔符前遇到错误，
// 它返回在错误前读取的数据和错误本身（通常是 [io.EOF]）。
// ReadBytes 返回 err != nil 当且仅当返回的数据不以
// delim 结尾。
func (b *Buffer) ReadBytes(delim byte) (line []byte, err error) {
	slice, err := b.readSlice(delim)
	// 返回 slice 的副本。缓冲区的支持数组可能
	// 被后续调用覆盖。
	line = append(line, slice...)
	return line, err
}

// readSlice 类似于 ReadBytes，但返回对内部缓冲区数据的引用。
func (b *Buffer) readSlice(delim byte) (line []byte, err error) {
	i := IndexByte(b.buf[b.off:], delim)
	end := b.off + i + 1
	if i < 0 {
		end = len(b.buf)
		err = io.EOF
	}
	line = b.buf[b.off:end]
	b.off = end
	b.lastRead = opRead
	return line, err
}

// ReadString 读取直到输入中 delim 的第一次出现，
// 返回一个字符串，包含直到并包括分隔符的数据。
// 如果 ReadString 在找到分隔符前遇到错误，
// 它返回在错误前读取的数据和错误本身（通常是 [io.EOF]）。
// ReadString 返回 err != nil 当且仅当返回的数据不以
// delim 结尾。
func (b *Buffer) ReadString(delim byte) (line string, err error) {
	slice, err := b.readSlice(delim)
	return string(slice), err
}

// NewBuffer 使用 buf 作为其
// 初始内容创建并初始化一个新的 [Buffer]。新的 [Buffer] 取得 buf 的所有权，
// 调用者不应在此调用后使用 buf。NewBuffer 用于
// 准备一个 [Buffer] 以读取现有数据。它也可以用于设置
// 用于写入的内部缓冲区的初始大小。为了做到这一点，
// buf 应该具有所需的容量但长度为零。
//
// 在大多数情况下，new([Buffer])（或只是声明一个 [Buffer] 变量）是
// 足以初始化 [Buffer]。
func NewBuffer(buf []byte) *Buffer { return &Buffer{buf: buf} }

// NewBufferString 使用字符串 s 作为其
// 初始内容创建并初始化一个新的 [Buffer]。它用于
// 准备一个缓冲区以读取现有的字符串。
//
// 在大多数情况下，new([Buffer])（或只是声明一个 [Buffer] 变量）是
// 足以初始化 [Buffer]。
func NewBufferString(s string) *Buffer {
	return &Buffer{buf: []byte(s)}
}
