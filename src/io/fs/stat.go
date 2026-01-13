// 版权所有 2020 The Go Authors。保留所有权利。
// 本源代码的使用受 BSD 风格许可证约束，
// 该许可证可在 LICENSE 文件中找到。

package fs

// StatFS 是具有 Stat 方法的文件系统。
type StatFS interface {
	FS

	// Stat 返回描述文件的 FileInfo。
	// 如果发生错误，它应该是 *PathError 类型。
	Stat(name string) (FileInfo, error)
}

// Stat 从文件系统返回描述指定文件的 [FileInfo]。
//
// 如果 fs 实现了 [StatFS]，Stat 将调用 fs.Stat。
// 否则，Stat 打开 [File] 以对其进行 stat。
func Stat(fsys FS, name string) (FileInfo, error) {
	if fsys, ok := fsys.(StatFS); ok {
		return fsys.Stat(name)
	}

	file, err := fsys.Open(name)
	if err != nil {
		return nil, err
	}
	defer file.Close()
	return file.Stat()
}
