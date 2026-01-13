// 版权所有 2009 The Go Authors。保留所有权利。
// 本源代码的使用受 BSD 风格许可证约束，
// 该许可证可在 LICENSE 文件中找到。

// Package sha256 实现了 FIPS 180-4 中定义的 SHA224 和 SHA256 哈希算法。
package sha256

import (
	"crypto"
	"crypto/internal/boring"
	"crypto/internal/fips140/sha256"
	"hash"
)

func init() {
	crypto.RegisterHash(crypto.SHA224, New224)
	crypto.RegisterHash(crypto.SHA256, New)
}

// SHA256 校验和的大小（以字节为单位）。
const Size = 32

// SHA224 校验和的大小（以字节为单位）。
const Size224 = 28

// SHA256 和 SHA224 的块大小（以字节为单位）。
const BlockSize = 64

// New 返回一个计算 SHA256 校验和的新 [hash.Hash]。Hash
// 还实现了 [encoding.BinaryMarshaler]、[encoding.BinaryAppender] 和
// [encoding.BinaryUnmarshaler] 来编组和解组内部
// 的哈希状态。
func New() hash.Hash {
	if boring.Enabled {
		return boring.NewSHA256()
	}
	return sha256.New()
}

// New224 返回一个计算 SHA224 校验和的新 [hash.Hash]。Hash
// 还实现了 [encoding.BinaryMarshaler]、[encoding.BinaryAppender] 和
// [encoding.BinaryUnmarshaler] 来编组和解组内部
// 的哈希状态。
func New224() hash.Hash {
	if boring.Enabled {
		return boring.NewSHA224()
	}
	return sha256.New224()
}

// Sum256 返回数据的 SHA256 校验和。
func Sum256(data []byte) [Size]byte {
	if boring.Enabled {
		return boring.SHA256(data)
	}
	h := New()
	h.Write(data)
	var sum [Size]byte
	h.Sum(sum[:0])
	return sum
}

// Sum224 返回数据的 SHA224 校验和。
func Sum224(data []byte) [Size224]byte {
	if boring.Enabled {
		return boring.SHA224(data)
	}
	h := New224()
	h.Write(data)
	var sum [Size224]byte
	h.Sum(sum[:0])
	return sum
}
