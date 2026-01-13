// 版权所有 2010 The Go Authors。保留所有权利。
// 本源代码的使用受 BSD 风格许可证约束，
// 该许可证可在 LICENSE 文件中找到。

// cipher 包实现了可以包装在底层块密码实现外的标准块密码模式。
// 参见 https://csrc.nist.gov/groups/ST/toolkit/BCM/current_modes.html
// 和 NIST 特别出版物 800-38A。
package cipher

// Block 表示使用给定密钥的块密码实现。
// 它提供加密或解密单个块的能力。
// 模式实现将该能力扩展到块流。
type Block interface {
	// BlockSize 返回密码的块大小。
	BlockSize() int

	// Encrypt 将 src 中的第一个块加密到 dst。
	// dst 和 src 必须完全重叠或完全不重叠。
	Encrypt(dst, src []byte)

	// Decrypt 将 src 中的第一个块解密到 dst。
	// dst 和 src 必须完全重叠或完全不重叠。
	Decrypt(dst, src []byte)
}

// Stream 表示一个流密码。
type Stream interface {
	// XORKeyStream 将给定切片中的每个字节与密码密钥流中的一个字节进行异或。
	// dst 和 src 必须完全重叠或完全不重叠。
	//
	// 如果 len(dst) < len(src)，XORKeyStream 应该触发 panic。
	// 传递比 src 更大的 dst 是可接受的，在这种情况下，XORKeyStream
	// 只会更新 dst[:len(src)]，不会触及 dst 的其余部分。
	//
	// 多次调用 XORKeyStream 的行为就像 src 缓冲区的连接
	// 是在单次运行中传递的一样。也就是说，Stream 维护状态，
	// 不会在每次 XORKeyStream 调用时重置。
	XORKeyStream(dst, src []byte)
}

// BlockMode 表示以基于块的模式（CBC、ECB 等）运行的块密码。
type BlockMode interface {
	// BlockSize 返回模式的块大小。
	BlockSize() int

	// CryptBlocks 加密或解密多个块。src 的长度必须是块大小的倍数。
	// dst 和 src 必须完全重叠或完全不重叠。
	//
	// 如果 len(dst) < len(src)，CryptBlocks 应该触发 panic。
	// 传递比 src 更大的 dst 是可接受的，在这种情况下，CryptBlocks
	// 只会更新 dst[:len(src)]，不会触及 dst 的其余部分。
	//
	// 多次调用 CryptBlocks 的行为就像 src 缓冲区的连接
	// 是在单次运行中传递的一样。也就是说，BlockMode 维护状态，
	// 不会在每次 CryptBlocks 调用时重置。
	CryptBlocks(dst, src []byte)
}

// AEAD 是一种提供带关联数据的认证加密的密码模式。
// 有关该方法的描述，请参见
// https://en.wikipedia.org/wiki/Authenticated_encryption。
type AEAD interface {
	// NonceSize 返回必须传递给 Seal 和 Open 的随机数大小。
	NonceSize() int

	// Overhead 返回明文与其密文长度之间的最大差值。
	Overhead() int

	// Seal 加密并认证明文，认证附加数据，并将结果追加到 dst，
	// 返回更新后的切片。对于给定的密钥，随机数必须是 NonceSize()
	// 字节长，并且在所有时间内都是唯一的。
	//
	// 要重用明文的存储空间来存放加密输出，请使用 plaintext[:0] 作为 dst。
	// 否则，dst 的剩余容量不得与明文重叠。
	// dst 和 additionalData 不能重叠。
	Seal(dst, nonce, plaintext, additionalData []byte) []byte

	// Open 解密并认证密文，认证附加数据，如果成功，则将结果明文
	// 追加到 dst，返回更新后的切片。随机数必须是 NonceSize() 字节长，
	// 它和附加数据都必须与传递给 Seal 的值匹配。
	//
	// 要重用密文的存储空间来存放解密输出，请使用 ciphertext[:0] 作为 dst。
	// 否则，dst 的剩余容量不得与密文重叠。
	// dst 和 additionalData 不能重叠。
	//
	// 即使函数失败，dst 的内容（直到其容量）也可能被覆盖。
	Open(dst, nonce, ciphertext, additionalData []byte) ([]byte, error)
}
