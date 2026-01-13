// 版权所有 2010 The Go Authors。保留所有权利。
// 本源代码的使用受 BSD 风格许可证约束，
// 该许可证可在 LICENSE 文件中找到。

// CFB（密码反馈）模式。

package cipher

import (
	"crypto/internal/fips140/alias"
	"crypto/internal/fips140only"
	"crypto/subtle"
)

type cfb struct {
	b       Block
	next    []byte
	out     []byte
	outUsed int

	decrypt bool
}

func (x *cfb) XORKeyStream(dst, src []byte) {
	if len(dst) < len(src) {
		panic("crypto/cipher: output smaller than input")
	}
	if alias.InexactOverlap(dst[:len(src)], src) {
		panic("crypto/cipher: invalid buffer overlap")
	}
	for len(src) > 0 {
		if x.outUsed == len(x.out) {
			x.b.Encrypt(x.out, x.next)
			x.outUsed = 0
		}

		if x.decrypt {
			// 我们可以在解密时预先计算更大的密钥流段。
			// 这将允许更大批量的异或操作，我们应该能够
			// 匹配 CTR/OFB 的性能。
			copy(x.next[x.outUsed:], src)
		}
		n := subtle.XORBytes(dst, src, x.out[x.outUsed:])
		if !x.decrypt {
			copy(x.next[x.outUsed:], dst)
		}
		dst = dst[n:]
		src = src[n:]
		x.outUsed += n
	}
}

// NewCFBEncrypter 返回一个使用给定 [Block] 以密码反馈模式加密的 [Stream]。
// iv 必须与 [Block] 的块大小长度相同。
//
// 已弃用：CFB 模式未经认证，这通常使主动攻击能够操纵和恢复明文。
// 建议应用程序改用 [AEAD] 模式。标准库的 CFB 实现也未经优化，
// 且未作为 FIPS 140-3 模块的一部分进行验证。
// 如果需要未经认证的 [Stream] 模式，请改用 [NewCTR]。
func NewCFBEncrypter(block Block, iv []byte) Stream {
	if fips140only.Enforced() {
		panic("crypto/cipher: use of CFB is not allowed in FIPS 140-only mode")
	}
	return newCFB(block, iv, false)
}

// NewCFBDecrypter 返回一个使用给定 [Block] 以密码反馈模式解密的 [Stream]。
// iv 必须与 [Block] 的块大小长度相同。
//
// 已弃用：CFB 模式未经认证，这通常使主动攻击能够操纵和恢复明文。
// 建议应用程序改用 [AEAD] 模式。标准库的 CFB 实现也未经优化，
// 且未作为 FIPS 140-3 模块的一部分进行验证。
// 如果需要未经认证的 [Stream] 模式，请改用 [NewCTR]。
func NewCFBDecrypter(block Block, iv []byte) Stream {
	if fips140only.Enforced() {
		panic("crypto/cipher: use of CFB is not allowed in FIPS 140-only mode")
	}
	return newCFB(block, iv, true)
}

func newCFB(block Block, iv []byte, decrypt bool) Stream {
	blockSize := block.BlockSize()
	if len(iv) != blockSize {
		// 堆栈跟踪将指示是解密还是加密。
		panic("cipher.newCFB: IV length must equal block size")
	}
	x := &cfb{
		b:       block,
		out:     make([]byte, blockSize),
		next:    make([]byte, blockSize),
		outUsed: blockSize,
		decrypt: decrypt,
	}
	copy(x.next, iv)

	return x
}
