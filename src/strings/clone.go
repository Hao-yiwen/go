// 版权所有 2021 The Go Authors。保留所有权利。
// 本源代码的使用受 BSD 风格许可证约束，
// 该许可证可在 LICENSE 文件中找到。

package strings

import (
	"internal/stringslite"
)

// Clone 返回 s 的一个全新副本。
// 它保证将 s 复制到一个新的内存分配中，
// 这在只保留一个很大字符串中的小子串时非常重要。
// 使用 Clone 可以帮助这类程序使用更少的内存。
// 当然，由于使用 Clone 会创建副本，
// 过度使用 Clone 会使程序使用更多内存。
// Clone 通常应该很少使用，且只在
// 性能分析表明需要时才使用。
// 对于长度为零的字符串，将返回 "" 字符串
// 且不会进行内存分配。
func Clone(s string) string {
	return stringslite.Clone(s)
}
