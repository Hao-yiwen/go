// 版权所有 2010 The Go Authors。保留所有权利。
// 本源代码的使用受 BSD 风格许可证约束，
// 该许可证可在 LICENSE 文件中找到。

package ioutil

import (
	"os"
)

// TempFile 在目录 dir 中创建一个新的临时文件，
// 打开该文件用于读写，并返回生成的 *[os.File]。
// 文件名通过获取 pattern 并在末尾添加随机字符串来生成。
// 如果 pattern 包含 "*"，随机字符串将替换最后一个 "*"。
// 如果 dir 是空字符串，TempFile 使用默认的临时文件目录
// （参见 [os.TempDir]）。
// 同时调用 TempFile 的多个程序不会选择相同的文件。
// 调用者可以使用 f.Name() 来获取文件的路径名。
// 当不再需要时，移除该文件是调用者的责任。
//
// 已弃用：从 Go 1.17 开始，此函数只是简单调用 [os.CreateTemp]。
//
//go:fix inline
func TempFile(dir, pattern string) (f *os.File, err error) {
	return os.CreateTemp(dir, pattern)
}

// TempDir 在目录 dir 中创建一个新的临时目录。
// 目录名通过获取 pattern 并在末尾应用随机字符串来生成。
// 如果 pattern 包含 "*"，随机字符串将替换最后一个 "*"。
// TempDir 返回新目录的名称。
// 如果 dir 是空字符串，TempDir 使用默认的临时文件目录
// （参见 [os.TempDir]）。
// 同时调用 TempDir 的多个程序不会选择相同的目录。
// 当不再需要时，移除该目录是调用者的责任。
//
// 已弃用：从 Go 1.17 开始，此函数只是简单调用 [os.MkdirTemp]。
//
//go:fix inline
func TempDir(dir, pattern string) (name string, err error) {
	return os.MkdirTemp(dir, pattern)
}
