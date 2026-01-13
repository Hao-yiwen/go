// 版权所有 2020 The Go Authors。保留所有权利。
// 本源代码的使用受 BSD 风格许可证约束，
// 该许可证可在 LICENSE 文件中找到。

package fs

import (
	"errors"
	"path"
)

// SubFS 是具有 Sub 方法的文件系统。
type SubFS interface {
	FS

	// Sub 返回一个 FS，对应于以 dir 为根的子树。
	Sub(dir string) (FS, error)
}

// Sub 返回一个 [FS]，对应于 fsys 的 dir 为根的子树。
//
// 如果 dir 是 "."，Sub 返回未改变的 fsys。
// 否则，如果 fs 实现了 [SubFS]，Sub 返回 fsys.Sub(dir)。
// 否则，Sub 返回一个新的 [FS] 实现 sub，
// 该实现在实际上将 sub.Open(name) 实现为 fsys.Open(path.Join(dir, name))。
// 该实现还适当地转换对 ReadDir、ReadFile、
// ReadLink、Lstat 和 Glob 的调用。
//
// 请注意，Sub(os.DirFS("/"), "prefix") 等同于 os.DirFS("/prefix")，
// 并且两者都不保证避免对 "/prefix" 之外的操作系统进行访问，
// 因为 [os.DirFS] 的实现不会检查 "/prefix" 内指向
// 其他目录的符号链接。也就是说，[os.DirFS] 不是
// chroot 风格安全机制的一般替代品，Sub 不改变这一事实。
func Sub(fsys FS, dir string) (FS, error) {
	if !ValidPath(dir) {
		return nil, &PathError{Op: "sub", Path: dir, Err: ErrInvalid}
	}
	if dir == "." {
		return fsys, nil
	}
	if fsys, ok := fsys.(SubFS); ok {
		return fsys.Sub(dir)
	}
	return &subFS{fsys, dir}, nil
}

var _ FS = (*subFS)(nil)
var _ ReadDirFS = (*subFS)(nil)
var _ ReadFileFS = (*subFS)(nil)
var _ ReadLinkFS = (*subFS)(nil)
var _ GlobFS = (*subFS)(nil)

type subFS struct {
	fsys FS
	dir  string
}

// fullName 将 name 映射到完全限定的名称 dir/name。
func (f *subFS) fullName(op string, name string) (string, error) {
	if !ValidPath(name) {
		return "", &PathError{Op: op, Path: name, Err: ErrInvalid}
	}
	return path.Join(f.dir, name), nil
}

// shorten 将应该以 f.dir 开头的 name 映射回 f.dir 之后的后缀。
func (f *subFS) shorten(name string) (rel string, ok bool) {
	if name == f.dir {
		return ".", true
	}
	if len(name) >= len(f.dir)+2 && name[len(f.dir)] == '/' && name[:len(f.dir)] == f.dir {
		return name[len(f.dir)+1:], true
	}
	return "", false
}

// fixErr 通过去掉 f.dir 来缩短 PathErrors 中报告的任何名称。
func (f *subFS) fixErr(err error) error {
	if e, ok := err.(*PathError); ok {
		if short, ok := f.shorten(e.Path); ok {
			e.Path = short
		}
	}
	return err
}

func (f *subFS) Open(name string) (File, error) {
	full, err := f.fullName("open", name)
	if err != nil {
		return nil, err
	}
	file, err := f.fsys.Open(full)
	return file, f.fixErr(err)
}

func (f *subFS) ReadDir(name string) ([]DirEntry, error) {
	full, err := f.fullName("read", name)
	if err != nil {
		return nil, err
	}
	dir, err := ReadDir(f.fsys, full)
	return dir, f.fixErr(err)
}

func (f *subFS) ReadFile(name string) ([]byte, error) {
	full, err := f.fullName("read", name)
	if err != nil {
		return nil, err
	}
	data, err := ReadFile(f.fsys, full)
	return data, f.fixErr(err)
}

func (f *subFS) ReadLink(name string) (string, error) {
	full, err := f.fullName("readlink", name)
	if err != nil {
		return "", err
	}
	target, err := ReadLink(f.fsys, full)
	if err != nil {
		return "", f.fixErr(err)
	}
	return target, nil
}

func (f *subFS) Lstat(name string) (FileInfo, error) {
	full, err := f.fullName("lstat", name)
	if err != nil {
		return nil, err
	}
	info, err := Lstat(f.fsys, full)
	if err != nil {
		return nil, f.fixErr(err)
	}
	return info, nil
}

func (f *subFS) Glob(pattern string) ([]string, error) {
	// 检查模式格式是否正确。
	if _, err := path.Match(pattern, ""); err != nil {
		return nil, err
	}
	if pattern == "." {
		return []string{"."}, nil
	}

	full := f.dir + "/" + pattern
	list, err := Glob(f.fsys, full)
	for i, name := range list {
		name, ok := f.shorten(name)
		if !ok {
			return nil, errors.New("invalid result from inner fsys Glob: " + name + " not in " + f.dir) // 无法在此包中使用 fmt
		}
		list[i] = name
	}
	return list, f.fixErr(err)
}

func (f *subFS) Sub(dir string) (FS, error) {
	if dir == "." {
		return f, nil
	}
	full, err := f.fullName("sub", dir)
	if err != nil {
		return nil, err
	}
	return &subFS{f.fsys, full}, nil
}
