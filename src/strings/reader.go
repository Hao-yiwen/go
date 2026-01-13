// 版权所有 2009 The Go Authors。保留所有权利。
// 本源代码的使用受 BSD 风格许可证约束，
// 该许可证可在 LICENSE 文件中找到。

package strings

import (
	"errors"
	"io"
	"unicode/utf8"
)

// Reader 通过从字符串读取来实现 [io.Reader]、[io.ReaderAt]、[io.ByteReader]、[io.ByteScanner]、
// [io.RuneReader]、[io.RuneScanner]、[io.Seeker] 和 [io.WriterTo] 接口。
// Reader 的零值操作起来像一个空字符串的 Reader。
type Reader struct {
	s        string
	i        int64 // 当前读取索引
	prevRune int   // 前一个 rune 的索引；或 < 0
}

// Len 返回字符串未读部分的字节数。
func (r *Reader) Len() int {
	if r.i >= int64(len(r.s)) {
		return 0
	}
	return int(int64(len(r.s)) - r.i)
}

// Size 返回底层字符串的原始长度。
// Size 是通过 [Reader.ReadAt] 可读取的字节数。
// 返回值始终相同，不受对任何其他方法调用的影响。
func (r *Reader) Size() int64 { return int64(len(r.s)) }

// Read 实现 [io.Reader] 接口。
func (r *Reader) Read(b []byte) (n int, err error) {
	if r.i >= int64(len(r.s)) {
		return 0, io.EOF
	}
	r.prevRune = -1
	n = copy(b, r.s[r.i:])
	r.i += int64(n)
	return
}

// ReadAt 实现 [io.ReaderAt] 接口。
func (r *Reader) ReadAt(b []byte, off int64) (n int, err error) {
	// 不能修改状态 - 参见 io.ReaderAt
	if off < 0 {
		return 0, errors.New("strings.Reader.ReadAt: negative offset")
	}
	if off >= int64(len(r.s)) {
		return 0, io.EOF
	}
	n = copy(b, r.s[off:])
	if n < len(b) {
		err = io.EOF
	}
	return
}

// ReadByte 实现 [io.ByteReader] 接口。
func (r *Reader) ReadByte() (byte, error) {
	r.prevRune = -1
	if r.i >= int64(len(r.s)) {
		return 0, io.EOF
	}
	b := r.s[r.i]
	r.i++
	return b, nil
}

// UnreadByte 实现 [io.ByteScanner] 接口。
func (r *Reader) UnreadByte() error {
	if r.i <= 0 {
		return errors.New("strings.Reader.UnreadByte: at beginning of string")
	}
	r.prevRune = -1
	r.i--
	return nil
}

// ReadRune 实现 [io.RuneReader] 接口。
func (r *Reader) ReadRune() (ch rune, size int, err error) {
	if r.i >= int64(len(r.s)) {
		r.prevRune = -1
		return 0, 0, io.EOF
	}
	r.prevRune = int(r.i)
	ch, size = utf8.DecodeRuneInString(r.s[r.i:])
	r.i += int64(size)
	return
}

// UnreadRune 实现 [io.RuneScanner] 接口。
func (r *Reader) UnreadRune() error {
	if r.i <= 0 {
		return errors.New("strings.Reader.UnreadRune: at beginning of string")
	}
	if r.prevRune < 0 {
		return errors.New("strings.Reader.UnreadRune: previous operation was not ReadRune")
	}
	r.i = int64(r.prevRune)
	r.prevRune = -1
	return nil
}

// Seek 实现 [io.Seeker] 接口。
func (r *Reader) Seek(offset int64, whence int) (int64, error) {
	r.prevRune = -1
	var abs int64
	switch whence {
	case io.SeekStart:
		abs = offset
	case io.SeekCurrent:
		abs = r.i + offset
	case io.SeekEnd:
		abs = int64(len(r.s)) + offset
	default:
		return 0, errors.New("strings.Reader.Seek: invalid whence")
	}
	if abs < 0 {
		return 0, errors.New("strings.Reader.Seek: negative position")
	}
	r.i = abs
	return abs, nil
}

// WriteTo 实现 [io.WriterTo] 接口。
func (r *Reader) WriteTo(w io.Writer) (n int64, err error) {
	r.prevRune = -1
	if r.i >= int64(len(r.s)) {
		return 0, nil
	}
	s := r.s[r.i:]
	m, err := io.WriteString(w, s)
	if m > len(s) {
		panic("strings.Reader.WriteTo: invalid WriteString count")
	}
	r.i += int64(m)
	n = int64(m)
	if m != len(s) && err == nil {
		err = io.ErrShortWrite
	}
	return
}

// Reset 将 [Reader] 重置为从 s 读取。
func (r *Reader) Reset(s string) { *r = Reader{s, 0, -1} }

// NewReader 返回一个从 s 读取的新 [Reader]。
// 它类似于 [bytes.NewBufferString]，但更高效且不可写。
func NewReader(s string) *Reader { return &Reader{s, 0, -1} }
