// 版权所有 2020 The Go Authors。保留所有权利。
// 本源代码的使用受 BSD 风格许可证约束，
// 该许可证可在 LICENSE 文件中找到。

package fs

import (
	"errors"
	"internal/bytealg"
	"slices"
)

// ReadDirFS 是由文件系统实现的接口，
// 该接口提供了 [ReadDir] 的优化实现。
type ReadDirFS interface {
	FS

	// ReadDir 读取指定的目录，
	// 并返回按文件名排序的目录项列表。
	ReadDir(name string) ([]DirEntry, error)
}

// ReadDir 读取指定的目录，
// 并返回按文件名排序的目录项列表。
//
// 如果 fs 实现了 [ReadDirFS]，ReadDir 将调用 fs.ReadDir。
// 否则 ReadDir 将调用 fs.Open 并在返回的文件上
// 使用 ReadDir 和 Close。
func ReadDir(fsys FS, name string) ([]DirEntry, error) {
	if fsys, ok := fsys.(ReadDirFS); ok {
		return fsys.ReadDir(name)
	}

	file, err := fsys.Open(name)
	if err != nil {
		return nil, err
	}
	defer file.Close()

	dir, ok := file.(ReadDirFile)
	if !ok {
		return nil, &PathError{Op: "readdir", Path: name, Err: errors.New("not implemented")}
	}

	list, err := dir.ReadDir(-1)
	slices.SortFunc(list, func(a, b DirEntry) int {
		return bytealg.CompareString(a.Name(), b.Name())
	})
	return list, err
}

// dirInfo 是基于 FileInfo 的 DirEntry。
type dirInfo struct {
	fileInfo FileInfo
}

func (di dirInfo) IsDir() bool {
	return di.fileInfo.IsDir()
}

func (di dirInfo) Type() FileMode {
	return di.fileInfo.Mode().Type()
}

func (di dirInfo) Info() (FileInfo, error) {
	return di.fileInfo, nil
}

func (di dirInfo) Name() string {
	return di.fileInfo.Name()
}

func (di dirInfo) String() string {
	return FormatDirEntry(di)
}

// FileInfoToDirEntry 返回一个 [DirEntry]，该 [DirEntry] 返回来自 info 的信息。
// 如果 info 为 nil，FileInfoToDirEntry 返回 nil。
func FileInfoToDirEntry(info FileInfo) DirEntry {
	if info == nil {
		return nil
	}
	return dirInfo{fileInfo: info}
}
