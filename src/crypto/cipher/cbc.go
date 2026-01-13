// 版权所有 2009 The Go Authors。保留所有权利。
// 本源代码的使用受 BSD 风格许可证约束，
// 该许可证可在 LICENSE 文件中找到。

// 密码块链接（CBC）模式。

// CBC 通过在应用块密码之前将每个明文块与前一个密文块进行异或（链接）
// 来提供机密性。

// 参见 NIST SP 800-38A，第 10-11 页

package cipher

import (
	"bytes"
	"crypto/internal/fips140/aes"
	"crypto/internal/fips140/alias"
	"crypto/internal/fips140only"
	"crypto/subtle"
)

type cbc struct {
	b         Block
	blockSize int
	iv        []byte
	tmp       []byte
}

func newCBC(b Block, iv []byte) *cbc {
	return &cbc{
		b:         b,
		blockSize: b.BlockSize(),
		iv:        bytes.Clone(iv),
		tmp:       make([]byte, b.BlockSize()),
	}
}

type cbcEncrypter cbc

// cbcEncAble 是一个由具有 CBC 加密特定优化实现的密码实现的接口。
// crypto/aes 不再使用它，我们最终希望将其移除。
type cbcEncAble interface {
	NewCBCEncrypter(iv []byte) BlockMode
}

// NewCBCEncrypter 返回一个以密码块链接模式加密的 BlockMode，
// 使用给定的 Block。iv 的长度必须与 Block 的块大小相同。
func NewCBCEncrypter(b Block, iv []byte) BlockMode {
	if len(iv) != b.BlockSize() {
		panic("cipher.NewCBCEncrypter: IV length must equal block size")
	}
	if b, ok := b.(*aes.Block); ok {
		return aes.NewCBCEncrypter(b, [16]byte(iv))
	}
	if fips140only.Enforced() {
		panic("crypto/cipher: use of CBC with non-AES ciphers is not allowed in FIPS 140-only mode")
	}
	if cbc, ok := b.(cbcEncAble); ok {
		return cbc.NewCBCEncrypter(iv)
	}
	return (*cbcEncrypter)(newCBC(b, iv))
}

// newCBCGenericEncrypter 返回一个以密码块链接模式加密的 BlockMode，
// 使用给定的 Block。iv 的长度必须与 Block 的块大小相同。
// 这始终返回通用的非汇编加密器，用于模糊测试。
func newCBCGenericEncrypter(b Block, iv []byte) BlockMode {
	if len(iv) != b.BlockSize() {
		panic("cipher.NewCBCEncrypter: IV length must equal block size")
	}
	return (*cbcEncrypter)(newCBC(b, iv))
}

func (x *cbcEncrypter) BlockSize() int { return x.blockSize }

func (x *cbcEncrypter) CryptBlocks(dst, src []byte) {
	if len(src)%x.blockSize != 0 {
		panic("crypto/cipher: input not full blocks")
	}
	if len(dst) < len(src) {
		panic("crypto/cipher: output smaller than input")
	}
	if alias.InexactOverlap(dst[:len(src)], src) {
		panic("crypto/cipher: invalid buffer overlap")
	}
	if _, ok := x.b.(*aes.Block); ok {
		panic("crypto/cipher: internal error: generic CBC used with AES")
	}

	iv := x.iv

	for len(src) > 0 {
		// 将异或结果写入 dst，然后就地加密。
		subtle.XORBytes(dst[:x.blockSize], src[:x.blockSize], iv)
		x.b.Encrypt(dst[:x.blockSize], dst[:x.blockSize])

		// 使用此块作为下一个 iv 移动到下一个块。
		iv = dst[:x.blockSize]
		src = src[x.blockSize:]
		dst = dst[x.blockSize:]
	}

	// 为下一次 CryptBlocks 调用保存 iv。
	copy(x.iv, iv)
}

func (x *cbcEncrypter) SetIV(iv []byte) {
	if len(iv) != len(x.iv) {
		panic("cipher: incorrect length IV")
	}
	copy(x.iv, iv)
}

type cbcDecrypter cbc

// cbcDecAble 是一个由具有 CBC 解密特定优化实现的密码实现的接口。
// crypto/aes 不再使用它，我们最终希望将其移除。
type cbcDecAble interface {
	NewCBCDecrypter(iv []byte) BlockMode
}

// NewCBCDecrypter 返回一个以密码块链接模式解密的 BlockMode，
// 使用给定的 Block。iv 的长度必须与 Block 的块大小相同，
// 并且必须与用于加密数据的 iv 匹配。
func NewCBCDecrypter(b Block, iv []byte) BlockMode {
	if len(iv) != b.BlockSize() {
		panic("cipher.NewCBCDecrypter: IV length must equal block size")
	}
	if b, ok := b.(*aes.Block); ok {
		return aes.NewCBCDecrypter(b, [16]byte(iv))
	}
	if fips140only.Enforced() {
		panic("crypto/cipher: use of CBC with non-AES ciphers is not allowed in FIPS 140-only mode")
	}
	if cbc, ok := b.(cbcDecAble); ok {
		return cbc.NewCBCDecrypter(iv)
	}
	return (*cbcDecrypter)(newCBC(b, iv))
}

// newCBCGenericDecrypter 返回一个以密码块链接模式加密的 BlockMode，
// 使用给定的 Block。iv 的长度必须与 Block 的块大小相同。
// 这始终返回通用的非汇编解密器，用于模糊测试。
func newCBCGenericDecrypter(b Block, iv []byte) BlockMode {
	if len(iv) != b.BlockSize() {
		panic("cipher.NewCBCDecrypter: IV length must equal block size")
	}
	return (*cbcDecrypter)(newCBC(b, iv))
}

func (x *cbcDecrypter) BlockSize() int { return x.blockSize }

func (x *cbcDecrypter) CryptBlocks(dst, src []byte) {
	if len(src)%x.blockSize != 0 {
		panic("crypto/cipher: input not full blocks")
	}
	if len(dst) < len(src) {
		panic("crypto/cipher: output smaller than input")
	}
	if alias.InexactOverlap(dst[:len(src)], src) {
		panic("crypto/cipher: invalid buffer overlap")
	}
	if _, ok := x.b.(*aes.Block); ok {
		panic("crypto/cipher: internal error: generic CBC used with AES")
	}
	if len(src) == 0 {
		return
	}

	// 对于每个块，我们需要将解密后的数据与前一个块的密文（iv）进行异或。
	// 为了避免每次都复制，我们反向遍历块。
	end := len(src)
	start := end - x.blockSize
	prev := start - x.blockSize

	// 复制最后一个密文块作为新的 iv 的准备。
	copy(x.tmp, src[start:end])

	// 遍历除第一个块以外的所有块。
	for start > 0 {
		x.b.Decrypt(dst[start:end], src[start:end])
		subtle.XORBytes(dst[start:end], dst[start:end], src[prev:start])

		end = start
		start = prev
		prev -= x.blockSize
	}

	// 第一个块比较特殊，因为它使用保存的 iv。
	x.b.Decrypt(dst[start:end], src[start:end])
	subtle.XORBytes(dst[start:end], dst[start:end], x.iv)

	// 将新的 iv 设置为我们之前复制的第一个块。
	x.iv, x.tmp = x.tmp, x.iv
}

func (x *cbcDecrypter) SetIV(iv []byte) {
	if len(iv) != len(x.iv) {
		panic("cipher: incorrect length IV")
	}
	copy(x.iv, iv)
}
