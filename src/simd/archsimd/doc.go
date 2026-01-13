// 版权所有 2025 The Go Authors。保留所有权利。
// 本源代码的使用受 BSD 风格许可证约束，
// 该许可证可在 LICENSE 文件中找到。

//go:build goexperiment.simd

// Package archsimd 提供对特定于体系结构的 SIMD 操作的访问。
//
// 这是一个低级别的包，公开硬件特定的功能。
// 它目前支持 AMD64。
//
// 此包是实验性的，不受 Go 1 兼容性承诺的约束。
// 它仅在使用 GOEXPERIMENT=simd 环境变量设置构建时存在。
//
// # 向量类型和操作
//
// 向量类型定义为结构体，如 Int8x16 和 Float64x8，对应于
// 硬件的向量寄存器。在 AMD64 上，支持 128 位、256 位和 512 位向量。
//
// 掩码类型定义类似，如 Mask8x16，表示为
// 不透明类型，处理底层表示中的差异。
// 掩码可以转换为/从相应的整数向量类型，或
// 转换为/从位掩码。
//
// 操作主要定义为向量类型的方法。其中大多数
// 是编译器内在函数，直接对应于硬件指令。
//
// 常见操作包括：
//   - Load/Store：从内存加载向量或将向量存储到内存。
//   - 算术：Add、Sub、Mul 等。
//   - 按位：And、Or、Xor 等。
//   - 比较：Equal、Greater 等，产生掩码。
//   - 转换：在不同向量类型之间转换。
//   - 字段选择和重新排列：GetElem、Permute 等。
//   - 掩码：Masked、Merge。
//
// 编译器识别某些操作模式，可能会优化
// 它们为更高效的指令。例如，在 AVX512 上，Add 操作
// 后跟 Masked 可能会优化为掩码 add 指令。
// 出于这个原因，并非所有硬件指令都作为 API 提供。
//
// # CPU 功能检查
//
// 该包提供全局变量来检查运行时可用的 CPU 功能。
// 例如，在 AMD64 上，[X86] 变量提供检查
// AVX2、AVX512 等的方法。
// 建议在使用相应的
// 向量操作之前检查 CPU 功能。
//
// # Notes
//
//   - This package is not portable, as the available types and operations depend
//     on the target architecture. It is not recommended to expose the SIMD types
//     defined in this package in public APIs.
//   - For performance reasons, it is recommended to use the vector types directly
//     as values. It is not recommended to take the address of a vector type,
//     allocate it in the heap, or put it in an aggregate type.
package archsimd

// BUG(cherry): Using a vector type as a type parameter may not work.

// BUG(cherry): Using reflect Call to call a vector function/method may not work.
