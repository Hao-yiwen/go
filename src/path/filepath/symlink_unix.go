// 版权所有 2014 The Go Authors。保留所有权利。
// 本源代码的使用受 BSD 风格许可证约束，
// 该许可证可在 LICENSE 文件中找到。

//go:build !windows && !plan9

package filepath

func evalSymlinks(path string) (string, error) {
	return walkSymlinks(path)
}
