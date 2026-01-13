// 版权所有 2023 The Go Authors。保留所有权利。
// 本源代码的使用受 BSD 风格许可证约束，
// 该许可证可在 LICENSE 文件中找到。

//go:build 386 || amd64 || amd64p32 || alpha || arm || arm64 || loong64 || mipsle || mips64le || mips64p32le || nios2 || ppc64le || riscv || riscv64 || sh || wasm

package binary

type nativeEndian struct {
	littleEndian
}

// NativeEndian 是 native-endian implementation of [ByteOrder] and [AppendByteOrder].
var NativeEndian nativeEndian
