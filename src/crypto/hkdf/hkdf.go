// 版权所有 2024 The Go Authors。保留所有权利。
// 本源代码的使用受 BSD 风格许可证约束，
// 该许可证可在 LICENSE 文件中找到。

// hkdf 包实现了 RFC 5869 中定义的基于 HMAC 的提取和扩展密钥导出
// 函数 (HKDF)。
//
// HKDF 是一种密码学密钥导出函数 (KDF)，目的是
// 将有限的输入密钥材料扩展为一个或多个密码学
// 强秘密密钥。
package hkdf

import (
	"crypto/internal/fips140/hkdf"
	"crypto/internal/fips140hash"
	"crypto/internal/fips140only"
	"errors"
	"hash"
)

// Extract 从输入秘密和可选的独立盐为使用 [Expand] 生成伪随机密钥。
//
// 仅当您需要通过多个
// Expand 调用和不同的上下文值重新使用提取的密钥时，才使用此函数。
// 大多数常见场景，包括生成多个密钥，应改为使用 [Key]。
func Extract[H hash.Hash](h func() H, secret, salt []byte) ([]byte, error) {
	fh := fips140hash.UnwrapNew(h)
	if err := checkFIPS140Only(fh, secret); err != nil {
		return nil, err
	}
	return hkdf.Extract(fh, secret, salt), nil
}

// Expand 从给定的哈希、密钥和可选的上下文信息导出密钥，
// 返回长度为 keyLength 的 []byte，可用作密码学密钥。
// 跳过提取步骤。
//
// 密钥应该由 [Extract] 生成，或者是均匀分布的
// 随机或伪随机密码学强密钥。参见 RFC 5869，第
// 3.3 节。大多数常见场景将希望使用 [Key] 代替。
func Expand[H hash.Hash](h func() H, pseudorandomKey []byte, info string, keyLength int) ([]byte, error) {
	fh := fips140hash.UnwrapNew(h)
	if err := checkFIPS140Only(fh, pseudorandomKey); err != nil {
		return nil, err
	}

	limit := fh().Size() * 255
	if keyLength > limit {
		return nil, errors.New("hkdf: requested key length too large")
	}

	return hkdf.Expand(fh, pseudorandomKey, info, keyLength), nil
}

// Key 从给定的哈希、秘密、盐和上下文信息导出密钥，
// 返回长度为 keyLength 的 []byte，可用作密码学密钥。
// Salt 和 info 可以为 nil。
func Key[Hash hash.Hash](h func() Hash, secret, salt []byte, info string, keyLength int) ([]byte, error) {
	fh := fips140hash.UnwrapNew(h)
	if err := checkFIPS140Only(fh, secret); err != nil {
		return nil, err
	}

	limit := fh().Size() * 255
	if keyLength > limit {
		return nil, errors.New("hkdf: requested key length too large")
	}

	return hkdf.Key(fh, secret, salt, info, keyLength), nil
}

func checkFIPS140Only[Hash hash.Hash](h func() Hash, key []byte) error {
	if !fips140only.Enforced() {
		return nil
	}
	if len(key) < 112/8 {
		return errors.New("crypto/hkdf: use of keys shorter than 112 bits is not allowed in FIPS 140-only mode")
	}
	if !fips140only.ApprovedHash(h()) {
		return errors.New("crypto/hkdf: use of hash functions other than SHA-2 or SHA-3 is not allowed in FIPS 140-only mode")
	}
	return nil
}
