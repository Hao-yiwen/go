// 版权所有 2009 The Go Authors。保留所有权利。
// 本源代码的使用受 BSD 风格许可证约束，
// 该许可证可在 LICENSE 文件中找到。

// aes 包实现了 AES 加密（以前称为 Rijndael），
// 如美国联邦信息处理标准出版物 197 中所定义。
//
// 本包中的 AES 操作未使用常量时间算法实现。
// 例外情况是在启用了 AES 硬件支持的系统上运行时，
// 这使得这些操作成为常量时间的。例如使用 AES-NI 扩展的 amd64 系统
// 和使用消息安全辅助扩展的 s390x 系统。
// 在这类系统上，当 NewCipher 的结果传递给 cipher.NewGCM 时，
// GCM 使用的 GHASH 操作也是常量时间的。
package aes

import (
	"crypto/cipher"
	"crypto/internal/boring"
	"crypto/internal/fips140/aes"
	"strconv"
)

// AES 块大小（以字节为单位）。
const BlockSize = 16

type KeySizeError int

func (k KeySizeError) Error() string {
	return "crypto/aes: invalid key size " + strconv.Itoa(int(k))
}

// NewCipher 创建并返回一个新的 [cipher.Block]。
// key 参数必须是 AES 密钥，
// 可以是 16、24 或 32 字节，分别对应
// AES-128、AES-192 或 AES-256。
func NewCipher(key []byte) (cipher.Block, error) {
	k := len(key)
	switch k {
	default:
		return nil, KeySizeError(k)
	case 16, 24, 32:
		break
	}
	if boring.Enabled {
		return boring.NewAESCipher(key)
	}
	return aes.New(key)
}
