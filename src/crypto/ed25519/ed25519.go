// 版权所有 2016 The Go Authors。保留所有权利。
// 本源代码的使用受 BSD 风格许可证约束，
// 该许可证可在 LICENSE 文件中找到。

// ed25519 包实现了 Ed25519 签名算法。参见
// https://ed25519.cr.yp.to/。
//
// 这些函数也与 RFC 8032 中定义的 "Ed25519" 函数兼容。
// 然而，与 RFC 8032 的表述不同，此包的私钥
// 表示包含公钥后缀，以使多个签名
// 使用同一密钥更高效。此包将 RFC
// 8032 私钥称为 "种子"。
//
// 涉及私钥的操作使用常数时间
// 算法实现。
package ed25519

import (
	"crypto"
	"crypto/internal/fips140/ed25519"
	"crypto/internal/fips140cache"
	"crypto/internal/fips140only"
	"crypto/internal/rand"
	cryptorand "crypto/rand"
	"crypto/subtle"
	"errors"
	"internal/godebug"
	"io"
	"strconv"
)

const (
	// PublicKeySize 是此包中使用的公钥的字节大小。
	PublicKeySize = 32
	// PrivateKeySize 是此包中使用的私钥的字节大小。
	PrivateKeySize = 64
	// SignatureSize 是此包生成和验证的签名的字节大小。
	SignatureSize = 64
	// SeedSize 是私钥种子的字节大小。这些是 RFC 8032 使用的私钥表示。
	SeedSize = 32
)

// PublicKey 是 Ed25519 公钥的类型。
type PublicKey []byte

// 在 PublicKey 上实现的任何方法可能也需要在
// PrivateKey 上实现，因为后者嵌入了前者并将公开其方法。

// Equal 报告 pub 和 x 是否具有相同的值。
func (pub PublicKey) Equal(x crypto.PublicKey) bool {
	xx, ok := x.(PublicKey)
	if !ok {
		return false
	}
	return subtle.ConstantTimeCompare(pub, xx) == 1
}

// PrivateKey 是 Ed25519 私钥的类型。它实现了 [crypto.Signer]。
type PrivateKey []byte

// Public 返回与 priv 对应的 [PublicKey]。
func (priv PrivateKey) Public() crypto.PublicKey {
	publicKey := make([]byte, PublicKeySize)
	copy(publicKey, priv[32:])
	return PublicKey(publicKey)
}

// Equal 报告 priv 和 x 是否具有相同的值。
func (priv PrivateKey) Equal(x crypto.PrivateKey) bool {
	xx, ok := x.(PrivateKey)
	if !ok {
		return false
	}
	return subtle.ConstantTimeCompare(priv, xx) == 1
}

// Seed 返回与 priv 对应的私钥种子。提供此方法是为了
// 与 RFC 8032 互操作。RFC 8032 的私钥对应于
// 此包中的种子。
func (priv PrivateKey) Seed() []byte {
	return append(make([]byte, 0, SeedSize), priv[:SeedSize]...)
}

// privateKeyCache 使用指向底层存储的第一个字节的指针作为
// 键，因为 [PrivateKey] 是按值传递的切片头。
var privateKeyCache fips140cache.Cache[byte, ed25519.PrivateKey]

// Sign 使用 priv 对给定的消息进行签名。rand 被忽略，可以为 nil。
//
// 如果 opts.HashFunc() 是 [crypto.SHA512]，则使用预哈希的变体 Ed25519ph
// 并期望消息是 SHA-512 哈希，否则 opts.HashFunc() 必须
// 是 [crypto.Hash](0) 并且消息不能被哈希，因为 Ed25519 对
// 要签名的消息执行两次传递。
//
// [Options] 类型的值可以用作 opts，或者可以直接使用 crypto.Hash(0) 或
// crypto.SHA512 来分别选择纯 Ed25519 或 Ed25519ph。
func (priv PrivateKey) Sign(rand io.Reader, message []byte, opts crypto.SignerOpts) (signature []byte, err error) {
	k, err := privateKeyCache.Get(&priv[0], func() (*ed25519.PrivateKey, error) {
		return ed25519.NewPrivateKey(priv)
	}, func(k *ed25519.PrivateKey) bool {
		return subtle.ConstantTimeCompare(priv, k.Bytes()) == 1
	})
	if err != nil {
		return nil, err
	}
	hash := opts.HashFunc()
	context := ""
	if opts, ok := opts.(*Options); ok {
		context = opts.Context
	}
	switch {
	case hash == crypto.SHA512: // Ed25519ph
		return ed25519.SignPH(k, message, context)
	case hash == crypto.Hash(0) && context != "": // Ed25519ctx
		if fips140only.Enforced() {
			return nil, errors.New("crypto/ed25519: use of Ed25519ctx is not allowed in FIPS 140-only mode")
		}
		return ed25519.SignCtx(k, message, context)
	case hash == crypto.Hash(0): // Ed25519
		return ed25519.Sign(k, message), nil
	default:
		return nil, errors.New("ed25519: expected opts.HashFunc() zero (unhashed message, for standard Ed25519) or SHA-512 (for Ed25519ph)")
	}
}

// Options 可与 [PrivateKey.Sign] 或 [VerifyWithOptions]
// 一起使用来选择 Ed25519 变体。
type Options struct {
	// Hash 对于常规 Ed25519 可以为零，或对于 Ed25519ph 为 crypto.SHA512。
	Hash crypto.Hash

	// Context（如果不为空）选择 Ed25519ctx 或为 Ed25519ph 提供上下文字符串。
	// 其长度最多为 255 个字节。
	Context string
}

// HashFunc returns o.Hash.
func (o *Options) HashFunc() crypto.Hash { return o.Hash }

var cryptocustomrand = godebug.New("cryptocustomrand")

// GenerateKey 使用来自 random 的熵生成公钥/私钥对。
//
// 如果 random 为 nil，则使用安全的随机源。(在 Go 1.26 之前，
// 如果应用程序设置了自定义 [crypto/rand.Reader]，则会使用它。
// 该行为可以通过 GODEBUG=cryptocustomrand=1 恢复。此设置将
// 在未来的 Go 版本中移除。改为使用 [testing/cryptotest.SetGlobalRandom]。)
//
// 此函数的输出是确定性的，等价于从 random 读取
// [SeedSize] 字节，并将它们传递给 [NewKeyFromSeed]。
func GenerateKey(random io.Reader) (PublicKey, PrivateKey, error) {
	if random == nil {
		if cryptocustomrand.Value() == "1" {
			random = cryptorand.Reader
			if !rand.IsDefaultReader(random) {
				cryptocustomrand.IncNonDefault()
			}
		} else {
			random = rand.Reader
		}
	}

	seed := make([]byte, SeedSize)
	if _, err := io.ReadFull(random, seed); err != nil {
		return nil, nil, err
	}

	privateKey := NewKeyFromSeed(seed)
	publicKey := privateKey.Public().(PublicKey)
	return publicKey, privateKey, nil
}

// NewKeyFromSeed 从种子计算私钥。如果
// len(seed) 不是 [SeedSize]，它将发生恐慌。提供此函数是为了与
// RFC 8032 互操作。RFC 8032 的私钥对应于此
// 包中的种子。
func NewKeyFromSeed(seed []byte) PrivateKey {
	// 概述函数体，以便返回的密钥可以是堆栈分配的。
	privateKey := make([]byte, PrivateKeySize)
	newKeyFromSeed(privateKey, seed)
	return privateKey
}

func newKeyFromSeed(privateKey, seed []byte) {
	k, err := ed25519.NewPrivateKeyFromSeed(seed)
	if err != nil {
		// NewPrivateKeyFromSeed 仅在种子长度不正确时返回错误。
		panic("ed25519: bad seed length: " + strconv.Itoa(len(seed)))
	}
	copy(privateKey, k.Bytes())
}

// Sign 使用 privateKey 对消息进行签名并返回签名。如果
// len(privateKey) 不是 [PrivateKeySize]，它将发生恐慌。
func Sign(privateKey PrivateKey, message []byte) []byte {
	// 概述函数体，以便返回的签名可以是
	// 堆栈分配的。
	signature := make([]byte, SignatureSize)
	sign(signature, privateKey, message)
	return signature
}

func sign(signature []byte, privateKey PrivateKey, message []byte) {
	k, err := privateKeyCache.Get(&privateKey[0], func() (*ed25519.PrivateKey, error) {
		return ed25519.NewPrivateKey(privateKey)
	}, func(k *ed25519.PrivateKey) bool {
		return subtle.ConstantTimeCompare(privateKey, k.Bytes()) == 1
	})
	if err != nil {
		panic("ed25519: bad private key: " + err.Error())
	}
	sig := ed25519.Sign(k, message)
	copy(signature, sig)
}

// Verify 报告 sig 是否是 publicKey 对消息的有效签名。如果
// len(publicKey) 不是 [PublicKeySize]，它将发生恐慌。
//
// 输入不被视为机密，可能会泄露通过计时侧
// 信道，或如果攻击者控制了部分输入。
func Verify(publicKey PublicKey, message, sig []byte) bool {
	return VerifyWithOptions(publicKey, message, sig, &Options{Hash: crypto.Hash(0)}) == nil
}

// VerifyWithOptions 报告 sig 是否是 publicKey 对消息的有效签名。
// 有效签名通过返回 nil 错误来指示。如果
// len(publicKey) 不是 [PublicKeySize]，它将发生恐慌。
//
// 如果 opts.Hash 是 [crypto.SHA512]，则使用预哈希的变体 Ed25519ph，
// 并期望消息是 SHA-512 哈希，否则 opts.Hash 必须是
// [crypto.Hash](0) 并且消息不能被哈希，因为 Ed25519 对
// 要签名的消息执行两次传递。
//
// 输入不被视为机密，可能会泄露通过计时侧
// 信道，或如果攻击者控制了部分输入。
func VerifyWithOptions(publicKey PublicKey, message, sig []byte, opts *Options) error {
	if l := len(publicKey); l != PublicKeySize {
		panic("ed25519: bad public key length: " + strconv.Itoa(l))
	}
	k, err := ed25519.NewPublicKey(publicKey)
	if err != nil {
		return err
	}
	switch {
	case opts.Hash == crypto.SHA512: // Ed25519ph
		return ed25519.VerifyPH(k, message, sig, opts.Context)
	case opts.Hash == crypto.Hash(0) && opts.Context != "": // Ed25519ctx
		if fips140only.Enforced() {
			return errors.New("crypto/ed25519: use of Ed25519ctx is not allowed in FIPS 140-only mode")
		}
		return ed25519.VerifyCtx(k, message, sig, opts.Context)
	case opts.Hash == crypto.Hash(0): // Ed25519
		return ed25519.Verify(k, message, sig)
	default:
		return errors.New("ed25519: expected opts.Hash zero (unhashed message, for standard Ed25519) or SHA-512 (for Ed25519ph)")
	}
}
