// 版权所有 2009 The Go Authors。保留所有权利。
// 本源代码的使用受 BSD 风格许可证约束，
// 该许可证可在 LICENSE 文件中找到。

// ioutil 包实现了一些 I/O 实用函数。
//
// 已弃用：从 Go 1.16 开始，相同的功能现在由
// [io] 包或 [os] 包提供，新代码中应优先使用这些实现。
// 有关详细信息，请参阅具体函数文档。
package ioutil

import (
	"io"
	"io/fs"
	"os"
	"slices"
	"strings"
)

// ReadAll 从 r 读取直到发生错误或 EOF，并返回读取的数据。
// 成功的调用返回 err == nil，而不是 err == EOF。因为 ReadAll
// 被定义为从 src 读取直到 EOF，所以它不会将 Read 返回的 EOF
// 视为需要报告的错误。
//
// 已弃用：从 Go 1.16 开始，此函数只是简单调用 [io.ReadAll]。
//
//go:fix inline
func ReadAll(r io.Reader) ([]byte, error) {
	return io.ReadAll(r)
}

// ReadFile 读取由 filename 指定的文件并返回其内容。
// 成功的调用返回 err == nil，而不是 err == EOF。因为 ReadFile
// 读取整个文件，所以它不会将 Read 返回的 EOF 视为需要报告的错误。
//
// 已弃用：从 Go 1.16 开始，此函数只是简单调用 [os.ReadFile]。
//
//go:fix inline
func ReadFile(filename string) ([]byte, error) {
	return os.ReadFile(filename)
}

// WriteFile 将数据写入由 filename 指定的文件。
// 如果文件不存在，WriteFile 会用权限 perm（在 umask 之前）创建它；
// 否则 WriteFile 会在写入前截断它，而不改变权限。
//
// 已弃用：从 Go 1.16 开始，此函数只是简单调用 [os.WriteFile]。
//
//go:fix inline
func WriteFile(filename string, data []byte, perm fs.FileMode) error {
	return os.WriteFile(filename, data, perm)
}

// ReadDir 读取由 dirname 指定的目录并返回
// 该目录内容的 fs.FileInfo 列表，按文件名排序。
// 如果读取目录时发生错误，ReadDir 不返回任何目录条目，同时返回错误。
//
// 已弃用：从 Go 1.16 开始，[os.ReadDir] 是更高效和正确的选择：
// 它返回 [fs.DirEntry] 列表而不是 [fs.FileInfo]，
// 并且在读取目录中途发生错误时返回部分结果。
//
// 如果您必须继续获取 [fs.FileInfo] 列表，您仍然可以：
//
//	entries, err := os.ReadDir(dirname)
//	if err != nil { ... }
//	infos := make([]fs.FileInfo, 0, len(entries))
//	for _, entry := range entries {
//		info, err := entry.Info()
//		if err != nil { ... }
//		infos = append(infos, info)
//	}
func ReadDir(dirname string) ([]fs.FileInfo, error) {
	f, err := os.Open(dirname)
	if err != nil {
		return nil, err
	}
	list, err := f.Readdir(-1)
	f.Close()
	if err != nil {
		return nil, err
	}
	slices.SortFunc(list, func(a, b os.FileInfo) int {
		return strings.Compare(a.Name(), b.Name())
	})
	return list, nil
}

// NopCloser 返回一个 ReadCloser，它包装了提供的 Reader r
// 并带有一个空操作的 Close 方法。
//
// 已弃用：从 Go 1.16 开始，此函数只是简单调用 [io.NopCloser]。
//
//go:fix inline
func NopCloser(r io.Reader) io.ReadCloser {
	return io.NopCloser(r)
}

// Discard 是一个 io.Writer，所有的 Write 调用都会成功
// 而不做任何事情。
//
// 已弃用：从 Go 1.16 开始，此值就是 [io.Discard]。
var Discard io.Writer = io.Discard
