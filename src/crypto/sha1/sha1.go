// 版权所有 2009 The Go Authors。保留所有权利。
// 本源代码的使用受 BSD 风格许可证约束，
// 该许可证可在 LICENSE 文件中找到。

// Package sha1 实现了 RFC 3174 中定义的 SHA-1 哈希算法。
//
// SHA-1 在密码学上已损坏，不应用于安全
// 应用程序。
package sha1

import (
	"crypto"
	"crypto/internal/boring"
	"crypto/internal/fips140only"
	"errors"
	"hash"
	"internal/byteorder"
)

func init() {
	crypto.RegisterHash(crypto.SHA1, New)
}

// SHA-1 校验和的大小（以字节为单位）。
const Size = 20

// SHA-1 的块大小（以字节为单位）。
const BlockSize = 64

const (
	chunk = 64
	init0 = 0x67452301
	init1 = 0xEFCDAB89
	init2 = 0x98BADCFE
	init3 = 0x10325476
	init4 = 0xC3D2E1F0
)

// digest 表示校验和的部分计算。
type digest struct {
	h   [5]uint32
	x   [chunk]byte
	nx  int
	len uint64
}

const (
	magic         = "sha\x01"
	marshaledSize = len(magic) + 5*4 + chunk + 8
)

func (d *digest) MarshalBinary() ([]byte, error) {
	return d.AppendBinary(make([]byte, 0, marshaledSize))
}

func (d *digest) AppendBinary(b []byte) ([]byte, error) {
	b = append(b, magic...)
	b = byteorder.BEAppendUint32(b, d.h[0])
	b = byteorder.BEAppendUint32(b, d.h[1])
	b = byteorder.BEAppendUint32(b, d.h[2])
	b = byteorder.BEAppendUint32(b, d.h[3])
	b = byteorder.BEAppendUint32(b, d.h[4])
	b = append(b, d.x[:d.nx]...)
	b = append(b, make([]byte, len(d.x)-d.nx)...)
	b = byteorder.BEAppendUint64(b, d.len)
	return b, nil
}

func (d *digest) UnmarshalBinary(b []byte) error {
	if len(b) < len(magic) || string(b[:len(magic)]) != magic {
		return errors.New("crypto/sha1: invalid hash state identifier")
	}
	if len(b) != marshaledSize {
		return errors.New("crypto/sha1: invalid hash state size")
	}
	b = b[len(magic):]
	b, d.h[0] = consumeUint32(b)
	b, d.h[1] = consumeUint32(b)
	b, d.h[2] = consumeUint32(b)
	b, d.h[3] = consumeUint32(b)
	b, d.h[4] = consumeUint32(b)
	b = b[copy(d.x[:], b):]
	b, d.len = consumeUint64(b)
	d.nx = int(d.len % chunk)
	return nil
}

func consumeUint64(b []byte) ([]byte, uint64) {
	return b[8:], byteorder.BEUint64(b)
}

func consumeUint32(b []byte) ([]byte, uint32) {
	return b[4:], byteorder.BEUint32(b)
}

func (d *digest) Clone() (hash.Cloner, error) {
	r := *d
	return &r, nil
}

func (d *digest) Reset() {
	d.h[0] = init0
	d.h[1] = init1
	d.h[2] = init2
	d.h[3] = init3
	d.h[4] = init4
	d.nx = 0
	d.len = 0
}

// New 返回一个计算 SHA1 校验和的新 [hash.Hash]。Hash
// 还实现了 [encoding.BinaryMarshaler]、[encoding.BinaryAppender] 和
// [encoding.BinaryUnmarshaler] 来编组和解组内部
// 的哈希状态。
func New() hash.Hash {
	if boring.Enabled {
		return boring.NewSHA1()
	}
	d := new(digest)
	d.Reset()
	return d
}

func (d *digest) Size() int { return Size }

func (d *digest) BlockSize() int { return BlockSize }

func (d *digest) Write(p []byte) (nn int, err error) {
	if fips140only.Enforced() {
		return 0, errors.New("crypto/sha1: use of SHA-1 is not allowed in FIPS 140-only mode")
	}
	boring.Unreachable()
	nn = len(p)
	d.len += uint64(nn)
	if d.nx > 0 {
		n := copy(d.x[d.nx:], p)
		d.nx += n
		if d.nx == chunk {
			block(d, d.x[:])
			d.nx = 0
		}
		p = p[n:]
	}
	if len(p) >= chunk {
		n := len(p) &^ (chunk - 1)
		block(d, p[:n])
		p = p[n:]
	}
	if len(p) > 0 {
		d.nx = copy(d.x[:], p)
	}
	return
}

func (d *digest) Sum(in []byte) []byte {
	boring.Unreachable()
	// 制作 d 的副本，以便调用者可以继续写入和求和。
	d0 := *d
	hash := d0.checkSum()
	return append(in, hash[:]...)
}

func (d *digest) checkSum() [Size]byte {
	if fips140only.Enforced() {
		panic("crypto/sha1: use of SHA-1 is not allowed in FIPS 140-only mode")
	}

	len := d.len
	// 填充。添加 1 位和 0 位，直到 56 字节模 64。
	var tmp [64 + 8]byte // 填充 + 长度缓冲区
	tmp[0] = 0x80
	var t uint64
	if len%64 < 56 {
		t = 56 - len%64
	} else {
		t = 64 + 56 - len%64
	}

	// 长度（以位为单位）。
	len <<= 3
	padlen := tmp[:t+8]
	byteorder.BEPutUint64(padlen[t:], len)
	d.Write(padlen)

	if d.nx != 0 {
		panic("d.nx != 0")
	}

	var digest [Size]byte

	byteorder.BEPutUint32(digest[0:], d.h[0])
	byteorder.BEPutUint32(digest[4:], d.h[1])
	byteorder.BEPutUint32(digest[8:], d.h[2])
	byteorder.BEPutUint32(digest[12:], d.h[3])
	byteorder.BEPutUint32(digest[16:], d.h[4])

	return digest
}

// ConstantTimeSum 计算与 [Sum] 相同的结果，但在常数时间内
func (d *digest) ConstantTimeSum(in []byte) []byte {
	d0 := *d
	hash := d0.constSum()
	return append(in, hash[:]...)
}

func (d *digest) constSum() [Size]byte {
	if fips140only.Enforced() {
		panic("crypto/sha1: use of SHA-1 is not allowed in FIPS 140-only mode")
	}

	var length [8]byte
	l := d.len << 3
	for i := uint(0); i < 8; i++ {
		length[i] = byte(l >> (56 - 8*i))
	}

	nx := byte(d.nx)
	t := nx - 56                 // if nx < 56 then the MSB of t is one
	mask1b := byte(int8(t) >> 7) // mask1b is 0xFF iff one block is enough

	separator := byte(0x80) // 一旦使用，就重置为 0x00
	for i := byte(0); i < chunk; i++ {
		mask := byte(int8(i-nx) >> 7) // 数据结束后为 0x00

		// 如果到达数据末尾，替换为 0x80 或 0x00
		d.x[i] = (^mask & separator) | (mask & d.x[i])

		// 一旦使用，将分隔符清零
		separator &= mask

		if i >= 56 {
			// 如果所有内容都适合在一个块中，我们可能需要在这里写入长度
			d.x[i] |= mask1b & length[i-56]
		}
	}

	// 压缩，并且只有当所有内容都适合一个块时才保留摘要
	block(d, d.x[:])

	var digest [Size]byte
	for i, s := range d.h {
		digest[i*4] = mask1b & byte(s>>24)
		digest[i*4+1] = mask1b & byte(s>>16)
		digest[i*4+2] = mask1b & byte(s>>8)
		digest[i*4+3] = mask1b & byte(s)
	}

	for i := byte(0); i < chunk; i++ {
		// 第二个块，总是超过数据末尾，可能以 0x80 开头
		if i < 56 {
			d.x[i] = separator
			separator = 0
		} else {
			d.x[i] = length[i-56]
		}
	}

	// 压缩，并且只有当我们实际需要第二个块时才保留摘要
	block(d, d.x[:])

	for i, s := range d.h {
		digest[i*4] |= ^mask1b & byte(s>>24)
		digest[i*4+1] |= ^mask1b & byte(s>>16)
		digest[i*4+2] |= ^mask1b & byte(s>>8)
		digest[i*4+3] |= ^mask1b & byte(s)
	}

	return digest
}

// Sum 返回数据的 SHA-1 校验和。
func Sum(data []byte) [Size]byte {
	if boring.Enabled {
		return boring.SHA1(data)
	}
	if fips140only.Enforced() {
		panic("crypto/sha1: use of SHA-1 is not allowed in FIPS 140-only mode")
	}
	var d digest
	d.Reset()
	d.Write(data)
	return d.checkSum()
}
