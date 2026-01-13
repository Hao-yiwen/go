// 版权所有 2017 The Go Authors。保留所有权利。
// 本源代码的使用受 BSD 风格许可证约束，
// 该许可证可在 LICENSE 文件中找到。

package os

import "internal/testlog"

// Stat 返回描述指定文件的 [FileInfo]。
// 如果发生错误，它的类型将是 [*PathError]。
func Stat(name string) (FileInfo, error) {
	testlog.Stat(name)
	return statNolog(name)
}

// Lstat 返回描述指定文件的 [FileInfo]。
// 如果文件是符号链接，返回的 FileInfo 描述符号链接。
// Lstat 不会尝试跟随链接。
// 如果发生错误，它的类型将是 [*PathError]。
//
// 在 Windows 上，如果文件是作为另一个命名实体（如符号链接或
// 挂载文件夹）代理的重解析点，返回的 FileInfo 描述该重解析点，
// 并且不会尝试解析它。
func Lstat(name string) (FileInfo, error) {
	testlog.Stat(name)
	return lstatNolog(name)
}
