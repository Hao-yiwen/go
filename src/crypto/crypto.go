// 版权所有 2011 The Go Authors。保留所有权利。
// 本源代码的使用受 BSD 风格许可证约束，
// 该许可证可在 LICENSE 文件中找到。

// crypto 包汇集了常用的加密常量。
package crypto

import (
	"hash"
	"io"
	"strconv"
)

// Hash 标识在其他包中实现的加密哈希函数。
type Hash uint

// HashFunc 简单地返回 h 的值，使得 [Hash] 实现 [SignerOpts] 接口。
func (h Hash) HashFunc() Hash {
	return h
}

func (h Hash) String() string {
	switch h {
	case MD4:
		return "MD4"
	case MD5:
		return "MD5"
	case SHA1:
		return "SHA-1"
	case SHA224:
		return "SHA-224"
	case SHA256:
		return "SHA-256"
	case SHA384:
		return "SHA-384"
	case SHA512:
		return "SHA-512"
	case MD5SHA1:
		return "MD5+SHA1"
	case RIPEMD160:
		return "RIPEMD-160"
	case SHA3_224:
		return "SHA3-224"
	case SHA3_256:
		return "SHA3-256"
	case SHA3_384:
		return "SHA3-384"
	case SHA3_512:
		return "SHA3-512"
	case SHA512_224:
		return "SHA-512/224"
	case SHA512_256:
		return "SHA-512/256"
	case BLAKE2s_256:
		return "BLAKE2s-256"
	case BLAKE2b_256:
		return "BLAKE2b-256"
	case BLAKE2b_384:
		return "BLAKE2b-384"
	case BLAKE2b_512:
		return "BLAKE2b-512"
	default:
		return "unknown hash value " + strconv.Itoa(int(h))
	}
}

const (
	MD4         Hash = 1 + iota // 导入 golang.org/x/crypto/md4
	MD5                         // 导入 crypto/md5
	SHA1                        // 导入 crypto/sha1
	SHA224                      // 导入 crypto/sha256
	SHA256                      // 导入 crypto/sha256
	SHA384                      // 导入 crypto/sha512
	SHA512                      // 导入 crypto/sha512
	MD5SHA1                     // 无实现；MD5+SHA1 用于 TLS RSA
	RIPEMD160                   // 导入 golang.org/x/crypto/ripemd160
	SHA3_224                    // 导入 crypto/sha3
	SHA3_256                    // 导入 crypto/sha3
	SHA3_384                    // 导入 crypto/sha3
	SHA3_512                    // 导入 crypto/sha3
	SHA512_224                  // 导入 crypto/sha512
	SHA512_256                  // 导入 crypto/sha512
	BLAKE2s_256                 // 导入 golang.org/x/crypto/blake2s
	BLAKE2b_256                 // 导入 golang.org/x/crypto/blake2b
	BLAKE2b_384                 // 导入 golang.org/x/crypto/blake2b
	BLAKE2b_512                 // 导入 golang.org/x/crypto/blake2b
	maxHash
)

var digestSizes = []uint8{
	MD4:         16,
	MD5:         16,
	SHA1:        20,
	SHA224:      28,
	SHA256:      32,
	SHA384:      48,
	SHA512:      64,
	SHA512_224:  28,
	SHA512_256:  32,
	SHA3_224:    28,
	SHA3_256:    32,
	SHA3_384:    48,
	SHA3_512:    64,
	MD5SHA1:     36,
	RIPEMD160:   20,
	BLAKE2s_256: 32,
	BLAKE2b_256: 32,
	BLAKE2b_384: 48,
	BLAKE2b_512: 64,
}

// Size 返回给定哈希函数生成的摘要的字节长度。
// 它不要求相关的哈希函数被链接到程序中。
func (h Hash) Size() int {
	if h > 0 && h < maxHash {
		return int(digestSizes[h])
	}
	panic("crypto: Size of unknown hash function")
}

var hashes = make([]func() hash.Hash, maxHash)

// New 返回一个新的 hash.Hash，用于计算给定的哈希函数。
// 如果哈希函数未链接到二进制文件中，New 会触发 panic。
func (h Hash) New() hash.Hash {
	if h > 0 && h < maxHash {
		f := hashes[h]
		if f != nil {
			return f()
		}
	}
	panic("crypto: requested hash function #" + strconv.Itoa(int(h)) + " is unavailable")
}

// Available 报告给定的哈希函数是否已链接到二进制文件中。
func (h Hash) Available() bool {
	return h < maxHash && hashes[h] != nil
}

// RegisterHash 注册一个返回给定哈希函数新实例的函数。
// 这旨在从实现哈希函数的包的 init 函数中调用。
func RegisterHash(h Hash, f func() hash.Hash) {
	if h >= maxHash {
		panic("crypto: RegisterHash of unknown hash function")
	}
	hashes[h] = f
}

// PublicKey 表示使用未指定算法的公钥。
//
// 尽管出于向后兼容的原因，此类型是一个空接口，
// 但标准库中的所有公钥类型都实现了以下接口
//
//	interface{
//	    Equal(x crypto.PublicKey) bool
//	}
//
// 这可用于在应用程序中增强类型安全性。
type PublicKey any

// PrivateKey 表示使用未指定算法的私钥。
//
// 尽管出于向后兼容的原因，此类型是一个空接口，
// 但标准库中的所有私钥类型都实现了以下接口
//
//	interface{
//	    Public() crypto.PublicKey
//	    Equal(x crypto.PrivateKey) bool
//	}
//
// 以及特定用途的接口，如 [Signer] 和 [Decrypter]，
// 这些可用于在应用程序中增强类型安全性。
type PrivateKey any

// Signer 是一个接口，用于可进行签名操作的不透明私钥。
// 例如，保存在硬件模块中的 RSA 密钥。
type Signer interface {
	// Public 返回与不透明私钥对应的公钥。
	Public() PublicKey

	// Sign 使用私钥对摘要进行签名，可能会使用来自 rand 的熵。
	// 对于 RSA 密钥，生成的签名应该是 PKCS #1 v1.5 或 PSS 签名
	// （由 opts 指示）。对于 (EC)DSA 密钥，它应该是 DER 序列化的
	// ASN.1 签名结构。
	//
	// Hash 实现了 SignerOpts 接口，在大多数情况下，可以简单地
	// 将使用的哈希函数作为 opts 传入。Sign 也可能尝试将 opts
	// 类型断言为其他类型，以获取特定于算法的值。
	// 详情请参阅每个包中的文档。
	//
	// 注意，当需要对较大消息的哈希进行签名时，
	// 调用者负责对较大消息进行哈希，并将哈希值（作为 digest）
	// 和哈希函数（作为 opts）传递给 Sign。
	Sign(rand io.Reader, digest []byte, opts SignerOpts) (signature []byte, err error)
}

// MessageSigner 是一个接口，用于可进行签名操作的不透明私钥，
// 其中消息不由调用者预先哈希。它是 Signer 接口的超集，
// 因此可以传递给接受 Signer 的 API，这些 API 可能会尝试进行接口升级。
//
// 给定相同的 opts，MessageSigner.SignMessage 和 MessageSigner.Sign
// 应该产生相同的结果。特别是，只有当 Signer 也接受未预先哈希的消息时，
// MessageSigner.SignMessage 才应接受零值 opts.HashFunc。
//
// 不提供预哈希 Sign API 的实现应该通过始终返回错误来实现 Signer.Sign。
type MessageSigner interface {
	Signer
	SignMessage(rand io.Reader, msg []byte, opts SignerOpts) (signature []byte, err error)
}

// SignerOpts 包含使用 [Signer] 进行签名的选项。
type SignerOpts interface {
	// HashFunc 返回用于生成传递给 Signer.Sign 的消息的哈希函数标识符，
	// 如果没有进行哈希，则返回零。
	HashFunc() Hash
}

// Decrypter 是一个接口，用于可进行非对称解密操作的不透明私钥。
// 例如，保存在硬件模块中的 RSA 密钥。
type Decrypter interface {
	// Public 返回与不透明私钥对应的公钥。
	Public() PublicKey

	// Decrypt 解密 msg。opts 参数应该适合所使用的原语。
	// 详情请参阅每个实现中的文档。
	Decrypt(rand io.Reader, msg []byte, opts DecrypterOpts) (plaintext []byte, err error)
}

type DecrypterOpts any

// SignMessage 使用 signer 对 msg 进行签名。如果 signer 实现了 [MessageSigner]，
// 则直接调用 [MessageSigner.SignMessage]。否则，使用 opts.HashFunc() 对 msg
// 进行哈希，然后使用 [Signer.Sign] 进行签名。
func SignMessage(signer Signer, rand io.Reader, msg []byte, opts SignerOpts) (signature []byte, err error) {
	if ms, ok := signer.(MessageSigner); ok {
		return ms.SignMessage(rand, msg, opts)
	}
	if opts.HashFunc() != 0 {
		h := opts.HashFunc().New()
		h.Write(msg)
		msg = h.Sum(nil)
	}
	return signer.Sign(rand, msg, opts)
}

// Decapsulator 是一个接口，用于可进行解封装操作的不透明 KEM 私钥。
// 例如，保存在硬件模块中的 ML-KEM 密钥。
//
// 例如，它由 [crypto/mlkem.DecapsulationKey768] 实现。
type Decapsulator interface {
	Encapsulator() Encapsulator
	Decapsulate(ciphertext []byte) (sharedKey []byte, err error)
}

// Encapsulator 是一个接口，用于可进行封装操作的 KEM 公钥。
//
// 例如，它由 [crypto/mlkem.EncapsulationKey768] 实现。
type Encapsulator interface {
	Bytes() []byte
	Encapsulate() (sharedKey, ciphertext []byte)
}
