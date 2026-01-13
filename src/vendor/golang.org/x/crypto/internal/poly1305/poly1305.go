// 版权所有 2012 The Go Authors。保留所有权利。
// 本源代码的使用受 BSD 风格许可证约束，
// 该许可证可在 LICENSE 文件中找到。

// poly1305 包实现了 Poly1305 一次性消息认证码，
// 如 https://cr.yp.to/mac/poly1305-20050329.pdf 中所规定。
//
// Poly1305 是一个快速的一次性认证函数。攻击者在没有密钥的情况下
// 为消息生成认证器是不可行的。但是，密钥只能用于单个消息。
// 使用相同的密钥认证两个不同的消息允许攻击者伪造使用相同密钥的
// 其他消息的认证器。
//
// Poly1305 最初与 AES 耦合以创建 Poly1305-AES。AES 使用固定密钥
// 从随机数生成一次性密钥。但是，在此包中不使用 AES，
// 而是直接指定一次性密钥。
package poly1305

import "crypto/subtle"

// TagSize 是 poly1305 认证器的大小，以字节为单位。
const TagSize = 16

// Sum 使用一次性密钥为 msg 生成认证器，并将 16 字节结果放入 out。
// 使用相同的密钥认证两个不同的消息允许攻击者随意伪造消息。
func Sum(out *[16]byte, m []byte, key *[32]byte) {
	h := New(key)
	h.Write(m)
	h.Sum(out[:0])
}

// Verify 如果 mac 是使用给定密钥的 m 的有效认证器，则返回 true。
func Verify(mac *[16]byte, m []byte, key *[32]byte) bool {
	var tmp [16]byte
	Sum(&tmp, m, key)
	return subtle.ConstantTimeCompare(tmp[:], mac[:]) == 1
}

// New 返回一个新的 MAC，使用给定的密钥计算写入其中的所有数据的认证标签。
// 这允许逐步写入消息，而不是将其作为单个切片传递。
// 普通用户应该使用 Sum 函数。
//
// 密钥对于每条消息必须是唯一的，因为使用相同的密钥认证两个不同的消息
// 允许攻击者随意伪造消息。
func New(key *[32]byte) *MAC {
	m := &MAC{}
	initialize(key, &m.macState)
	return m
}

// MAC 是一个 io.Writer，计算写入其中的数据的认证标签。
//
// MAC 不能像常见的 hash.Hash 实现那样使用，
// 因为两次使用 poly1305 密钥会破坏其安全性。
// 因此，在调用 Sum 或 Verify 后向正在运行的 MAC 写入数据会导致 panic。
type MAC struct {
	mac // 平台相关的实现

	finalized bool
}

// Size 返回 Sum 将返回的字节数。
func (h *MAC) Size() int { return TagSize }

// Write 向正在运行的消息认证码添加更多数据。它从不返回错误。
//
// 在首次调用 Sum 或 Verify 后不得调用它。
func (h *MAC) Write(p []byte) (n int, err error) {
	if h.finalized {
		panic("poly1305: write to MAC after Sum or Verify")
	}
	return h.mac.Write(p)
}

// Sum 计算写入消息认证码的所有数据的认证器。
func (h *MAC) Sum(b []byte) []byte {
	var mac [TagSize]byte
	h.mac.Sum(&mac)
	h.finalized = true
	return append(b, mac[:]...)
}

// Verify 返回写入消息认证码的所有数据的认证器是否与预期值匹配。
func (h *MAC) Verify(expected []byte) bool {
	var mac [TagSize]byte
	h.mac.Sum(&mac)
	h.finalized = true
	return subtle.ConstantTimeCompare(expected, mac[:]) == 1
}
