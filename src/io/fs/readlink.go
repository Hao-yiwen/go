// 版权所有 2023 The Go Authors。保留所有权利。
// 本源代码的使用受 BSD 风格许可证约束，
// 该许可证可在 LICENSE 文件中找到。

package fs

// ReadLinkFS 是由支持读取符号链接的文件系统实现的接口。
type ReadLinkFS interface {
	FS

	// ReadLink 返回指定符号链接的目标。
	// 如果发生错误，它应该是 [*PathError] 类型。
	ReadLink(name string) (string, error)

	// Lstat 返回描述指定文件的 [FileInfo]。
	// 如果文件是符号链接，返回的 [FileInfo] 描述符号链接本身。
	// Lstat 不尝试跟随链接。
	// 如果发生错误，它应该是 [*PathError] 类型。
	Lstat(name string) (FileInfo, error)
}

// ReadLink 返回指定符号链接的目标。
//
// 如果 fsys 没有实现 [ReadLinkFS]，则 ReadLink 返回错误。
func ReadLink(fsys FS, name string) (string, error) {
	sym, ok := fsys.(ReadLinkFS)
	if !ok {
		return "", &PathError{Op: "readlink", Path: name, Err: ErrInvalid}
	}
	return sym.ReadLink(name)
}

// Lstat 返回描述指定文件的 [FileInfo]。
// 如果文件是符号链接，返回的 [FileInfo] 描述符号链接本身。
// Lstat 不尝试跟随链接。
//
// 如果 fsys 没有实现 [ReadLinkFS]，则 Lstat 与 [Stat] 相同。
func Lstat(fsys FS, name string) (FileInfo, error) {
	sym, ok := fsys.(ReadLinkFS)
	if !ok {
		return Stat(fsys, name)
	}
	return sym.Lstat(name)
}
