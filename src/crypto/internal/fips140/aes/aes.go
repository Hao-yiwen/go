// 版权所有 2009 The Go Authors。保留所有权利。
// 本源代码的使用受 BSD 风格许可证约束，
// 该许可证可在 LICENSE 文件中找到。

package aes

import (
	"crypto/internal/fips140"
	"crypto/internal/fips140/alias"
	"strconv"
)

// BlockSize 是 AES 块的大小（以字节为单位）。
const BlockSize = 16

// Block 是使用特定密钥的 AES 实例。
// 它对并发使用是安全的。
type Block struct {
	block
}

// blockExpanded 是除了 s390x 外所有架构使用的块类型，
// s390x 将原始密钥直接传递给其指令。
type blockExpanded struct {
	rounds int
	// 轮密钥，其中仅使用前 (rounds + 1) × (128 ÷ 32) 个字。
	enc [60]uint32
	dec [60]uint32
}

const (
	// AES-128 有 128 位密钥、10 轮，使用 11 个 128 位轮密钥
	// (11×128÷32 = 44 个 32 位字)。

	// AES-192 有 192 位密钥、12 轮，使用 13 个 128 位轮密钥
	// (13×128÷32 = 52 个 32 位字)。

	// AES-256 有 256 位密钥、14 轮，使用 15 个 128 位轮密钥
	// (15×128÷32 = 60 个 32 位字)。

	aes128KeySize = 16
	aes192KeySize = 24
	aes256KeySize = 32

	aes128Rounds = 10
	aes192Rounds = 12
	aes256Rounds = 14
)

// roundKeysSize 返回 c.end 或 c.dec 中使用的 uint32 的数量。
func (b *blockExpanded) roundKeysSize() int {
	return (b.rounds + 1) * (128 / 32)
}

type KeySizeError int

func (k KeySizeError) Error() string {
	return "crypto/aes: invalid key size " + strconv.Itoa(int(k))
}

// New 创建并返回一个新的 [cipher.Block] 实现。
// key 参数应该是 AES 密钥，可以是 16、24 或 32 字节，分别对应
// AES-128、AES-192 或 AES-256。
func New(key []byte) (*Block, error) {
	// 此调用是大纲（outline）以允许分配在父堆栈上进行。
	return newOutlined(&Block{}, key)
}

// newOutlined 被标记为 go:noinline 以避免将其内联到 New 中，
// 这样会使 New 变得太复杂而无法内联。
//
//go:noinline
func newOutlined(b *Block, key []byte) (*Block, error) {
	switch len(key) {
	case aes128KeySize, aes192KeySize, aes256KeySize:
	default:
		return nil, KeySizeError(len(key))
	}
	return newBlock(b, key), nil
}

func newBlockExpanded(c *blockExpanded, key []byte) {
	switch len(key) {
	case aes128KeySize:
		c.rounds = aes128Rounds
	case aes192KeySize:
		c.rounds = aes192Rounds
	case aes256KeySize:
		c.rounds = aes256Rounds
	}
	expandKeyGeneric(c, key)
}

func (c *Block) BlockSize() int { return BlockSize }

func (c *Block) Encrypt(dst, src []byte) {
	// AES-ECB 在 FIPS 140-3 模式下不被批准。
	fips140.RecordNonApproved()
	if len(src) < BlockSize {
		panic("crypto/aes: input not full block")
	}
	if len(dst) < BlockSize {
		panic("crypto/aes: output not full block")
	}
	if alias.InexactOverlap(dst[:BlockSize], src[:BlockSize]) {
		panic("crypto/aes: invalid buffer overlap")
	}
	encryptBlock(c, dst, src)
}

func (c *Block) Decrypt(dst, src []byte) {
	// AES-ECB 在 FIPS 140-3 模式下不被批准。
	fips140.RecordNonApproved()
	if len(src) < BlockSize {
		panic("crypto/aes: input not full block")
	}
	if len(dst) < BlockSize {
		panic("crypto/aes: output not full block")
	}
	if alias.InexactOverlap(dst[:BlockSize], src[:BlockSize]) {
		panic("crypto/aes: invalid buffer overlap")
	}
	decryptBlock(c, dst, src)
}

// EncryptBlockInternal 对一个块应用 AES 加密函数。
//
// 这是一个仅供 gcm 包使用的内部函数。
func EncryptBlockInternal(c *Block, dst, src []byte) {
	encryptBlock(c, dst, src)
}
