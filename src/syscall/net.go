// 版权所有 2017 The Go Authors。保留所有权利。
// 本源代码的使用受 BSD 风格许可证约束，
// 该许可证可在 LICENSE 文件中找到。

package syscall

// RawConn 是一个原始网络连接。
type RawConn interface {
	// Control 在底层连接的文件描述符或句柄上调用 f。
	// 文件描述符 fd 保证在 f 执行期间保持有效，但在 f 返回后不再有效。
	Control(f func(fd uintptr)) error

	// Read 在底层连接的文件描述符或句柄上调用 f；
	// f 应该尝试从文件描述符读取。
	// 如果 f 返回 true，Read 返回。否则 Read 阻塞等待连接准备好读取，
	// 并反复重试。
	// 文件描述符保证在 f 执行期间保持有效，但在 f 返回后不再有效。
	Read(f func(fd uintptr) (done bool)) error

	// Write 类似于 Read，但用于写入。
	Write(f func(fd uintptr) (done bool)) error
}

// Conn 由 net 和 os 包中的一些类型实现，
// 以提供对底层文件描述符或句柄的访问。
type Conn interface {
	// SyscallConn 返回一个原始网络连接。
	SyscallConn() (RawConn, error)
}
