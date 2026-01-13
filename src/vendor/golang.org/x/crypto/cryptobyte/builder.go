// 版权所有 2017 The Go Authors。保留所有权利。
// 本源代码的使用受 BSD 风格许可证约束，
// 该许可证可在 LICENSE 文件中找到。

package cryptobyte

import (
	"errors"
	"fmt"
)

// Builder 从固定长度和长度前缀值构建字节字符串。
// Builder 要么根据需要分配空间，要么是"固定的"，这意味着
// 它们写入给定的缓冲区，如果缓冲区用尽则产生错误。
//
// 零值是一个可用的 Builder，根据需要分配空间。
//
// 简单值使用 Builder 上的方法进行编组并追加到 Builder。
// 长度前缀值通过提供 BuilderContinuation 进行编组，
// 这是一个将值的内部内容写入给定 Builder 的函数。
// 有关详细信息，请参阅 BuilderContinuation 的文档。
type Builder struct {
	err            error
	result         []byte
	fixedSize      bool
	child          *Builder
	offset         int
	pendingLenLen  int
	pendingIsASN1  bool
	inContinuation *bool
}

// NewBuilder 创建一个将其输出追加到给定缓冲区的 Builder。
// 与 append() 一样，如果超出其容量，切片将被重新分配。
// 使用 Bytes 获取最终缓冲区。
func NewBuilder(buffer []byte) *Builder {
	return &Builder{
		result: buffer,
	}
}

// NewFixedBuilder 创建一个将其输出追加到给定缓冲区的 Builder。
// 此构建器不会重新分配输出缓冲区。超出缓冲区容量的写入将被视为错误。
func NewFixedBuilder(buffer []byte) *Builder {
	return &Builder{
		result:    buffer,
		fixedSize: true,
	}
}

// SetError 设置要从 Bytes 返回的错误值。调用 SetError 后执行的写入将被忽略。
func (b *Builder) SetError(err error) {
	b.err = err
}

// Bytes 返回构建器写入的字节，如果在构建过程中发生错误则返回错误。
func (b *Builder) Bytes() ([]byte, error) {
	if b.err != nil {
		return nil, b.err
	}
	return b.result[b.offset:], nil
}

// BytesOrPanic 返回构建器写入的字节，如果在构建过程中发生错误则 panic。
func (b *Builder) BytesOrPanic() []byte {
	if b.err != nil {
		panic(b.err)
	}
	return b.result[b.offset:]
}

// AddUint8 向字节字符串追加一个 8 位值。
func (b *Builder) AddUint8(v uint8) {
	b.add(byte(v))
}

// AddUint16 向字节字符串追加一个大端序 16 位值。
func (b *Builder) AddUint16(v uint16) {
	b.add(byte(v>>8), byte(v))
}

// AddUint24 向字节字符串追加一个大端序 24 位值。32 位输入值的最高字节被静默截断。
func (b *Builder) AddUint24(v uint32) {
	b.add(byte(v>>16), byte(v>>8), byte(v))
}

// AddUint32 向字节字符串追加一个大端序 32 位值。
func (b *Builder) AddUint32(v uint32) {
	b.add(byte(v>>24), byte(v>>16), byte(v>>8), byte(v))
}

// AddUint48 向字节字符串追加一个大端序 48 位值。
func (b *Builder) AddUint48(v uint64) {
	b.add(byte(v>>40), byte(v>>32), byte(v>>24), byte(v>>16), byte(v>>8), byte(v))
}

// AddUint64 向字节字符串追加一个大端序 64 位值。
func (b *Builder) AddUint64(v uint64) {
	b.add(byte(v>>56), byte(v>>48), byte(v>>40), byte(v>>32), byte(v>>24), byte(v>>16), byte(v>>8), byte(v))
}

// AddBytes 向字节字符串追加一个字节序列。
func (b *Builder) AddBytes(v []byte) {
	b.add(v...)
}

// BuilderContinuation 是用于构建长度前缀字节序列的延续传递接口。
// 用于长度前缀序列的 Builder 方法（AddUint8LengthPrefixed 等）将调用
// 提供给它们的 BuilderContinuation。传递给延续的子构建器可用于构建
// 长度前缀序列的内容。例如：
//
//	parent := cryptobyte.NewBuilder()
//	parent.AddUint8LengthPrefixed(func (child *Builder) {
//	  child.AddUint8(42)
//	  child.AddUint8LengthPrefixed(func (grandchild *Builder) {
//	    grandchild.AddUint8(5)
//	  })
//	})
//
// 向子构建器写入超过保留长度前缀允许的字节数是错误的。延续返回后，
// 子构建器必须被视为无效，即用户不得存储超出延续生命周期的子构建器的
// 任何副本或引用。
//
// 如果延续以 BuildError 类型的值 panic，则内部错误将作为 Bytes 的错误返回。
// 如果子构建器以其他方式 panic，则 Bytes 将以相同的值重新 panic。
type BuilderContinuation func(child *Builder)

// BuildError 包装一个错误。如果 BuilderContinuation 以此值 panic，
// panic 将被恢复，内部错误将从 Builder.Bytes 返回。
type BuildError struct {
	Err error
}

// AddUint8LengthPrefixed 添加一个 8 位长度前缀的字节序列。
func (b *Builder) AddUint8LengthPrefixed(f BuilderContinuation) {
	b.addLengthPrefixed(1, false, f)
}

// AddUint16LengthPrefixed 添加一个大端序 16 位长度前缀的字节序列。
func (b *Builder) AddUint16LengthPrefixed(f BuilderContinuation) {
	b.addLengthPrefixed(2, false, f)
}

// AddUint24LengthPrefixed 添加一个大端序 24 位长度前缀的字节序列。
func (b *Builder) AddUint24LengthPrefixed(f BuilderContinuation) {
	b.addLengthPrefixed(3, false, f)
}

// AddUint32LengthPrefixed 添加一个大端序 32 位长度前缀的字节序列。
func (b *Builder) AddUint32LengthPrefixed(f BuilderContinuation) {
	b.addLengthPrefixed(4, false, f)
}

func (b *Builder) callContinuation(f BuilderContinuation, arg *Builder) {
	if !*b.inContinuation {
		*b.inContinuation = true

		defer func() {
			*b.inContinuation = false

			r := recover()
			if r == nil {
				return
			}

			if buildError, ok := r.(BuildError); ok {
				b.err = buildError.Err
			} else {
				panic(r)
			}
		}()
	}

	f(arg)
}

func (b *Builder) addLengthPrefixed(lenLen int, isASN1 bool, f BuilderContinuation) {
	// 如果构建器遇到错误，后续写入可以被忽略。
	if b.err != nil {
		return
	}

	offset := len(b.result)
	b.add(make([]byte, lenLen)...)

	if b.inContinuation == nil {
		b.inContinuation = new(bool)
	}

	b.child = &Builder{
		result:         b.result,
		fixedSize:      b.fixedSize,
		offset:         offset,
		pendingLenLen:  lenLen,
		pendingIsASN1:  isASN1,
		inContinuation: b.inContinuation,
	}

	b.callContinuation(f, b.child)
	b.flushChild()
	if b.child != nil {
		panic("cryptobyte: internal error")
	}
}

func (b *Builder) flushChild() {
	if b.child == nil {
		return
	}
	b.child.flushChild()
	child := b.child
	b.child = nil

	if child.err != nil {
		b.err = child.err
		return
	}

	length := len(child.result) - child.pendingLenLen - child.offset

	if length < 0 {
		panic("cryptobyte: internal error") // result 意外收缩
	}

	if child.pendingIsASN1 {
		// 对于 ASN.1，我们为长度保留了一个字节。如果这是错误的，
		// 我们必须移动内容以腾出空间。
		if child.pendingLenLen != 1 {
			panic("cryptobyte: internal error")
		}
		var lenLen, lenByte uint8
		if int64(length) > 0xfffffffe {
			b.err = errors.New("pending ASN.1 child too long")
			return
		} else if length > 0xffffff {
			lenLen = 5
			lenByte = 0x80 | 4
		} else if length > 0xffff {
			lenLen = 4
			lenByte = 0x80 | 3
		} else if length > 0xff {
			lenLen = 3
			lenByte = 0x80 | 2
		} else if length > 0x7f {
			lenLen = 2
			lenByte = 0x80 | 1
		} else {
			lenLen = 1
			lenByte = uint8(length)
			length = 0
		}

		// 插入初始长度字节，为后续长度字节腾出空间，并调整偏移量。
		child.result[child.offset] = lenByte
		extraBytes := int(lenLen - 1)
		if extraBytes != 0 {
			child.add(make([]byte, extraBytes)...)
			childStart := child.offset + child.pendingLenLen
			copy(child.result[childStart+extraBytes:], child.result[childStart:])
		}
		child.offset++
		child.pendingLenLen = extraBytes
	}

	l := length
	for i := child.pendingLenLen - 1; i >= 0; i-- {
		child.result[child.offset+i] = uint8(l)
		l >>= 8
	}
	if l != 0 {
		b.err = fmt.Errorf("cryptobyte: pending child length %d exceeds %d-byte length prefix", length, child.pendingLenLen)
		return
	}

	if b.fixedSize && &b.result[0] != &child.result[0] {
		panic("cryptobyte: BuilderContinuation reallocated a fixed-size buffer")
	}

	b.result = child.result
}

func (b *Builder) add(bytes ...byte) {
	if b.err != nil {
		return
	}
	if b.child != nil {
		panic("cryptobyte: attempted write while child is pending")
	}
	if len(b.result)+len(bytes) < len(bytes) {
		b.err = errors.New("cryptobyte: length overflow")
	}
	if b.fixedSize && len(b.result)+len(bytes) > cap(b.result) {
		b.err = errors.New("cryptobyte: Builder is exceeding its fixed-size buffer")
		return
	}
	b.result = append(b.result, bytes...)
}

// Unwrite 回滚直接写入 Builder 的非负 n 个字节。
// 传递给延续的子构建器尝试从其父构建器撤销写入字节将导致 panic。
func (b *Builder) Unwrite(n int) {
	if b.err != nil {
		return
	}
	if b.child != nil {
		panic("cryptobyte: attempted unwrite while child is pending")
	}
	length := len(b.result) - b.pendingLenLen - b.offset
	if length < 0 {
		panic("cryptobyte: internal error")
	}
	if n < 0 {
		panic("cryptobyte: attempted to unwrite negative number of bytes")
	}
	if n > length {
		panic("cryptobyte: attempted to unwrite more than was written")
	}
	b.result = b.result[:len(b.result)-n]
}

// MarshalingValue 将自身编组到 Builder 中。
type MarshalingValue interface {
	// Marshal 由 Builder.AddValue 调用。它接收一个指向构建器的指针，
	// 用于将自身编组到其中。它可能返回在编组过程中发生的错误，
	// 例如未设置或无效的值。
	Marshal(b *Builder) error
}

// AddValue 对 v 调用 Marshal，传递一个指向要追加到的构建器的指针。
// 如果 Marshal 返回错误，它将设置在 Builder 上，以便后续追加不生效。
func (b *Builder) AddValue(v MarshalingValue) {
	err := v.Marshal(b)
	if err != nil {
		b.err = err
	}
}
