// 版权所有 2009 The Go Authors。保留所有权利。
// 本源代码的使用受 BSD 风格许可证约束，
// 该许可证可在 LICENSE 文件中找到。

//go:generate go run gen.go -output md5block.go

// Package md5 实现了 RFC 1321 中定义的 MD5 哈希算法。
//
// MD5 在密码学上已损坏，不应用于安全
// 应用程序。
package md5

import (
	"crypto"
	"crypto/internal/fips140only"
	"errors"
	"hash"
	"internal/byteorder"
)

func init() {
	crypto.RegisterHash(crypto.MD5, New)
}

// MD5 校验和的大小（以字节为单位）。
const Size = 16

// MD5 的块大小（以字节为单位）。
const BlockSize = 64

// 可以传递给 block() 的最大字节数。限制存在
// 因为依赖汇编例程的实现是不可抢占的。
const maxAsmIters = 1024
const maxAsmSize = BlockSize * maxAsmIters // 64KiB

const (
	init0 = 0x67452301
	init1 = 0xEFCDAB89
	init2 = 0x98BADCFE
	init3 = 0x10325476
)

// digest 表示校验和的部分计算。
type digest struct {
	s   [4]uint32
	x   [BlockSize]byte
	nx  int
	len uint64
}

func (d *digest) Reset() {
	d.s[0] = init0
	d.s[1] = init1
	d.s[2] = init2
	d.s[3] = init3
	d.nx = 0
	d.len = 0
}

const (
	magic         = "md5\x01"
	marshaledSize = len(magic) + 4*4 + BlockSize + 8
)

func (d *digest) MarshalBinary() ([]byte, error) {
	return d.AppendBinary(make([]byte, 0, marshaledSize))
}

func (d *digest) AppendBinary(b []byte) ([]byte, error) {
	b = append(b, magic...)
	b = byteorder.BEAppendUint32(b, d.s[0])
	b = byteorder.BEAppendUint32(b, d.s[1])
	b = byteorder.BEAppendUint32(b, d.s[2])
	b = byteorder.BEAppendUint32(b, d.s[3])
	b = append(b, d.x[:d.nx]...)
	b = append(b, make([]byte, len(d.x)-d.nx)...)
	b = byteorder.BEAppendUint64(b, d.len)
	return b, nil
}

func (d *digest) UnmarshalBinary(b []byte) error {
	if len(b) < len(magic) || string(b[:len(magic)]) != magic {
		return errors.New("crypto/md5: invalid hash state identifier")
	}
	if len(b) != marshaledSize {
		return errors.New("crypto/md5: invalid hash state size")
	}
	b = b[len(magic):]
	b, d.s[0] = consumeUint32(b)
	b, d.s[1] = consumeUint32(b)
	b, d.s[2] = consumeUint32(b)
	b, d.s[3] = consumeUint32(b)
	b = b[copy(d.x[:], b):]
	b, d.len = consumeUint64(b)
	d.nx = int(d.len % BlockSize)
	return nil
}

func consumeUint64(b []byte) ([]byte, uint64) {
	return b[8:], byteorder.BEUint64(b[0:8])
}

func consumeUint32(b []byte) ([]byte, uint32) {
	return b[4:], byteorder.BEUint32(b[0:4])
}

func (d *digest) Clone() (hash.Cloner, error) {
	r := *d
	return &r, nil
}

// New 返回一个计算 MD5 校验和的新 [hash.Hash]。Hash
// 还实现了 [encoding.BinaryMarshaler]、[encoding.BinaryAppender] 和
// [encoding.BinaryUnmarshaler] 来编组和解组内部
// 的哈希状态。
func New() hash.Hash {
	d := new(digest)
	d.Reset()
	return d
}

func (d *digest) Size() int { return Size }

func (d *digest) BlockSize() int { return BlockSize }

func (d *digest) Write(p []byte) (nn int, err error) {
	if fips140only.Enforced() {
		return 0, errors.New("crypto/md5: use of MD5 is not allowed in FIPS 140-only mode")
	}
	// 注意，我们当前调用 block 或 blockGeneric
	// 直接（使用 haveAsm 防护），因为这允许
	// 逃逸分析来查看 p 和 d 不逃逸。
	nn = len(p)
	d.len += uint64(nn)
	if d.nx > 0 {
		n := copy(d.x[d.nx:], p)
		d.nx += n
		if d.nx == BlockSize {
			if haveAsm {
				block(d, d.x[:])
			} else {
				blockGeneric(d, d.x[:])
			}
			d.nx = 0
		}
		p = p[n:]
	}
	if len(p) >= BlockSize {
		n := len(p) &^ (BlockSize - 1)
		if haveAsm {
			for n > maxAsmSize {
				block(d, p[:maxAsmSize])
				p = p[maxAsmSize:]
				n -= maxAsmSize
			}
			block(d, p[:n])
		} else {
			blockGeneric(d, p[:n])
		}
		p = p[n:]
	}
	if len(p) > 0 {
		d.nx = copy(d.x[:], p)
	}
	return
}

func (d *digest) Sum(in []byte) []byte {
	// 制作 d 的副本，以便调用者可以继续写入和求和。
	d0 := *d
	hash := d0.checkSum()
	return append(in, hash[:]...)
}

func (d *digest) checkSum() [Size]byte {
	if fips140only.Enforced() {
		panic("crypto/md5: use of MD5 is not allowed in FIPS 140-only mode")
	}

	// 将 0x80 追加到消息末尾，然后追加零
	// 直到长度是 56 字节的倍数。最后追加
	// 8 个字节代表消息长度（以位为单位）。
	//
	// 1 字节结束标记 :: 0-63 字节填充 :: 8 字节长度
	tmp := [1 + 63 + 8]byte{0x80}
	pad := (55 - d.len) % 64                     // 计算填充字节数
	byteorder.LEPutUint64(tmp[1+pad:], d.len<<3) // 追加长度（以位为单位）
	d.Write(tmp[:1+pad+8])

	// 之前的写入确保整数个
	// 块（即 64 字节的倍数）已被哈希。
	if d.nx != 0 {
		panic("d.nx != 0")
	}

	var digest [Size]byte
	byteorder.LEPutUint32(digest[0:], d.s[0])
	byteorder.LEPutUint32(digest[4:], d.s[1])
	byteorder.LEPutUint32(digest[8:], d.s[2])
	byteorder.LEPutUint32(digest[12:], d.s[3])
	return digest
}

// Sum 返回数据的 MD5 校验和。
func Sum(data []byte) [Size]byte {
	var d digest
	d.Reset()
	d.Write(data)
	return d.checkSum()
}
