// 版权所有 2023 The Go Authors。保留所有权利。
// 本源代码的使用受 BSD 风格许可证约束，
// 该许可证可在 LICENSE 文件中找到。

package abi

const (
	// StackNosplitBase 是 NOSPLIT 函数链可以使用的基础最大字节数。
	//
	// 此值必须乘以栈保护乘数，因此不要直接使用它。
	// 参见 runtime/stack.go:stackNosplit 和 cmd/internal/objabi/stack.go:StackNosplit。
	StackNosplitBase = 800

	// 根据函数的栈帧是小、大还是巨大，我们有三种不同的栈边界检查序列。

	// 栈分裂检查后，允许 SP 在栈保护之下 StackSmall 字节。
	//
	// 需要帧 <= StackSmall 的函数可以直接在栈保护和 SP 之间
	// 使用单次比较来执行栈检查，因为我们确保栈保护之外有
	// StackSmall 字节的栈空间可用。
	StackSmall = 128

	// 需要帧 <= StackBig 的函数可以假定 SP-framesize 和
	// stackGuard-StackSmall 都不会下溢，因此可以使用更高效的检查。
	// 为确保这一点，StackBig 必须 <= 零地址处未映射空间的大小。
	StackBig = 4096
)
