// 版权所有 2009 The Go Authors。保留所有权利。
// 本源代码的使用受 BSD 风格许可证约束，
// 该许可证可在 LICENSE 文件中找到。

// adler32 包实现了 Adler-32 校验和。
//
// 它在 RFC 1950 中定义：
//
//	Adler-32 由每字节累加的两个和组成：s1 是
//	所有字节的和，s2 是所有 s1 值的和。两个和
//	都对 65521 取模。s1 初始化为 1，s2 初始化为零。
//	Adler-32 校验和以 s2*65536 + s1 的形式存储，
//	采用最高有效字节优先（网络）字节序。
package adler32

import (
	"errors"
	"hash"
	"internal/byteorder"
)

const (
	// mod 是小于 65536 的最大素数。
	mod = 65521
	// nmax 是满足以下条件的最大 n：
	// 255 * n * (n+1) / 2 + (n+1) * (mod-1) <= 2^32-1。
	// 它在 RFC 1950 中提到（搜索 "5552"）。
	nmax = 5552
)

// Size 是 Adler-32 校验和的字节大小。
const Size = 4

// digest 表示校验和的部分计算结果。
// 低 16 位是 s1，高 16 位是 s2。
type digest uint32

func (d *digest) Reset() { *d = 1 }

// New 返回一个新的 hash.Hash32，用于计算 Adler-32 校验和。
// 其 Sum 方法将以大端字节序输出值。
// 返回的 Hash32 还实现了 [encoding.BinaryMarshaler] 和
// [encoding.BinaryUnmarshaler]，用于序列化和反序列化
// 哈希的内部状态。
func New() hash.Hash32 {
	d := new(digest)
	d.Reset()
	return d
}

func (d *digest) Size() int { return Size }

func (d *digest) BlockSize() int { return 4 }

const (
	magic         = "adl\x01"
	marshaledSize = len(magic) + 4
)

func (d *digest) AppendBinary(b []byte) ([]byte, error) {
	b = append(b, magic...)
	b = byteorder.BEAppendUint32(b, uint32(*d))
	return b, nil
}

func (d *digest) MarshalBinary() ([]byte, error) {
	return d.AppendBinary(make([]byte, 0, marshaledSize))
}

func (d *digest) UnmarshalBinary(b []byte) error {
	if len(b) < len(magic) || string(b[:len(magic)]) != magic {
		return errors.New("hash/adler32: invalid hash state identifier")
	}
	if len(b) != marshaledSize {
		return errors.New("hash/adler32: invalid hash state size")
	}
	*d = digest(byteorder.BEUint32(b[len(magic):]))
	return nil
}

func (d *digest) Clone() (hash.Cloner, error) {
	r := *d
	return &r, nil
}

// 将 p 添加到正在运行的校验和 d 中。
func update(d digest, p []byte) digest {
	s1, s2 := uint32(d&0xffff), uint32(d>>16)
	for len(p) > 0 {
		var q []byte
		if len(p) > nmax {
			p, q = p[:nmax], p[nmax:]
		}
		for len(p) >= 4 {
			s1 += uint32(p[0])
			s2 += s1
			s1 += uint32(p[1])
			s2 += s1
			s1 += uint32(p[2])
			s2 += s1
			s1 += uint32(p[3])
			s2 += s1
			p = p[4:]
		}
		for _, x := range p {
			s1 += uint32(x)
			s2 += s1
		}
		s1 %= mod
		s2 %= mod
		p = q
	}
	return digest(s2<<16 | s1)
}

func (d *digest) Write(p []byte) (nn int, err error) {
	*d = update(*d, p)
	return len(p), nil
}

func (d *digest) Sum32() uint32 { return uint32(*d) }

func (d *digest) Sum(in []byte) []byte {
	s := uint32(*d)
	return append(in, byte(s>>24), byte(s>>16), byte(s>>8), byte(s))
}

// Checksum 返回 data 的 Adler-32 校验和。
func Checksum(data []byte) uint32 { return uint32(update(1, data)) }
