// 版权所有 2015 The Go Authors。保留所有权利。
// 本源代码的使用受 BSD 风格许可证约束，
// 该许可证可在 LICENSE 文件中找到。

package strings

import "internal/bytealg"

// Compare 返回一个整数，按字典序比较两个字符串。
// 如果 a == b，结果为 0；如果 a < b，结果为 -1；如果 a > b，结果为 +1。
//
// 当需要执行三路比较时使用 Compare（例如与
// [slices.SortFunc] 一起使用）。使用内置的字符串比较运算符
// ==、<、> 等通常更清晰且总是更快。
func Compare(a, b string) int {
	return bytealg.CompareString(a, b)
}
