// 版权所有 2009 The Go Authors。保留所有权利。
// 本源代码的使用受 BSD 风格许可证约束，
// 该许可证可在 LICENSE 文件中找到。

// hash 包提供哈希函数的接口。
package hash

import "io"

// Hash 是所有哈希函数实现的通用接口。
//
// 标准库中的哈希实现（例如 [hash/crc32] 和
// [crypto/sha256]）实现了 [encoding.BinaryMarshaler]、[encoding.BinaryAppender]、
// [encoding.BinaryUnmarshaler] 和 [Cloner] 接口。对哈希实现进行序列化
// 可以保存其内部状态，以便稍后进行额外处理，
// 而无需重新写入之前写入哈希的数据。
// 哈希状态可能包含原始形式的输入部分，
// 用户应自行处理任何可能的安全影响。
//
// 兼容性：hash 或 crypto 包的任何未来更改都将努力
// 保持与使用先前版本编码的状态的兼容性。
// 也就是说，任何已发布版本的包都应该能够
// 解码由任何先前发布版本写入的数据，
// 但受安全修复等问题的影响。
// 有关背景，请参阅 Go 兼容性文档：https://golang.org/doc/go1compat
type Hash interface {
	// Write（通过嵌入的 io.Writer 接口）向正在运行的哈希添加更多数据。
	// 它永远不会返回错误。
	io.Writer

	// Sum 将当前哈希值追加到 b 并返回结果切片。
	// 它不会改变底层哈希状态。
	Sum(b []byte) []byte

	// Reset 将 Hash 重置为其初始状态。
	Reset()

	// Size 返回 Sum 将返回的字节数。
	Size() int

	// BlockSize 返回哈希的底层块大小。
	// Write 方法必须能够接受任意数量的数据，
	// 但如果所有写入都是块大小的倍数，
	// 它可能会更高效地运行。
	BlockSize() int
}

// Hash32 是所有 32 位哈希函数实现的通用接口。
type Hash32 interface {
	Hash
	Sum32() uint32
}

// Hash64 是所有 64 位哈希函数实现的通用接口。
type Hash64 interface {
	Hash
	Sum64() uint64
}

// Cloner 是一个哈希函数，其状态可以被克隆，
// 返回一个具有等效且独立状态的值。
//
// 标准库中的所有 [Hash] 实现都实现了此接口，
// 除非设置了 GOFIPS140=v1.0.0。
//
// 如果哈希只能在运行时确定是否可以被克隆（例如，如果它包装了
// 另一个哈希），Clone 可能会返回一个包装 [errors.ErrUnsupported] 的错误。
// 否则，Clone 必须始终返回 nil 错误。
type Cloner interface {
	Hash
	Clone() (Cloner, error)
}

// XOF（可扩展输出函数）是一种具有任意或无限输出长度的哈希函数。
type XOF interface {
	// Write 将更多数据吸收到 XOF 的状态中。如果在 Read 之后调用，它会 panic。
	io.Writer

	// Read 从 XOF 读取更多输出。如果 XOF 输出长度有限制，
	// 它可能会返回 io.EOF。
	io.Reader

	// Reset 将 XOF 重置为其初始状态。
	Reset()

	// BlockSize 返回 XOF 的底层块大小。
	// Write 方法必须能够接受任意数量的数据，
	// 但如果所有写入都是块大小的倍数，
	// 它可能会更高效地运行。
	BlockSize() int
}
