// 版权所有 2011 The Go Authors。保留所有权利。
// 本源代码的使用受 BSD 风格许可证约束，
// 该许可证可在 LICENSE 文件中找到。

// ecdsa 包实现了椭圆曲线数字签名算法，
// 如 [FIPS 186-5] 中所定义。
//
// 此包生成的签名不是确定性的，但熵与私钥和消息混合，
// 在随机源失败的情况下实现相同级别的安全性。
//
// 只要使用 [elliptic.P224]、[elliptic.P256]、[elliptic.P384] 或
// [elliptic.P521] 返回的 [elliptic.Curve]，涉及私钥的操作
// 就使用常量时间算法实现。
//
// [FIPS 186-5]: https://nvlpubs.nist.gov/nistpubs/FIPS/NIST.FIPS.186-5.pdf
package ecdsa

import (
	"crypto"
	"crypto/ecdh"
	"crypto/elliptic"
	"crypto/internal/boring"
	"crypto/internal/boring/bbig"
	"crypto/internal/fips140/ecdsa"
	"crypto/internal/fips140/nistec"
	"crypto/internal/fips140cache"
	"crypto/internal/fips140hash"
	"crypto/internal/fips140only"
	"crypto/internal/rand"
	"crypto/sha512"
	"crypto/subtle"
	"errors"
	"io"
	"math/big"

	"golang.org/x/crypto/cryptobyte"
	"golang.org/x/crypto/cryptobyte/asn1"
)

// PublicKey 表示 ECDSA 公钥。
type PublicKey struct {
	elliptic.Curve

	// X, Y 是公钥点的坐标。
	//
	// 已弃用：修改原始坐标可能产生无效的密钥，并可能使内部优化失效；
	// 此外，[big.Int] 方法不适合用于操作加密值。要编码和解码
	// PublicKey 值，请使用 [PublicKey.Bytes] 和 [ParseUncompressedPublicKey]
	// 或 [crypto/x509.MarshalPKIXPublicKey] 和 [crypto/x509.ParsePKIXPublicKey]。
	// 对于 ECDH，请使用 [crypto/ecdh]。对于底层椭圆曲线操作，
	// 请使用第三方模块如 filippo.io/nistec。
	X, Y *big.Int
}

// 在 PublicKey 上实现的任何方法可能也需要在 PrivateKey 上实现，
// 因为后者嵌入了前者并将暴露其方法。

// ECDH 将 k 作为 [ecdh.PublicKey] 返回。如果密钥根据
// [ecdh.Curve.NewPublicKey] 的定义无效，或者 Curve 不受
// crypto/ecdh 支持，则返回错误。
func (pub *PublicKey) ECDH() (*ecdh.PublicKey, error) {
	c := curveToECDH(pub.Curve)
	if c == nil {
		return nil, errors.New("ecdsa: unsupported curve by crypto/ecdh")
	}
	k, err := pub.Bytes()
	if err != nil {
		return nil, err
	}
	return c.NewPublicKey(k)
}

// Equal 报告 pub 和 x 是否具有相同的值。
//
// 只有当两个密钥具有相同的 Curve 值时，才认为它们具有相同的值。
// 请注意，例如 [elliptic.P256] 和 elliptic.P256().Params() 是不同的值，
// 因为后者是通用的非常量时间实现。
func (pub *PublicKey) Equal(x crypto.PublicKey) bool {
	xx, ok := x.(*PublicKey)
	if !ok {
		return false
	}
	return bigIntEqual(pub.X, xx.X) && bigIntEqual(pub.Y, xx.Y) &&
		// 标准库的 Curve 实现是单例的，所以此检查对它们有效。
		// 其他 Curves 即使不是单例也可能是等价的，但没有确定的方法来检查，
		// 宁可安全起见。
		pub.Curve == xx.Curve
}

// ParseUncompressedPublicKey 解析按照 SEC 1 版本 2.0 第 2.3.3 节
// 编码为未压缩点的公钥（也称为 X9.62 未压缩格式）。
// 如果点不是未压缩形式、不在曲线上或是无穷远点，则返回错误。
//
// curve 必须是 [elliptic.P224]、[elliptic.P256]、[elliptic.P384] 或
// [elliptic.P521] 之一，否则 ParseUncompressedPublicKey 返回错误。
//
// ParseUncompressedPublicKey 接受与 [ecdh.Curve.NewPublicKey] 对
// NIST 曲线相同的格式，但返回 [PublicKey] 而不是 [ecdh.PublicKey]。
//
// 请注意，公钥更常见的编码格式是 DER（或 PEM）格式，
// 可以使用 [crypto/x509.ParsePKIXPublicKey]（和 [encoding/pem]）解析。
func ParseUncompressedPublicKey(curve elliptic.Curve, data []byte) (*PublicKey, error) {
	if len(data) < 1 || data[0] != 4 {
		return nil, errors.New("ecdsa: invalid uncompressed public key")
	}
	switch curve {
	case elliptic.P224():
		return parseUncompressedPublicKey(ecdsa.P224(), curve, data)
	case elliptic.P256():
		return parseUncompressedPublicKey(ecdsa.P256(), curve, data)
	case elliptic.P384():
		return parseUncompressedPublicKey(ecdsa.P384(), curve, data)
	case elliptic.P521():
		return parseUncompressedPublicKey(ecdsa.P521(), curve, data)
	default:
		return nil, errors.New("ecdsa: curve not supported by ParseUncompressedPublicKey")
	}
}

func parseUncompressedPublicKey[P ecdsa.Point[P]](c *ecdsa.Curve[P], curve elliptic.Curve, data []byte) (*PublicKey, error) {
	k, err := ecdsa.NewPublicKey(c, data)
	if err != nil {
		return nil, err
	}
	return publicKeyFromFIPS(curve, k)
}

// Bytes encodes the public key as an uncompressed point according to SEC 1,
// Version 2.0, Section 2.3.3 (also known as the X9.62 uncompressed format).
// It returns an error if the public key is invalid.
//
// PublicKey.Curve must be one of [elliptic.P224], [elliptic.P256],
// [elliptic.P384], or [elliptic.P521], or Bytes returns an error.
//
// Bytes returns the same format as [ecdh.PublicKey.Bytes] does for NIST curves.
//
// Note that public keys are more commonly encoded in DER (or PEM) format, which
// can be generated with [crypto/x509.MarshalPKIXPublicKey] (and [encoding/pem]).
func (pub *PublicKey) Bytes() ([]byte, error) {
	switch pub.Curve {
	case elliptic.P224():
		return publicKeyBytes(ecdsa.P224(), pub)
	case elliptic.P256():
		return publicKeyBytes(ecdsa.P256(), pub)
	case elliptic.P384():
		return publicKeyBytes(ecdsa.P384(), pub)
	case elliptic.P521():
		return publicKeyBytes(ecdsa.P521(), pub)
	default:
		return nil, errors.New("ecdsa: curve not supported by PublicKey.Bytes")
	}
}

func publicKeyBytes[P ecdsa.Point[P]](c *ecdsa.Curve[P], pub *PublicKey) ([]byte, error) {
	k, err := publicKeyToFIPS(c, pub)
	if err != nil {
		return nil, err
	}
	return k.Bytes(), nil
}

// PrivateKey represents an ECDSA private key.
type PrivateKey struct {
	PublicKey

	// D is the private scalar value.
	//
	// Deprecated: modifying the raw value can produce invalid keys, and may
	// invalidate internal optimizations; moreover, [big.Int] methods are not
	// suitable for operating on cryptographic values. To encode and decode
	// PrivateKey values, use [PrivateKey.Bytes] and [ParseRawPrivateKey] or
	// [crypto/x509.MarshalPKCS8PrivateKey] and [crypto/x509.ParsePKCS8PrivateKey].
	// For ECDH, use [crypto/ecdh].
	D *big.Int
}

// ECDH returns k as a [ecdh.PrivateKey]. It returns an error if the key is
// invalid according to the definition of [ecdh.Curve.NewPrivateKey], or if the
// Curve is not supported by [crypto/ecdh].
func (priv *PrivateKey) ECDH() (*ecdh.PrivateKey, error) {
	c := curveToECDH(priv.Curve)
	if c == nil {
		return nil, errors.New("ecdsa: unsupported curve by crypto/ecdh")
	}
	k, err := priv.Bytes()
	if err != nil {
		return nil, err
	}
	return c.NewPrivateKey(k)
}

func curveToECDH(c elliptic.Curve) ecdh.Curve {
	switch c {
	case elliptic.P256():
		return ecdh.P256()
	case elliptic.P384():
		return ecdh.P384()
	case elliptic.P521():
		return ecdh.P521()
	default:
		return nil
	}
}

// Public returns the public key corresponding to priv.
func (priv *PrivateKey) Public() crypto.PublicKey {
	return &priv.PublicKey
}

// Equal reports whether priv and x have the same value.
//
// See [PublicKey.Equal] for details on how Curve is compared.
func (priv *PrivateKey) Equal(x crypto.PrivateKey) bool {
	xx, ok := x.(*PrivateKey)
	if !ok {
		return false
	}
	return priv.PublicKey.Equal(&xx.PublicKey) && bigIntEqual(priv.D, xx.D)
}

// bigIntEqual reports whether a and b are equal leaking only their bit length
// through timing side-channels.
func bigIntEqual(a, b *big.Int) bool {
	return subtle.ConstantTimeCompare(a.Bytes(), b.Bytes()) == 1
}

// ParseRawPrivateKey parses a private key encoded as a fixed-length big-endian
// integer, according to SEC 1, Version 2.0, Section 2.3.6 (sometimes referred
// to as the raw format). It returns an error if the value is not reduced modulo
// the curve's order, or if it's zero.
//
// curve must be one of [elliptic.P224], [elliptic.P256], [elliptic.P384], or
// [elliptic.P521], or ParseRawPrivateKey returns an error.
//
// ParseRawPrivateKey accepts the same format as [ecdh.Curve.NewPrivateKey] does
// for NIST curves, but returns a [PrivateKey] instead of an [ecdh.PrivateKey].
//
// Note that private keys are more commonly encoded in ASN.1 or PKCS#8 format,
// which can be parsed with [crypto/x509.ParseECPrivateKey] or
// [crypto/x509.ParsePKCS8PrivateKey] (and [encoding/pem]).
func ParseRawPrivateKey(curve elliptic.Curve, data []byte) (*PrivateKey, error) {
	switch curve {
	case elliptic.P224():
		return parseRawPrivateKey(ecdsa.P224(), nistec.NewP224Point, curve, data)
	case elliptic.P256():
		return parseRawPrivateKey(ecdsa.P256(), nistec.NewP256Point, curve, data)
	case elliptic.P384():
		return parseRawPrivateKey(ecdsa.P384(), nistec.NewP384Point, curve, data)
	case elliptic.P521():
		return parseRawPrivateKey(ecdsa.P521(), nistec.NewP521Point, curve, data)
	default:
		return nil, errors.New("ecdsa: curve not supported by ParseRawPrivateKey")
	}
}

func parseRawPrivateKey[P ecdsa.Point[P]](c *ecdsa.Curve[P], newPoint func() P, curve elliptic.Curve, data []byte) (*PrivateKey, error) {
	q, err := newPoint().ScalarBaseMult(data)
	if err != nil {
		return nil, err
	}
	k, err := ecdsa.NewPrivateKey(c, data, q.Bytes())
	if err != nil {
		return nil, err
	}
	return privateKeyFromFIPS(curve, k)
}

// Bytes encodes the private key as a fixed-length big-endian integer according
// to SEC 1, Version 2.0, Section 2.3.6 (sometimes referred to as the raw
// format). It returns an error if the private key is invalid.
//
// PrivateKey.Curve must be one of [elliptic.P224], [elliptic.P256],
// [elliptic.P384], or [elliptic.P521], or Bytes returns an error.
//
// Bytes returns the same format as [ecdh.PrivateKey.Bytes] does for NIST curves.
//
// Note that private keys are more commonly encoded in ASN.1 or PKCS#8 format,
// which can be generated with [crypto/x509.MarshalECPrivateKey] or
// [crypto/x509.MarshalPKCS8PrivateKey] (and [encoding/pem]).
func (priv *PrivateKey) Bytes() ([]byte, error) {
	switch priv.Curve {
	case elliptic.P224():
		return privateKeyBytes(ecdsa.P224(), priv)
	case elliptic.P256():
		return privateKeyBytes(ecdsa.P256(), priv)
	case elliptic.P384():
		return privateKeyBytes(ecdsa.P384(), priv)
	case elliptic.P521():
		return privateKeyBytes(ecdsa.P521(), priv)
	default:
		return nil, errors.New("ecdsa: curve not supported by PrivateKey.Bytes")
	}
}

func privateKeyBytes[P ecdsa.Point[P]](c *ecdsa.Curve[P], priv *PrivateKey) ([]byte, error) {
	k, err := privateKeyToFIPS(c, priv)
	if err != nil {
		return nil, err
	}
	return k.Bytes(), nil
}

// Sign signs a hash (which should be the result of hashing a larger message
// with opts.HashFunc()) using the private key, priv. If the hash is longer than
// the bit-length of the private key's curve order, the hash will be truncated
// to that length. It returns the ASN.1 encoded signature, like [SignASN1].
//
// If random is not nil, the signature is randomized. Most applications should use
// [crypto/rand.Reader] as random, but unless GODEBUG=cryptocustomrand=1 is set, a
// secure source of random bytes is always used, and the actual Reader is ignored.
// The GODEBUG setting will be removed in a future Go release. Instead, use
// [testing/cryptotest.SetGlobalRandom].
//
// If random is nil, Sign will produce a deterministic signature according to RFC
// 6979. When producing a deterministic signature, opts.HashFunc() must be the
// function used to produce digest and priv.Curve must be one of
// [elliptic.P224], [elliptic.P256], [elliptic.P384], or [elliptic.P521].
func (priv *PrivateKey) Sign(random io.Reader, digest []byte, opts crypto.SignerOpts) ([]byte, error) {
	if random == nil {
		return signRFC6979(priv, digest, opts)
	}
	random = rand.CustomReader(random)
	return SignASN1(random, priv, digest)
}

// GenerateKey generates a new ECDSA private key for the specified curve.
//
// Since Go 1.26, a secure source of random bytes is always used, and the Reader is
// ignored unless GODEBUG=cryptocustomrand=1 is set. This setting will be removed
// in a future Go release. Instead, use [testing/cryptotest.SetGlobalRandom].
func GenerateKey(c elliptic.Curve, r io.Reader) (*PrivateKey, error) {
	if boring.Enabled && rand.IsDefaultReader(r) {
		x, y, d, err := boring.GenerateKeyECDSA(c.Params().Name)
		if err != nil {
			return nil, err
		}
		return &PrivateKey{PublicKey: PublicKey{Curve: c, X: bbig.Dec(x), Y: bbig.Dec(y)}, D: bbig.Dec(d)}, nil
	}
	boring.UnreachableExceptTests()

	r = rand.CustomReader(r)

	switch c.Params() {
	case elliptic.P224().Params():
		return generateFIPS(c, ecdsa.P224(), r)
	case elliptic.P256().Params():
		return generateFIPS(c, ecdsa.P256(), r)
	case elliptic.P384().Params():
		return generateFIPS(c, ecdsa.P384(), r)
	case elliptic.P521().Params():
		return generateFIPS(c, ecdsa.P521(), r)
	default:
		return generateLegacy(c, r)
	}
}

func generateFIPS[P ecdsa.Point[P]](curve elliptic.Curve, c *ecdsa.Curve[P], rand io.Reader) (*PrivateKey, error) {
	if fips140only.Enforced() && !fips140only.ApprovedRandomReader(rand) {
		return nil, errors.New("crypto/ecdsa: only crypto/rand.Reader is allowed in FIPS 140-only mode")
	}
	privateKey, err := ecdsa.GenerateKey(c, rand)
	if err != nil {
		return nil, err
	}
	return privateKeyFromFIPS(curve, privateKey)
}

// SignASN1 signs a hash (which should be the result of hashing a larger message)
// using the private key, priv. If the hash is longer than the bit-length of the
// private key's curve order, the hash will be truncated to that length. It
// returns the ASN.1 encoded signature.
//
// The signature is randomized. Since Go 1.26, a secure source of random bytes
// is always used, and the Reader is ignored unless GODEBUG=cryptocustomrand=1
// is set. This setting will be removed in a future Go release. Instead, use
// [testing/cryptotest.SetGlobalRandom].
func SignASN1(r io.Reader, priv *PrivateKey, hash []byte) ([]byte, error) {
	if boring.Enabled && rand.IsDefaultReader(r) {
		b, err := boringPrivateKey(priv)
		if err != nil {
			return nil, err
		}
		return boring.SignMarshalECDSA(b, hash)
	}
	boring.UnreachableExceptTests()

	r = rand.CustomReader(r)

	switch priv.Curve.Params() {
	case elliptic.P224().Params():
		return signFIPS(ecdsa.P224(), priv, r, hash)
	case elliptic.P256().Params():
		return signFIPS(ecdsa.P256(), priv, r, hash)
	case elliptic.P384().Params():
		return signFIPS(ecdsa.P384(), priv, r, hash)
	case elliptic.P521().Params():
		return signFIPS(ecdsa.P521(), priv, r, hash)
	default:
		return signLegacy(priv, r, hash)
	}
}

func signFIPS[P ecdsa.Point[P]](c *ecdsa.Curve[P], priv *PrivateKey, rand io.Reader, hash []byte) ([]byte, error) {
	if fips140only.Enforced() && !fips140only.ApprovedRandomReader(rand) {
		return nil, errors.New("crypto/ecdsa: only crypto/rand.Reader is allowed in FIPS 140-only mode")
	}
	k, err := privateKeyToFIPS(c, priv)
	if err != nil {
		return nil, err
	}
	// Always using SHA-512 instead of the hash that computed hash is
	// technically a violation of draft-irtf-cfrg-det-sigs-with-noise-04 but in
	// our API we don't get to know what it was, and this has no security impact.
	sig, err := ecdsa.Sign(c, sha512.New, k, rand, hash)
	if err != nil {
		return nil, err
	}
	return encodeSignature(sig.R, sig.S)
}

func signRFC6979(priv *PrivateKey, hash []byte, opts crypto.SignerOpts) ([]byte, error) {
	if opts == nil {
		return nil, errors.New("ecdsa: Sign called with nil opts")
	}
	h := opts.HashFunc()
	if h.Size() != len(hash) {
		return nil, errors.New("ecdsa: hash length does not match hash function")
	}
	switch priv.Curve.Params() {
	case elliptic.P224().Params():
		return signFIPSDeterministic(ecdsa.P224(), h, priv, hash)
	case elliptic.P256().Params():
		return signFIPSDeterministic(ecdsa.P256(), h, priv, hash)
	case elliptic.P384().Params():
		return signFIPSDeterministic(ecdsa.P384(), h, priv, hash)
	case elliptic.P521().Params():
		return signFIPSDeterministic(ecdsa.P521(), h, priv, hash)
	default:
		return nil, errors.New("ecdsa: curve not supported by deterministic signatures")
	}
}

func signFIPSDeterministic[P ecdsa.Point[P]](c *ecdsa.Curve[P], hashFunc crypto.Hash, priv *PrivateKey, hash []byte) ([]byte, error) {
	k, err := privateKeyToFIPS(c, priv)
	if err != nil {
		return nil, err
	}
	h := fips140hash.UnwrapNew(hashFunc.New)
	if fips140only.Enforced() && !fips140only.ApprovedHash(h()) {
		return nil, errors.New("crypto/ecdsa: use of hash functions other than SHA-2 or SHA-3 is not allowed in FIPS 140-only mode")
	}
	sig, err := ecdsa.SignDeterministic(c, h, k, hash)
	if err != nil {
		return nil, err
	}
	return encodeSignature(sig.R, sig.S)
}

func encodeSignature(r, s []byte) ([]byte, error) {
	var b cryptobyte.Builder
	b.AddASN1(asn1.SEQUENCE, func(b *cryptobyte.Builder) {
		addASN1IntBytes(b, r)
		addASN1IntBytes(b, s)
	})
	return b.Bytes()
}

// addASN1IntBytes encodes in ASN.1 a positive integer represented as
// a big-endian byte slice with zero or more leading zeroes.
func addASN1IntBytes(b *cryptobyte.Builder, bytes []byte) {
	for len(bytes) > 0 && bytes[0] == 0 {
		bytes = bytes[1:]
	}
	if len(bytes) == 0 {
		b.SetError(errors.New("invalid integer"))
		return
	}
	b.AddASN1(asn1.INTEGER, func(c *cryptobyte.Builder) {
		if bytes[0]&0x80 != 0 {
			c.AddUint8(0)
		}
		c.AddBytes(bytes)
	})
}

// VerifyASN1 verifies the ASN.1 encoded signature, sig, of hash using the
// public key, pub. Its return value records whether the signature is valid.
//
// The inputs are not considered confidential, and may leak through timing side
// channels, or if an attacker has control of part of the inputs.
func VerifyASN1(pub *PublicKey, hash, sig []byte) bool {
	if boring.Enabled {
		key, err := boringPublicKey(pub)
		if err != nil {
			return false
		}
		return boring.VerifyECDSA(key, hash, sig)
	}
	boring.UnreachableExceptTests()

	switch pub.Curve.Params() {
	case elliptic.P224().Params():
		return verifyFIPS(ecdsa.P224(), pub, hash, sig)
	case elliptic.P256().Params():
		return verifyFIPS(ecdsa.P256(), pub, hash, sig)
	case elliptic.P384().Params():
		return verifyFIPS(ecdsa.P384(), pub, hash, sig)
	case elliptic.P521().Params():
		return verifyFIPS(ecdsa.P521(), pub, hash, sig)
	default:
		return verifyLegacy(pub, hash, sig)
	}
}

func verifyFIPS[P ecdsa.Point[P]](c *ecdsa.Curve[P], pub *PublicKey, hash, sig []byte) bool {
	r, s, err := parseSignature(sig)
	if err != nil {
		return false
	}
	k, err := publicKeyToFIPS(c, pub)
	if err != nil {
		return false
	}
	if err := ecdsa.Verify(c, k, hash, &ecdsa.Signature{R: r, S: s}); err != nil {
		return false
	}
	return true
}

func parseSignature(sig []byte) (r, s []byte, err error) {
	var inner cryptobyte.String
	input := cryptobyte.String(sig)
	if !input.ReadASN1(&inner, asn1.SEQUENCE) ||
		!input.Empty() ||
		!inner.ReadASN1Integer(&r) ||
		!inner.ReadASN1Integer(&s) ||
		!inner.Empty() {
		return nil, nil, errors.New("invalid ASN.1")
	}
	return r, s, nil
}

func publicKeyFromFIPS(curve elliptic.Curve, pub *ecdsa.PublicKey) (*PublicKey, error) {
	x, y, err := pointToAffine(curve, pub.Bytes())
	if err != nil {
		return nil, err
	}
	return &PublicKey{Curve: curve, X: x, Y: y}, nil
}

func privateKeyFromFIPS(curve elliptic.Curve, priv *ecdsa.PrivateKey) (*PrivateKey, error) {
	pub, err := publicKeyFromFIPS(curve, priv.PublicKey())
	if err != nil {
		return nil, err
	}
	return &PrivateKey{PublicKey: *pub, D: new(big.Int).SetBytes(priv.Bytes())}, nil
}

func publicKeyToFIPS[P ecdsa.Point[P]](c *ecdsa.Curve[P], pub *PublicKey) (*ecdsa.PublicKey, error) {
	Q, err := pointFromAffine(pub.Curve, pub.X, pub.Y)
	if err != nil {
		return nil, err
	}
	return ecdsa.NewPublicKey(c, Q)
}

var privateKeyCache fips140cache.Cache[PrivateKey, ecdsa.PrivateKey]

func privateKeyToFIPS[P ecdsa.Point[P]](c *ecdsa.Curve[P], priv *PrivateKey) (*ecdsa.PrivateKey, error) {
	Q, err := pointFromAffine(priv.Curve, priv.X, priv.Y)
	if err != nil {
		return nil, err
	}

	// Reject values that would not get correctly encoded.
	if priv.D.BitLen() > priv.Curve.Params().N.BitLen() {
		return nil, errors.New("ecdsa: private key scalar too large")
	}
	if priv.D.Sign() <= 0 {
		return nil, errors.New("ecdsa: private key scalar is zero or negative")
	}

	size := (priv.Curve.Params().N.BitLen() + 7) / 8
	const maxScalarSize = 66 // enough for a P-521 private key
	if size > maxScalarSize {
		return nil, errors.New("ecdsa: internal error: curve size too large")
	}
	D := priv.D.FillBytes(make([]byte, size, maxScalarSize))

	return privateKeyCache.Get(priv, func() (*ecdsa.PrivateKey, error) {
		return ecdsa.NewPrivateKey(c, D, Q)
	}, func(k *ecdsa.PrivateKey) bool {
		return subtle.ConstantTimeCompare(k.PublicKey().Bytes(), Q) == 1 &&
			subtle.ConstantTimeCompare(k.Bytes(), D) == 1
	})
}

// pointFromAffine is used to convert the PublicKey to a nistec SetBytes input.
func pointFromAffine(curve elliptic.Curve, x, y *big.Int) ([]byte, error) {
	bitSize := curve.Params().BitSize
	// Reject values that would not get correctly encoded.
	if x.Sign() < 0 || y.Sign() < 0 {
		return nil, errors.New("negative coordinate")
	}
	if x.BitLen() > bitSize || y.BitLen() > bitSize {
		return nil, errors.New("overflowing coordinate")
	}
	// Encode the coordinates and let [ecdsa.NewPublicKey] reject invalid points.
	byteLen := (bitSize + 7) / 8
	buf := make([]byte, 1+2*byteLen)
	buf[0] = 4 // uncompressed point
	x.FillBytes(buf[1 : 1+byteLen])
	y.FillBytes(buf[1+byteLen : 1+2*byteLen])
	return buf, nil
}

// pointToAffine is used to convert a nistec Bytes encoding to a PublicKey.
func pointToAffine(curve elliptic.Curve, p []byte) (x, y *big.Int, err error) {
	if len(p) == 1 && p[0] == 0 {
		// This is the encoding of the point at infinity.
		return nil, nil, errors.New("ecdsa: public key point is the infinity")
	}
	byteLen := (curve.Params().BitSize + 7) / 8
	x = new(big.Int).SetBytes(p[1 : 1+byteLen])
	y = new(big.Int).SetBytes(p[1+byteLen:])
	return x, y, nil
}
