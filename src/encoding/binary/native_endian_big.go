// 版权所有 2023 The Go Authors。保留所有权利。
// 本源代码的使用受 BSD 风格许可证约束，
// 该许可证可在 LICENSE 文件中找到。

//go:build armbe || arm64be || m68k || mips || mips64 || mips64p32 || ppc || ppc64 || s390 || s390x || shbe || sparc || sparc64

package binary

type nativeEndian struct {
	bigEndian
}

// NativeEndian 是 native-endian implementation of [ByteOrder] and [AppendByteOrder].
var NativeEndian nativeEndian
