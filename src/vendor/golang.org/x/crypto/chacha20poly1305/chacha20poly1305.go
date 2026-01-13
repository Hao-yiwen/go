// 版权所有 2016 The Go Authors。保留所有权利。
// 本源代码的使用受 BSD 风格许可证约束，
// 该许可证可在 LICENSE 文件中找到。

// chacha20poly1305 包实现了 ChaCha20-Poly1305 AEAD 及其扩展随机数变体
// XChaCha20-Poly1305，如 RFC 8439 和 draft-irtf-cfrg-xchacha-01 中所规定。
package chacha20poly1305

import (
	"crypto/cipher"
	"errors"
)

const (
	// KeySize 是此 AEAD 使用的密钥大小，以字节为单位。
	KeySize = 32

	// NonceSize 是此 AEAD 标准变体使用的随机数大小，以字节为单位。
	//
	// 注意，如果同一密钥重复使用超过 2³² 次，
	// 这个长度太短，不能安全地随机生成。
	NonceSize = 12

	// NonceSizeX 是此 AEAD 的 XChaCha20-Poly1305 变体使用的随机数大小，
	// 以字节为单位。
	NonceSizeX = 24

	// Overhead 是 Poly1305 认证标签的大小，
	// 也是密文长度与明文长度之间的差值。
	Overhead = 16
)

type chacha20poly1305 struct {
	key [KeySize]byte
}

// New 返回使用给定 256 位密钥的 ChaCha20-Poly1305 AEAD。
func New(key []byte) (cipher.AEAD, error) {
	if fips140Enforced() {
		return nil, errors.New("chacha20poly1305: use of ChaCha20Poly1305 is not allowed in FIPS 140-only mode")
	}
	if len(key) != KeySize {
		return nil, errors.New("chacha20poly1305: bad key length")
	}
	ret := new(chacha20poly1305)
	copy(ret.key[:], key)
	return ret, nil
}

func (c *chacha20poly1305) NonceSize() int {
	return NonceSize
}

func (c *chacha20poly1305) Overhead() int {
	return Overhead
}

func (c *chacha20poly1305) Seal(dst, nonce, plaintext, additionalData []byte) []byte {
	if len(nonce) != NonceSize {
		panic("chacha20poly1305: bad nonce length passed to Seal")
	}

	if uint64(len(plaintext)) > (1<<38)-64 {
		panic("chacha20poly1305: plaintext too large")
	}

	return c.seal(dst, nonce, plaintext, additionalData)
}

var errOpen = errors.New("chacha20poly1305: message authentication failed")

func (c *chacha20poly1305) Open(dst, nonce, ciphertext, additionalData []byte) ([]byte, error) {
	if len(nonce) != NonceSize {
		panic("chacha20poly1305: bad nonce length passed to Open")
	}
	if len(ciphertext) < 16 {
		return nil, errOpen
	}
	if uint64(len(ciphertext)) > (1<<38)-48 {
		panic("chacha20poly1305: ciphertext too large")
	}

	return c.open(dst, nonce, ciphertext, additionalData)
}

// sliceForAppend 接受一个切片和请求的字节数。它返回一个切片，
// 其内容是给定切片的内容后跟那么多字节，以及第二个别名切片，
// 只包含额外的字节。如果原始切片有足够的容量，则不执行分配。
func sliceForAppend(in []byte, n int) (head, tail []byte) {
	if total := len(in) + n; cap(in) >= total {
		head = in[:total]
	} else {
		head = make([]byte, total)
		copy(head, in)
	}
	tail = head[len(in):]
	return
}
