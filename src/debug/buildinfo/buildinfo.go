// 版权所有 2021 The Go Authors。保留所有权利。
// 本源代码的使用受 BSD 风格许可证约束，
// 该许可证可在 LICENSE 文件中找到。

// Package buildinfo 提供对嵌入在 Go 二进制文件中的构建信息的访问。
// 这包括 Go 工具链版本，以及使用的模块集合（针对以模块模式构建的二进制文件）。
//
// 对于当前运行的二进制文件，可以通过 runtime/debug.ReadBuildInfo 获取构建信息。
package buildinfo

import (
	"bytes"
	"debug/elf"
	"debug/macho"
	"debug/pe"
	"debug/plan9obj"
	"encoding/binary"
	"errors"
	"fmt"
	"internal/saferio"
	"internal/xcoff"
	"io"
	"io/fs"
	"os"
	"runtime/debug"
	_ "unsafe" // 用于 linkname
)

// BuildInfo 的类型别名。我们不能将类型移到这里，因为
// runtime/debug 需要导入此包，这将使其成为一个更大的依赖。
type BuildInfo = debug.BuildInfo

// errUnrecognizedFormat 在给定的可执行文件不是已知格式、
// 违反该格式的规则，或读取文件时发生 I/O 错误时返回。
var errUnrecognizedFormat = errors.New("unrecognized file format")

// errNotGoExe 在给定的可执行文件有效但不包含 Go 构建信息时返回。
//
// errNotGoExe 本应是内部细节，
// 但被广泛使用的包通过 linkname 访问它。
// 著名的违规者包括：
//   - github.com/quay/claircore
//
// 请勿删除或更改类型签名。
// 参见 go.dev/issue/67401。
//
//go:linkname errNotGoExe
var errNotGoExe = errors.New("not a Go executable")

// 链接器留下的构建信息块通过 32 字节的头部来识别，
// 由 buildInfoMagic（14 字节）组成，后跟版本相关的字段。
var buildInfoMagic = []byte("\xff Go buildinf:")

const (
	buildInfoAlign      = 16
	buildInfoHeaderSize = 32
)

// ReadFile 返回嵌入在指定路径的 Go 二进制文件中的构建信息。
// 大多数信息仅适用于以模块支持构建的二进制文件。
func ReadFile(name string) (info *BuildInfo, err error) {
	defer func() {
		if _, ok := errors.AsType[*fs.PathError](err); ok {
			err = fmt.Errorf("could not read Go build info: %w", err)
		} else if err != nil {
			err = fmt.Errorf("could not read Go build info from %s: %w", name, err)
		}
	}()

	f, err := os.Open(name)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	return Read(f)
}

// Read 返回嵌入在通过给定 ReaderAt 访问的 Go 二进制文件中的构建信息。
// 大多数信息仅适用于以模块支持构建的二进制文件。
func Read(r io.ReaderAt) (*BuildInfo, error) {
	vers, mod, err := readRawBuildInfo(r)
	if err != nil {
		return nil, err
	}
	bi, err := debug.ParseBuildInfo(mod)
	if err != nil {
		return nil, err
	}
	bi.GoVersion = vers
	return bi, nil
}

type exe interface {
	// DataStart 返回应包含构建信息的段或节的虚拟地址和大小。
	// 这可以是特殊命名的节，也可以是第一个可写的非零数据段。
	DataStart() (uint64, uint64)

	// DataReader 返回一个 io.ReaderAt，从 addr 读取到包含 addr 的段或节的末尾。
	DataReader(addr uint64) (io.ReaderAt, error)
}

// readRawBuildInfo 从 Go 二进制文件中提取 Go 工具链版本和模块信息字符串。
// 成功时，vers 应该是非空的。如果二进制文件未启用模块构建，则 mod 为空。
func readRawBuildInfo(r io.ReaderAt) (vers, mod string, err error) {
	// 读取文件的前几个字节以识别格式，然后委托给
	// 特定格式的函数来加载段和节头。
	ident := make([]byte, 16)
	if n, err := r.ReadAt(ident, 0); n < len(ident) || err != nil {
		return "", "", errUnrecognizedFormat
	}

	var x exe
	switch {
	case bytes.HasPrefix(ident, []byte("\x7FELF")):
		f, err := elf.NewFile(r)
		if err != nil {
			return "", "", errUnrecognizedFormat
		}
		x = &elfExe{f}
	case bytes.HasPrefix(ident, []byte("MZ")):
		f, err := pe.NewFile(r)
		if err != nil {
			return "", "", errUnrecognizedFormat
		}
		x = &peExe{f}
	case bytes.HasPrefix(ident, []byte("\xFE\xED\xFA")) || bytes.HasPrefix(ident[1:], []byte("\xFA\xED\xFE")):
		f, err := macho.NewFile(r)
		if err != nil {
			return "", "", errUnrecognizedFormat
		}
		x = &machoExe{f}
	case bytes.HasPrefix(ident, []byte("\xCA\xFE\xBA\xBE")) || bytes.HasPrefix(ident, []byte("\xCA\xFE\xBA\xBF")):
		f, err := macho.NewFatFile(r)
		if err != nil || len(f.Arches) == 0 {
			return "", "", errUnrecognizedFormat
		}
		x = &machoExe{f.Arches[0].File}
	case bytes.HasPrefix(ident, []byte{0x01, 0xDF}) || bytes.HasPrefix(ident, []byte{0x01, 0xF7}):
		f, err := xcoff.NewFile(r)
		if err != nil {
			return "", "", errUnrecognizedFormat
		}
		x = &xcoffExe{f}
	case hasPlan9Magic(ident):
		f, err := plan9obj.NewFile(r)
		if err != nil {
			return "", "", errUnrecognizedFormat
		}
		x = &plan9objExe{f}
	default:
		return "", "", errUnrecognizedFormat
	}

	// 读取段或节以查找构建信息块。
	// 在某些平台上，该块会在自己的节中，DataStart 返回该节的地址。
	// 在其他平台上，它在数据段的某处；链接器会将其放在靠近开头的位置。
	// 参见 cmd/link/internal/ld.Link.buildinfo。
	dataAddr, dataSize := x.DataStart()
	if dataSize == 0 {
		return "", "", errNotGoExe
	}

	addr, err := searchMagic(x, dataAddr, dataSize)
	if err != nil {
		return "", "", err
	}

	// 首先读取完整的头部。
	header, err := readData(x, addr, buildInfoHeaderSize)
	if err == io.EOF {
		return "", "", errNotGoExe
	} else if err != nil {
		return "", "", err
	}
	if len(header) < buildInfoHeaderSize {
		return "", "", errNotGoExe
	}

	const (
		ptrSizeOffset = 14
		flagsOffset   = 15
		versPtrOffset = 16

		flagsEndianMask   = 0x1
		flagsEndianLittle = 0x0
		flagsEndianBig    = 0x1

		flagsVersionMask = 0x2
		flagsVersionPtr  = 0x0
		flagsVersionInl  = 0x2
	)

	// 解码该块。该块是一个 32 字节的头部，可选地后跟
	// 2 个 varint 前缀的字符串内容。
	//
	// type buildInfoHeader struct {
	// 	magic       [14]byte
	// 	ptrSize     uint8 // 如果是 flagsVersionPtr 则使用
	// 	flags       uint8
	// 	versPtr     targetUintptr // 如果是 flagsVersionPtr 则使用
	// 	modPtr      targetUintptr // 如果是 flagsVersionPtr 则使用
	// }
	//
	// flags 字段的版本位决定了格式的细节。
	//
	// 在 1.18 之前，flags 版本位是 flagsVersionPtr。在这种情况下，
	// 头部包含指向头部中版本和 modinfo Go 字符串的指针。
	// ptrSize 字段指示指针的大小，flag 的字节序位指示指针的字节序。
	//
	// 从 1.18 开始，flags 版本位是 flagsVersionInl。在这种情况下，
	// 头部后跟内联的字符串内容，作为长度前缀（varint）的字符串内容。
	// 首先是版本字符串，紧接着是 modinfo 字符串。
	flags := header[flagsOffset]
	if flags&flagsVersionMask == flagsVersionInl {
		vers, addr, err = decodeString(x, addr+buildInfoHeaderSize)
		if err != nil {
			return "", "", err
		}
		mod, _, err = decodeString(x, addr)
		if err != nil {
			return "", "", err
		}
	} else {
		// flagsVersionPtr（<1.18）
		ptrSize := int(header[ptrSizeOffset])
		bigEndian := flags&flagsEndianMask == flagsEndianBig
		var bo binary.ByteOrder
		if bigEndian {
			bo = binary.BigEndian
		} else {
			bo = binary.LittleEndian
		}
		var readPtr func([]byte) uint64
		if ptrSize == 4 {
			readPtr = func(b []byte) uint64 { return uint64(bo.Uint32(b)) }
		} else if ptrSize == 8 {
			readPtr = bo.Uint64
		} else {
			return "", "", errNotGoExe
		}
		vers = readString(x, ptrSize, readPtr, readPtr(header[versPtrOffset:]))
		mod = readString(x, ptrSize, readPtr, readPtr(header[versPtrOffset+ptrSize:]))
	}
	if vers == "" {
		return "", "", errNotGoExe
	}
	if len(mod) >= 33 && mod[len(mod)-17] == '\n' {
		// 剥离模块框架：分隔模块信息的哨兵字符串。
		// 这些是 cmd/go/internal/modload.infoStart 和 infoEnd。
		mod = mod[16 : len(mod)-16]
	} else {
		mod = ""
	}

	return vers, mod, nil
}

func hasPlan9Magic(magic []byte) bool {
	if len(magic) >= 4 {
		m := binary.BigEndian.Uint32(magic)
		switch m {
		case plan9obj.Magic386, plan9obj.MagicAMD64, plan9obj.MagicARM:
			return true
		}
	}
	return false
}

func decodeString(x exe, addr uint64) (string, uint64, error) {
	// varint 长度后跟 length 字节的数据。

	// 注意：ReadData 从包含 addr 的节中读取 _最多_ size 字节。
	// 所以我们不需要检查 size 是否会溢出该节。
	b, err := readData(x, addr, binary.MaxVarintLen64)
	if err == io.EOF {
		return "", 0, errNotGoExe
	} else if err != nil {
		return "", 0, err
	}

	length, n := binary.Uvarint(b)
	if n <= 0 {
		return "", 0, errNotGoExe
	}
	addr += uint64(n)

	b, err = readData(x, addr, length)
	if err == io.EOF {
		return "", 0, errNotGoExe
	} else if err == io.ErrUnexpectedEOF {
		// 长度太大无法分配。明显是无效值。
		return "", 0, errNotGoExe
	} else if err != nil {
		return "", 0, err
	}
	if uint64(len(b)) < length {
		// 节在我们读取完整字符串之前就结束了。
		return "", 0, errNotGoExe
	}

	return string(b), addr + length, nil
}

// readString 返回可执行文件 x 中地址 addr 处的字符串。
func readString(x exe, ptrSize int, readPtr func([]byte) uint64, addr uint64) string {
	hdr, err := readData(x, addr, uint64(2*ptrSize))
	if err != nil || len(hdr) < 2*ptrSize {
		return ""
	}
	dataAddr := readPtr(hdr)
	dataLen := readPtr(hdr[ptrSize:])
	data, err := readData(x, dataAddr, dataLen)
	if err != nil || uint64(len(data)) < dataLen {
		return ""
	}
	return string(data)
}

const searchChunkSize = 1 << 20 // 1 MB

// searchMagic 返回数据范围 [addr, addr+size) 中 buildInfoMagic 的第一个对齐实例。
// 如果未找到则返回错误。
func searchMagic(x exe, start, size uint64) (uint64, error) {
	end := start + size
	if end < start {
		// 溢出。
		return 0, errUnrecognizedFormat
	}

	// 向上取整 start；魔数不可能出现在初始未对齐部分。
	start = (start + buildInfoAlign - 1) &^ (buildInfoAlign - 1)
	if start >= end {
		return 0, errNotGoExe
	}

	var buf []byte
	for start < end {
		// 分块读取以避免在数据较大时消耗过多内存。
		//
		// 通常处理魔数跨越块边界的情况会比较麻烦，
		// 但由于它必须 16 字节对齐，我们知道它会落在单个块内。
		remaining := end - start
		chunkSize := uint64(searchChunkSize)
		if chunkSize > remaining {
			chunkSize = remaining
		}

		if buf == nil {
			buf = make([]byte, chunkSize)
		} else {
			// 注意：chunkSize 只能减小，且仅在最后一块时减小。
			buf = buf[:chunkSize]
			clear(buf)
		}

		n, err := readDataInto(x, start, buf)
		if err == io.EOF {
			// 在找到魔数之前遇到 EOF；一定不是 Go 可执行文件。
			return 0, errNotGoExe
		} else if err != nil {
			return 0, err
		}

		data := buf[:n]
		for len(data) > 0 {
			i := bytes.Index(data, buildInfoMagic)
			if i < 0 {
				break
			}
			if remaining-uint64(i) < buildInfoHeaderSize {
				// 找到魔数，但剩余空间不足以容纳完整头部。
				return 0, errNotGoExe
			}
			if i%buildInfoAlign != 0 {
				// 找到魔数，但未对齐。继续搜索。
				next := (i + buildInfoAlign - 1) &^ (buildInfoAlign - 1)
				if next > len(data) {
					// 损坏的目标文件：剩余计数表示还有更多数据，
					// 但我们没有读取到。
					return 0, errNotGoExe
				}
				data = data[next:]
				continue
			}
			// 匹配成功！
			return start + uint64(i), nil
		}

		start += chunkSize
	}

	return 0, errNotGoExe
}

func readData(x exe, addr, size uint64) ([]byte, error) {
	r, err := x.DataReader(addr)
	if err != nil {
		return nil, err
	}

	b, err := saferio.ReadDataAt(r, size, 0)
	if len(b) > 0 && err == io.EOF {
		err = nil
	}
	return b, err
}

func readDataInto(x exe, addr uint64, b []byte) (int, error) {
	r, err := x.DataReader(addr)
	if err != nil {
		return 0, err
	}

	n, err := r.ReadAt(b, 0)
	if n > 0 && err == io.EOF {
		err = nil
	}
	return n, err
}

// elfExe 是 exe 接口的 ELF 实现。
type elfExe struct {
	f *elf.File
}

func (x *elfExe) DataReader(addr uint64) (io.ReaderAt, error) {
	for _, prog := range x.f.Progs {
		if prog.Vaddr <= addr && addr <= prog.Vaddr+prog.Filesz-1 {
			remaining := prog.Vaddr + prog.Filesz - addr
			return io.NewSectionReader(prog, int64(addr-prog.Vaddr), int64(remaining)), nil
		}
	}
	return nil, errUnrecognizedFormat
}

func (x *elfExe) DataStart() (uint64, uint64) {
	for _, s := range x.f.Sections {
		if s.Name == ".go.buildinfo" {
			return s.Addr, s.Size
		}
	}
	for _, p := range x.f.Progs {
		if p.Type == elf.PT_LOAD && p.Flags&(elf.PF_X|elf.PF_W) == elf.PF_W {
			return p.Vaddr, p.Memsz
		}
	}
	return 0, 0
}

// peExe 是 exe 接口的 PE（Windows 可移植可执行文件）实现。
type peExe struct {
	f *pe.File
}

func (x *peExe) imageBase() uint64 {
	switch oh := x.f.OptionalHeader.(type) {
	case *pe.OptionalHeader32:
		return uint64(oh.ImageBase)
	case *pe.OptionalHeader64:
		return oh.ImageBase
	}
	return 0
}

func (x *peExe) DataReader(addr uint64) (io.ReaderAt, error) {
	addr -= x.imageBase()
	for _, sect := range x.f.Sections {
		if uint64(sect.VirtualAddress) <= addr && addr <= uint64(sect.VirtualAddress+sect.Size-1) {
			remaining := uint64(sect.VirtualAddress+sect.Size) - addr
			return io.NewSectionReader(sect, int64(addr-uint64(sect.VirtualAddress)), int64(remaining)), nil
		}
	}
	return nil, errUnrecognizedFormat
}

func (x *peExe) DataStart() (uint64, uint64) {
	// 假设数据在第一个可写节中。
	const (
		IMAGE_SCN_CNT_CODE               = 0x00000020
		IMAGE_SCN_CNT_INITIALIZED_DATA   = 0x00000040
		IMAGE_SCN_CNT_UNINITIALIZED_DATA = 0x00000080
		IMAGE_SCN_MEM_EXECUTE            = 0x20000000
		IMAGE_SCN_MEM_READ               = 0x40000000
		IMAGE_SCN_MEM_WRITE              = 0x80000000
		IMAGE_SCN_MEM_DISCARDABLE        = 0x2000000
		IMAGE_SCN_LNK_NRELOC_OVFL        = 0x1000000
		IMAGE_SCN_ALIGN_32BYTES          = 0x600000
	)
	for _, sect := range x.f.Sections {
		if sect.VirtualAddress != 0 && sect.Size != 0 &&
			sect.Characteristics&^IMAGE_SCN_ALIGN_32BYTES == IMAGE_SCN_CNT_INITIALIZED_DATA|IMAGE_SCN_MEM_READ|IMAGE_SCN_MEM_WRITE {
			return uint64(sect.VirtualAddress) + x.imageBase(), uint64(sect.VirtualSize)
		}
	}
	return 0, 0
}

// machoExe 是 exe 接口的 Mach-O（Apple macOS/iOS）实现。
type machoExe struct {
	f *macho.File
}

func (x *machoExe) DataReader(addr uint64) (io.ReaderAt, error) {
	for _, load := range x.f.Loads {
		seg, ok := load.(*macho.Segment)
		if !ok {
			continue
		}
		if seg.Addr <= addr && addr <= seg.Addr+seg.Filesz-1 {
			if seg.Name == "__PAGEZERO" {
				continue
			}
			remaining := seg.Addr + seg.Filesz - addr
			return io.NewSectionReader(seg, int64(addr-seg.Addr), int64(remaining)), nil
		}
	}
	return nil, errUnrecognizedFormat
}

func (x *machoExe) DataStart() (uint64, uint64) {
	// 查找名为 "__go_buildinfo" 的节。
	for _, sec := range x.f.Sections {
		if sec.Name == "__go_buildinfo" {
			return sec.Addr, sec.Size
		}
	}
	// 尝试第一个非空的可写段。
	const RW = 3
	for _, load := range x.f.Loads {
		seg, ok := load.(*macho.Segment)
		if ok && seg.Addr != 0 && seg.Filesz != 0 && seg.Prot == RW && seg.Maxprot == RW {
			return seg.Addr, seg.Memsz
		}
	}
	return 0, 0
}

// xcoffExe 是 exe 接口的 XCOFF（AIX 扩展 COFF）实现。
type xcoffExe struct {
	f *xcoff.File
}

func (x *xcoffExe) DataReader(addr uint64) (io.ReaderAt, error) {
	for _, sect := range x.f.Sections {
		if sect.VirtualAddress <= addr && addr <= sect.VirtualAddress+sect.Size-1 {
			remaining := sect.VirtualAddress + sect.Size - addr
			return io.NewSectionReader(sect, int64(addr-sect.VirtualAddress), int64(remaining)), nil
		}
	}
	return nil, errors.New("address not mapped")
}

func (x *xcoffExe) DataStart() (uint64, uint64) {
	if s := x.f.SectionByType(xcoff.STYP_DATA); s != nil {
		return s.VirtualAddress, s.Size
	}
	return 0, 0
}

// plan9objExe 是 exe 接口的 Plan 9 a.out 实现。
type plan9objExe struct {
	f *plan9obj.File
}

func (x *plan9objExe) DataStart() (uint64, uint64) {
	if s := x.f.Section("data"); s != nil {
		return uint64(s.Offset), uint64(s.Size)
	}
	return 0, 0
}

func (x *plan9objExe) DataReader(addr uint64) (io.ReaderAt, error) {
	for _, sect := range x.f.Sections {
		if uint64(sect.Offset) <= addr && addr <= uint64(sect.Offset+sect.Size-1) {
			remaining := uint64(sect.Offset+sect.Size) - addr
			return io.NewSectionReader(sect, int64(addr-uint64(sect.Offset)), int64(remaining)), nil
		}
	}
	return nil, errors.New("address not mapped")
}
