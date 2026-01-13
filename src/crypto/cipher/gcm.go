// 版权所有 2024 The Go Authors。保留所有权利。
// 本源代码的使用受 BSD 风格许可证约束，
// 该许可证可在 LICENSE 文件中找到。

package cipher

import (
	"crypto/internal/fips140/aes"
	"crypto/internal/fips140/aes/gcm"
	"crypto/internal/fips140/alias"
	"crypto/internal/fips140only"
	"crypto/subtle"
	"errors"
	"internal/byteorder"
)

const (
	gcmBlockSize         = 16
	gcmStandardNonceSize = 12
	gcmTagSize           = 16
	gcmMinimumTagSize    = 12 // NIST SP 800-38D 建议使用 12 字节或更多字节的标签。
)

// NewGCM 返回给定的 128 位块密码，包装在具有标准随机数长度的
// 伽罗瓦计数器模式中。
//
// 一般来说，此 GCM 实现执行的 GHASH 操作不是常量时间的。
// 例外情况是当底层 [Block] 是在具有 AES 硬件支持的系统上
// 由 aes.NewCipher 创建时。详情请参阅 [crypto/aes] 包文档。
func NewGCM(cipher Block) (AEAD, error) {
	if fips140only.Enforced() {
		return nil, errors.New("crypto/cipher: use of GCM with arbitrary IVs is not allowed in FIPS 140-only mode, use NewGCMWithRandomNonce")
	}
	return newGCM(cipher, gcmStandardNonceSize, gcmTagSize)
}

// NewGCMWithNonceSize 返回给定的 128 位块密码，包装在伽罗瓦计数器模式中，
// 接受给定长度的随机数。长度不能为零。
//
// 仅当您需要与使用非标准随机数长度的现有加密系统兼容时才使用此函数。
// 所有其他用户应使用 [NewGCM]，它更快且更能抵抗误用。
func NewGCMWithNonceSize(cipher Block, size int) (AEAD, error) {
	if fips140only.Enforced() {
		return nil, errors.New("crypto/cipher: use of GCM with arbitrary IVs is not allowed in FIPS 140-only mode, use NewGCMWithRandomNonce")
	}
	return newGCM(cipher, size, gcmTagSize)
}

// NewGCMWithTagSize 返回给定的 128 位块密码，包装在伽罗瓦计数器模式中，
// 生成给定长度的标签。
//
// 允许的标签大小在 12 到 16 字节之间。
//
// 仅当您需要与使用非标准标签长度的现有加密系统兼容时才使用此函数。
// 所有其他用户应使用 [NewGCM]，它更能抵抗误用。
func NewGCMWithTagSize(cipher Block, tagSize int) (AEAD, error) {
	if fips140only.Enforced() {
		return nil, errors.New("crypto/cipher: use of GCM with arbitrary IVs is not allowed in FIPS 140-only mode, use NewGCMWithRandomNonce")
	}
	return newGCM(cipher, gcmStandardNonceSize, tagSize)
}

func newGCM(cipher Block, nonceSize, tagSize int) (AEAD, error) {
	c, ok := cipher.(*aes.Block)
	if !ok {
		if fips140only.Enforced() {
			return nil, errors.New("crypto/cipher: use of GCM with non-AES ciphers is not allowed in FIPS 140-only mode")
		}
		return newGCMFallback(cipher, nonceSize, tagSize)
	}
	// 我们不直接返回 gcm.New，因为即使 *gcm.GCM 为 nil，
	// 它也总是返回类型为 *gcm.GCM 的非 nil AEAD 接口值。
	g, err := gcm.New(c, nonceSize, tagSize)
	if err != nil {
		return nil, err
	}
	return g, nil
}

// NewGCMWithRandomNonce 返回包装在伽罗瓦计数器模式中的给定密码，
// 使用随机生成的随机数。密码必须由 [crypto/aes.NewCipher] 创建。
//
// 它生成一个随机的 96 位随机数，由 Seal 预置到密文中，
// 并由 Open 从密文中提取。AEAD 的 NonceSize 为零，
// 而 Overhead 为 28 字节（随机数大小和标签大小的组合）。
//
// 给定的密钥不得用于加密超过 2^32 条消息，以将随机数
// 冲突的风险限制在可忽略的水平。
func NewGCMWithRandomNonce(cipher Block) (AEAD, error) {
	c, ok := cipher.(*aes.Block)
	if !ok {
		return nil, errors.New("cipher: NewGCMWithRandomNonce requires aes.Block")
	}
	g, err := gcm.New(c, gcmStandardNonceSize, gcmTagSize)
	if err != nil {
		return nil, err
	}
	return gcmWithRandomNonce{g}, nil
}

type gcmWithRandomNonce struct {
	*gcm.GCM
}

func (g gcmWithRandomNonce) NonceSize() int {
	return 0
}

func (g gcmWithRandomNonce) Overhead() int {
	return gcmStandardNonceSize + gcmTagSize
}

func (g gcmWithRandomNonce) Seal(dst, nonce, plaintext, additionalData []byte) []byte {
	if len(nonce) != 0 {
		panic("crypto/cipher: non-empty nonce passed to GCMWithRandomNonce")
	}

	ret, out := sliceForAppend(dst, gcmStandardNonceSize+len(plaintext)+gcmTagSize)
	if alias.InexactOverlap(out, plaintext) {
		panic("crypto/cipher: invalid buffer overlap of output and input")
	}
	if alias.AnyOverlap(out, additionalData) {
		panic("crypto/cipher: invalid buffer overlap of output and additional data")
	}
	nonce = out[:gcmStandardNonceSize]
	ciphertext := out[gcmStandardNonceSize:]

	// AEAD 接口允许使用 plaintext[:0] 或 ciphertext[:0] 作为 dst。
	//
	// 当尝试预置或修剪随机数时，这是个问题，因为实际的 AES-GCTR 块
	// 最终会重叠但不完全重叠。
	//
	// 在 Open 中，我们在输入*之前*写入输出，所以除非我们做一些奇怪的事情，
	// 比如反向处理块，否则它会正常工作。
	//
	// 在 Seal 中，我们可以反向处理输入，或者在写入之前故意预先加载。
	//
	// 然而，crypto/internal/fips140/aes/gcm API 也检查精确重叠，
	// 所以目前如果我们检测到重叠，就只做一个 memmove。
	//
	//     ┌───────────────────────────┬ ─ ─
	//     │PPPPPPPPPPPPPPPPPPPPPPPPPPP│    │
	//     └▽─────────────────────────▲┴ ─ ─
	//       ╲ Seal                    ╲
	//        ╲                    Open ╲
	//     ┌───▼─────────────────────────△──┐
	//     │NN|CCCCCCCCCCCCCCCCCCCCCCCCCCC|T│
	//     └────────────────────────────────┘
	//
	if alias.AnyOverlap(out, plaintext) {
		copy(ciphertext, plaintext)
		plaintext = ciphertext[:len(plaintext)]
	}

	gcm.SealWithRandomNonce(g.GCM, nonce, ciphertext, plaintext, additionalData)
	return ret
}

func (g gcmWithRandomNonce) Open(dst, nonce, ciphertext, additionalData []byte) ([]byte, error) {
	if len(nonce) != 0 {
		panic("crypto/cipher: non-empty nonce passed to GCMWithRandomNonce")
	}
	if len(ciphertext) < gcmStandardNonceSize+gcmTagSize {
		return nil, errOpen
	}

	ret, out := sliceForAppend(dst, len(ciphertext)-gcmStandardNonceSize-gcmTagSize)
	if alias.InexactOverlap(out, ciphertext) {
		panic("crypto/cipher: invalid buffer overlap of output and input")
	}
	if alias.AnyOverlap(out, additionalData) {
		panic("crypto/cipher: invalid buffer overlap of output and additional data")
	}
	// 参见 Seal 中的讨论。注意，如果此时有任何重叠，那是因为 out = ciphertext，
	// 所以即使我们切掉了标签，out 也必须有足够的容量。还要注意 [AEAD] 如何指定
	// "dst 的内容，直到其容量，可能会被覆盖"。
	if alias.AnyOverlap(out, ciphertext) {
		nonce = make([]byte, gcmStandardNonceSize)
		copy(nonce, ciphertext)
		copy(out[:len(ciphertext)], ciphertext[gcmStandardNonceSize:])
		ciphertext = out[:len(ciphertext)-gcmStandardNonceSize]
	} else {
		nonce = ciphertext[:gcmStandardNonceSize]
		ciphertext = ciphertext[gcmStandardNonceSize:]
	}

	_, err := g.GCM.Open(out[:0], nonce, ciphertext, additionalData)
	if err != nil {
		return nil, err
	}
	return ret, nil
}

// gcmAble 是一个由具有 GCM 特定优化实现的密码实现的接口。
// crypto/aes 不再使用它，我们最终希望将其移除。
type gcmAble interface {
	NewGCM(nonceSize, tagSize int) (AEAD, error)
}

func newGCMFallback(cipher Block, nonceSize, tagSize int) (AEAD, error) {
	if tagSize < gcmMinimumTagSize || tagSize > gcmBlockSize {
		return nil, errors.New("cipher: incorrect tag size given to GCM")
	}
	if nonceSize <= 0 {
		return nil, errors.New("cipher: the nonce can't have zero length")
	}
	if cipher, ok := cipher.(gcmAble); ok {
		return cipher.NewGCM(nonceSize, tagSize)
	}
	if cipher.BlockSize() != gcmBlockSize {
		return nil, errors.New("cipher: NewGCM requires 128-bit block cipher")
	}
	return &gcmFallback{cipher: cipher, nonceSize: nonceSize, tagSize: tagSize}, nil
}

// gcmFallback 仅用于非 AES 密码，遗憾的是我们在理论上支持它。
// 它是 crypto/internal/fips140/aes/gcm/gcm_generic.go 中
// 通用实现的副本，详情请参阅该文件。
type gcmFallback struct {
	cipher    Block
	nonceSize int
	tagSize   int
}

func (g *gcmFallback) NonceSize() int {
	return g.nonceSize
}

func (g *gcmFallback) Overhead() int {
	return g.tagSize
}

func (g *gcmFallback) Seal(dst, nonce, plaintext, additionalData []byte) []byte {
	if len(nonce) != g.nonceSize {
		panic("crypto/cipher: incorrect nonce length given to GCM")
	}
	if g.nonceSize == 0 {
		panic("crypto/cipher: incorrect GCM nonce size")
	}
	if uint64(len(plaintext)) > uint64((1<<32)-2)*gcmBlockSize {
		panic("crypto/cipher: message too large for GCM")
	}

	ret, out := sliceForAppend(dst, len(plaintext)+g.tagSize)
	if alias.InexactOverlap(out, plaintext) {
		panic("crypto/cipher: invalid buffer overlap of output and input")
	}
	if alias.AnyOverlap(out, additionalData) {
		panic("crypto/cipher: invalid buffer overlap of output and additional data")
	}

	var H, counter, tagMask [gcmBlockSize]byte
	g.cipher.Encrypt(H[:], H[:])
	deriveCounter(&H, &counter, nonce)
	gcmCounterCryptGeneric(g.cipher, tagMask[:], tagMask[:], &counter)

	gcmCounterCryptGeneric(g.cipher, out, plaintext, &counter)

	var tag [gcmTagSize]byte
	gcmAuth(tag[:], &H, &tagMask, out[:len(plaintext)], additionalData)
	copy(out[len(plaintext):], tag[:])

	return ret
}

var errOpen = errors.New("cipher: message authentication failed")

func (g *gcmFallback) Open(dst, nonce, ciphertext, additionalData []byte) ([]byte, error) {
	if len(nonce) != g.nonceSize {
		panic("crypto/cipher: incorrect nonce length given to GCM")
	}
	if g.tagSize < gcmMinimumTagSize {
		panic("crypto/cipher: incorrect GCM tag size")
	}

	if len(ciphertext) < g.tagSize {
		return nil, errOpen
	}
	if uint64(len(ciphertext)) > uint64((1<<32)-2)*gcmBlockSize+uint64(g.tagSize) {
		return nil, errOpen
	}

	ret, out := sliceForAppend(dst, len(ciphertext)-g.tagSize)
	if alias.InexactOverlap(out, ciphertext) {
		panic("crypto/cipher: invalid buffer overlap of output and input")
	}
	if alias.AnyOverlap(out, additionalData) {
		panic("crypto/cipher: invalid buffer overlap of output and additional data")
	}

	var H, counter, tagMask [gcmBlockSize]byte
	g.cipher.Encrypt(H[:], H[:])
	deriveCounter(&H, &counter, nonce)
	gcmCounterCryptGeneric(g.cipher, tagMask[:], tagMask[:], &counter)

	tag := ciphertext[len(ciphertext)-g.tagSize:]
	ciphertext = ciphertext[:len(ciphertext)-g.tagSize]

	var expectedTag [gcmTagSize]byte
	gcmAuth(expectedTag[:], &H, &tagMask, ciphertext, additionalData)
	if subtle.ConstantTimeCompare(expectedTag[:g.tagSize], tag) != 1 {
		// 我们有时会同时进行解密和认证，所以在标签不匹配时会覆盖 dst。
		// 为了在各平台上保持一致，并避免释放未经认证的明文，
		// 我们在出错时清除缓冲区。
		clear(out)
		return nil, errOpen
	}

	gcmCounterCryptGeneric(g.cipher, out, ciphertext, &counter)

	return ret, nil
}

func deriveCounter(H, counter *[gcmBlockSize]byte, nonce []byte) {
	if len(nonce) == gcmStandardNonceSize {
		copy(counter[:], nonce)
		counter[gcmBlockSize-1] = 1
	} else {
		lenBlock := make([]byte, 16)
		byteorder.BEPutUint64(lenBlock[8:], uint64(len(nonce))*8)
		J := gcm.GHASH(H, nonce, lenBlock)
		copy(counter[:], J)
	}
}

func gcmCounterCryptGeneric(b Block, out, src []byte, counter *[gcmBlockSize]byte) {
	var mask [gcmBlockSize]byte
	for len(src) >= gcmBlockSize {
		b.Encrypt(mask[:], counter[:])
		gcmInc32(counter)

		subtle.XORBytes(out, src, mask[:])
		out = out[gcmBlockSize:]
		src = src[gcmBlockSize:]
	}
	if len(src) > 0 {
		b.Encrypt(mask[:], counter[:])
		gcmInc32(counter)
		subtle.XORBytes(out, src, mask[:])
	}
}

func gcmInc32(counterBlock *[gcmBlockSize]byte) {
	ctr := counterBlock[len(counterBlock)-4:]
	byteorder.BEPutUint32(ctr, byteorder.BEUint32(ctr)+1)
}

func gcmAuth(out []byte, H, tagMask *[gcmBlockSize]byte, ciphertext, additionalData []byte) {
	lenBlock := make([]byte, 16)
	byteorder.BEPutUint64(lenBlock[:8], uint64(len(additionalData))*8)
	byteorder.BEPutUint64(lenBlock[8:], uint64(len(ciphertext))*8)
	S := gcm.GHASH(H, additionalData, ciphertext, lenBlock)
	subtle.XORBytes(out, S, tagMask[:])
}

// sliceForAppend 接受一个切片和请求的字节数。它返回一个切片，
// 其内容是给定切片的内容后跟那么多字节，以及第二个切片，
// 它是对第一个切片的别名，只包含额外的字节。
// 如果原始切片有足够的容量，则不执行分配。
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
