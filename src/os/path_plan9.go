// 版权所有 2011 The Go Authors。保留所有权利。
// 本源代码的使用受 BSD 风格许可证约束，
// 该许可证可在 LICENSE 文件中找到。

package os

const (
	PathSeparator     = '/'    // 特定于操作系统的路径分隔符
	PathListSeparator = '\000' // 特定于操作系统的路径列表分隔符
)

// IsPathSeparator 报告 c 是否为目录分隔符字符。
func IsPathSeparator(c uint8) bool {
	return PathSeparator == c
}
