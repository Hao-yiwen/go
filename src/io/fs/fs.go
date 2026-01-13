// 版权所有 2020 The Go Authors。保留所有权利。
// 本源代码的使用受 BSD 风格许可证约束，
// 该许可证可在 LICENSE 文件中找到。

// fs 包定义了文件系统的基本接口。
// 文件系统可以由主机操作系统提供，
// 也可以由其他包提供。
//
// # 路径名
//
// 此包中的接口都使用相同的路径名语法操作，
// 与主机操作系统无关。
//
// 路径名是 UTF-8 编码的、无根的、斜杠分隔的路径元素序列，
// 如 "x/y/z"。
// 路径名不得包含 "." 或 ".." 或空字符串的元素，
// 除了特殊情况下可以使用名称 "." 表示根目录。
// 路径不得以斜杠开头或结尾："/x" 和 "x/" 是无效的。
//
// # 测试
//
// 有关测试文件系统实现的支持，请参阅 [testing/fstest] 包。
package fs

import (
	"internal/oserror"
	"time"
	"unicode/utf8"
)

// FS 提供对分层文件系统的访问。
//
// FS 接口是文件系统所需的最小实现。
// 文件系统可以实现额外的接口，
// 如 [ReadFileFS]，以提供额外或优化的功能。
//
// [testing/fstest.TestFS] 可用于测试 FS 实现的正确性。
type FS interface {
	// Open 打开指定的文件。
	// 必须调用 [File.Close] 以释放任何相关资源。
	//
	// 当 Open 返回错误时，它应该是 *PathError 类型，
	// Op 字段设置为 "open"，Path 字段设置为 name，
	// Err 字段描述问题。
	//
	// Open 应该拒绝尝试打开不满足 ValidPath(name) 的名称，
	// 返回一个 Err 设置为 ErrInvalid 或 ErrNotExist 的 *PathError。
	Open(name string) (File, error)
}

// ValidPath 报告给定的路径名是否可以在调用 Open 时使用。
//
// 注意，在所有系统上路径都是斜杠分隔的，即使在 Windows 上也是如此。
// 包含其他字符（如反斜杠和冒号）的路径被接受为有效，
// 但这些字符绝不能被 [FS] 实现解释为路径元素分隔符。
// 有关更多详细信息，请参阅 [路径名] 部分。
//
// [路径名]: https://pkg.go.dev/io/fs#hdr-Path_Names
func ValidPath(name string) bool {
	if !utf8.ValidString(name) {
		return false
	}

	if name == "." {
		// 特殊情况
		return true
	}

	// 遍历 name 中的元素，检查每个元素。
	for {
		i := 0
		for i < len(name) && name[i] != '/' {
			i++
		}
		elem := name[:i]
		if elem == "" || elem == "." || elem == ".." {
			return false
		}
		if i == len(name) {
			return true // 到达干净的结尾
		}
		name = name[i+1:]
	}
}

// File 提供对单个文件的访问。
// File 接口是文件所需的最小实现。
// 目录文件还应实现 [ReadDirFile]。
// 文件可以实现 [io.ReaderAt] 或 [io.Seeker] 作为优化。
type File interface {
	Stat() (FileInfo, error)
	Read([]byte) (int, error)
	Close() error
}

// DirEntry 是从目录读取的条目
// （使用 [ReadDir] 函数或 [ReadDirFile] 的 ReadDir 方法）。
type DirEntry interface {
	// Name 返回条目描述的文件（或子目录）的名称。
	// 此名称仅是路径的最后一个元素（基本名称），而不是整个路径。
	// 例如，Name 将返回 "hello.go" 而不是 "home/gopher/hello.go"。
	Name() string

	// IsDir 报告条目是否描述目录。
	IsDir() bool

	// Type 返回条目的类型位。
	// 类型位是通常 FileMode 位的子集，即 FileMode.Type 方法返回的那些。
	Type() FileMode

	// Info 返回条目描述的文件或子目录的 FileInfo。
	// 返回的 FileInfo 可能来自原始目录读取时，
	// 或来自调用 Info 时。如果自目录读取以来文件已被删除或重命名，
	// Info 可能返回满足 errors.Is(err, ErrNotExist) 的错误。
	// 如果条目表示符号链接，Info 报告链接本身的信息，
	// 而不是链接目标的信息。
	Info() (FileInfo, error)
}

// ReadDirFile 是一个目录文件，其条目可以使用 ReadDir 方法读取。
// 每个目录文件都应实现此接口。
// （任何文件都可以实现此接口，
// 但如果这样做，ReadDir 应该为非目录返回错误。）
type ReadDirFile interface {
	File

	// ReadDir 读取目录的内容并以目录顺序返回
	// 最多 n 个 DirEntry 值的切片。
	// 对同一文件的后续调用将产生更多的 DirEntry 值。
	//
	// 如果 n > 0，ReadDir 最多返回 n 个 DirEntry 结构。
	// 在这种情况下，如果 ReadDir 返回空切片，它将返回
	// 一个非 nil 错误解释原因。
	// 在目录末尾，错误是 io.EOF。
	// （ReadDir 必须返回 io.EOF 本身，而不是包装 io.EOF 的错误。）
	//
	// 如果 n <= 0，ReadDir 在单个切片中返回目录中所有剩余的 DirEntry 值。
	// 在这种情况下，如果 ReadDir 成功（一直读取到目录末尾），
	// 它返回切片和 nil 错误。
	// 如果在目录末尾之前遇到错误，
	// ReadDir 返回到该点为止读取的 DirEntry 列表和非 nil 错误。
	ReadDir(n int) ([]DirEntry, error)
}

// 通用文件系统错误。
// 文件系统返回的错误可以使用 [errors.Is] 与这些错误进行测试。
var (
	ErrInvalid    = errInvalid()    // "无效参数"
	ErrPermission = errPermission() // "权限被拒绝"
	ErrExist      = errExist()      // "文件已存在"
	ErrNotExist   = errNotExist()   // "文件不存在"
	ErrClosed     = errClosed()     // "文件已关闭"
)

func errInvalid() error    { return oserror.ErrInvalid }
func errPermission() error { return oserror.ErrPermission }
func errExist() error      { return oserror.ErrExist }
func errNotExist() error   { return oserror.ErrNotExist }
func errClosed() error     { return oserror.ErrClosed }

// FileInfo 描述文件并由 [Stat] 返回。
type FileInfo interface {
	Name() string       // 文件的基本名称
	Size() int64        // 常规文件的字节长度；其他文件系统相关
	Mode() FileMode     // 文件模式位
	ModTime() time.Time // 修改时间
	IsDir() bool        // Mode().IsDir() 的缩写
	Sys() any           // 底层数据源（可以返回 nil）
}

// FileMode 表示文件的模式和权限位。
// 这些位在所有系统上具有相同的定义，以便
// 文件信息可以在系统之间可移植地传输。
// 并非所有位都适用于所有系统。
// 唯一必需的位是目录的 [ModeDir]。
type FileMode uint32

// 定义的文件模式位是 [FileMode] 的最高有效位。
// 最低有效的九位是标准 Unix rwxrwxrwx 权限。
// 这些位的值应被视为公共 API 的一部分，
// 可用于线协议或磁盘表示：它们不得更改，
// 尽管可能会添加新位。
const (
	// 单字母是 String 方法格式化使用的缩写。
	ModeDir        FileMode = 1 << (32 - 1 - iota) // d: 是目录
	ModeAppend                                     // a: 仅追加
	ModeExclusive                                  // l: 独占使用
	ModeTemporary                                  // T: 临时文件；仅 Plan 9
	ModeSymlink                                    // L: 符号链接
	ModeDevice                                     // D: 设备文件
	ModeNamedPipe                                  // p: 命名管道（FIFO）
	ModeSocket                                     // S: Unix 域套接字
	ModeSetuid                                     // u: setuid
	ModeSetgid                                     // g: setgid
	ModeCharDevice                                 // c: Unix 字符设备，当设置 ModeDevice 时
	ModeSticky                                     // t: 粘滞位
	ModeIrregular                                  // ?: 非常规文件；对此文件一无所知

	// 类型位的掩码。对于常规文件，不会设置任何位。
	ModeType = ModeDir | ModeSymlink | ModeNamedPipe | ModeSocket | ModeDevice | ModeCharDevice | ModeIrregular

	ModePerm FileMode = 0777 // Unix 权限位
)

func (m FileMode) String() string {
	const str = "dalTLDpSugct?"
	var buf [32]byte // Mode 是 uint32。
	w := 0
	for i, c := range str {
		if m&(1<<uint(32-1-i)) != 0 {
			buf[w] = byte(c)
			w++
		}
	}
	if w == 0 {
		buf[w] = '-'
		w++
	}
	const rwx = "rwxrwxrwx"
	for i, c := range rwx {
		if m&(1<<uint(9-1-i)) != 0 {
			buf[w] = byte(c)
		} else {
			buf[w] = '-'
		}
		w++
	}
	return string(buf[:w])
}

// IsDir 报告 m 是否描述目录。
// 也就是说，它测试 m 中是否设置了 [ModeDir] 位。
func (m FileMode) IsDir() bool {
	return m&ModeDir != 0
}

// IsRegular 报告 m 是否描述常规文件。
// 也就是说，它测试没有设置任何模式类型位。
func (m FileMode) IsRegular() bool {
	return m&ModeType == 0
}

// Perm 返回 m 中的 Unix 权限位（m & [ModePerm]）。
func (m FileMode) Perm() FileMode {
	return m & ModePerm
}

// Type 返回 m 中的类型位（m & [ModeType]）。
func (m FileMode) Type() FileMode {
	return m & ModeType
}

// PathError 记录错误以及导致它的操作和文件路径。
type PathError struct {
	Op   string
	Path string
	Err  error
}

func (e *PathError) Error() string { return e.Op + " " + e.Path + ": " + e.Err.Error() }

func (e *PathError) Unwrap() error { return e.Err }

// Timeout 报告此错误是否表示超时。
func (e *PathError) Timeout() bool {
	t, ok := e.Err.(interface{ Timeout() bool })
	return ok && t.Timeout()
}
