// 版权所有 2017 The Go Authors。保留所有权利。
// 本源代码的使用受 BSD 风格许可证约束，
// 该许可证可在 LICENSE 文件中找到。

package strings

import (
	"internal/abi"
	"internal/bytealg"
	"unicode/utf8"
	"unsafe"
)

// Builder 用于通过 [Builder.Write] 方法高效地构建字符串。
// 它最大限度地减少内存复制。零值可以直接使用。
// 不要复制非零的 Builder。
type Builder struct {
	addr *Builder // 接收者的地址，用于检测值复制

	// 外部用户不应直接访问此缓冲区，因为
	// 该切片在某些时候会使用 unsafe 转换为字符串，而且
	// len(buf) 和 cap(buf) 之间的数据可能未初始化。
	buf []byte
}

// copyCheck 实现动态检查，防止在复制非零 Builder 后进行修改，
// 这样做是不安全的（参见 #25907，#47276）。
//
// 我们不能向 Builder 添加 noCopy 字段来让 vet 的 copylocks
// 检查报告复制，因为 copylocks 无法可靠地
// 区分零值和非零值的情况。
func (b *Builder) copyCheck() {
	if b.addr == nil {
		// 这个技巧绕过了 Go 逃逸分析的一个缺陷，
		// 该缺陷导致 b 逃逸并被分配到堆上。
		// 参见 issue 23382。
		// TODO: 一旦 issue 7921 被修复，这应该恢复为
		// 简单的 "b.addr = b"。
		b.addr = (*Builder)(abi.NoEscape(unsafe.Pointer(b)))
	} else if b.addr != b {
		panic("strings: illegal use of non-zero Builder copied by value")
	}
}

// String 返回累积的字符串。
func (b *Builder) String() string {
	return unsafe.String(unsafe.SliceData(b.buf), len(b.buf))
}

// Len 返回累积的字节数；b.Len() == len(b.String())。
func (b *Builder) Len() int { return len(b.buf) }

// Cap 返回 builder 底层字节切片的容量。它是
// 为正在构建的字符串分配的总空间，包括已经
// 写入的任何字节。
func (b *Builder) Cap() int { return cap(b.buf) }

// Reset 将 [Builder] 重置为空。
func (b *Builder) Reset() {
	b.addr = nil
	b.buf = nil
}

// grow 将缓冲区复制到一个新的、更大的缓冲区，使得
// 超出 len(b.buf) 的容量至少有 n 个字节。
func (b *Builder) grow(n int) {
	buf := bytealg.MakeNoZero(2*cap(b.buf) + n)[:len(b.buf)]
	copy(buf, b.buf)
	b.buf = buf
}

// Grow 在必要时增长 b 的容量，以保证有空间容纳
// 另外 n 个字节。在 Grow(n) 之后，至少可以向 b 写入 n 个字节
// 而无需另外分配内存。如果 n 为负数，Grow 会 panic。
func (b *Builder) Grow(n int) {
	b.copyCheck()
	if n < 0 {
		panic("strings.Builder.Grow: negative count")
	}
	if cap(b.buf)-len(b.buf) < n {
		b.grow(n)
	}
}

// Write 将 p 的内容追加到 b 的缓冲区。
// Write 总是返回 len(p), nil。
func (b *Builder) Write(p []byte) (int, error) {
	b.copyCheck()
	b.buf = append(b.buf, p...)
	return len(p), nil
}

// WriteByte 将字节 c 追加到 b 的缓冲区。
// 返回的错误总是 nil。
func (b *Builder) WriteByte(c byte) error {
	b.copyCheck()
	b.buf = append(b.buf, c)
	return nil
}

// WriteRune 将 Unicode 码点 r 的 UTF-8 编码追加到 b 的缓冲区。
// 它返回 r 的长度和 nil 错误。
func (b *Builder) WriteRune(r rune) (int, error) {
	b.copyCheck()
	n := len(b.buf)
	b.buf = utf8.AppendRune(b.buf, r)
	return len(b.buf) - n, nil
}

// WriteString 将 s 的内容追加到 b 的缓冲区。
// 它返回 s 的长度和 nil 错误。
func (b *Builder) WriteString(s string) (int, error) {
	b.copyCheck()
	b.buf = append(b.buf, s...)
	return len(s), nil
}
