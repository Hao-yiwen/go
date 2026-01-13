// 版权所有 2023 The Go Authors。保留所有权利。
// 本源代码的使用受 BSD 风格许可证约束，
// 该许可证可在 LICENSE 文件中找到。

package abi

// 这些函数是 Go 类型数据结构的构建时版本。

// 它们的内容必须与其定义保持同步。
// 由于主机和目标类型大小可能不同，编译器和链接器无法使用
// 它们可能从 unsafe.Sizeof 和 Alignof、runtime、reflect 或 reflectlite
// 获取的主机信息。

// CommonSize 返回给定 ptrSize 的编译目标的 sizeof(Type)
func CommonSize(ptrSize int) int { return 4*ptrSize + 8 + 8 }

// StructFieldSize 返回给定 ptrSize 的编译目标的 sizeof(StructField)
func StructFieldSize(ptrSize int) int { return 3 * ptrSize }

// UncommonSize 返回 sizeof(UncommonType)。目前这不依赖于 ptrSize。
// 这个导出函数位于 internal 包中，因此将来可能会改为依赖于 ptrSize。
func UncommonSize() uint64 { return 4 + 2 + 2 + 4 + 4 }

// TFlagOff 返回给定 ptrSize 的编译目标的 Type.TFlag 偏移量
func TFlagOff(ptrSize int) int { return 2*ptrSize + 4 }

// ITabTypeOff 返回给定 ptrSize 的编译目标的 ITab.Type 偏移量
func ITabTypeOff(ptrSize int) int { return ptrSize }
