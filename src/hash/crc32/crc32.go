// 版权所有 2009 The Go Authors。保留所有权利。
// 本源代码的使用受 BSD 风格许可证约束，
// 该许可证可在 LICENSE 文件中找到。

// crc32 包实现了 32 位循环冗余校验，即 CRC-32 校验和。
// 有关信息，请参阅 https://en.wikipedia.org/wiki/Cyclic_redundancy_check。
//
// 多项式以 LSB 优先形式表示，也称为反转表示。
//
// 有关信息，请参阅
// https://en.wikipedia.org/wiki/Mathematics_of_cyclic_redundancy_checks#Reversed_representations_and_reciprocal_polynomials。
package crc32

import (
	"errors"
	"hash"
	"internal/byteorder"
	"sync"
	"sync/atomic"
)

// Size 是 CRC-32 校验和的字节大小。
const Size = 4

// 预定义的多项式。
const (
	// IEEE 是目前最常用的 CRC-32 多项式。
	// 被以太网（IEEE 802.3）、v.42、fddi、gzip、zip、png 等使用...
	IEEE = 0xedb88320

	// Castagnoli 多项式，用于 iSCSI。
	// 比 IEEE 具有更好的错误检测特性。
	// https://dx.doi.org/10.1109/26.231911
	Castagnoli = 0x82f63b78

	// Koopman 多项式。
	// 同样比 IEEE 具有更好的错误检测特性。
	// https://dx.doi.org/10.1109/DSN.2002.1028931
	Koopman = 0xeb31d82e
)

// Table 是一个 256 字的表，用于高效处理多项式。
type Table [256]uint32

// 本文件使用了在特定架构文件中实现的函数。
// 它们实现的接口如下：
//
//    // archAvailableIEEE 报告是否有特定架构的 CRC32-IEEE
//    // 算法可用。
//    archAvailableIEEE() bool
//
//    // archInitIEEE 初始化特定架构的 CRC32-IEEE 算法。
//    // 只有在 archAvailableIEEE() 返回 true 时才能调用。
//    archInitIEEE()
//
//    // archUpdateIEEE 更新给定的 CRC32-IEEE。只有在
//    // 之前调用过 archInitIEEE() 时才能调用。
//    archUpdateIEEE(crc uint32, p []byte) uint32
//
//    // archAvailableCastagnoli 报告是否有特定架构的
//    // CRC32-C 算法可用。
//    archAvailableCastagnoli() bool
//
//    // archInitCastagnoli 初始化特定架构的 CRC32-C
//    // 算法。只有在 archAvailableCastagnoli() 返回
//    // true 时才能调用。
//    archInitCastagnoli()
//
//    // archUpdateCastagnoli 更新给定的 CRC32-C。只有在
//    // 之前调用过 archInitCastagnoli() 时才能调用。
//    archUpdateCastagnoli(crc uint32, p []byte) uint32

// castagnoliTable 指向一个为 Castagnoli 多项式延迟初始化的 Table。
// 当被要求创建 Castagnoli 表时，MakeTable 将始终返回此值，
// 这样我们可以与其比较以找出调用者何时使用此多项式。
var castagnoliTable *Table
var castagnoliTable8 *slicing8Table
var updateCastagnoli func(crc uint32, p []byte) uint32
var haveCastagnoli atomic.Bool

var castagnoliInitOnce = sync.OnceFunc(func() {
	castagnoliTable = simpleMakeTable(Castagnoli)

	if archAvailableCastagnoli() {
		archInitCastagnoli()
		updateCastagnoli = archUpdateCastagnoli
	} else {
		// 初始化 slicing-by-8 表。
		castagnoliTable8 = slicingMakeTable(Castagnoli)
		updateCastagnoli = func(crc uint32, p []byte) uint32 {
			return slicingUpdate(crc, castagnoliTable8, p)
		}
	}

	haveCastagnoli.Store(true)
})

// IEEETable 是 [IEEE] 多项式的表。
var IEEETable = simpleMakeTable(IEEE)

// ieeeTable8 是 IEEE 的 slicing8Table
var ieeeTable8 *slicing8Table
var updateIEEE func(crc uint32, p []byte) uint32

var ieeeInitOnce = sync.OnceFunc(func() {
	if archAvailableIEEE() {
		archInitIEEE()
		updateIEEE = archUpdateIEEE
	} else {
		// 初始化 slicing-by-8 表。
		ieeeTable8 = slicingMakeTable(IEEE)
		updateIEEE = func(crc uint32, p []byte) uint32 {
			return slicingUpdate(crc, ieeeTable8, p)
		}
	}
})

// MakeTable 返回从指定多项式构造的 [Table]。
// 此 [Table] 的内容不得修改。
func MakeTable(poly uint32) *Table {
	switch poly {
	case IEEE:
		ieeeInitOnce()
		return IEEETable
	case Castagnoli:
		castagnoliInitOnce()
		return castagnoliTable
	default:
		return simpleMakeTable(poly)
	}
}

// digest 表示校验和的部分计算结果。
type digest struct {
	crc uint32
	tab *Table
}

// New 创建一个新的 [hash.Hash32]，使用 [Table] 表示的多项式
// 计算 CRC-32 校验和。其 Sum 方法将以大端字节序输出值。
// 返回的 Hash32 还实现了 [encoding.BinaryMarshaler] 和
// [encoding.BinaryUnmarshaler]，用于序列化和反序列化哈希的内部状态。
func New(tab *Table) hash.Hash32 {
	if tab == IEEETable {
		ieeeInitOnce()
	}
	return &digest{0, tab}
}

// NewIEEE 创建一个新的 [hash.Hash32]，使用 [IEEE] 多项式
// 计算 CRC-32 校验和。其 Sum 方法将以大端字节序输出值。
// 返回的 Hash32 还实现了 [encoding.BinaryMarshaler] 和
// [encoding.BinaryUnmarshaler]，用于序列化和反序列化哈希的内部状态。
func NewIEEE() hash.Hash32 { return New(IEEETable) }

func (d *digest) Size() int { return Size }

func (d *digest) BlockSize() int { return 1 }

func (d *digest) Reset() { d.crc = 0 }

const (
	magic         = "crc\x01"
	marshaledSize = len(magic) + 4 + 4
)

func (d *digest) AppendBinary(b []byte) ([]byte, error) {
	b = append(b, magic...)
	b = byteorder.BEAppendUint32(b, tableSum(d.tab))
	b = byteorder.BEAppendUint32(b, d.crc)
	return b, nil
}

func (d *digest) MarshalBinary() ([]byte, error) {
	return d.AppendBinary(make([]byte, 0, marshaledSize))

}

func (d *digest) UnmarshalBinary(b []byte) error {
	if len(b) < len(magic) || string(b[:len(magic)]) != magic {
		return errors.New("hash/crc32: invalid hash state identifier")
	}
	if len(b) != marshaledSize {
		return errors.New("hash/crc32: invalid hash state size")
	}
	if tableSum(d.tab) != byteorder.BEUint32(b[4:]) {
		return errors.New("hash/crc32: tables do not match")
	}
	d.crc = byteorder.BEUint32(b[8:])
	return nil
}

func (d *digest) Clone() (hash.Cloner, error) {
	r := *d
	return &r, nil
}

func update(crc uint32, tab *Table, p []byte, checkInitIEEE bool) uint32 {
	switch {
	case haveCastagnoli.Load() && tab == castagnoliTable:
		return updateCastagnoli(crc, p)
	case tab == IEEETable:
		if checkInitIEEE {
			ieeeInitOnce()
		}
		return updateIEEE(crc, p)
	default:
		return simpleUpdate(crc, tab, p)
	}
}

// Update 返回将 p 中的字节添加到 crc 的结果。
func Update(crc uint32, tab *Table, p []byte) uint32 {
	// 不幸的是，因为 IEEETable 是导出的，IEEE 可能在没有
	// 调用 MakeTable 的情况下被使用。在这种情况下，我们必须确保它被初始化。
	return update(crc, tab, p, true)
}

func (d *digest) Write(p []byte) (n int, err error) {
	// 我们只通过 New() 创建 digest 对象，
	// 在这种情况下它会处理初始化。
	d.crc = update(d.crc, d.tab, p, false)
	return len(p), nil
}

func (d *digest) Sum32() uint32 { return d.crc }

func (d *digest) Sum(in []byte) []byte {
	s := d.Sum32()
	return append(in, byte(s>>24), byte(s>>16), byte(s>>8), byte(s))
}

// Checksum 使用 [Table] 表示的多项式
// 返回 data 的 CRC-32 校验和。
func Checksum(data []byte, tab *Table) uint32 { return Update(0, tab, data) }

// ChecksumIEEE 使用 [IEEE] 多项式
// 返回 data 的 CRC-32 校验和。
func ChecksumIEEE(data []byte) uint32 {
	ieeeInitOnce()
	return updateIEEE(0, data)
}

// tableSum 返回表 t 的 IEEE 校验和。
func tableSum(t *Table) uint32 {
	var a [1024]byte
	b := a[:0]
	if t != nil {
		for _, x := range t {
			b = byteorder.BEAppendUint32(b, x)
		}
	}
	return ChecksumIEEE(b)
}
