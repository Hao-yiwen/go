// 版权所有 2009 The Go Authors。保留所有权利。
// 本源代码的使用受 BSD 风格许可证约束，
// 该许可证可在 LICENSE 文件中找到。

// 计数器（CTR）模式。

// CTR 通过重复加密递增的计数器并将生成的数据流与输入进行异或，
// 将块密码转换为流密码。

// 参见 NIST SP 800-38A，第 13-15 页

package cipher

import (
	"bytes"
	"crypto/internal/fips140/aes"
	"crypto/internal/fips140/alias"
	"crypto/internal/fips140only"
	"crypto/subtle"
)

type ctr struct {
	b       Block
	ctr     []byte
	out     []byte
	outUsed int
}

const streamBufferSize = 512

// ctrAble 是一个由具有 CTR 特定优化实现的密码实现的接口。
// crypto/aes 不再使用它，我们最终希望将其移除。
type ctrAble interface {
	NewCTR(iv []byte) Stream
}

// NewCTR 返回一个使用给定 [Block] 在计数器模式下加密/解密的 [Stream]。
// iv 的长度必须与 [Block] 的块大小相同。
func NewCTR(block Block, iv []byte) Stream {
	if block, ok := block.(*aes.Block); ok {
		return aesCtrWrapper{aes.NewCTR(block, iv)}
	}
	if fips140only.Enforced() {
		panic("crypto/cipher: use of CTR with non-AES ciphers is not allowed in FIPS 140-only mode")
	}
	if ctr, ok := block.(ctrAble); ok {
		return ctr.NewCTR(iv)
	}
	if len(iv) != block.BlockSize() {
		panic("cipher.NewCTR: IV length must equal block size")
	}
	bufSize := streamBufferSize
	if bufSize < block.BlockSize() {
		bufSize = block.BlockSize()
	}
	return &ctr{
		b:       block,
		ctr:     bytes.Clone(iv),
		out:     make([]byte, 0, bufSize),
		outUsed: 0,
	}
}

// aesCtrWrapper 隐藏 aes.CTR 的额外方法。
type aesCtrWrapper struct {
	c *aes.CTR
}

func (x aesCtrWrapper) XORKeyStream(dst, src []byte) {
	x.c.XORKeyStream(dst, src)
}

func (x *ctr) refill() {
	remain := len(x.out) - x.outUsed
	copy(x.out, x.out[x.outUsed:])
	x.out = x.out[:cap(x.out)]
	bs := x.b.BlockSize()
	for remain <= len(x.out)-bs {
		x.b.Encrypt(x.out[remain:], x.ctr)
		remain += bs

		// 递增计数器
		for i := len(x.ctr) - 1; i >= 0; i-- {
			x.ctr[i]++
			if x.ctr[i] != 0 {
				break
			}
		}
	}
	x.out = x.out[:remain]
	x.outUsed = 0
}

func (x *ctr) XORKeyStream(dst, src []byte) {
	if len(dst) < len(src) {
		panic("crypto/cipher: output smaller than input")
	}
	if alias.InexactOverlap(dst[:len(src)], src) {
		panic("crypto/cipher: invalid buffer overlap")
	}
	if _, ok := x.b.(*aes.Block); ok {
		panic("crypto/cipher: internal error: generic CTR used with AES")
	}
	for len(src) > 0 {
		if x.outUsed >= len(x.out)-x.b.BlockSize() {
			x.refill()
		}
		n := subtle.XORBytes(dst, src, x.out[x.outUsed:])
		dst = dst[n:]
		src = src[n:]
		x.outUsed += n
	}
}
