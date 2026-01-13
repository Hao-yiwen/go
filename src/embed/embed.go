// Copyright 2020 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// Package embed 提供对运行的 Go 程序中嵌入文件的访问。
//
// 导入 "embed" 的 Go 源文件可以使用 //go:embed 指令
// 用编译时从包目录或子目录读取的文件的内容初始化 string、[]byte 或 [FS] 类型的变量。
//
// 例如，这里是三种将名为 hello.txt 的文件嵌入
// 并在运行时打印其内容的方法。
//
// 将一个文件嵌入到字符串中：
//
//	import _ "embed"
//
//	//go:embed hello.txt
//	var s string
//	print(s)
//
// 将一个文件嵌入到字节切片中：
//
//	import _ "embed"
//
//	//go:embed hello.txt
//	var b []byte
//	print(string(b))
//
// 将一个或多个文件嵌入到文件系统中：
//
//	import "embed"
//
//	//go:embed hello.txt
//	var f embed.FS
//	data, _ := f.ReadFile("hello.txt")
//	print(string(data))
//
// # 指令
//
// 变量声明上方的 //go:embed 指令指定要嵌入哪些文件，
// 使用一个或多个 path.Match 模式。
//
// 该指令必须紧接在包含单个变量声明的行之前。
// 指令和声明之间只允许空白行和 '//' 行注释。
//
// 变量的类型必须是字符串类型、字节类型的切片，
// 或 [FS]（或 [FS] 的别名）。
//
// 例如：
//
//	package server
//
//	import "embed"
//
//	// content 保存我们的静态 web 服务器内容。
//	//go:embed image/* template/*
//	//go:embed html/index.html
//	var content embed.FS
//
// Go 构建系统将识别这些指令并安排将声明的变量
// （在上面的例子中是 content）填充为与模式匹配的文件。
//
// //go:embed 指令为了简洁起见接受多个空格分隔的模式，
// 但它也可以重复，以避免当有许多模式时行过长。
// 模式是相对于包含源文件的包目录解释的。
// 路径分隔符是正斜杠，即使在 Windows 系统上也是如此。
// 模式不能包含 '.' 或 '..' 或空路径元素，
// 也不能以斜杠开头或结尾。要匹配当前目录中的所有内容，
// 请使用 '*' 而不是 '.'。要允许命名包含空格的文件，
// 模式可以写成 Go 双引号或反引号字符串文字。
//
// 如果一个模式命名一个目录，该目录根目录下的所有文件都会被
// 嵌入（递归地），除了名称以 '.' 或 '_' 开头的文件
// 被排除。所以上面例子中的变量几乎等同于：
//
//	// content 是我们的静态 web 服务器内容。
//	//go:embed image template html/index.html
//	var content embed.FS
//
// 不同之处在于 'image/*' 嵌入 'image/.tempfile' 而 'image' 不嵌入。
// 两者都不嵌入 'image/dir/.tempfile'。
//
// 如果一个模式以前缀 'all:' 开头，那么遍历目录的规则就会改变
// 以包含以 '.' 或 '_' 开头的文件。例如，'all:image' 嵌入
// 'image/.tempfile' 和 'image/dir/.tempfile' 两者。
//
// //go:embed 指令可以与导出和非导出变量一起使用，
// 取决于包是否想让数据对其他包可用。
// 它只能与包范围的变量一起使用，不能与局部变量一起使用。
//
// 模式必须不匹配包的模块外的文件，例如 '.git/*'、符号链接、
// 'vendor/'，或任何包含 go.mod 的目录（这些是单独的模块）。
// 模式必须不匹配名称包含特殊标点字符 " * < > ? ` ' | / \ 和 : 的文件。
// 空目录的匹配被忽略。之后，//go:embed 行中的每个模式
// 必须至少匹配一个文件或非空目录。
//
// 如果任何模式无效或匹配无效，构建将失败。
//
// # 字符串和字节
//
// 字符串或 []byte 类型变量的 //go:embed 行只能有一个模式，
// 且该模式只能匹配一个文件。字符串或 []byte 用以下初始化
// 该文件的内容。
//
// 即使使用字符串或 []byte，//go:embed 指令也需要导入 "embed"。
// 在不引用 [embed.FS] 的源文件中，使用空导入（import _ "embed"）。
//
// # 文件系统
//
// 对于嵌入单个文件，字符串或 []byte 类型的变量通常最好。
// [FS] 类型启用嵌入文件树，如静态目录
// web 服务器内容，如上面的例子。
//
// FS 实现了 [io/fs] 包的 [FS] 接口，所以它可以用于任何
// 理解文件系统的包，包括 [net/http]、[text/template] 和 [html/template]。
//
// 例如，给定上面例子中的内容变量，我们可以写成：
//
//	http.Handle("/static/", http.StripPrefix("/static/", http.FileServer(http.FS(content))))
//
//	template.ParseFS(content, "*.tmpl")
//
// # 工具
//
// 为了支持分析 Go 包的工具，在 //go:embed 行中找到的模式
// 在 "go list" 输出中可用。参见 "go help list" 输出中的
// EmbedPatterns、TestEmbedPatterns 和 XTestEmbedPatterns 字段。
package embed

import (
	"errors"
	"internal/bytealg"
	"internal/stringslite"
	"io"
	"io/fs"
	"time"
)

// FS 是一个只读的文件集合，通常用 //go:embed 指令初始化。
// 声明时没有 //go:embed 指令的 FS 是一个空文件系统。
//
// FS 是一个只读的值，所以可以安全地从多个 goroutine
// 同时使用，也可以安全地相互分配 FS 类型的值。
//
// FS 实现了 fs.FS，所以它可以与任何理解
// 文件系统接口的包一起使用，包括 net/http、text/template 和 html/template。
//
// 关于初始化 FS 的更多详细信息，请参见包文档。
type FS struct {
	// 编译器知道此结构的布局。
	// 参见 cmd/compile/internal/staticdata 的 WriteEmbed。
	//
	// 文件列表按名称排序，但不是按简单的字符串比较。
	// 而是每个文件的名称采用"dir/elem"或"dir/elem/"的形式。
	// 可选的尾部斜杠表示该文件本身是一个目录。
	// 文件列表首先按 dir 排序（如果缺少 dir，则认为是"."）
	// 然后按 base，所以这个文件列表：
	//
	//	p
	//	q/
	//	q/r
	//	q/s/
	//	q/s/t
	//	q/s/u
	//	q/v
	//	w
	//
	// 实际上是按以下方式排序的：
	//
	//	p       # dir=.    elem=p
	//	q/      # dir=.    elem=q
	//	w       # dir=.    elem=w
	//	q/r     # dir=q    elem=r
	//	q/s/    # dir=q    elem=s
	//	q/v     # dir=q    elem=v
	//	q/s/t   # dir=q/s  elem=t
	//	q/s/u   # dir=q/s  elem=u
	//
	// 这个顺序将目录内容放在一起，分成连续的部分
	// 列表，允许目录读取使用二进制搜索来查找
	// 相关的条目序列。
	files *[]file
}

// split 将名称分割为 dir 和 elem，如上面 FS 结构中的
// 注释所述。isDir 报告是否存在最后的尾部斜杠，
// 表示名称是否是目录。
func split(name string) (dir, elem string, isDir bool) {
	name, isDir = stringslite.CutSuffix(name, "/")
	i := bytealg.LastIndexByteString(name, '/')
	if i < 0 {
		return ".", name, isDir
	}
	return name[:i], name[i+1:], isDir
}

var (
	_ fs.ReadDirFS  = FS{}
	_ fs.ReadFileFS = FS{}
)

// file 是 FS 中的单个文件。
// 它实现了 fs.FileInfo 和 fs.DirEntry。
type file struct {
	// 编译器知道此结构的布局。
	// 参见 cmd/compile/internal/staticdata 的 WriteEmbed。
	name string
	data string
	hash [16]byte // 截断的 SHA256 哈希
}

var (
	_ fs.FileInfo = (*file)(nil)
	_ fs.DirEntry = (*file)(nil)
)

func (f *file) Name() string               { _, elem, _ := split(f.name); return elem }
func (f *file) Size() int64                { return int64(len(f.data)) }
func (f *file) ModTime() time.Time         { return time.Time{} }
func (f *file) IsDir() bool                { _, _, isDir := split(f.name); return isDir }
func (f *file) Sys() any                   { return nil }
func (f *file) Type() fs.FileMode          { return f.Mode().Type() }
func (f *file) Info() (fs.FileInfo, error) { return f, nil }

func (f *file) Mode() fs.FileMode {
	if f.IsDir() {
		return fs.ModeDir | 0555
	}
	return 0444
}

func (f *file) String() string {
	return fs.FormatFileInfo(f)
}

// dotFile 是根目录的文件，
// 在 FS 的文件列表中被省略。
var dotFile = &file{name: "./"}

// lookup 返回命名的文件，如果不存在则返回 nil。
func (f FS) lookup(name string) *file {
	if !fs.ValidPath(name) {
		// 编译器永远不应该发出无效名称的文件，
		// 所以此检查并非严格必要（如果名称无效，
		// 我们不应该在下面找到匹配项），但它是一个很好的后备。
		return nil
	}
	if name == "." {
		return dotFile
	}
	if f.files == nil {
		return nil
	}

	// 二进制搜索以找到名称在列表中的位置，
	// 然后检查名称是否在该位置。
	dir, elem, _ := split(name)
	files := *f.files
	i := sortSearch(len(files), func(i int) bool {
		idir, ielem, _ := split(files[i].name)
		return idir > dir || idir == dir && ielem >= elem
	})
	if i < len(files) && stringslite.TrimSuffix(files[i].name, "/") == name {
		return &files[i]
	}
	return nil
}

// readDir 返回对应于目录 dir 的文件列表。
func (f FS) readDir(dir string) []file {
	if f.files == nil {
		return nil
	}
	// 二进制搜索以找到 dir 在列表中的开始和结束位置
	// 然后返回列表的该切片。
	files := *f.files
	i := sortSearch(len(files), func(i int) bool {
		idir, _, _ := split(files[i].name)
		return idir >= dir
	})
	j := sortSearch(len(files), func(j int) bool {
		jdir, _, _ := split(files[j].name)
		return jdir > dir
	})
	return files[i:j]
}

// Open 打开命名的文件以进行读取，并将其作为 [fs.File] 返回。
//
// 当文件不是目录时，返回的文件实现了 [io.Seeker] 和 [io.ReaderAt]。
func (f FS) Open(name string) (fs.File, error) {
	file := f.lookup(name)
	if file == nil {
		return nil, &fs.PathError{Op: "open", Path: name, Err: fs.ErrNotExist}
	}
	if file.IsDir() {
		return &openDir{file, f.readDir(name), 0}, nil
	}
	return &openFile{file, 0}, nil
}

// ReadDir 读取并返回整个命名目录。
func (f FS) ReadDir(name string) ([]fs.DirEntry, error) {
	file, err := f.Open(name)
	if err != nil {
		return nil, err
	}
	dir, ok := file.(*openDir)
	if !ok {
		return nil, &fs.PathError{Op: "read", Path: name, Err: errors.New("not a directory")}
	}
	list := make([]fs.DirEntry, len(dir.files))
	for i := range list {
		list[i] = &dir.files[i]
	}
	return list, nil
}

// ReadFile 读取并返回命名文件的内容。
func (f FS) ReadFile(name string) ([]byte, error) {
	file, err := f.Open(name)
	if err != nil {
		return nil, err
	}
	ofile, ok := file.(*openFile)
	if !ok {
		return nil, &fs.PathError{Op: "read", Path: name, Err: errors.New("is a directory")}
	}
	return []byte(ofile.f.data), nil
}

// openFile 是打开用于读取的常规文件。
type openFile struct {
	f      *file // 文件本身
	offset int64 // 当前读取偏移
}

var (
	_ io.Seeker   = (*openFile)(nil)
	_ io.ReaderAt = (*openFile)(nil)
)

func (f *openFile) Close() error               { return nil }
func (f *openFile) Stat() (fs.FileInfo, error) { return f.f, nil }

func (f *openFile) Read(b []byte) (int, error) {
	if f.offset >= int64(len(f.f.data)) {
		return 0, io.EOF
	}
	if f.offset < 0 {
		return 0, &fs.PathError{Op: "read", Path: f.f.name, Err: fs.ErrInvalid}
	}
	n := copy(b, f.f.data[f.offset:])
	f.offset += int64(n)
	return n, nil
}

func (f *openFile) Seek(offset int64, whence int) (int64, error) {
	switch whence {
	case 0:
		// offset += 0
	case 1:
		offset += f.offset
	case 2:
		offset += int64(len(f.f.data))
	}
	if offset < 0 || offset > int64(len(f.f.data)) {
		return 0, &fs.PathError{Op: "seek", Path: f.f.name, Err: fs.ErrInvalid}
	}
	f.offset = offset
	return offset, nil
}

func (f *openFile) ReadAt(b []byte, offset int64) (int, error) {
	if offset < 0 || offset > int64(len(f.f.data)) {
		return 0, &fs.PathError{Op: "read", Path: f.f.name, Err: fs.ErrInvalid}
	}
	n := copy(b, f.f.data[offset:])
	if n < len(b) {
		return n, io.EOF
	}
	return n, nil
}

// openDir 是打开用于读取的目录。
type openDir struct {
	f      *file  // 目录文件本身
	files  []file // 目录内容
	offset int    // 读取偏移，是文件切片的索引
}

func (d *openDir) Close() error               { return nil }
func (d *openDir) Stat() (fs.FileInfo, error) { return d.f, nil }

func (d *openDir) Read([]byte) (int, error) {
	return 0, &fs.PathError{Op: "read", Path: d.f.name, Err: errors.New("is a directory")}
}

func (d *openDir) ReadDir(count int) ([]fs.DirEntry, error) {
	n := len(d.files) - d.offset
	if n == 0 {
		if count <= 0 {
			return nil, nil
		}
		return nil, io.EOF
	}
	if count > 0 && n > count {
		n = count
	}
	list := make([]fs.DirEntry, n)
	for i := range list {
		list[i] = &d.files[d.offset+i]
	}
	d.offset += n
	return list, nil
}

// sortSearch 类似于 sort.Search，避免导入。
func sortSearch(n int, f func(int) bool) int {
	// 定义 f(-1) == false 和 f(n) == true。
	// 不变量：f(i-1) == false，f(j) == true。
	i, j := 0, n
	for i < j {
		h := int(uint(i+j) >> 1) // 避免计算 h 时溢出
		// i ≤ h < j
		if !f(h) {
			i = h + 1 // 保持 f(i-1) == false
		} else {
			j = h // 保持 f(j) == true
		}
	}
	// i == j，f(i-1) == false，f(j) (= f(i)) == true  =>  答案是 i。
	return i
}
