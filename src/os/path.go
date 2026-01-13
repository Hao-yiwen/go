// 版权所有 2009 The Go Authors。保留所有权利。
// 本源代码的使用受 BSD 风格许可证约束，
// 该许可证可在 LICENSE 文件中找到。

package os

import (
	"internal/filepathlite"
	"syscall"
)

// MkdirAll 创建名为 path 的目录以及任何必要的父目录，
// 并返回 nil，否则返回错误。
// 权限位 perm（应用 umask 之前）用于 MkdirAll 创建的
// 所有目录。
// 如果 path 已经是一个目录，MkdirAll 不执行任何操作并返回 nil。
func MkdirAll(path string, perm FileMode) error {
	// 快速路径：如果我们能判断 path 是目录还是文件，则以成功或错误结束。
	dir, err := Stat(path)
	if err == nil {
		if dir.IsDir() {
			return nil
		}
		return &PathError{Op: "mkdir", Path: path, Err: syscall.ENOTDIR}
	}

	// 慢速路径：确保父目录存在，然后为 path 调用 Mkdir。

	// 通过首先删除任何尾部路径分隔符，然后向后扫描直到
	// 找到路径分隔符或到达字符串开头，从 path 中提取父文件夹。
	i := len(path) - 1
	for i >= 0 && IsPathSeparator(path[i]) {
		i--
	}
	for i >= 0 && !IsPathSeparator(path[i]) {
		i--
	}
	if i < 0 {
		i = 0
	}

	// 如果存在父目录且它不是卷名，则递归以确保父目录存在。
	if parent := path[:i]; len(parent) > len(filepathlite.VolumeName(path)) {
		err = MkdirAll(parent, perm)
		if err != nil {
			return err
		}
	}

	// 父目录现在存在；调用 Mkdir 并使用其结果。
	err = Mkdir(path, perm)
	if err != nil {
		// 通过再次检查目录是否存在来处理类似 "foo/." 的参数。
		dir, err1 := Lstat(path)
		if err1 == nil && dir.IsDir() {
			return nil
		}
		return err
	}
	return nil
}

// RemoveAll 删除 path 及其包含的所有子项。
// 它删除所有能删除的内容，但返回遇到的第一个错误。
// 如果路径不存在，RemoveAll 返回 nil（无错误）。
// 如果发生错误，它的类型将是 [*PathError]。
func RemoveAll(path string) error {
	return removeAll(path)
}

// endsWithDot 报告 path 的最后一个组件是否是 "."。
func endsWithDot(path string) bool {
	if path == "." {
		return true
	}
	if len(path) >= 2 && path[len(path)-1] == '.' && IsPathSeparator(path[len(path)-2]) {
		return true
	}
	return false
}
