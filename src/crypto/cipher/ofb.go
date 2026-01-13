// 版权所有 2011 The Go Authors。保留所有权利。
// 本源代码的使用受 BSD 风格许可证约束，
// 该许可证可在 LICENSE 文件中找到。

// OFB（输出反馈）模式。

package cipher

import (
	"crypto/internal/fips140/alias"
	"crypto/internal/fips140only"
	"crypto/subtle"
)

type ofb struct {
	b       Block
	cipher  []byte
	out     []byte
	outUsed int
}

// NewOFB 返回一个使用块密码 b 在输出反馈模式下加密或解密的 [Stream]。
// 初始化向量 iv 的长度必须等于 b 的块大小。
//
// 已弃用：OFB 模式未经认证，这通常使主动攻击能够操纵和恢复明文。
// 建议应用程序改用 [AEAD] 模式。标准库的 OFB 实现也未经优化，
// 且未作为 FIPS 140-3 模块的一部分进行验证。
// 如果需要未经认证的 [Stream] 模式，请改用 [NewCTR]。
func NewOFB(b Block, iv []byte) Stream {
	if fips140only.Enforced() {
		panic("crypto/cipher: use of OFB is not allowed in FIPS 140-only mode")
	}

	blockSize := b.BlockSize()
	if len(iv) != blockSize {
		panic("cipher.NewOFB: IV length must equal block size")
	}
	bufSize := streamBufferSize
	if bufSize < blockSize {
		bufSize = blockSize
	}
	x := &ofb{
		b:       b,
		cipher:  make([]byte, blockSize),
		out:     make([]byte, 0, bufSize),
		outUsed: 0,
	}

	copy(x.cipher, iv)
	return x
}

func (x *ofb) refill() {
	bs := x.b.BlockSize()
	remain := len(x.out) - x.outUsed
	if remain > x.outUsed {
		return
	}
	copy(x.out, x.out[x.outUsed:])
	x.out = x.out[:cap(x.out)]
	for remain < len(x.out)-bs {
		x.b.Encrypt(x.cipher, x.cipher)
		copy(x.out[remain:], x.cipher)
		remain += bs
	}
	x.out = x.out[:remain]
	x.outUsed = 0
}

func (x *ofb) XORKeyStream(dst, src []byte) {
	if len(dst) < len(src) {
		panic("crypto/cipher: output smaller than input")
	}
	if alias.InexactOverlap(dst[:len(src)], src) {
		panic("crypto/cipher: invalid buffer overlap")
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
