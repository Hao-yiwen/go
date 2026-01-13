// 版权所有 2009 The Go Authors。保留所有权利。
// 本源代码的使用受 BSD 风格许可证约束，
// 该许可证可在 LICENSE 文件中找到。

/*
hmac 包实现了密钥哈希消息认证码（HMAC），
如美国联邦信息处理标准出版物 198 中所定义。
HMAC 是一种使用密钥对消息进行签名的加密哈希。
接收方通过使用相同的密钥重新计算哈希来验证。

接收方应注意使用 Equal 来比较 MAC，以避免时序侧信道攻击：

	// ValidMAC 报告 messageMAC 是否是 message 的有效 HMAC 标签。
	func ValidMAC(message, messageMAC, key []byte) bool {
		mac := hmac.New(sha256.New, key)
		mac.Write(message)
		expectedMAC := mac.Sum(nil)
		return hmac.Equal(messageMAC, expectedMAC)
	}
*/
package hmac

import (
	"crypto/internal/boring"
	"crypto/internal/fips140/hmac"
	"crypto/internal/fips140hash"
	"crypto/internal/fips140only"
	"crypto/subtle"
	"hash"
)

// New 使用给定的 [hash.Hash] 类型和密钥返回一个新的 HMAC 哈希。
// 像 [crypto/sha256.New] 这样的 New 函数可以用作 h。
// h 每次被调用时都必须返回一个新的 Hash。
// 请注意，与标准库中的其他哈希实现不同，返回的 Hash
// 不实现 [encoding.BinaryMarshaler] 或 [encoding.BinaryUnmarshaler]。
func New(h func() hash.Hash, key []byte) hash.Hash {
	if boring.Enabled {
		hm := boring.NewHMAC(h, key)
		if hm != nil {
			return hm
		}
		// BoringCrypto 无法识别 h，因此回退到标准 Go 代码。
	}
	h = fips140hash.UnwrapNew(h)
	if fips140only.Enforced() {
		if len(key) < 112/8 {
			panic("crypto/hmac: use of keys shorter than 112 bits is not allowed in FIPS 140-only mode")
		}
		if !fips140only.ApprovedHash(h()) {
			panic("crypto/hmac: use of hash functions other than SHA-2 or SHA-3 is not allowed in FIPS 140-only mode")
		}
	}
	return hmac.New(h, key)
}

// Equal 比较两个 MAC 是否相等，而不泄露时序信息。
func Equal(mac1, mac2 []byte) bool {
	// 如果 MAC 的长度不同，我们不必保持常量时间，
	// 因为这表明使用了完全不同的哈希函数。
	return subtle.ConstantTimeCompare(mac1, mac2) == 1
}
