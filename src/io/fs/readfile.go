// 版权所有 2020 The Go Authors。保留所有权利。
// 本源代码的使用受 BSD 风格许可证约束，
// 该许可证可在 LICENSE 文件中找到。

package fs

import "io"

// ReadFileFS 是由文件系统实现的接口，
// 该接口提供了 [ReadFile] 的优化实现。
type ReadFileFS interface {
	FS

	// ReadFile 读取指定的文件并返回其内容。
	// 成功的调用返回 nil 错误，而不是 io.EOF。
	// （因为 ReadFile 读取整个文件，
	// 来自最后一次 Read 的预期 EOF 不被视为需要报告的错误。）
	//
	// 调用者被允许修改返回的字节切片。
	// 此方法应返回底层数据的副本。
	ReadFile(name string) ([]byte, error)
}

// ReadFile 从文件系统 fs 读取指定的文件并返回其内容。
// 成功的调用返回 nil 错误，而不是 [io.EOF]。
// （因为 ReadFile 读取整个文件，
// 来自最后一次 Read 的预期 EOF 不被视为需要报告的错误。）
//
// 如果 fs 实现了 [ReadFileFS]，ReadFile 将调用 fs.ReadFile。
// 否则 ReadFile 将调用 fs.Open 并在返回的 [File] 上使用
// Read 和 Close。
func ReadFile(fsys FS, name string) ([]byte, error) {
	if fsys, ok := fsys.(ReadFileFS); ok {
		return fsys.ReadFile(name)
	}

	file, err := fsys.Open(name)
	if err != nil {
		return nil, err
	}
	defer file.Close()

	var size int
	if info, err := file.Stat(); err == nil {
		size64 := info.Size()
		if int64(int(size64)) == size64 {
			size = int(size64)
		}
	}

	data := make([]byte, 0, size+1)
	for {
		if len(data) >= cap(data) {
			d := append(data[:cap(data)], 0)
			data = d[:len(data)]
		}
		n, err := file.Read(data[len(data):cap(data)])
		data = data[:len(data)+n]
		if err != nil {
			if err == io.EOF {
				err = nil
			}
			return data, err
		}
	}
}
