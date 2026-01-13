// 版权所有 2010 The Go Authors。保留所有权利。
// 本源代码的使用受 BSD 风格许可证约束，
// 该许可证可在 LICENSE 文件中找到。

/*
Package zip 提供对 ZIP 归档的读写支持。

详见 [ZIP 规范]。

本包不支持跨磁盘跨越。

关于 ZIP64 的注意：

为了向后兼容，FileHeader 同时包含 32 位和 64 位大小字段。
64 位字段始终包含正确的值，对于普通归档文件，两个字段都相同。
对于需要 ZIP64 格式的文件，32 位字段将设置为 0xffffffff，
必须改用 64 位字段。

[ZIP 规范]: https://support.pkware.com/pkzip/appnote
*/
package zip

import (
	"io/fs"
	"path"
	"time"
)

// 压缩方法。
const (
	Store   uint16 = 0 // 无压缩
	Deflate uint16 = 8 // DEFLATE 压缩
)

const (
	fileHeaderSignature      = 0x04034b50
	directoryHeaderSignature = 0x02014b50
	directoryEndSignature    = 0x06054b50
	directory64LocSignature  = 0x07064b50
	directory64EndSignature  = 0x06064b50
	dataDescriptorSignature  = 0x08074b50 // 事实标准；OS X Finder 需要
	fileHeaderLen            = 30         // + 文件名 + 扩展字段
	directoryHeaderLen       = 46         // + 文件名 + 扩展字段 + 注释
	directoryEndLen          = 22         // + 注释
	dataDescriptorLen        = 16         // 四个 uint32：描述符签名、crc32、压缩大小、大小
	dataDescriptor64Len      = 24         // 两个 uint32：签名、crc32 | 两个 uint64：压缩大小、大小
	directory64LocLen        = 20         //
	directory64EndLen        = 56         // + 扩展字段

	// CreatorVersion 首字节的常数。
	creatorFAT    = 0
	creatorUnix   = 3
	creatorNTFS   = 11
	creatorVFAT   = 14
	creatorMacOSX = 19

	// 版本号。
	zipVersion20 = 20 // 2.0
	zipVersion45 = 45 // 4.5（读写 zip64 归档）

	// 非 zip64 文件的限制。
	uint16max = (1 << 16) - 1
	uint32max = (1 << 32) - 1

	// 扩展头 ID。
	//
	// ID 0..31 由 PKWARE 保留供官方使用。
	// 高于该范围的 ID 由第三方供应商定义。
	// 由于 ZIP 缺乏高精度时间戳（也没有关于日期字段中使用的时区的官方规范），
	// 许多竞争的扩展字段已被发明。广泛使用有效地使它们成为"官方"。
	//
	// 详见 http://mdfs.net/Docs/Comp/Archiving/Zip/ExtraField
	zip64ExtraID       = 0x0001 // Zip64 扩展信息
	ntfsExtraID        = 0x000a // NTFS
	unixExtraID        = 0x000d // UNIX
	extTimeExtraID     = 0x5455 // 扩展时间戳
	infoZipUnixExtraID = 0x5855 // Info-ZIP Unix 扩展
)

// FileHeader 描述 ZIP 文件中的一个文件。
// 详见 [ZIP 规范]。
//
// [ZIP 规范]: https://support.pkware.com/pkzip/appnote
type FileHeader struct {
	// Name 是文件的名称。
	//
	// 它必须是相对路径，不能以驱动器号（例如 "C:"）开头，
	// 并且必须使用正斜杠而不是反斜杠。尾部斜杠
	// 表示该文件是目录，不应有数据。
	Name string

	// Comment 是任何由用户定义的长度小于 64KiB 的任意字符串。
	Comment string

	// NonUTF8 表示 Name 和 Comment 未用 UTF-8 编码。
	//
	// 按照规范，唯一允许的其他编码应该是 CP-437，
	// 但历史上许多 ZIP 读取器将 Name 和 Comment 解释为
	// 系统的本地字符编码。
	//
	// 仅当用户打算为特定本地化地区编码不可移植的
	// ZIP 文件时，才应设置此标志。否则，Writer
	// 会自动为有效的 UTF-8 字符串设置 ZIP 格式的 UTF-8 标志。
	NonUTF8 bool

	CreatorVersion uint16
	ReaderVersion  uint16
	Flags          uint16

	// Method 是压缩方法。如果为零，则使用 Store。
	Method uint16

	// Modified 是文件的修改时间。
	//
	// 读取时，扩展时间戳优先于旧版 MS-DOS 日期字段，
	// 时间之间的偏移量用作时区。
	// 如果仅存在 MS-DOS 日期，则假设时区为 UTC。
	//
	// 写入时，始终发出扩展时间戳（与时区无关）。
	// 旧版 MS-DOS 日期字段根据修改时间的位置进行编码。
	Modified time.Time

	// ModifiedTime 是 MS-DOS 编码的时间。
	//
	// 已弃用：请改用 Modified。
	ModifiedTime uint16

	// ModifiedDate 是 MS-DOS 编码的日期。
	//
	// 已弃用：请改用 Modified。
	ModifiedDate uint16

	// CRC32 是文件内容的 CRC32 校验和。
	CRC32 uint32

	// CompressedSize 是文件的压缩大小，以字节为单位。
	// 如果未压缩或压缩大小不适合 32 位，
	// CompressedSize 将设置为 ^uint32(0)。
	//
	// 已弃用：请改用 CompressedSize64。
	CompressedSize uint32

	// UncompressedSize 是文件的未压缩大小，以字节为单位。
	// 如果未压缩或压缩大小不适合 32 位，
	// UncompressedSize 将设置为 ^uint32(0)。
	//
	// 已弃用：请改用 UncompressedSize64。
	UncompressedSize uint32

	// CompressedSize64 是文件的压缩大小，以字节为单位。
	CompressedSize64 uint64

	// UncompressedSize64 是文件的未压缩大小，以字节为单位。
	UncompressedSize64 uint64

	Extra         []byte
	ExternalAttrs uint32 // 含义取决于 CreatorVersion
}

// FileInfo 返回 [FileHeader] 的 fs.FileInfo。
func (h *FileHeader) FileInfo() fs.FileInfo {
	return headerFileInfo{h}
}

// headerFileInfo 实现 [fs.FileInfo]。
type headerFileInfo struct {
	fh *FileHeader
}

func (fi headerFileInfo) Name() string { return path.Base(fi.fh.Name) }
func (fi headerFileInfo) Size() int64 {
	if fi.fh.UncompressedSize64 > 0 {
		return int64(fi.fh.UncompressedSize64)
	}
	return int64(fi.fh.UncompressedSize)
}
func (fi headerFileInfo) IsDir() bool { return fi.Mode().IsDir() }
func (fi headerFileInfo) ModTime() time.Time {
	if fi.fh.Modified.IsZero() {
		return fi.fh.ModTime()
	}
	return fi.fh.Modified.UTC()
}
func (fi headerFileInfo) Mode() fs.FileMode { return fi.fh.Mode() }
func (fi headerFileInfo) Type() fs.FileMode { return fi.fh.Mode().Type() }
func (fi headerFileInfo) Sys() any          { return fi.fh }

func (fi headerFileInfo) Info() (fs.FileInfo, error) { return fi, nil }

func (fi headerFileInfo) String() string {
	return fs.FormatFileInfo(fi)
}

// FileInfoHeader 从 fs.FileInfo 创建一个部分填充的 [FileHeader]。
// 由于 fs.FileInfo 的 Name 方法仅返回它所描述的文件的基本名称，
// 可能需要修改返回头部的 Name 字段以提供文件的完整路径名。
// 如果需要压缩，调用者应设置 FileHeader.Method 字段；默认情况下不设置。
func FileInfoHeader(fi fs.FileInfo) (*FileHeader, error) {
	size := fi.Size()
	fh := &FileHeader{
		Name:               fi.Name(),
		UncompressedSize64: uint64(size),
	}
	fh.SetModTime(fi.ModTime())
	fh.SetMode(fi.Mode())
	if fh.UncompressedSize64 > uint32max {
		fh.UncompressedSize = uint32max
	} else {
		fh.UncompressedSize = uint32(fh.UncompressedSize64)
	}
	return fh, nil
}

type directoryEnd struct {
	diskNbr            uint32 // 未使用
	dirDiskNbr         uint32 // 未使用
	dirRecordsThisDisk uint64 // 未使用
	directoryRecords   uint64
	directorySize      uint64
	directoryOffset    uint64 // 相对于文件
	commentLen         uint16
	comment            string
}

// timeZone 返回基于提供的偏移量的 *time.Location。
// 如果偏移量不合理，则使用零偏移量。
func timeZone(offset time.Duration) *time.Location {
	const (
		minOffset   = -12 * time.Hour  // 例如 Baker 岛在 -12:00
		maxOffset   = +14 * time.Hour  // 例如 Line 岛在 +14:00
		offsetAlias = 15 * time.Minute // 例如尼泊尔在 +5:45
	)
	offset = offset.Round(offsetAlias)
	if offset < minOffset || maxOffset < offset {
		offset = 0
	}
	return time.FixedZone("", int(offset/time.Second))
}

// msDosTimeToTime 将 MS-DOS 日期和时间转换为 time.Time。
// 分辨率为 2 秒。
// 详见：https://learn.microsoft.com/en-us/windows/win32/api/winbase/nf-winbase-dosdatetimetofiletime
func msDosTimeToTime(dosDate, dosTime uint16) time.Time {
	return time.Date(
		// 日期位 0-4：月份的日期；5-8：月份；9-15：自 1980 以来的年数
		int(dosDate>>9+1980),
		time.Month(dosDate>>5&0xf),
		int(dosDate&0x1f),

		// 时间位 0-4：秒/2；5-10：分钟；11-15：小时
		int(dosTime>>11),
		int(dosTime>>5&0x3f),
		int(dosTime&0x1f*2),
		0, // 纳秒

		time.UTC,
	)
}

// timeToMsDosTime 将 time.Time 转换为 MS-DOS 日期和时间。
// 分辨率为 2 秒。
// 详见：https://learn.microsoft.com/en-us/windows/win32/api/winbase/nf-winbase-filetimetodosdatetime
func timeToMsDosTime(t time.Time) (fDate uint16, fTime uint16) {
	fDate = uint16(t.Day() + int(t.Month())<<5 + (t.Year()-1980)<<9)
	fTime = uint16(t.Second()/2 + t.Minute()<<5 + t.Hour()<<11)
	return
}

// ModTime 使用旧版 [ModifiedDate] 和 [ModifiedTime] 字段返回 UTC 中的修改时间。
//
// 已弃用：请改用 [Modified]。
func (h *FileHeader) ModTime() time.Time {
	return msDosTimeToTime(h.ModifiedDate, h.ModifiedTime)
}

// SetModTime 将 [Modified]、[ModifiedTime] 和 [ModifiedDate] 字段
// 设置为 UTC 中的给定时间。
//
// 已弃用：请改用 [Modified]。
func (h *FileHeader) SetModTime(t time.Time) {
	t = t.UTC() // 为了兼容性转换为 UTC
	h.Modified = t
	h.ModifiedDate, h.ModifiedTime = timeToMsDosTime(t)
}

const (
	// Unix 常数。规范未提及它们，
	// 但这些似乎是工具约定的值。
	s_IFMT   = 0xf000
	s_IFSOCK = 0xc000
	s_IFLNK  = 0xa000
	s_IFREG  = 0x8000
	s_IFBLK  = 0x6000
	s_IFDIR  = 0x4000
	s_IFCHR  = 0x2000
	s_IFIFO  = 0x1000
	s_ISUID  = 0x800
	s_ISGID  = 0x400
	s_ISVTX  = 0x200

	msdosDir      = 0x10
	msdosReadOnly = 0x01
)

// Mode 返回 [FileHeader] 的权限和模式位。
func (h *FileHeader) Mode() (mode fs.FileMode) {
	switch h.CreatorVersion >> 8 {
	case creatorUnix, creatorMacOSX:
		mode = unixModeToFileMode(h.ExternalAttrs >> 16)
	case creatorNTFS, creatorVFAT, creatorFAT:
		mode = msdosModeToFileMode(h.ExternalAttrs)
	}
	if len(h.Name) > 0 && h.Name[len(h.Name)-1] == '/' {
		mode |= fs.ModeDir
	}
	return mode
}

// SetMode 改变 [FileHeader] 的权限和模式位。
func (h *FileHeader) SetMode(mode fs.FileMode) {
	h.CreatorVersion = h.CreatorVersion&0xff | creatorUnix<<8
	h.ExternalAttrs = fileModeToUnixMode(mode) << 16

	// 同样设置 MSDOS 属性，如原始 zip 一样。
	if mode&fs.ModeDir != 0 {
		h.ExternalAttrs |= msdosDir
	}
	if mode&0200 == 0 {
		h.ExternalAttrs |= msdosReadOnly
	}
}

// isZip64 报告文件大小是否超过 32 位限制
func (h *FileHeader) isZip64() bool {
	return h.CompressedSize64 >= uint32max || h.UncompressedSize64 >= uint32max
}

func (h *FileHeader) hasDataDescriptor() bool {
	return h.Flags&0x8 != 0
}

func msdosModeToFileMode(m uint32) (mode fs.FileMode) {
	if m&msdosDir != 0 {
		mode = fs.ModeDir | 0777
	} else {
		mode = 0666
	}
	if m&msdosReadOnly != 0 {
		mode &^= 0222
	}
	return mode
}

func fileModeToUnixMode(mode fs.FileMode) uint32 {
	var m uint32
	switch mode & fs.ModeType {
	default:
		m = s_IFREG
	case fs.ModeDir:
		m = s_IFDIR
	case fs.ModeSymlink:
		m = s_IFLNK
	case fs.ModeNamedPipe:
		m = s_IFIFO
	case fs.ModeSocket:
		m = s_IFSOCK
	case fs.ModeDevice:
		m = s_IFBLK
	case fs.ModeDevice | fs.ModeCharDevice:
		m = s_IFCHR
	}
	if mode&fs.ModeSetuid != 0 {
		m |= s_ISUID
	}
	if mode&fs.ModeSetgid != 0 {
		m |= s_ISGID
	}
	if mode&fs.ModeSticky != 0 {
		m |= s_ISVTX
	}
	return m | uint32(mode&0777)
}

func unixModeToFileMode(m uint32) fs.FileMode {
	mode := fs.FileMode(m & 0777)
	switch m & s_IFMT {
	case s_IFBLK:
		mode |= fs.ModeDevice
	case s_IFCHR:
		mode |= fs.ModeDevice | fs.ModeCharDevice
	case s_IFDIR:
		mode |= fs.ModeDir
	case s_IFIFO:
		mode |= fs.ModeNamedPipe
	case s_IFLNK:
		mode |= fs.ModeSymlink
	case s_IFREG:
		// nothing to do
	case s_IFSOCK:
		mode |= fs.ModeSocket
	}
	if m&s_ISGID != 0 {
		mode |= fs.ModeSetgid
	}
	if m&s_ISUID != 0 {
		mode |= fs.ModeSetuid
	}
	if m&s_ISVTX != 0 {
		mode |= fs.ModeSticky
	}
	return mode
}
