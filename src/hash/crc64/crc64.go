// 版权所有 2009 The Go Authors。保留所有权利。
// 本源代码的使用受 BSD 风格许可证约束，
// 该许可证可在 LICENSE 文件中找到。

// crc64 包实现了 64 位循环冗余校验，即 CRC-64 校验和。
// 有关信息，请参阅 https://en.wikipedia.org/wiki/Cyclic_redundancy_check。
package crc64

import (
	"errors"
	"hash"
	"internal/byteorder"
	"sync"
)

// Size 是 CRC-64 校验和的字节大小。
const Size = 8

// 预定义的多项式。
const (
	// ISO 多项式，在 ISO 3309 中定义，用于 HDLC。
	ISO = 0xD800000000000000

	// ECMA 多项式，在 ECMA 182 中定义。
	ECMA = 0xC96C5795D7870F42
)

// Table 是一个 256 字的表，用于高效处理多项式。
type Table [256]uint64

var (
	slicing8TableISO  *[8]Table
	slicing8TableECMA *[8]Table
)

var buildSlicing8TablesOnce = sync.OnceFunc(buildSlicing8Tables)

func buildSlicing8Tables() {
	slicing8TableISO = makeSlicingBy8Table(makeTable(ISO))
	slicing8TableECMA = makeSlicingBy8Table(makeTable(ECMA))
}

// MakeTable 返回从指定多项式构造的 [Table]。
// 此 [Table] 的内容不得修改。
func MakeTable(poly uint64) *Table {
	buildSlicing8TablesOnce()
	switch poly {
	case ISO:
		return &slicing8TableISO[0]
	case ECMA:
		return &slicing8TableECMA[0]
	default:
		return makeTable(poly)
	}
}

func makeTable(poly uint64) *Table {
	t := new(Table)
	for i := 0; i < 256; i++ {
		crc := uint64(i)
		for j := 0; j < 8; j++ {
			if crc&1 == 1 {
				crc = (crc >> 1) ^ poly
			} else {
				crc >>= 1
			}
		}
		t[i] = crc
	}
	return t
}

func makeSlicingBy8Table(t *Table) *[8]Table {
	var helperTable [8]Table
	helperTable[0] = *t
	for i := 0; i < 256; i++ {
		crc := t[i]
		for j := 1; j < 8; j++ {
			crc = t[crc&0xff] ^ (crc >> 8)
			helperTable[j][i] = crc
		}
	}
	return &helperTable
}

// digest 表示校验和的部分计算结果。
type digest struct {
	crc uint64
	tab *Table
}

// New 创建一个新的 hash.Hash64，使用 [Table] 表示的多项式
// 计算 CRC-64 校验和。其 Sum 方法将以大端字节序输出值。
// 返回的 Hash64 还实现了 [encoding.BinaryMarshaler] 和
// [encoding.BinaryUnmarshaler]，用于序列化和反序列化哈希的内部状态。
func New(tab *Table) hash.Hash64 { return &digest{0, tab} }

func (d *digest) Size() int { return Size }

func (d *digest) BlockSize() int { return 1 }

func (d *digest) Reset() { d.crc = 0 }

const (
	magic         = "crc\x02"
	marshaledSize = len(magic) + 8 + 8
)

func (d *digest) AppendBinary(b []byte) ([]byte, error) {
	b = append(b, magic...)
	b = byteorder.BEAppendUint64(b, tableSum(d.tab))
	b = byteorder.BEAppendUint64(b, d.crc)
	return b, nil
}

func (d *digest) MarshalBinary() ([]byte, error) {
	return d.AppendBinary(make([]byte, 0, marshaledSize))
}

func (d *digest) UnmarshalBinary(b []byte) error {
	if len(b) < len(magic) || string(b[:len(magic)]) != magic {
		return errors.New("hash/crc64: invalid hash state identifier")
	}
	if len(b) != marshaledSize {
		return errors.New("hash/crc64: invalid hash state size")
	}
	if tableSum(d.tab) != byteorder.BEUint64(b[4:]) {
		return errors.New("hash/crc64: tables do not match")
	}
	d.crc = byteorder.BEUint64(b[12:])
	return nil
}

func (d *digest) Clone() (hash.Cloner, error) {
	r := *d
	return &r, nil
}

func update(crc uint64, tab *Table, p []byte) uint64 {
	buildSlicing8TablesOnce()
	crc = ^crc
	// 表比较开销较大，因此对于小尺寸避免比较
	for len(p) >= 64 {
		var helperTable *[8]Table
		if *tab == slicing8TableECMA[0] {
			helperTable = slicing8TableECMA
		} else if *tab == slicing8TableISO[0] {
			helperTable = slicing8TableISO
			// 对于较小的尺寸，创建扩展表太耗时
		} else if len(p) >= 2048 {
			// 根据各种 x86 和 arm CPU 之间的测试，2k 目前是一个合理的阈值。
			// 这在将来可能会改变。
			helperTable = makeSlicingBy8Table(tab)
		} else {
			break
		}
		// 使用 slicing-by-8 更新
		for len(p) > 8 {
			crc ^= byteorder.LEUint64(p)
			crc = helperTable[7][crc&0xff] ^
				helperTable[6][(crc>>8)&0xff] ^
				helperTable[5][(crc>>16)&0xff] ^
				helperTable[4][(crc>>24)&0xff] ^
				helperTable[3][(crc>>32)&0xff] ^
				helperTable[2][(crc>>40)&0xff] ^
				helperTable[1][(crc>>48)&0xff] ^
				helperTable[0][crc>>56]
			p = p[8:]
		}
	}
	// 对于剩余部分或小尺寸
	for _, v := range p {
		crc = tab[byte(crc)^v] ^ (crc >> 8)
	}
	return ^crc
}

// Update 返回将 p 中的字节添加到 crc 的结果。
func Update(crc uint64, tab *Table, p []byte) uint64 {
	return update(crc, tab, p)
}

func (d *digest) Write(p []byte) (n int, err error) {
	d.crc = update(d.crc, d.tab, p)
	return len(p), nil
}

func (d *digest) Sum64() uint64 { return d.crc }

func (d *digest) Sum(in []byte) []byte {
	s := d.Sum64()
	return append(in, byte(s>>56), byte(s>>48), byte(s>>40), byte(s>>32), byte(s>>24), byte(s>>16), byte(s>>8), byte(s))
}

// Checksum 使用 [Table] 表示的多项式
// 返回 data 的 CRC-64 校验和。
func Checksum(data []byte, tab *Table) uint64 { return update(0, tab, data) }

// tableSum 返回表 t 的 ISO 校验和。
func tableSum(t *Table) uint64 {
	var a [2048]byte
	b := a[:0]
	if t != nil {
		for _, x := range t {
			b = byteorder.BEAppendUint64(b, x)
		}
	}
	return Checksum(b, MakeTable(ISO))
}
