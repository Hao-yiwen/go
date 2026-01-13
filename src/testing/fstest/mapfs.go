// 版权所有 2020 The Go Authors。保留所有权利。
// 本源代码的使用受 BSD 风格许可证约束，
// 该许可证可在 LICENSE 文件中找到。

package fstest

import (
	"io"
	"io/fs"
	"path"
	"slices"
	"strings"
	"time"
)

// MapFS 是一个简单的内存文件系统，用于测试，
// 表示为从路径名称（Open 的参数）
// 到关于它们代表的文件、目录或符号链接的信息的映射。
//
// 映射不需要为映射中包含的文件包括父目录；
// 如果需要，这些将被合成。
// 但目录仍然可以通过设置 [MapFile.Mode] 的 [fs.ModeDir] 位来包含；
// 这可能对于详细控制目录的 [fs.FileInfo]
// 或创建空目录是必要的。
//
// 文件系统操作直接从映射读取，
// 这样可以根据需要通过编辑映射来更改文件系统。
// 这意味着文件系统操作必须不与映射的更改并发运行，
// 这会导致竞态条件。
// 另一个含义是打开或读取目录需要
// 遍历整个映射，所以 MapFS 通常应该用于不超过
// 几百个条目或目录读取的情况。
type MapFS map[string]*MapFile

// MapFile 描述 [MapFS] 中的单个文件。
type MapFile struct {
	Data    []byte      // 文件内容或符号链接目标
	Mode    fs.FileMode // fs.FileInfo.Mode
	ModTime time.Time   // fs.FileInfo.ModTime
	Sys     any         // fs.FileInfo.Sys
}

var _ fs.FS = MapFS(nil)
var _ fs.ReadLinkFS = MapFS(nil)
var _ fs.File = (*openMapFile)(nil)

// Open 在跟随任何符号链接后打开指定的文件。
func (fsys MapFS) Open(name string) (fs.File, error) {
	if !fs.ValidPath(name) {
		return nil, &fs.PathError{Op: "open", Path: name, Err: fs.ErrNotExist}
	}
	realName, ok := fsys.resolveSymlinks(name)
	if !ok {
		return nil, &fs.PathError{Op: "open", Path: name, Err: fs.ErrNotExist}
	}

	file := fsys[realName]
	if file != nil && file.Mode&fs.ModeDir == 0 {
		// 普通文件
		return &openMapFile{name, mapFileInfo{path.Base(name), file}, 0}, nil
	}

	// 目录，可能被合成。
	// 注意 file 在这里可以是 nil：映射不需要为其所有文件包含显式父目录。
	// 但 file 也可以是非 nil，以防用户想要为目录显式设置元数据。
	// 无论哪种方式，我们需要构造该目录的子项列表。
	var list []mapFileInfo
	var need = make(map[string]bool)
	if realName == "." {
		for fname, f := range fsys {
			i := strings.Index(fname, "/")
			if i < 0 {
				if fname != "." {
					list = append(list, mapFileInfo{fname, f})
				}
			} else {
				need[fname[:i]] = true
			}
		}
	} else {
		prefix := realName + "/"
		for fname, f := range fsys {
			if strings.HasPrefix(fname, prefix) {
				felem := fname[len(prefix):]
				i := strings.Index(felem, "/")
				if i < 0 {
					list = append(list, mapFileInfo{felem, f})
				} else {
					need[fname[len(prefix):len(prefix)+i]] = true
				}
			}
		}
		// 如果目录名不在映射中，
		// 并且映射中没有该名称的子项，
		// 则将目录视为不存在。
		if file == nil && list == nil && len(need) == 0 {
			return nil, &fs.PathError{Op: "open", Path: name, Err: fs.ErrNotExist}
		}
	}
	for _, fi := range list {
		delete(need, fi.name)
	}
	for name := range need {
		list = append(list, mapFileInfo{name, &MapFile{Mode: fs.ModeDir | 0555}})
	}
	slices.SortFunc(list, func(a, b mapFileInfo) int {
		return strings.Compare(a.name, b.name)
	})

	if file == nil {
		file = &MapFile{Mode: fs.ModeDir | 0555}
	}
	var elem string
	if name == "." {
		elem = "."
	} else {
		elem = name[strings.LastIndex(name, "/")+1:]
	}
	return &mapDir{name, mapFileInfo{elem, file}, list, 0}, nil
}

func (fsys MapFS) resolveSymlinks(name string) (_ string, ok bool) {
	// 快速路径：如果符号链接在映射中，解析它。
	if file := fsys[name]; file != nil && file.Mode.Type() == fs.ModeSymlink {
		target := string(file.Data)
		if path.IsAbs(target) {
			return "", false
		}
		return fsys.resolveSymlinks(path.Join(path.Dir(name), target))
	}

	// 检查每个父目录（从根开始）是否是符号链接。
	for i := 0; i < len(name); {
		j := strings.Index(name[i:], "/")
		var dir string
		if j < 0 {
			dir = name
			i = len(name)
		} else {
			dir = name[:i+j]
			i += j
		}
		if file := fsys[dir]; file != nil && file.Mode.Type() == fs.ModeSymlink {
			target := string(file.Data)
			if path.IsAbs(target) {
				return "", false
			}
			return fsys.resolveSymlinks(path.Join(path.Dir(dir), target) + name[i:])
		}
		i += len("/")
	}
	return name, fs.ValidPath(name)
}

// ReadLink 返回指定符号链接的目标。
func (fsys MapFS) ReadLink(name string) (string, error) {
	info, err := fsys.lstat(name)
	if err != nil {
		return "", &fs.PathError{Op: "readlink", Path: name, Err: err}
	}
	if info.f.Mode.Type() != fs.ModeSymlink {
		return "", &fs.PathError{Op: "readlink", Path: name, Err: fs.ErrInvalid}
	}
	return string(info.f.Data), nil
}

// Lstat 返回描述指定文件的 FileInfo。
// 如果文件是符号链接，返回的 FileInfo 描述符号链接。
// Lstat 不尝试跟随链接。
func (fsys MapFS) Lstat(name string) (fs.FileInfo, error) {
	info, err := fsys.lstat(name)
	if err != nil {
		return nil, &fs.PathError{Op: "lstat", Path: name, Err: err}
	}
	return info, nil
}

func (fsys MapFS) lstat(name string) (*mapFileInfo, error) {
	if !fs.ValidPath(name) {
		return nil, fs.ErrNotExist
	}
	realDir, ok := fsys.resolveSymlinks(path.Dir(name))
	if !ok {
		return nil, fs.ErrNotExist
	}
	elem := path.Base(name)
	realName := path.Join(realDir, elem)

	file := fsys[realName]
	if file != nil {
		return &mapFileInfo{elem, file}, nil
	}

	if realName == "." {
		return &mapFileInfo{elem, &MapFile{Mode: fs.ModeDir | 0555}}, nil
	}
	// 可能是目录。
	prefix := realName + "/"
	for fname := range fsys {
		if strings.HasPrefix(fname, prefix) {
			return &mapFileInfo{elem, &MapFile{Mode: fs.ModeDir | 0555}}, nil
		}
	}
	// 如果目录名不在映射中，
	// 并且映射中没有该名称的子项，
	// 则将目录视为不存在。
	return nil, fs.ErrNotExist
}

// fsOnly 是一个包装器，隐藏除 fs.FS 方法之外的所有内容，
// 避免在用辅助函数实现特殊方法时出现无限递归。
// (通常，使用包 fs 辅助函数实现这些方法是冗余和不必要的，
// 但有这些方法可能会使 MapFS 在测试中使用时执行更多代码路径。)
type fsOnly struct{ fs.FS }

func (fsys MapFS) ReadFile(name string) ([]byte, error) {
	return fs.ReadFile(fsOnly{fsys}, name)
}

func (fsys MapFS) Stat(name string) (fs.FileInfo, error) {
	return fs.Stat(fsOnly{fsys}, name)
}

func (fsys MapFS) ReadDir(name string) ([]fs.DirEntry, error) {
	return fs.ReadDir(fsOnly{fsys}, name)
}

func (fsys MapFS) Glob(pattern string) ([]string, error) {
	return fs.Glob(fsOnly{fsys}, pattern)
}

type noSub struct {
	MapFS
}

func (noSub) Sub() {} // 不是 fs.SubFS 签名

func (fsys MapFS) Sub(dir string) (fs.FS, error) {
	return fs.Sub(noSub{fsys}, dir)
}

// mapFileInfo 为给定的映射文件实现 fs.FileInfo 和 fs.DirEntry。
type mapFileInfo struct {
	name string
	f    *MapFile
}

func (i *mapFileInfo) Name() string               { return path.Base(i.name) }
func (i *mapFileInfo) Size() int64                { return int64(len(i.f.Data)) }
func (i *mapFileInfo) Mode() fs.FileMode          { return i.f.Mode }
func (i *mapFileInfo) Type() fs.FileMode          { return i.f.Mode.Type() }
func (i *mapFileInfo) ModTime() time.Time         { return i.f.ModTime }
func (i *mapFileInfo) IsDir() bool                { return i.f.Mode&fs.ModeDir != 0 }
func (i *mapFileInfo) Sys() any                   { return i.f.Sys }
func (i *mapFileInfo) Info() (fs.FileInfo, error) { return i, nil }

func (i *mapFileInfo) String() string {
	return fs.FormatFileInfo(i)
}

// openMapFile 是打开供读取的常规（非目录）fs.File。
type openMapFile struct {
	path string
	mapFileInfo
	offset int64
}

func (f *openMapFile) Stat() (fs.FileInfo, error) { return &f.mapFileInfo, nil }

func (f *openMapFile) Close() error { return nil }

func (f *openMapFile) Read(b []byte) (int, error) {
	if f.offset >= int64(len(f.f.Data)) {
		return 0, io.EOF
	}
	if f.offset < 0 {
		return 0, &fs.PathError{Op: "read", Path: f.path, Err: fs.ErrInvalid}
	}
	n := copy(b, f.f.Data[f.offset:])
	f.offset += int64(n)
	return n, nil
}

func (f *openMapFile) Seek(offset int64, whence int) (int64, error) {
	switch whence {
	case 0:
		// offset += 0
	case 1:
		offset += f.offset
	case 2:
		offset += int64(len(f.f.Data))
	}
	if offset < 0 || offset > int64(len(f.f.Data)) {
		return 0, &fs.PathError{Op: "seek", Path: f.path, Err: fs.ErrInvalid}
	}
	f.offset = offset
	return offset, nil
}

func (f *openMapFile) ReadAt(b []byte, offset int64) (int, error) {
	if offset < 0 || offset > int64(len(f.f.Data)) {
		return 0, &fs.PathError{Op: "read", Path: f.path, Err: fs.ErrInvalid}
	}
	n := copy(b, f.f.Data[offset:])
	if n < len(b) {
		return n, io.EOF
	}
	return n, nil
}

// mapDir 是打开供读取的目录 fs.File（也是 fs.ReadDirFile）。
type mapDir struct {
	path string
	mapFileInfo
	entry  []mapFileInfo
	offset int
}

func (d *mapDir) Stat() (fs.FileInfo, error) { return &d.mapFileInfo, nil }
func (d *mapDir) Close() error               { return nil }
func (d *mapDir) Read(b []byte) (int, error) {
	return 0, &fs.PathError{Op: "read", Path: d.path, Err: fs.ErrInvalid}
}

func (d *mapDir) ReadDir(count int) ([]fs.DirEntry, error) {
	n := len(d.entry) - d.offset
	if n == 0 && count > 0 {
		return nil, io.EOF
	}
	if count > 0 && n > count {
		n = count
	}
	list := make([]fs.DirEntry, n)
	for i := range list {
		list[i] = &d.entry[d.offset+i]
	}
	d.offset += n
	return list, nil
}
