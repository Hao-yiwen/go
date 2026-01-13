// 版权所有 2009 The Go Authors。保留所有权利。
// 本源代码的使用受 BSD 风格许可证约束，
// 该许可证可在 LICENSE 文件中找到。

package os

import (
	"io/fs"
	"syscall"
)

// Getpagesize 返回底层系统的内存页大小。
func Getpagesize() int { return syscall.Getpagesize() }

// File 表示一个打开的文件描述符。
//
// File 的方法可以安全地并发使用。
type File struct {
	*file // 操作系统特定
}

// FileInfo 描述一个文件，由 [Stat] 和 [Lstat] 返回。
type FileInfo = fs.FileInfo

// FileMode 表示文件的模式和权限位。
// 这些位在所有系统上具有相同的定义，
// 以便可以将文件信息从一个系统可移植地移动到另一个系统。
// 并非所有位都适用于所有系统。
// 对于目录，唯一必需的位是 [ModeDir]。
type FileMode = fs.FileMode

// 定义的文件模式位是 [FileMode] 的最高有效位。
// 九个最低有效位是标准的 Unix rwxrwxrwx 权限。
// 这些位的值应被视为公共 API 的一部分，
// 可用于传输协议或磁盘表示：它们不能更改，
// 但可能会添加新位。
const (
	// 单个字母是 String 方法格式化使用的缩写。
	ModeDir        = fs.ModeDir        // d: 是一个目录
	ModeAppend     = fs.ModeAppend     // a: 只能追加
	ModeExclusive  = fs.ModeExclusive  // l: 独占使用
	ModeTemporary  = fs.ModeTemporary  // T: 临时文件；仅限 Plan 9
	ModeSymlink    = fs.ModeSymlink    // L: 符号链接
	ModeDevice     = fs.ModeDevice     // D: 设备文件
	ModeNamedPipe  = fs.ModeNamedPipe  // p: 命名管道（FIFO）
	ModeSocket     = fs.ModeSocket     // S: Unix 域套接字
	ModeSetuid     = fs.ModeSetuid     // u: setuid
	ModeSetgid     = fs.ModeSetgid     // g: setgid
	ModeCharDevice = fs.ModeCharDevice // c: Unix 字符设备，当设置 ModeDevice 时
	ModeSticky     = fs.ModeSticky     // t: sticky 位
	ModeIrregular  = fs.ModeIrregular  // ?: 非常规文件；对此文件一无所知

	// 类型位的掩码。对于常规文件，不会设置任何位。
	ModeType = fs.ModeType

	ModePerm = fs.ModePerm // Unix 权限位，0o777
)

func (fs *fileStat) Name() string { return fs.name }
func (fs *fileStat) IsDir() bool  { return fs.Mode().IsDir() }

// SameFile reports whether fi1 and fi2 describe the same file.
// For example, on Unix this means that the device and inode fields
// of the two underlying structures are identical; on other systems
// the decision may be based on the path names.
// SameFile only applies to results returned by this package's [Stat].
// It returns false in other cases.
func SameFile(fi1, fi2 FileInfo) bool {
	fs1, ok1 := fi1.(*fileStat)
	fs2, ok2 := fi2.(*fileStat)
	if !ok1 || !ok2 {
		return false
	}
	return sameFile(fs1, fs2)
}
