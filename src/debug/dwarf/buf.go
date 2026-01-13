// 版权所有 2009 The Go Authors。保留所有权利。
// 本源代码的使用受 BSD 风格许可证约束，
// 该许可证可在 LICENSE 文件中找到。

// DWARF 数据流的缓冲读取和解码。

package dwarf

import (
	"bytes"
	"encoding/binary"
	"strconv"
)

// 正在解码的数据缓冲区。
type buf struct {
	dwarf  *Data
	order  binary.ByteOrder
	format dataFormat
	name   string
	off    Offset
	data   []byte
	err    error
}

// 数据格式，除了字节序之外。这会影响某些字段格式的处理。
type dataFormat interface {
	// DWARF 版本号。零表示未知。
	version() int

	// 是否为 64 位 DWARF 格式？
	dwarf64() (dwarf64 bool, isKnown bool)

	// 地址的大小，以字节为单位。零表示未知。
	addrsize() int
}

// DWARF 的某些部分没有数据格式，例如缩写表。
type unknownFormat struct{}

func (u unknownFormat) version() int {
	return 0
}

func (u unknownFormat) dwarf64() (bool, bool) {
	return false, false
}

func (u unknownFormat) addrsize() int {
	return 0
}

func makeBuf(d *Data, format dataFormat, name string, off Offset, data []byte) buf {
	return buf{d, d.order, format, name, off, data, nil}
}

func (b *buf) uint8() uint8 {
	if len(b.data) < 1 {
		b.error("underflow")
		return 0
	}
	val := b.data[0]
	b.data = b.data[1:]
	b.off++
	return val
}

func (b *buf) bytes(n int) []byte {
	if n < 0 || len(b.data) < n {
		b.error("underflow")
		return nil
	}
	data := b.data[0:n]
	b.data = b.data[n:]
	b.off += Offset(n)
	return data
}

func (b *buf) skip(n int) { b.bytes(n) }

func (b *buf) string() string {
	i := bytes.IndexByte(b.data, 0)
	if i < 0 {
		b.error("underflow")
		return ""
	}

	s := string(b.data[0:i])
	b.data = b.data[i+1:]
	b.off += Offset(i + 1)
	return s
}

func (b *buf) uint16() uint16 {
	a := b.bytes(2)
	if a == nil {
		return 0
	}
	return b.order.Uint16(a)
}

func (b *buf) uint24() uint32 {
	a := b.bytes(3)
	if a == nil {
		return 0
	}
	if b.dwarf.bigEndian {
		return uint32(a[2]) | uint32(a[1])<<8 | uint32(a[0])<<16
	} else {
		return uint32(a[0]) | uint32(a[1])<<8 | uint32(a[2])<<16
	}
}

func (b *buf) uint32() uint32 {
	a := b.bytes(4)
	if a == nil {
		return 0
	}
	return b.order.Uint32(a)
}

func (b *buf) uint64() uint64 {
	a := b.bytes(8)
	if a == nil {
		return 0
	}
	return b.order.Uint64(a)
}

// 读取一个 varint，每字节 7 位，小端序。
// 0x80 位表示需要读取另一个字节。
func (b *buf) varint() (c uint64, bits uint) {
	for i := 0; i < len(b.data); i++ {
		byte := b.data[i]
		c |= uint64(byte&0x7F) << bits
		bits += 7
		if byte&0x80 == 0 {
			b.off += Offset(i + 1)
			b.data = b.data[i+1:]
			return c, bits
		}
	}
	return 0, 0
}

// 无符号整数就是一个 varint。
func (b *buf) uint() uint64 {
	x, _ := b.varint()
	return x
}

// 有符号整数是一个符号扩展的 varint。
func (b *buf) int() int64 {
	ux, bits := b.varint()
	x := int64(ux)
	if x&(1<<(bits-1)) != 0 {
		x |= -1 << bits
	}
	return x
}

// 地址大小的无符号整数。
func (b *buf) addr() uint64 {
	switch b.format.addrsize() {
	case 1:
		return uint64(b.uint8())
	case 2:
		return uint64(b.uint16())
	case 4:
		return uint64(b.uint32())
	case 8:
		return b.uint64()
	}
	b.error("unknown address size")
	return 0
}

func (b *buf) unitLength() (length Offset, dwarf64 bool) {
	length = Offset(b.uint32())
	if length == 0xffffffff {
		dwarf64 = true
		length = Offset(b.uint64())
	} else if length >= 0xfffffff0 {
		b.error("unit length has reserved value")
	}
	return
}

func (b *buf) error(s string) {
	if b.err == nil {
		b.data = nil
		b.err = DecodeError{b.name, b.off, s}
	}
}

type DecodeError struct {
	Name   string
	Offset Offset
	Err    string
}

func (e DecodeError) Error() string {
	return "decoding dwarf section " + e.Name + " at offset 0x" + strconv.FormatInt(int64(e.Offset), 16) + ": " + e.Err
}
