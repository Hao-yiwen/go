// 版权所有 2019 The Go Authors。保留所有权利。
// 本源代码的使用受 BSD 风格许可证约束，
// 该许可证可在 LICENSE 文件中找到。

// maphash 包提供字节序列和可比较值的哈希函数。
// 这些哈希函数旨在用于实现哈希表或其他需要将任意字符串
// 或字节序列映射到无符号 64 位整数均匀分布的数据结构。
// 每个不同的哈希表或数据结构实例应使用自己的 [Seed]。
//
// 这些哈希函数不是加密安全的。
//（加密用途请参阅 crypto/sha256 和 crypto/sha512。）
package maphash

import (
	"hash"
	"internal/abi"
)

// Seed 是一个随机值，用于选择 [Hash] 计算的特定哈希函数。
// 如果两个 Hash 使用相同的 Seed，它们将为任何给定输入计算相同的哈希值。
// 如果两个 Hash 使用不同的 Seed，它们很可能为任何给定输入计算不同的哈希值。
//
// Seed 必须通过调用 [MakeSeed] 初始化。
// 零值 seed 是未初始化的，不能用于 [Hash] 的 SetSeed 方法。
//
// 每个 Seed 值是单个进程本地的，不能被序列化或在不同进程中重新创建。
type Seed struct {
	s uint64
}

// Bytes 返回使用给定 seed 的 b 的哈希值。
//
// Bytes 等效于以下代码，但更方便和高效：
//
//	var h Hash
//	h.SetSeed(seed)
//	h.Write(b)
//	return h.Sum64()
func Bytes(seed Seed, b []byte) uint64 {
	state := seed.s
	if state == 0 {
		panic("maphash: use of uninitialized Seed")
	}

	if len(b) > bufSize {
		b = b[:len(b):len(b)] // merge len and cap calculations when reslicing
		for len(b) > bufSize {
			state = rthash(b[:bufSize], state)
			b = b[bufSize:]
		}
	}
	return rthash(b, state)
}

// String 返回使用给定 seed 的 s 的哈希值。
//
// String 等效于以下代码，但更方便和高效：
//
//	var h Hash
//	h.SetSeed(seed)
//	h.WriteString(s)
//	return h.Sum64()
func String(seed Seed, s string) uint64 {
	state := seed.s
	if state == 0 {
		panic("maphash: use of uninitialized Seed")
	}
	for len(s) > bufSize {
		state = rthashString(s[:bufSize], state)
		s = s[bufSize:]
	}
	return rthashString(s, state)
}

// Hash 计算字节序列的种子哈希。
//
// 零值 Hash 是一个可以立即使用的有效 Hash。
// 零值 Hash 在第一次调用 Reset、Write、Seed、Clone 或 Sum64 方法时
// 会为自己选择一个随机种子。
// 要控制种子，请使用 SetSeed。
//
// 计算的哈希值仅取决于初始种子和提供给 Hash 对象的字节序列，
// 而不取决于字节的提供方式。例如，以下三个序列
//
//	h.Write([]byte{'f','o','o'})
//	h.WriteByte('f'); h.WriteByte('o'); h.WriteByte('o')
//	h.WriteString("foo")
//
// 都具有相同的效果。
//
// Hash 旨在抗碰撞，即使在对手控制被哈希的字节序列的情况下也是如此。
//
// Hash 不能被多个 goroutine 安全地并发使用，但 Seed 可以。
// 如果多个 goroutine 必须计算相同的种子哈希，
// 每个都可以声明自己的 Hash 并使用共同的 Seed 调用 SetSeed。
type Hash struct {
	_     [0]func()     // 不可比较
	seed  Seed          // 用于此哈希的初始种子
	state Seed          // 所有已刷新字节的当前哈希
	buf   [bufSize]byte // 未刷新的字节缓冲区
	n     int           // 未刷新字节的数量
}

// bufSize 是 Hash 写入缓冲区的大小。
// 缓冲区通过始终使用完整缓冲区调用 rthash（尾部除外），
// 确保写入仅取决于字节序列，而不取决于 WriteByte/Write/WriteString 调用的序列。
const bufSize = 128

// initSeed 在必要时为哈希设置种子。
// initSeed 在任何实际使用 h.seed/h.state 的操作之前延迟调用。
// 请注意，在仅添加到 h.buf 的情况下，这不包括 Write/WriteByte/WriteString。
//（如果它们写入太多，它们会调用 h.flush，h.flush 会调用 h.initSeed。）
func (h *Hash) initSeed() {
	if h.seed.s == 0 {
		seed := MakeSeed()
		h.seed = seed
		h.state = seed
	}
}

// WriteByte 将 b 添加到 h 正在哈希的字节序列中。
// 它永远不会失败；error 返回值是为了实现 [io.ByteWriter]。
func (h *Hash) WriteByte(b byte) error {
	if h.n == len(h.buf) {
		h.flush()
	}
	h.buf[h.n] = b
	h.n++
	return nil
}

// Write 将 b 添加到 h 正在哈希的字节序列中。
// 它总是写入所有的 b 且永远不会失败；count 和 error 返回值是为了实现 [io.Writer]。
func (h *Hash) Write(b []byte) (int, error) {
	size := len(b)
	// 处理 h.buf 中剩余的字节。
	// h.n <= bufSize 始终为真。
	// 检查它几乎是免费的，并且可以让编译器消除边界检查。
	if h.n > 0 && h.n <= bufSize {
		k := copy(h.buf[h.n:], b)
		h.n += k
		if h.n < bufSize {
			// 将 b 的全部内容复制到 h.buf。
			return size, nil
		}
		b = b[k:]
		h.flush()
		// 这里不需要设置 h.n = 0；它在退出前就会发生。
	}
	// 尽可能多地处理完整缓冲区，无需复制，且只调用一次 initSeed。
	if len(b) > bufSize {
		h.initSeed()
		for len(b) > bufSize {
			h.state.s = rthash(b[:bufSize], h.state.s)
			b = b[bufSize:]
		}
	}
	// 复制尾部。
	copy(h.buf[:], b)
	h.n = len(b)
	return size, nil
}

// WriteString 将 s 的字节添加到 h 正在哈希的字节序列中。
// 它总是写入所有的 s 且永远不会失败；count 和 error 返回值是为了实现 [io.StringWriter]。
func (h *Hash) WriteString(s string) (int, error) {
	// WriteString 镜像 Write。有关注释，请参阅 Write。
	size := len(s)
	if h.n > 0 && h.n <= bufSize {
		k := copy(h.buf[h.n:], s)
		h.n += k
		if h.n < bufSize {
			return size, nil
		}
		s = s[k:]
		h.flush()
	}
	if len(s) > bufSize {
		h.initSeed()
		for len(s) > bufSize {
			h.state.s = rthashString(s[:bufSize], h.state.s)
			s = s[bufSize:]
		}
	}
	copy(h.buf[:], s)
	h.n = len(s)
	return size, nil
}

// Seed 返回 h 的种子值。
func (h *Hash) Seed() Seed {
	h.initSeed()
	return h.seed
}

// SetSeed 将 h 设置为使用 seed，该 seed 必须由 [MakeSeed] 或
// 另一个 [Hash.Seed] 方法返回。
// 具有相同 seed 的两个 [Hash] 对象行为相同。
// 具有不同 seed 的两个 [Hash] 对象很可能行为不同。
// 在此调用之前添加到 h 的任何字节都将被丢弃。
func (h *Hash) SetSeed(seed Seed) {
	if seed.s == 0 {
		panic("maphash: use of uninitialized Seed")
	}
	h.seed = seed
	h.state = seed
	h.n = 0
}

// Reset 丢弃添加到 h 的所有字节。
//（种子保持不变。）
func (h *Hash) Reset() {
	h.initSeed()
	h.state = h.seed
	h.n = 0
}

// 前置条件：缓冲区已满。
func (h *Hash) flush() {
	if h.n != len(h.buf) {
		panic("maphash: flush of partially full buffer")
	}
	h.initSeed()
	h.state.s = rthash(h.buf[:h.n], h.state.s)
	h.n = 0
}

// Sum64 返回 h 的当前 64 位值，该值取决于
// h 的种子和自上次调用 [Hash.Reset] 或 [Hash.SetSeed] 以来
// 添加到 h 的字节序列。
//
// Sum64 结果的所有位都接近均匀且独立分布，
// 因此可以通过使用位掩码、移位或模运算安全地减少。
func (h *Hash) Sum64() uint64 {
	h.initSeed()
	return rthash(h.buf[:h.n], h.state.s)
}

// MakeSeed 返回一个新的随机种子。
func MakeSeed() Seed {
	var s uint64
	for {
		s = randUint64()
		// 我们使用种子 0 表示未初始化的 seed/hash，
		// 所以继续尝试直到获得非零种子。
		if s != 0 {
			break
		}
	}
	return Seed{s: s}
}

// Sum 将哈希的当前 64 位值追加到 b。
// 它存在是为了实现 [hash.Hash]。
// 对于直接调用，使用 [Hash.Sum64] 更高效。
func (h *Hash) Sum(b []byte) []byte {
	x := h.Sum64()
	return append(b,
		byte(x>>0),
		byte(x>>8),
		byte(x>>16),
		byte(x>>24),
		byte(x>>32),
		byte(x>>40),
		byte(x>>48),
		byte(x>>56))
}

// Size 返回 h 的哈希值大小，8 字节。
func (h *Hash) Size() int { return 8 }

// BlockSize 返回 h 的块大小。
func (h *Hash) BlockSize() int { return len(h.buf) }

// Clone 实现 [hash.Cloner]。
func (h *Hash) Clone() (hash.Cloner, error) {
	h.initSeed()
	r := *h
	return &r, nil
}

// Comparable 返回使用给定 seed 的可比较值 v 的哈希，
// 使得如果 v1 == v2，则 Comparable(s, v1) == Comparable(s, v2)。
// 如果 v != v，则结果哈希是随机分布的。
func Comparable[T comparable](seed Seed, v T) uint64 {
	abi.EscapeNonString(v)
	return comparableHash(v, seed)
}

// WriteComparable 将 x 添加到 h 正在哈希的数据中。
func WriteComparable[T comparable](h *Hash, x T) {
	abi.EscapeNonString(x)
	// writeComparable（非 purego 模式）直接操作 h.state
	// 而不使用 h.buf。混入缓冲区长度，使其不会与
	// 缓冲写入交换，后者会改变 h.n 或改变 h.state。
	if h.n != 0 {
		writeComparable(h, h.n)
	}
	writeComparable(h, x)
}
