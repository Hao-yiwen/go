// 版权所有 2011 The Go Authors。保留所有权利。
// 本源代码的使用受 BSD 风格许可证约束，
// 该许可证可在 LICENSE 文件中找到。

package des

import (
	"crypto/cipher"
	"crypto/internal/fips140/alias"
	"crypto/internal/fips140only"
	"errors"
	"internal/byteorder"
	"strconv"
)

// DES 块大小（以字节为单位）。
const BlockSize = 8

type KeySizeError int

func (k KeySizeError) Error() string {
	return "crypto/des: invalid key size " + strconv.Itoa(int(k))
}

// desCipher 是 DES 加密的实例。
type desCipher struct {
	subkeys [16]uint64
}

// NewCipher 创建并返回一个新的 [cipher.Block]。
func NewCipher(key []byte) (cipher.Block, error) {
	if fips140only.Enforced() {
		return nil, errors.New("crypto/des: use of DES is not allowed in FIPS 140-only mode")
	}

	if len(key) != 8 {
		return nil, KeySizeError(len(key))
	}

	c := new(desCipher)
	c.generateSubkeys(key)
	return c, nil
}

func (c *desCipher) BlockSize() int { return BlockSize }

func (c *desCipher) Encrypt(dst, src []byte) {
	if len(src) < BlockSize {
		panic("crypto/des: input not full block")
	}
	if len(dst) < BlockSize {
		panic("crypto/des: output not full block")
	}
	if alias.InexactOverlap(dst[:BlockSize], src[:BlockSize]) {
		panic("crypto/des: invalid buffer overlap")
	}
	cryptBlock(c.subkeys[:], dst, src, false)
}

func (c *desCipher) Decrypt(dst, src []byte) {
	if len(src) < BlockSize {
		panic("crypto/des: input not full block")
	}
	if len(dst) < BlockSize {
		panic("crypto/des: output not full block")
	}
	if alias.InexactOverlap(dst[:BlockSize], src[:BlockSize]) {
		panic("crypto/des: invalid buffer overlap")
	}
	cryptBlock(c.subkeys[:], dst, src, true)
}

// tripleDESCipher 是 TripleDES 加密的实例。
type tripleDESCipher struct {
	cipher1, cipher2, cipher3 desCipher
}

// NewTripleDESCipher 创建并返回一个新的 [cipher.Block]。
func NewTripleDESCipher(key []byte) (cipher.Block, error) {
	if fips140only.Enforced() {
		return nil, errors.New("crypto/des: use of TripleDES is not allowed in FIPS 140-only mode")
	}

	if len(key) != 24 {
		return nil, KeySizeError(len(key))
	}

	c := new(tripleDESCipher)
	c.cipher1.generateSubkeys(key[:8])
	c.cipher2.generateSubkeys(key[8:16])
	c.cipher3.generateSubkeys(key[16:])
	return c, nil
}

func (c *tripleDESCipher) BlockSize() int { return BlockSize }

func (c *tripleDESCipher) Encrypt(dst, src []byte) {
	if len(src) < BlockSize {
		panic("crypto/des: input not full block")
	}
	if len(dst) < BlockSize {
		panic("crypto/des: output not full block")
	}
	if alias.InexactOverlap(dst[:BlockSize], src[:BlockSize]) {
		panic("crypto/des: invalid buffer overlap")
	}

	b := byteorder.BEUint64(src)
	b = permuteInitialBlock(b)
	left, right := uint32(b>>32), uint32(b)

	left = (left << 1) | (left >> 31)
	right = (right << 1) | (right >> 31)

	for i := 0; i < 8; i++ {
		left, right = feistel(left, right, c.cipher1.subkeys[2*i], c.cipher1.subkeys[2*i+1])
	}
	for i := 0; i < 8; i++ {
		right, left = feistel(right, left, c.cipher2.subkeys[15-2*i], c.cipher2.subkeys[15-(2*i+1)])
	}
	for i := 0; i < 8; i++ {
		left, right = feistel(left, right, c.cipher3.subkeys[2*i], c.cipher3.subkeys[2*i+1])
	}

	left = (left << 31) | (left >> 1)
	right = (right << 31) | (right >> 1)

	preOutput := (uint64(right) << 32) | uint64(left)
	byteorder.BEPutUint64(dst, permuteFinalBlock(preOutput))
}

func (c *tripleDESCipher) Decrypt(dst, src []byte) {
	if len(src) < BlockSize {
		panic("crypto/des: input not full block")
	}
	if len(dst) < BlockSize {
		panic("crypto/des: output not full block")
	}
	if alias.InexactOverlap(dst[:BlockSize], src[:BlockSize]) {
		panic("crypto/des: invalid buffer overlap")
	}

	b := byteorder.BEUint64(src)
	b = permuteInitialBlock(b)
	left, right := uint32(b>>32), uint32(b)

	left = (left << 1) | (left >> 31)
	right = (right << 1) | (right >> 31)

	for i := 0; i < 8; i++ {
		left, right = feistel(left, right, c.cipher3.subkeys[15-2*i], c.cipher3.subkeys[15-(2*i+1)])
	}
	for i := 0; i < 8; i++ {
		right, left = feistel(right, left, c.cipher2.subkeys[2*i], c.cipher2.subkeys[2*i+1])
	}
	for i := 0; i < 8; i++ {
		left, right = feistel(left, right, c.cipher1.subkeys[15-2*i], c.cipher1.subkeys[15-(2*i+1)])
	}

	left = (left << 31) | (left >> 1)
	right = (right << 31) | (right >> 1)

	preOutput := (uint64(right) << 32) | uint64(left)
	byteorder.BEPutUint64(dst, permuteFinalBlock(preOutput))
}
