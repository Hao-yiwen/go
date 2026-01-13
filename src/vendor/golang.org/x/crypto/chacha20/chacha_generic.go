// 版权所有 2016 The Go Authors。保留所有权利。
// 本源代码的使用受 BSD 风格许可证约束，
// 该许可证可在 LICENSE 文件中找到。

// chacha20 包实现了 RFC 8439 和 draft-irtf-cfrg-xchacha-01 中规定的
// ChaCha20 和 XChaCha20 加密算法。
package chacha20

import (
	"crypto/cipher"
	"encoding/binary"
	"errors"
	"math/bits"

	"golang.org/x/crypto/internal/alias"
)

const (
	// KeySize 是此密码使用的密钥大小，以字节为单位。
	KeySize = 32

	// NonceSize 是此密码标准变体使用的随机数大小，以字节为单位。
	//
	// 注意，如果同一密钥重复使用超过 2³² 次，
	// 这个长度太短，不能安全地随机生成。
	NonceSize = 12

	// NonceSizeX 是此密码的 XChaCha20 变体使用的随机数大小，以字节为单位。
	NonceSizeX = 24
)

// Cipher 是使用特定密钥和随机数的 ChaCha20 或 XChaCha20 的有状态实例。
// *Cipher 实现了 cipher.Stream 接口。
type Cipher struct {
	// ChaCha20 状态是 16 个字：4 个常量，8 个密钥，1 个计数器
	//（每个块后递增），以及 3 个随机数。
	key     [8]uint32
	counter uint32
	nonce   [3]uint32

	// buf 的最后 len 个字节是上一次 XORKeyStream 调用剩余的密钥流字节。
	// buf 的大小取决于 xorKeyStreamBlocks 一次计算多少个块。
	buf [bufSize]byte
	len int

	// overflow 在计数器溢出时设置，此时不能再生成更多块，
	// 下一次 XORKeyStream 调用应该 panic。
	overflow bool

	// 第一轮中与计数器无关的结果在第一次计算后被缓存。
	precompDone      bool
	p1, p5, p9, p13  uint32
	p2, p6, p10, p14 uint32
	p3, p7, p11, p15 uint32
}

var _ cipher.Stream = (*Cipher)(nil)

// NewUnauthenticatedCipher 使用给定的 32 字节密钥和 12 或 24 字节随机数
// 创建一个新的 ChaCha20 流密码。如果提供 24 字节的随机数，
// 将使用 XChaCha20 构造。如果密钥或随机数长度不符合要求，则返回错误。
//
// 注意，ChaCha20 与所有流密码一样，不提供认证功能，
// 允许攻击者悄悄篡改明文。因此，它更适合作为构建块，
// 而不是独立的加密机制。请考虑使用 golang.org/x/crypto/chacha20poly1305 包。
func NewUnauthenticatedCipher(key, nonce []byte) (*Cipher, error) {
	// 此函数被拆分为包装器，以便 Cipher 分配可以内联，
	// 并且根据调用者如何使用返回值，不会逃逸到堆上。
	c := &Cipher{}
	return newUnauthenticatedCipher(c, key, nonce)
}

func newUnauthenticatedCipher(c *Cipher, key, nonce []byte) (*Cipher, error) {
	if len(key) != KeySize {
		return nil, errors.New("chacha20: wrong key size")
	}
	if len(nonce) == NonceSizeX {
		// XChaCha20 使用 ChaCha20 核心将 16 字节的随机数混合到派生密钥中，
		// 使其能够处理 24 字节的随机数。
		// 参见 draft-irtf-cfrg-xchacha-01，第 2.3 节。
		key, _ = HChaCha20(key, nonce[0:16])
		cNonce := make([]byte, NonceSize)
		copy(cNonce[4:12], nonce[16:24])
		nonce = cNonce
	} else if len(nonce) != NonceSize {
		return nil, errors.New("chacha20: wrong nonce size")
	}

	key, nonce = key[:KeySize], nonce[:NonceSize] // 边界检查消除提示
	c.key = [8]uint32{
		binary.LittleEndian.Uint32(key[0:4]),
		binary.LittleEndian.Uint32(key[4:8]),
		binary.LittleEndian.Uint32(key[8:12]),
		binary.LittleEndian.Uint32(key[12:16]),
		binary.LittleEndian.Uint32(key[16:20]),
		binary.LittleEndian.Uint32(key[20:24]),
		binary.LittleEndian.Uint32(key[24:28]),
		binary.LittleEndian.Uint32(key[28:32]),
	}
	c.nonce = [3]uint32{
		binary.LittleEndian.Uint32(nonce[0:4]),
		binary.LittleEndian.Uint32(nonce[4:8]),
		binary.LittleEndian.Uint32(nonce[8:12]),
	}
	return c, nil
}

// ChaCha20 状态的前 4 个常量字。
const (
	j0 uint32 = 0x61707865 // expa
	j1 uint32 = 0x3320646e // nd 3
	j2 uint32 = 0x79622d32 // 2-by
	j3 uint32 = 0x6b206574 // te k
)

const blockSize = 64

// quarterRound 是 ChaCha20 的核心。它对 4 个状态字的位进行混洗。
// 在 ChaCha20 的 20 轮中，每轮执行 4 次，每轮对所有 16 个字操作，
// 一次按列或对角线分组处理 4 个字。
func quarterRound(a, b, c, d uint32) (uint32, uint32, uint32, uint32) {
	a += b
	d ^= a
	d = bits.RotateLeft32(d, 16)
	c += d
	b ^= c
	b = bits.RotateLeft32(b, 12)
	a += b
	d ^= a
	d = bits.RotateLeft32(d, 8)
	c += d
	b ^= c
	b = bits.RotateLeft32(b, 7)
	return a, b, c, d
}

// SetCounter 设置 Cipher 计数器。下一次调用 XORKeyStream 将
// 表现得好像到目前为止已经加密了 (64 * counter) 个字节。
//
// 为防止意外重用计数器，如果 counter 小于当前值，SetCounter 将 panic。
//
// 注意，XORKeyStream 的执行时间与计数器值相关。
func (s *Cipher) SetCounter(counter uint32) {
	// 在内部，s 可能缓冲多个块，这使得此实现稍微复杂。
	// 在检查计数器是否回滚时，我们必须同时使用 s.counter 和 s.len
	// 来确定我们已经输出了多少个块。
	outputCounter := s.counter - uint32(s.len)/blockSize
	if s.overflow || counter < outputCounter {
		panic("chacha20: SetCounter attempted to rollback counter")
	}

	// 在一般情况下，我们设置新的计数器值并将 s.len 重置为 0，
	// 使下一次调用 XORKeyStream 时重新填充缓冲区。但是，如果
	// 我们在现有缓冲区内前进，可以通过简单地设置 s.len 来节省工作。
	if counter < s.counter {
		s.len = int(s.counter-counter) * blockSize
	} else {
		s.counter = counter
		s.len = 0
	}
}

// XORKeyStream 将给定切片中的每个字节与密码密钥流中的一个字节进行异或。
// dst 和 src 必须完全重叠或完全不重叠。
//
// 如果 len(dst) < len(src)，XORKeyStream 将 panic。可以传递比 src 更大的 dst，
// 在这种情况下，XORKeyStream 只会更新 dst[:len(src)]，不会触及 dst 的其余部分。
//
// 多次调用 XORKeyStream 的行为就像将 src 缓冲区的连接在一次运行中传递一样。
// 也就是说，Cipher 维护状态，不会在每次 XORKeyStream 调用时重置。
func (s *Cipher) XORKeyStream(dst, src []byte) {
	if len(src) == 0 {
		return
	}
	if len(dst) < len(src) {
		panic("chacha20: output smaller than input")
	}
	dst = dst[:len(src)]
	if alias.InexactOverlap(dst, src) {
		panic("chacha20: invalid buffer overlap")
	}

	// 首先，排空上一次 XORKeyStream 调用剩余的密钥流。
	if s.len != 0 {
		keyStream := s.buf[bufSize-s.len:]
		if len(src) < len(keyStream) {
			keyStream = keyStream[:len(src)]
		}
		_ = src[len(keyStream)-1] // 边界检查消除提示
		for i, b := range keyStream {
			dst[i] = src[i] ^ b
		}
		s.len -= len(keyStream)
		dst, src = dst[len(keyStream):], src[len(keyStream):]
	}
	if len(src) == 0 {
		return
	}

	// 如果需要让计数器溢出并继续生成输出，立即 panic。
	// 如果只是达到最后一个块，记住在缓冲区排空后不再生成更多输出。
	numBlocks := (uint64(len(src)) + blockSize - 1) / blockSize
	if s.overflow || uint64(s.counter)+numBlocks > 1<<32 {
		panic("chacha20: counter overflow")
	} else if uint64(s.counter)+numBlocks == 1<<32 {
		s.overflow = true
	}

	// xorKeyStreamBlocks 实现期望输入长度是 bufSize 的倍数。
	// 特定平台的实现一次处理多个块，因此 bufSize 是 blockSize 的倍数。

	full := len(src) - len(src)%bufSize
	if full > 0 {
		s.xorKeyStreamBlocks(dst[:full], src[:full])
	}
	dst, src = dst[full:], src[full:]

	// 如果使用多块 xorKeyStreamBlocks 会溢出，则使用一次处理一个块的通用实现。
	const blocksPerBuf = bufSize / blockSize
	if uint64(s.counter)+blocksPerBuf > 1<<32 {
		s.buf = [bufSize]byte{}
		numBlocks := (len(src) + blockSize - 1) / blockSize
		buf := s.buf[bufSize-numBlocks*blockSize:]
		copy(buf, src)
		s.xorKeyStreamBlocksGeneric(buf, buf)
		s.len = len(buf) - copy(dst, buf)
		return
	}

	// 如果有部分（多）块，为 xorKeyStreamBlocks 填充它，
	// 并保留剩余的密钥流用于下一次 XORKeyStream 调用。
	if len(src) > 0 {
		s.buf = [bufSize]byte{}
		copy(s.buf[:], src)
		s.xorKeyStreamBlocks(s.buf[:], s.buf[:])
		s.len = bufSize - copy(dst, s.buf[:])
	}
}

func (s *Cipher) xorKeyStreamBlocksGeneric(dst, src []byte) {
	if len(dst) != len(src) || len(dst)%blockSize != 0 {
		panic("chacha20: internal error: wrong dst and/or src length")
	}

	// 为了生成每个密钥流块，初始密码状态（如下所示）经过 20 轮混洗，
	// 交替按列（如 1、5、9、13）或按对角线（如 1、6、11、12）应用 quarterRounds。
	//
	//      0:cccccccc   1:cccccccc   2:cccccccc   3:cccccccc
	//      4:kkkkkkkk   5:kkkkkkkk   6:kkkkkkkk   7:kkkkkkkk
	//      8:kkkkkkkk   9:kkkkkkkk  10:kkkkkkkk  11:kkkkkkkk
	//     12:bbbbbbbb  13:nnnnnnnn  14:nnnnnnnn  15:nnnnnnnn
	//
	//            c=常量 k=密钥 b=块计数 n=随机数
	var (
		c0, c1, c2, c3   = j0, j1, j2, j3
		c4, c5, c6, c7   = s.key[0], s.key[1], s.key[2], s.key[3]
		c8, c9, c10, c11 = s.key[4], s.key[5], s.key[6], s.key[7]
		_, c13, c14, c15 = s.counter, s.nonce[0], s.nonce[1], s.nonce[2]
	)

	// 第一轮的四分之三不依赖于计数器，所以我们可以在这里计算它们，
	// 并在循环中的多个块以及未来的 XORKeyStream 调用中重用它们。
	if !s.precompDone {
		s.p1, s.p5, s.p9, s.p13 = quarterRound(c1, c5, c9, c13)
		s.p2, s.p6, s.p10, s.p14 = quarterRound(c2, c6, c10, c14)
		s.p3, s.p7, s.p11, s.p15 = quarterRound(c3, c7, c11, c15)
		s.precompDone = true
	}

	// len(src) > 0 的条件就足够了，但这也可以作为边界检查消除提示。
	for len(src) >= 64 && len(dst) >= 64 {
		// 第一列轮的剩余部分。
		fcr0, fcr4, fcr8, fcr12 := quarterRound(c0, c4, c8, s.counter)

		// 第二对角线轮。
		x0, x5, x10, x15 := quarterRound(fcr0, s.p5, s.p10, s.p15)
		x1, x6, x11, x12 := quarterRound(s.p1, s.p6, s.p11, fcr12)
		x2, x7, x8, x13 := quarterRound(s.p2, s.p7, fcr8, s.p13)
		x3, x4, x9, x14 := quarterRound(s.p3, fcr4, s.p9, s.p14)

		// 剩余的 18 轮。
		for i := 0; i < 9; i++ {
			// 列轮。
			x0, x4, x8, x12 = quarterRound(x0, x4, x8, x12)
			x1, x5, x9, x13 = quarterRound(x1, x5, x9, x13)
			x2, x6, x10, x14 = quarterRound(x2, x6, x10, x14)
			x3, x7, x11, x15 = quarterRound(x3, x7, x11, x15)

			// 对角线轮。
			x0, x5, x10, x15 = quarterRound(x0, x5, x10, x15)
			x1, x6, x11, x12 = quarterRound(x1, x6, x11, x12)
			x2, x7, x8, x13 = quarterRound(x2, x7, x8, x13)
			x3, x4, x9, x14 = quarterRound(x3, x4, x9, x14)
		}

		// 将初始状态加回以生成密钥流，然后将密钥流与源进行异或并写出结果。
		addXor(dst[0:4], src[0:4], x0, c0)
		addXor(dst[4:8], src[4:8], x1, c1)
		addXor(dst[8:12], src[8:12], x2, c2)
		addXor(dst[12:16], src[12:16], x3, c3)
		addXor(dst[16:20], src[16:20], x4, c4)
		addXor(dst[20:24], src[20:24], x5, c5)
		addXor(dst[24:28], src[24:28], x6, c6)
		addXor(dst[28:32], src[28:32], x7, c7)
		addXor(dst[32:36], src[32:36], x8, c8)
		addXor(dst[36:40], src[36:40], x9, c9)
		addXor(dst[40:44], src[40:44], x10, c10)
		addXor(dst[44:48], src[44:48], x11, c11)
		addXor(dst[48:52], src[48:52], x12, s.counter)
		addXor(dst[52:56], src[52:56], x13, c13)
		addXor(dst[56:60], src[56:60], x14, c14)
		addXor(dst[60:64], src[60:64], x15, c15)

		s.counter += 1

		src, dst = src[blockSize:], dst[blockSize:]
	}
}

// HChaCha20 使用 ChaCha20 核心从 32 字节密钥和 16 字节随机数生成派生密钥。
// 如果密钥或随机数长度不符合要求，则返回错误。它用作 XChaCha20 构造的一部分。
func HChaCha20(key, nonce []byte) ([]byte, error) {
	// 此函数被拆分为包装器，以便切片分配可以内联，
	// 并且根据调用者如何使用返回值，不会逃逸到堆上。
	out := make([]byte, 32)
	return hChaCha20(out, key, nonce)
}

func hChaCha20(out, key, nonce []byte) ([]byte, error) {
	if len(key) != KeySize {
		return nil, errors.New("chacha20: wrong HChaCha20 key size")
	}
	if len(nonce) != 16 {
		return nil, errors.New("chacha20: wrong HChaCha20 nonce size")
	}

	x0, x1, x2, x3 := j0, j1, j2, j3
	x4 := binary.LittleEndian.Uint32(key[0:4])
	x5 := binary.LittleEndian.Uint32(key[4:8])
	x6 := binary.LittleEndian.Uint32(key[8:12])
	x7 := binary.LittleEndian.Uint32(key[12:16])
	x8 := binary.LittleEndian.Uint32(key[16:20])
	x9 := binary.LittleEndian.Uint32(key[20:24])
	x10 := binary.LittleEndian.Uint32(key[24:28])
	x11 := binary.LittleEndian.Uint32(key[28:32])
	x12 := binary.LittleEndian.Uint32(nonce[0:4])
	x13 := binary.LittleEndian.Uint32(nonce[4:8])
	x14 := binary.LittleEndian.Uint32(nonce[8:12])
	x15 := binary.LittleEndian.Uint32(nonce[12:16])

	for i := 0; i < 10; i++ {
		// 对角线轮。
		x0, x4, x8, x12 = quarterRound(x0, x4, x8, x12)
		x1, x5, x9, x13 = quarterRound(x1, x5, x9, x13)
		x2, x6, x10, x14 = quarterRound(x2, x6, x10, x14)
		x3, x7, x11, x15 = quarterRound(x3, x7, x11, x15)

		// 列轮。
		x0, x5, x10, x15 = quarterRound(x0, x5, x10, x15)
		x1, x6, x11, x12 = quarterRound(x1, x6, x11, x12)
		x2, x7, x8, x13 = quarterRound(x2, x7, x8, x13)
		x3, x4, x9, x14 = quarterRound(x3, x4, x9, x14)
	}

	_ = out[31] // 边界检查消除提示
	binary.LittleEndian.PutUint32(out[0:4], x0)
	binary.LittleEndian.PutUint32(out[4:8], x1)
	binary.LittleEndian.PutUint32(out[8:12], x2)
	binary.LittleEndian.PutUint32(out[12:16], x3)
	binary.LittleEndian.PutUint32(out[16:20], x12)
	binary.LittleEndian.PutUint32(out[20:24], x13)
	binary.LittleEndian.PutUint32(out[24:28], x14)
	binary.LittleEndian.PutUint32(out[28:32], x15)
	return out, nil
}
