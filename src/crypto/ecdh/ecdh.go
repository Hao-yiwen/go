// 版权所有 2022 The Go Authors。保留所有权利。
// 本源代码的使用受 BSD 风格许可证约束，
// 该许可证可在 LICENSE 文件中找到。

// ecdh 包实现了 NIST 曲线和 Curve25519 上的椭圆曲线 Diffie-Hellman。
package ecdh

import (
	"crypto"
	"crypto/internal/boring"
	"crypto/internal/fips140/ecdh"
	"crypto/subtle"
	"errors"
	"io"
)

type Curve interface {
	// GenerateKey 生成一个随机 PrivateKey。
	//
	// 从 Go 1.26 开始，总是使用安全的随机字节来源，rand 被忽略
	// 除非设置了 GODEBUG=cryptocustomrand=1。此设置将
	// 在未来的 Go 版本中移除。改为使用 [testing/cryptotest.SetGlobalRandom]。
	GenerateKey(rand io.Reader) (*PrivateKey, error)

	// NewPrivateKey 检查密钥是否有效并返回一个 PrivateKey。
	//
	// 对于 NIST 曲线，这遵循 SEC 1 第 2.0 版第 2.3.6 节，
	// 其中涉及将字节解码为固定长度大端整数
	// 并检查结果是否低于曲线的阶。零
	// 私钥也被拒绝，因为相应公钥的编码
	// 会不规则。
	//
	// 对于 X25519，这仅检查标量长度。
	NewPrivateKey(key []byte) (*PrivateKey, error)

	// NewPublicKey 检查密钥是否有效并返回一个 PublicKey。
	//
	// 对于 NIST 曲线，这根据 SEC 1 第 2.0 版第 2.3.4 节解码未压缩点。
	// 压缩编码和无穷远点被拒绝。
	//
	// 对于 X25519，这仅检查 u 坐标长度。对抗性地选择的
	// 公钥可能导致 ECDH 返回错误。
	NewPublicKey(key []byte) (*PublicKey, error)

	// ecdh 执行 ECDH 交换并返回共享秘密。它被暴露
	// 为 PrivateKey.ECDH 方法。
	//
	// 私有方法也允许我们在未来扩展 ECDH 接口
	// 更多方法而不破坏向后兼容性。
	ecdh(local *PrivateKey, remote *PublicKey) ([]byte, error)
}

// PublicKey 是一个 ECDH 公钥，通常是通过网络发送的对等方的 ECDH 份额。
//
// 这些密钥可以使用 [crypto/x509.ParsePKIXPublicKey] 解析并使用
// [crypto/x509.MarshalPKIXPublicKey] 编码。对于 NIST 曲线，
// 它们需要在解析后使用 [crypto/ecdsa.PublicKey.ECDH] 转换。
type PublicKey struct {
	curve     Curve
	publicKey []byte
	boring    *boring.PublicKeyECDH
	fips      *ecdh.PublicKey
}

// Bytes 返回公钥编码的副本。
func (k *PublicKey) Bytes() []byte {
	// 将公钥复制到固定大小的缓冲区，该缓冲区在内联后
	// 可在调用者的堆栈上分配。
	var buf [133]byte
	return append(buf[:0], k.publicKey...)
}

// Equal 返回 x 是否表示与 k 相同的公钥。
//
// 注意可能存在具有不同编码的等价公钥，这些公钥
// 将从此检查返回 false，但作为 ECDH 的输入表现相同。
//
// 只要密钥类型及其曲线匹配，此检查在常数时间内执行。
func (k *PublicKey) Equal(x crypto.PublicKey) bool {
	xx, ok := x.(*PublicKey)
	if !ok {
		return false
	}
	return k.curve == xx.curve &&
		subtle.ConstantTimeCompare(k.publicKey, xx.publicKey) == 1
}

func (k *PublicKey) Curve() Curve {
	return k.curve
}

// KeyExchanger 是用于密钥交换操作的不透明私钥的接口。
// 例如，ECDH 密钥存储在硬件模块中。
//
// 由 [PrivateKey] 实现。
type KeyExchanger interface {
	PublicKey() *PublicKey
	Curve() Curve
	ECDH(*PublicKey) ([]byte, error)
}

var _ KeyExchanger = (*PrivateKey)(nil)

// PrivateKey 是一个 ECDH 私钥，通常保持秘密。
//
// 这些密钥可以使用 [crypto/x509.ParsePKCS8PrivateKey] 解析并使用
// [crypto/x509.MarshalPKCS8PrivateKey] 编码。对于 NIST 曲线，
// 它们需要在解析后使用 [crypto/ecdsa.PrivateKey.ECDH] 转换。
type PrivateKey struct {
	curve      Curve
	privateKey []byte
	publicKey  *PublicKey
	boring     *boring.PrivateKeyECDH
	fips       *ecdh.PrivateKey
}

// ECDH 执行 ECDH 交换并返回共享秘密。[PrivateKey] 和 [PublicKey]
// 必须使用相同的曲线。
//
// 对于 NIST 曲线，这执行 SEC 1 第 2.0 版第 3.3.1 节中指定的 ECDH，
// 并返回根据 SEC 1 第 2.0 版第 2.3.5 节编码的 x 坐标。
// 结果永远不是无穷远点。这也称为 NIST SP 800-56A Rev. 3 第 6.1.2.2 节
// 中指定的临时统一模型方案的共享秘密计算。
//
// 对于 [X25519]，这执行 RFC 7748 第 6.1 节中指定的 ECDH。如果
// 结果是全零值，ECDH 返回错误。
func (k *PrivateKey) ECDH(remote *PublicKey) ([]byte, error) {
	if k.curve != remote.curve {
		return nil, errors.New("crypto/ecdh: private key and public key curves do not match")
	}
	return k.curve.ecdh(k, remote)
}

// Bytes 返回私钥编码的副本。
func (k *PrivateKey) Bytes() []byte {
	// 将私钥复制到固定大小的缓冲区，该缓冲区在内联后
	// 可在调用者的堆栈上分配。
	var buf [66]byte
	return append(buf[:0], k.privateKey...)
}

// Equal 返回 x 是否表示与 k 相同的私钥。
//
// 注意可能存在具有不同编码的等价私钥，这些私钥
// 将从此检查返回 false，但作为 [ECDH] 的输入表现相同。
//
// 只要密钥类型及其曲线匹配，此检查在常数时间内执行。
func (k *PrivateKey) Equal(x crypto.PrivateKey) bool {
	xx, ok := x.(*PrivateKey)
	if !ok {
		return false
	}
	return k.curve == xx.curve &&
		subtle.ConstantTimeCompare(k.privateKey, xx.privateKey) == 1
}

func (k *PrivateKey) Curve() Curve {
	return k.curve
}

func (k *PrivateKey) PublicKey() *PublicKey {
	return k.publicKey
}

// Public 实现所有标准库私钥的隐式接口。
// 参见 [crypto.PrivateKey] 的文档。
func (k *PrivateKey) Public() crypto.PublicKey {
	return k.PublicKey()
}
