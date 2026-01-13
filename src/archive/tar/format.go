// 版权所有 2016 The Go Authors。保留所有权利。
// 本源代码的使用受 BSD 风格许可证约束，
// 该许可证可在 LICENSE 文件中找到。

package tar

import "strings"

// Format 表示 tar 归档格式。
//
// 原始的 tar 格式是在 Unix V7 中引入的。
// 此后，出现了多种竞争格式，试图标准化或扩展 V7 格式以克服其限制。
// 最常见的格式是 USTAR、PAX 和 GNU 格式，
// 每种格式都有自己的优点和限制。
//
// 下表展示了每种格式的能力：
//
//	                  |  USTAR |       PAX |       GNU
//	------------------+--------+-----------+----------
//	Name              |   256B | unlimited | unlimited
//	Linkname          |   100B | unlimited | unlimited
//	Size              | uint33 | unlimited |    uint89
//	Mode              | uint21 |    uint21 |    uint57
//	Uid/Gid           | uint21 | unlimited |    uint57
//	Uname/Gname       |    32B | unlimited |       32B
//	ModTime           | uint33 | unlimited |     int89
//	AccessTime        |    n/a | unlimited |     int89
//	ChangeTime        |    n/a | unlimited |     int89
//	Devmajor/Devminor | uint21 |    uint21 |    uint57
//	------------------+--------+-----------+----------
//	string encoding   |  ASCII |     UTF-8 |    binary
//	sub-second times  |     no |       yes |        no
//	sparse files      |     no |       yes |       yes
//
// 表格的上半部分显示 [Header] 字段，其中每种格式报告
// 每个字符串字段允许的最大字节数以及
// 用于存储每个数字字段的整数类型
// （其中时间戳存储为自 Unix 纪元以来的秒数）。
//
// 表格的下半部分显示每种格式的专用特性，
// 例如支持的字符串编码、对亚秒级时间戳的支持
// 或对稀疏文件的支持。
//
// Writer 目前不提供对稀疏文件的支持。
type Format int

// 用于标识各种 tar 格式的常量。
const (
	// 故意对公共 API 隐藏常量的含义。
	_ Format = (1 << iota) / 4 // 序列 0, 0, 1, 2, 4, 8, 等...

	// FormatUnknown 表示格式未知。
	FormatUnknown

	// 标准化之前原始 Unix V7 tar 工具的格式。
	formatV7

	// FormatUSTAR 表示 POSIX.1-1988 中定义的 USTAR 头格式。
	//
	// 虽然此格式与大多数 tar 读取器兼容，
	// 但该格式有一些限制，使其不适合某些用途。
	// 最值得注意的是，它不支持稀疏文件、大于 8GiB 的文件、
	// 超过 256 个字符的文件名以及非 ASCII 文件名。
	//
	// 参考：
	//	http://pubs.opengroup.org/onlinepubs/9699919799/utilities/pax.html#tag_20_92_13_06
	FormatUSTAR

	// FormatPAX 表示 POSIX.1-2001 中定义的 PAX 头格式。
	//
	// PAX 通过在原始头之前写入一个带有 Typeflag TypeXHeader 的特殊文件来扩展 USTAR。
	// 此文件包含一组键值记录，用于克服 USTAR 的不足，
	// 此外还提供了时间戳的亚秒级精度能力。
	//
	// 一些较新的格式通过定义自己的键并为关联值分配特定语义含义
	// 来为 PAX 添加自己的扩展。
	// 例如，PAX 中的稀疏文件支持是使用
	// GNU 手册中定义的键实现的（例如 "GNU.sparse.map"）。
	//
	// 参考：
	//	http://pubs.opengroup.org/onlinepubs/009695399/utilities/pax.html
	FormatPAX

	// FormatGNU 表示 GNU 头格式。
	//
	// GNU 头格式比 USTAR 和 PAX 标准更早，
	// 并且与它们不兼容。GNU 格式支持
	// 任意文件大小、任意编码和长度的文件名、
	// 稀疏文件以及其他功能。
	//
	// 除非目标应用程序只能解析 GNU 格式的归档，
	// 否则建议选择 PAX 而不是 GNU。
	//
	// 参考：
	//	https://www.gnu.org/software/tar/manual/html_node/Standard.html
	FormatGNU

	// Schily 的 tar 格式，与 USTAR 不兼容。
	// 这不包括对 PAX 格式的 STAR 扩展；这些属于 PAX 格式。
	formatSTAR

	formatMax
)

func (f Format) has(f2 Format) bool   { return f&f2 != 0 }
func (f *Format) mayBe(f2 Format)     { *f |= f2 }
func (f *Format) mayOnlyBe(f2 Format) { *f &= f2 }
func (f *Format) mustNotBe(f2 Format) { *f &^= f2 }

var formatNames = map[Format]string{
	formatV7: "V7", FormatUSTAR: "USTAR", FormatPAX: "PAX", FormatGNU: "GNU", formatSTAR: "STAR",
}

func (f Format) String() string {
	var ss []string
	for f2 := Format(1); f2 < formatMax; f2 <<= 1 {
		if f.has(f2) {
			ss = append(ss, formatNames[f2])
		}
	}
	switch len(ss) {
	case 0:
		return "<unknown>"
	case 1:
		return ss[0]
	default:
		return "(" + strings.Join(ss, " | ") + ")"
	}
}

// 用于标识各种格式的魔数。
const (
	magicGNU, versionGNU     = "ustar ", " \x00"
	magicUSTAR, versionUSTAR = "ustar\x00", "00"
	trailerSTAR              = "tar\x00"
)

// 来自各种 tar 规范的大小常量。
const (
	blockSize  = 512 // tar 流中每个块的大小
	nameSize   = 100 // USTAR 格式中 name 字段的最大长度
	prefixSize = 155 // USTAR 格式中 prefix 字段的最大长度

	// 特殊文件（PAX 头、GNU 长名称或链接）的最大长度。
	// 这与 libarchive 使用的限制相匹配。
	maxSpecialFileSize = 1 << 20
)

// blockPadding 计算将 offset 填充到最近块边界所需的字节数，
// 其中 0 <= n < blockSize。
func blockPadding(offset int64) (n int64) {
	return -offset & (blockSize - 1)
}

var zeroBlock block

type block [blockSize]byte

// 将块转换为任意格式。
func (b *block) toV7() *headerV7       { return (*headerV7)(b) }
func (b *block) toGNU() *headerGNU     { return (*headerGNU)(b) }
func (b *block) toSTAR() *headerSTAR   { return (*headerSTAR)(b) }
func (b *block) toUSTAR() *headerUSTAR { return (*headerUSTAR)(b) }
func (b *block) toSparse() sparseArray { return sparseArray(b[:]) }

// getFormat 根据校验和检查块是否是有效的 tar 头。
// 然后尝试根据魔数猜测具体格式。
// 如果校验和失败，则返回 FormatUnknown。
func (b *block) getFormat() Format {
	// 验证校验和。
	var p parser
	value := p.parseOctal(b.toV7().chksum())
	chksum1, chksum2 := b.computeChecksum()
	if p.err != nil || (value != chksum1 && value != chksum2) {
		return FormatUnknown
	}

	// 猜测魔数值。
	magic := string(b.toUSTAR().magic())
	version := string(b.toUSTAR().version())
	trailer := string(b.toSTAR().trailer())
	switch {
	case magic == magicUSTAR && trailer == trailerSTAR:
		return formatSTAR
	case magic == magicUSTAR:
		return FormatUSTAR | FormatPAX
	case magic == magicGNU && version == versionGNU:
		return FormatGNU
	default:
		return formatV7
	}
}

// setFormat 写入指定格式所需的魔数值，
// 然后相应地更新校验和。
func (b *block) setFormat(format Format) {
	// 设置魔数值。
	switch {
	case format.has(formatV7):
		// 不做任何操作。
	case format.has(FormatGNU):
		copy(b.toGNU().magic(), magicGNU)
		copy(b.toGNU().version(), versionGNU)
	case format.has(formatSTAR):
		copy(b.toSTAR().magic(), magicUSTAR)
		copy(b.toSTAR().version(), versionUSTAR)
		copy(b.toSTAR().trailer(), trailerSTAR)
	case format.has(FormatUSTAR | FormatPAX):
		copy(b.toUSTAR().magic(), magicUSTAR)
		copy(b.toUSTAR().version(), versionUSTAR)
	default:
		panic("invalid format")
	}

	// 更新校验和。
	// 此字段的特殊之处在于它以 NULL 后跟空格终止。
	var f formatter
	field := b.toV7().chksum()
	chksum, _ := b.computeChecksum() // 可能的值为 256..128776
	f.formatOctal(field[:7], chksum) // 永不失败，因为 128776 < 262143
	field[7] = ' '
}

// computeChecksum 计算头块的校验和。
// POSIX 规定使用无符号字节值的总和，但 Sun tar 使用
// 有符号字节值。
// 我们计算并返回两者。
func (b *block) computeChecksum() (unsigned, signed int64) {
	for i, c := range b {
		if 148 <= i && i < 156 {
			c = ' ' // 将校验和字段本身视为全空格。
		}
		unsigned += int64(c)
		signed += int64(int8(c))
	}
	return unsigned, signed
}

// reset 用全零清除块。
func (b *block) reset() {
	*b = block{}
}

type headerV7 [blockSize]byte

func (h *headerV7) name() []byte     { return h[000:][:100] }
func (h *headerV7) mode() []byte     { return h[100:][:8] }
func (h *headerV7) uid() []byte      { return h[108:][:8] }
func (h *headerV7) gid() []byte      { return h[116:][:8] }
func (h *headerV7) size() []byte     { return h[124:][:12] }
func (h *headerV7) modTime() []byte  { return h[136:][:12] }
func (h *headerV7) chksum() []byte   { return h[148:][:8] }
func (h *headerV7) typeFlag() []byte { return h[156:][:1] }
func (h *headerV7) linkName() []byte { return h[157:][:100] }

type headerGNU [blockSize]byte

func (h *headerGNU) v7() *headerV7       { return (*headerV7)(h) }
func (h *headerGNU) magic() []byte       { return h[257:][:6] }
func (h *headerGNU) version() []byte     { return h[263:][:2] }
func (h *headerGNU) userName() []byte    { return h[265:][:32] }
func (h *headerGNU) groupName() []byte   { return h[297:][:32] }
func (h *headerGNU) devMajor() []byte    { return h[329:][:8] }
func (h *headerGNU) devMinor() []byte    { return h[337:][:8] }
func (h *headerGNU) accessTime() []byte  { return h[345:][:12] }
func (h *headerGNU) changeTime() []byte  { return h[357:][:12] }
func (h *headerGNU) sparse() sparseArray { return sparseArray(h[386:][:24*4+1]) }
func (h *headerGNU) realSize() []byte    { return h[483:][:12] }

type headerSTAR [blockSize]byte

func (h *headerSTAR) v7() *headerV7      { return (*headerV7)(h) }
func (h *headerSTAR) magic() []byte      { return h[257:][:6] }
func (h *headerSTAR) version() []byte    { return h[263:][:2] }
func (h *headerSTAR) userName() []byte   { return h[265:][:32] }
func (h *headerSTAR) groupName() []byte  { return h[297:][:32] }
func (h *headerSTAR) devMajor() []byte   { return h[329:][:8] }
func (h *headerSTAR) devMinor() []byte   { return h[337:][:8] }
func (h *headerSTAR) prefix() []byte     { return h[345:][:131] }
func (h *headerSTAR) accessTime() []byte { return h[476:][:12] }
func (h *headerSTAR) changeTime() []byte { return h[488:][:12] }
func (h *headerSTAR) trailer() []byte    { return h[508:][:4] }

type headerUSTAR [blockSize]byte

func (h *headerUSTAR) v7() *headerV7     { return (*headerV7)(h) }
func (h *headerUSTAR) magic() []byte     { return h[257:][:6] }
func (h *headerUSTAR) version() []byte   { return h[263:][:2] }
func (h *headerUSTAR) userName() []byte  { return h[265:][:32] }
func (h *headerUSTAR) groupName() []byte { return h[297:][:32] }
func (h *headerUSTAR) devMajor() []byte  { return h[329:][:8] }
func (h *headerUSTAR) devMinor() []byte  { return h[337:][:8] }
func (h *headerUSTAR) prefix() []byte    { return h[345:][:155] }

type sparseArray []byte

func (s sparseArray) entry(i int) sparseElem { return sparseElem(s[i*24:]) }
func (s sparseArray) isExtended() []byte     { return s[24*s.maxEntries():][:1] }
func (s sparseArray) maxEntries() int        { return len(s) / 24 }

type sparseElem []byte

func (s sparseElem) offset() []byte { return s[00:][:12] }
func (s sparseElem) length() []byte { return s[12:][:12] }
