// 版权所有 2018 The Go Authors。保留所有权利。
// 本源代码的使用受 BSD 风格许可证约束，
// 该许可证可在 LICENSE 文件中找到。

package chacha20poly1305

import (
	"crypto/cipher"
	"errors"

	"golang.org/x/crypto/chacha20"
)

type xchacha20poly1305 struct {
	key [KeySize]byte
}

// NewX 返回使用给定 256 位密钥的 XChaCha20-Poly1305 AEAD。
//
// XChaCha20-Poly1305 是 ChaCha20-Poly1305 的变体，使用更长的随机数，
// 适合随机生成而没有碰撞风险。当无法轻松确保随机数唯一性时，
// 或者随机数是随机生成时，应优先使用它。
func NewX(key []byte) (cipher.AEAD, error) {
	if fips140Enforced() {
		return nil, errors.New("chacha20poly1305: use of ChaCha20Poly1305 is not allowed in FIPS 140-only mode")
	}
	if len(key) != KeySize {
		return nil, errors.New("chacha20poly1305: bad key length")
	}
	ret := new(xchacha20poly1305)
	copy(ret.key[:], key)
	return ret, nil
}

func (*xchacha20poly1305) NonceSize() int {
	return NonceSizeX
}

func (*xchacha20poly1305) Overhead() int {
	return Overhead
}

func (x *xchacha20poly1305) Seal(dst, nonce, plaintext, additionalData []byte) []byte {
	if len(nonce) != NonceSizeX {
		panic("chacha20poly1305: bad nonce length passed to Seal")
	}

	// XChaCha20-Poly1305 技术上支持 64 位计数器，所以没有大小限制。
	// 但是，由于我们重用了 ChaCha20-Poly1305 实现，计数器的后半部分不可用。
	// 这不太可能成为问题，因为 cipher.AEAD API 要求整个消息都在内存中，
	// 而计数器在 256 GB 时会溢出。
	if uint64(len(plaintext)) > (1<<38)-64 {
		panic("chacha20poly1305: plaintext too large")
	}

	c := new(chacha20poly1305)
	hKey, _ := chacha20.HChaCha20(x.key[:], nonce[0:16])
	copy(c.key[:], hKey)

	// 最终随机数的前 4 个字节是未使用的计数器空间。
	cNonce := make([]byte, NonceSize)
	copy(cNonce[4:12], nonce[16:24])

	return c.seal(dst, cNonce[:], plaintext, additionalData)
}

func (x *xchacha20poly1305) Open(dst, nonce, ciphertext, additionalData []byte) ([]byte, error) {
	if len(nonce) != NonceSizeX {
		panic("chacha20poly1305: bad nonce length passed to Open")
	}
	if len(ciphertext) < 16 {
		return nil, errOpen
	}
	if uint64(len(ciphertext)) > (1<<38)-48 {
		panic("chacha20poly1305: ciphertext too large")
	}

	c := new(chacha20poly1305)
	hKey, _ := chacha20.HChaCha20(x.key[:], nonce[0:16])
	copy(c.key[:], hKey)

	// 最终随机数的前 4 个字节是未使用的计数器空间。
	cNonce := make([]byte, NonceSize)
	copy(cNonce[4:12], nonce[16:24])

	return c.open(dst, cNonce[:], ciphertext, additionalData)
}
