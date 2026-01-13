// 版权所有 2012 The Go Authors。保留所有权利。
// 本源代码的使用受 BSD 风格许可证约束，
// 该许可证可在 LICENSE 文件中找到。

package os

// Hostname 返回内核报告的主机名。
func Hostname() (name string, err error) {
	return hostname()
}
