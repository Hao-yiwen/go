// 版权所有 2009 The Go Authors。保留所有权利。
// 本源代码的使用受 BSD 风格许可证约束，
// 该许可证可在 LICENSE 文件中找到。

// io 包提供了 I/O 原语的基本接口。
// 它的主要工作是将这些原语的现有实现（如 os 包中的实现）
// 包装成共享的公共接口以抽象其功能，以及一些其他相关的原语。
//
// 由于这些接口和原语包装了具有各种实现的底层操作，
// 除非另有说明，客户端不应假设它们对于并行执行是安全的。
package io

import (
	"errors"
	"sync"
)

// Seek 的 whence 值。
const (
	SeekStart   = 0 // 相对于文件起始位置进行定位
	SeekCurrent = 1 // 相对于当前偏移量进行定位
	SeekEnd     = 2 // 相对于文件末尾进行定位
)

// ErrShortWrite 表示写入接受的字节数少于请求的字节数，
// 但未能返回明确的错误。
var ErrShortWrite = errors.New("short write")

// errInvalidWrite 表示写入返回了不可能的计数。
var errInvalidWrite = errors.New("invalid write result")

// ErrShortBuffer 表示读取所需的缓冲区比提供的更长。
var ErrShortBuffer = errors.New("short buffer")

// EOF 是当没有更多输入可用时 Read 返回的错误。
// （Read 必须返回 EOF 本身，而不是包装 EOF 的错误，
// 因为调用者将使用 == 来测试 EOF。）
// 函数应该只在信号输入正常结束时返回 EOF。
// 如果在结构化数据流中意外发生 EOF，
// 适当的错误应该是 [ErrUnexpectedEOF] 或其他提供更多细节的错误。
var EOF = errors.New("EOF")

// ErrUnexpectedEOF 表示在读取固定大小的块或数据结构的
// 过程中遇到了 EOF。
var ErrUnexpectedEOF = errors.New("unexpected EOF")

// ErrNoProgress 是当 [Reader] 的某些客户端多次调用 Read
// 都未能返回任何数据或错误时返回的错误，
// 这通常是 [Reader] 实现有问题的标志。
var ErrNoProgress = errors.New("multiple Read calls return no data or error")

// Reader 是包装基本 Read 方法的接口。
//
// Read 最多读取 len(p) 个字节到 p 中。它返回读取的字节数
// （0 <= n <= len(p)）以及遇到的任何错误。即使 Read
// 返回 n < len(p)，它在调用期间也可能使用 p 的全部空间作为暂存空间。
// 如果有一些数据可用但不足 len(p) 字节，Read 通常
// 返回可用的数据而不是等待更多数据。
//
// 当 Read 在成功读取 n > 0 字节后遇到错误或文件结束条件时，
// 它返回读取的字节数。它可能从同一次调用返回（非 nil）错误，
// 或者从后续调用返回错误（并且 n == 0）。
// 这种一般情况的一个实例是，在输入流末尾返回非零字节数的 Reader
// 可能返回 err == EOF 或 err == nil。下一次 Read 应该
// 返回 0, EOF。
//
// 调用者应始终在考虑错误 err 之前处理返回的 n > 0 字节。
// 这样做可以正确处理在读取一些字节后发生的 I/O 错误，
// 以及两种允许的 EOF 行为。
//
// 如果 len(p) == 0，Read 应始终返回 n == 0。如果已知某些错误条件
// （如 EOF），它可能返回非 nil 错误。
//
// 不鼓励 Read 的实现返回零字节计数和 nil 错误，
// 除非 len(p) == 0。
// 调用者应将返回 0 和 nil 视为表示什么都没发生；
// 特别是它不表示 EOF。
//
// 实现不得保留 p。
type Reader interface {
	Read(p []byte) (n int, err error)
}

// Writer 是包装基本 Write 方法的接口。
//
// Write 从 p 向底层数据流写入 len(p) 个字节。
// 它返回从 p 写入的字节数（0 <= n <= len(p)）
// 以及导致写入提前停止的任何错误。
// 如果 Write 返回 n < len(p)，则必须返回非 nil 错误。
// Write 不得修改切片数据，即使是临时修改。
//
// 实现不得保留 p。
type Writer interface {
	Write(p []byte) (n int, err error)
}

// Closer 是包装基本 Close 方法的接口。
//
// 第一次调用之后 Close 的行为是未定义的。
// 具体实现可能会记录其自身的行为。
type Closer interface {
	Close() error
}

// Seeker 是包装基本 Seek 方法的接口。
//
// Seek 将下一次 Read 或 Write 的偏移量设置为 offset，
// 根据 whence 进行解释：
// [SeekStart] 表示相对于文件开始，
// [SeekCurrent] 表示相对于当前偏移量，
// [SeekEnd] 表示相对于末尾
// （例如，offset = -2 指定文件的倒数第二个字节）。
// Seek 返回相对于文件开始的新偏移量或错误（如果有）。
//
// 定位到文件开始之前的偏移量是错误的。
// 定位到任何正偏移量可能是允许的，但如果新偏移量超过
// 底层对象的大小，后续 I/O 操作的行为取决于实现。
type Seeker interface {
	Seek(offset int64, whence int) (int64, error)
}

// ReadWriter 是组合基本 Read 和 Write 方法的接口。
type ReadWriter interface {
	Reader
	Writer
}

// ReadCloser 是组合基本 Read 和 Close 方法的接口。
type ReadCloser interface {
	Reader
	Closer
}

// WriteCloser 是组合基本 Write 和 Close 方法的接口。
type WriteCloser interface {
	Writer
	Closer
}

// ReadWriteCloser 是组合基本 Read、Write 和 Close 方法的接口。
type ReadWriteCloser interface {
	Reader
	Writer
	Closer
}

// ReadSeeker 是组合基本 Read 和 Seek 方法的接口。
type ReadSeeker interface {
	Reader
	Seeker
}

// ReadSeekCloser 是组合基本 Read、Seek 和 Close
// 方法的接口。
type ReadSeekCloser interface {
	Reader
	Seeker
	Closer
}

// WriteSeeker 是组合基本 Write 和 Seek 方法的接口。
type WriteSeeker interface {
	Writer
	Seeker
}

// ReadWriteSeeker 是组合基本 Read、Write 和 Seek 方法的接口。
type ReadWriteSeeker interface {
	Reader
	Writer
	Seeker
}

// ReaderFrom 是包装 ReadFrom 方法的接口。
//
// ReadFrom 从 r 读取数据直到 EOF 或错误。
// 返回值 n 是读取的字节数。
// 读取期间遇到的任何错误（EOF 除外）也会返回。
//
// [Copy] 函数在可用时使用 [ReaderFrom]。
type ReaderFrom interface {
	ReadFrom(r Reader) (n int64, err error)
}

// WriterTo 是包装 WriteTo 方法的接口。
//
// WriteTo 向 w 写入数据直到没有更多数据可写或
// 发生错误。返回值 n 是写入的字节数。
// 写入期间遇到的任何错误也会返回。
//
// Copy 函数在可用时使用 WriterTo。
type WriterTo interface {
	WriteTo(w Writer) (n int64, err error)
}

// ReaderAt 是包装基本 ReadAt 方法的接口。
//
// ReadAt 从底层输入源的偏移量 off 处开始读取 len(p) 个字节到 p 中。
// 它返回读取的字节数（0 <= n <= len(p)）以及遇到的任何错误。
//
// 当 ReadAt 返回 n < len(p) 时，它返回一个非 nil 错误
// 解释为什么没有返回更多字节。在这方面，ReadAt 比 Read 更严格。
//
// 即使 ReadAt 返回 n < len(p)，它在调用期间也可能使用 p 的全部空间
// 作为暂存空间。如果有一些数据可用但不足 len(p) 字节，
// ReadAt 会阻塞直到所有数据可用或发生错误。
// 在这方面 ReadAt 与 Read 不同。
//
// 如果 ReadAt 返回的 n = len(p) 字节位于输入源的末尾，
// ReadAt 可能返回 err == EOF 或 err == nil。
//
// 如果 ReadAt 从具有定位偏移量的输入源读取，
// ReadAt 不应影响底层定位偏移量，也不应被其影响。
//
// ReadAt 的客户端可以在同一输入源上执行并行的 ReadAt 调用。
//
// 实现不得保留 p。
type ReaderAt interface {
	ReadAt(p []byte, off int64) (n int, err error)
}

// WriterAt 是包装基本 WriteAt 方法的接口。
//
// WriteAt 从 p 向底层数据流的偏移量 off 处写入 len(p) 个字节。
// 它返回从 p 写入的字节数（0 <= n <= len(p)）
// 以及导致写入提前停止的任何错误。
// 如果 WriteAt 返回 n < len(p)，则必须返回非 nil 错误。
//
// 如果 WriteAt 向具有定位偏移量的目标写入，
// WriteAt 不应影响底层定位偏移量，也不应被其影响。
//
// 如果范围不重叠，WriteAt 的客户端可以在同一目标上
// 执行并行的 WriteAt 调用。
//
// 实现不得保留 p。
type WriterAt interface {
	WriteAt(p []byte, off int64) (n int, err error)
}

// ByteReader 是包装 ReadByte 方法的接口。
//
// ReadByte 从输入中读取并返回下一个字节或
// 遇到的任何错误。如果 ReadByte 返回错误，则没有消耗
// 输入字节，返回的字节值是未定义的。
//
// ReadByte 为逐字节处理提供了高效的接口。
// 未实现 ByteReader 的 [Reader] 可以使用
// bufio.NewReader 包装以添加此方法。
type ByteReader interface {
	ReadByte() (byte, error)
}

// ByteScanner 是在基本 ReadByte 方法之上添加
// UnreadByte 方法的接口。
//
// UnreadByte 使下一次调用 ReadByte 返回上次读取的字节。
// 如果上一次操作不是成功调用 ReadByte，UnreadByte 可能
// 返回错误、取消读取上次读取的字节（或上次取消读取的字节之前的字节），
// 或者（在支持 [Seeker] 接口的实现中）定位到当前偏移量之前一个字节。
type ByteScanner interface {
	ByteReader
	UnreadByte() error
}

// ByteWriter 是包装 WriteByte 方法的接口。
type ByteWriter interface {
	WriteByte(c byte) error
}

// RuneReader 是包装 ReadRune 方法的接口。
//
// ReadRune 读取单个编码的 Unicode 字符
// 并返回该 rune 及其字节大小。如果没有可用的字符，
// 将设置 err。
type RuneReader interface {
	ReadRune() (r rune, size int, err error)
}

// RuneScanner 是在基本 ReadRune 方法之上添加
// UnreadRune 方法的接口。
//
// UnreadRune 使下一次调用 ReadRune 返回上次读取的 rune。
// 如果上一次操作不是成功调用 ReadRune，UnreadRune 可能
// 返回错误、取消读取上次读取的 rune（或上次取消读取的 rune 之前的 rune），
// 或者（在支持 [Seeker] 接口的实现中）定位到当前偏移量之前该 rune 的开始位置。
type RuneScanner interface {
	RuneReader
	UnreadRune() error
}

// StringWriter 是包装 WriteString 方法的接口。
type StringWriter interface {
	WriteString(s string) (n int, err error)
}

// WriteString 将字符串 s 的内容写入 w，w 接受字节切片。
// 如果 w 实现了 [StringWriter]，则直接调用 [StringWriter.WriteString]。
// 否则，[Writer.Write] 只被调用一次。
func WriteString(w Writer, s string) (n int, err error) {
	if sw, ok := w.(StringWriter); ok {
		return sw.WriteString(s)
	}
	return w.Write([]byte(s))
}

// ReadAtLeast 从 r 读取到 buf 中，直到读取了至少 min 个字节。
// 它返回复制的字节数，如果读取的字节数较少则返回错误。
// 只有在没有读取任何字节时，错误才是 EOF。
// 如果在读取少于 min 个字节后发生 EOF，
// ReadAtLeast 返回 [ErrUnexpectedEOF]。
// 如果 min 大于 buf 的长度，ReadAtLeast 返回 [ErrShortBuffer]。
// 返回时，当且仅当 err == nil 时，n >= min。
// 如果 r 在读取了至少 min 个字节后返回错误，该错误将被丢弃。
func ReadAtLeast(r Reader, buf []byte, min int) (n int, err error) {
	if len(buf) < min {
		return 0, ErrShortBuffer
	}
	for n < min && err == nil {
		var nn int
		nn, err = r.Read(buf[n:])
		n += nn
	}
	if n >= min {
		err = nil
	} else if n > 0 && err == EOF {
		err = ErrUnexpectedEOF
	}
	return
}

// ReadFull 从 r 精确读取 len(buf) 个字节到 buf 中。
// 它返回复制的字节数，如果读取的字节数较少则返回错误。
// 只有在没有读取任何字节时，错误才是 EOF。
// 如果在读取了一些但不是全部字节后发生 EOF，
// ReadFull 返回 [ErrUnexpectedEOF]。
// 返回时，当且仅当 err == nil 时，n == len(buf)。
// 如果 r 在读取了至少 len(buf) 个字节后返回错误，该错误将被丢弃。
func ReadFull(r Reader, buf []byte) (n int, err error) {
	return ReadAtLeast(r, buf, len(buf))
}

// CopyN 从 src 向 dst 复制 n 个字节（或直到发生错误）。
// 它返回复制的字节数以及复制过程中遇到的最早的错误。
// 返回时，当且仅当 err == nil 时，written == n。
//
// 如果 dst 实现了 [ReaderFrom]，则使用它来实现复制。
func CopyN(dst Writer, src Reader, n int64) (written int64, err error) {
	written, err = Copy(dst, LimitReader(src, n))
	if written == n {
		return n, nil
	}
	if written < n && err == nil {
		// src 提前停止了；一定是遇到了 EOF。
		err = EOF
	}
	return
}

// Copy 从 src 向 dst 复制，直到 src 上达到 EOF 或发生错误。
// 它返回复制的字节数以及复制过程中遇到的第一个错误（如果有）。
//
// 成功的 Copy 返回 err == nil，而不是 err == EOF。
// 因为 Copy 被定义为从 src 读取直到 EOF，
// 所以它不会将 Read 返回的 EOF 视为需要报告的错误。
//
// 如果 src 实现了 [WriterTo]，
// 则通过调用 src.WriteTo(dst) 来实现复制。
// 否则，如果 dst 实现了 [ReaderFrom]，
// 则通过调用 dst.ReadFrom(src) 来实现复制。
func Copy(dst Writer, src Reader) (written int64, err error) {
	return copyBuffer(dst, src, nil)
}

// CopyBuffer 与 Copy 相同，但它使用提供的缓冲区（如果需要）
// 而不是分配临时缓冲区。如果 buf 为 nil，则分配一个；
// 否则如果它的长度为零，CopyBuffer 会 panic。
//
// 如果 src 实现了 [WriterTo] 或 dst 实现了 [ReaderFrom]，
// 则不会使用 buf 来执行复制。
func CopyBuffer(dst Writer, src Reader, buf []byte) (written int64, err error) {
	if buf != nil && len(buf) == 0 {
		panic("empty buffer in CopyBuffer")
	}
	return copyBuffer(dst, src, buf)
}

// copyBuffer 是 Copy 和 CopyBuffer 的实际实现。
// 如果 buf 为 nil，则分配一个。
func copyBuffer(dst Writer, src Reader, buf []byte) (written int64, err error) {
	// 如果 reader 有 WriteTo 方法，使用它来完成复制。
	// 避免分配和复制。
	if wt, ok := src.(WriterTo); ok {
		return wt.WriteTo(dst)
	}
	// 类似地，如果 writer 有 ReadFrom 方法，使用它来完成复制。
	if rf, ok := dst.(ReaderFrom); ok {
		return rf.ReadFrom(src)
	}
	if buf == nil {
		size := 32 * 1024
		if l, ok := src.(*LimitedReader); ok && int64(size) > l.N {
			if l.N < 1 {
				size = 1
			} else {
				size = int(l.N)
			}
		}
		buf = make([]byte, size)
	}
	for {
		nr, er := src.Read(buf)
		if nr > 0 {
			nw, ew := dst.Write(buf[0:nr])
			if nw < 0 || nr < nw {
				nw = 0
				if ew == nil {
					ew = errInvalidWrite
				}
			}
			written += int64(nw)
			if ew != nil {
				err = ew
				break
			}
			if nr != nw {
				err = ErrShortWrite
				break
			}
		}
		if er != nil {
			if er != EOF {
				err = er
			}
			break
		}
	}
	return written, err
}

// LimitReader 返回一个从 r 读取的 Reader，
// 但在 n 个字节后以 EOF 停止。
// 底层实现是 *LimitedReader。
func LimitReader(r Reader, n int64) Reader { return &LimitedReader{r, n} }

// LimitedReader 从 R 读取但限制返回的数据量
// 仅为 N 个字节。每次调用 Read 都会更新 N
// 以反映新的剩余量。
// 当 N <= 0 或底层 R 返回 EOF 时，Read 返回 EOF。
type LimitedReader struct {
	R Reader // 底层 reader
	N int64  // 剩余的最大字节数
}

func (l *LimitedReader) Read(p []byte) (n int, err error) {
	if l.N <= 0 {
		return 0, EOF
	}
	if int64(len(p)) > l.N {
		p = p[0:l.N]
	}
	n, err = l.R.Read(p)
	l.N -= int64(n)
	return
}

// NewSectionReader 返回一个从 r 读取的 [SectionReader]，
// 从偏移量 off 开始，在 n 个字节后以 EOF 停止。
func NewSectionReader(r ReaderAt, off int64, n int64) *SectionReader {
	var remaining int64
	const maxint64 = 1<<63 - 1
	if off <= maxint64-n {
		remaining = n + off
	} else {
		// 溢出，无法返回错误。
		// 假设我们可以读取到偏移量 1<<63 - 1。
		remaining = maxint64
	}
	return &SectionReader{r, off, off, remaining, n}
}

// SectionReader 在底层 [ReaderAt] 的一个区段上
// 实现 Read、Seek 和 ReadAt。
type SectionReader struct {
	r     ReaderAt // 创建后不变
	base  int64    // 创建后不变
	off   int64
	limit int64 // 创建后不变
	n     int64 // 创建后不变
}

func (s *SectionReader) Read(p []byte) (n int, err error) {
	if s.off >= s.limit {
		return 0, EOF
	}
	if max := s.limit - s.off; int64(len(p)) > max {
		p = p[0:max]
	}
	n, err = s.r.ReadAt(p, s.off)
	s.off += int64(n)
	return
}

var errWhence = errors.New("Seek: invalid whence")
var errOffset = errors.New("Seek: invalid offset")

func (s *SectionReader) Seek(offset int64, whence int) (int64, error) {
	switch whence {
	default:
		return 0, errWhence
	case SeekStart:
		offset += s.base
	case SeekCurrent:
		offset += s.off
	case SeekEnd:
		offset += s.limit
	}
	if offset < s.base {
		return 0, errOffset
	}
	s.off = offset
	return offset - s.base, nil
}

func (s *SectionReader) ReadAt(p []byte, off int64) (n int, err error) {
	if off < 0 || off >= s.Size() {
		return 0, EOF
	}
	off += s.base
	if max := s.limit - off; int64(len(p)) > max {
		p = p[0:max]
		n, err = s.r.ReadAt(p, off)
		if err == nil {
			err = EOF
		}
		return n, err
	}
	return s.r.ReadAt(p, off)
}

// Size 返回区段的大小（以字节为单位）。
func (s *SectionReader) Size() int64 { return s.limit - s.base }

// Outer 返回该区段的底层 [ReaderAt] 和偏移量。
//
// 返回的值与创建 [SectionReader] 时传递给 [NewSectionReader]
// 的值相同。
func (s *SectionReader) Outer() (r ReaderAt, off int64, n int64) {
	return s.r, s.base, s.n
}

// OffsetWriter 将在偏移量 base 处的写入映射到底层 writer 的偏移量 base+off。
type OffsetWriter struct {
	w    WriterAt
	base int64 // 原始偏移量
	off  int64 // 当前偏移量
}

// NewOffsetWriter 返回一个从偏移量 off 开始写入 w 的 [OffsetWriter]。
func NewOffsetWriter(w WriterAt, off int64) *OffsetWriter {
	return &OffsetWriter{w, off, off}
}

func (o *OffsetWriter) Write(p []byte) (n int, err error) {
	n, err = o.w.WriteAt(p, o.off)
	o.off += int64(n)
	return
}

func (o *OffsetWriter) WriteAt(p []byte, off int64) (n int, err error) {
	if off < 0 {
		return 0, errOffset
	}

	off += o.base
	return o.w.WriteAt(p, off)
}

func (o *OffsetWriter) Seek(offset int64, whence int) (int64, error) {
	switch whence {
	default:
		return 0, errWhence
	case SeekStart:
		offset += o.base
	case SeekCurrent:
		offset += o.off
	}
	if offset < o.base {
		return 0, errOffset
	}
	o.off = offset
	return offset - o.base, nil
}

// TeeReader 返回一个 [Reader]，它将从 r 读取的内容写入 w。
// 通过它执行的所有从 r 的读取都与相应的向 w 的写入相匹配。
// 没有内部缓冲——写入必须在读取完成之前完成。
// 写入时遇到的任何错误都会报告为读取错误。
func TeeReader(r Reader, w Writer) Reader {
	return &teeReader{r, w}
}

type teeReader struct {
	r Reader
	w Writer
}

func (t *teeReader) Read(p []byte) (n int, err error) {
	n, err = t.r.Read(p)
	if n > 0 {
		if n, err := t.w.Write(p[:n]); err != nil {
			return n, err
		}
	}
	return
}

// Discard 是一个 [Writer]，所有的 Write 调用都会成功
// 而不做任何事情。
var Discard Writer = discard{}

type discard struct{}

// discard 实现了 ReaderFrom 作为优化，以便 Copy 到
// io.Discard 可以避免做不必要的工作。
var _ ReaderFrom = discard{}

func (discard) Write(p []byte) (int, error) {
	return len(p), nil
}

func (discard) WriteString(s string) (int, error) {
	return len(s), nil
}

var blackHolePool = sync.Pool{
	New: func() any {
		b := make([]byte, 8192)
		return &b
	},
}

func (discard) ReadFrom(r Reader) (n int64, err error) {
	bufp := blackHolePool.Get().(*[]byte)
	readSize := 0
	for {
		readSize, err = r.Read(*bufp)
		n += int64(readSize)
		if err != nil {
			blackHolePool.Put(bufp)
			if err == EOF {
				return n, nil
			}
			return
		}
	}
}

// NopCloser 返回一个 [ReadCloser]，它包装了提供的 [Reader] r
// 并带有一个空操作的 Close 方法。
// 如果 r 实现了 [WriterTo]，返回的 [ReadCloser] 将通过
// 转发调用到 r 来实现 [WriterTo]。
func NopCloser(r Reader) ReadCloser {
	if _, ok := r.(WriterTo); ok {
		return nopCloserWriterTo{r}
	}
	return nopCloser{r}
}

type nopCloser struct {
	Reader
}

func (nopCloser) Close() error { return nil }

type nopCloserWriterTo struct {
	Reader
}

func (nopCloserWriterTo) Close() error { return nil }

func (c nopCloserWriterTo) WriteTo(w Writer) (n int64, err error) {
	return c.Reader.(WriterTo).WriteTo(w)
}

// ReadAll 从 r 读取直到发生错误或 EOF，并返回读取的数据。
// 成功的调用返回 err == nil，而不是 err == EOF。因为 ReadAll
// 被定义为从 src 读取直到 EOF，所以它不会将 Read 返回的 EOF
// 视为需要报告的错误。
func ReadAll(r Reader) ([]byte, error) {
	// 构建指数增长大小的切片，
	// 然后在最后复制到一个大小正好的切片中。
	b := make([]byte, 0, 512)
	// 从 next 等于 256 开始（而不是比如 512 或 1024）
	// 可以为在早期增长阶段完成的小输入减少内存使用，
	// 但我们快速增长读取大小，使其不会对中等或大输入产生实质性影响。
	next := 256
	chunks := make([][]byte, 0, 4)
	// 不变式：finalSize = sum(len(c) for c in chunks)
	var finalSize int
	for {
		n, err := r.Read(b[len(b):cap(b)])
		b = b[:len(b)+n]
		if err != nil {
			if err == EOF {
				err = nil
			}
			if len(chunks) == 0 {
				return b, err
			}

			// 构建我们最终的正确大小的切片。
			finalSize += len(b)
			final := append([]byte(nil), make([]byte, finalSize)...)[:0]
			for _, chunk := range chunks {
				final = append(final, chunk...)
			}
			final = append(final, b...)
			return final, err
		}

		if cap(b)-len(b) < cap(b)/16 {
			// 移动到下一个中间切片。
			chunks = append(chunks, b)
			finalSize += len(b)
			b = append([]byte(nil), make([]byte, next)...)[:0]
			next += next / 2
		}
	}
}
