// 版权所有 2009 The Go Authors。保留所有权利。
// 本源代码的使用受 BSD 风格许可证约束，
// 该许可证可在 LICENSE 文件中找到。

/*
Package dwarf 提供对从可执行文件加载的 DWARF 调试信息的访问，
如 http://dwarfstd.org/doc/dwarf-2.0.0.pdf 中定义的 DWARF 2.0 标准所述。

# 安全性

此包并非设计为能够抵御恶意输入，且不在 https://go.dev/security/policy 的范围内。
特别是，解析目标文件时仅进行基本验证。因此，在解析不受信任的输入时应格外小心，
因为解析格式错误的文件可能会消耗大量资源或导致 panic。
*/
package dwarf

import (
	"encoding/binary"
	"errors"
)

// Data 表示从可执行文件（例如 ELF 或 Mach-O 可执行文件）加载的 DWARF 调试信息。
type Data struct {
	// 原始数据
	abbrev   []byte
	aranges  []byte
	frame    []byte
	info     []byte
	line     []byte
	pubnames []byte
	ranges   []byte
	str      []byte

	// DWARF 5 中新增的节。
	addr       []byte
	lineStr    []byte
	strOffsets []byte
	rngLists   []byte

	// 已解析的数据
	abbrevCache map[uint64]abbrevTable
	bigEndian   bool
	order       binary.ByteOrder
	typeCache   map[Offset]Type
	typeSigs    map[uint64]*typeUnit
	unit        []unit
}

var errSegmentSelector = errors.New("non-zero segment_selector size not supported")

// New 返回一个从给定参数初始化的新 [Data] 对象。
// 与其直接调用此函数，客户端通常应使用相应包 [debug/elf]、
// [debug/macho] 或 [debug/pe] 的 File 类型的 DWARF 方法。
//
// []byte 参数是目标文件中相应调试节的数据；
// 例如，对于 ELF 目标文件，abbrev 是 ".debug_abbrev" 节的内容。
func New(abbrev, aranges, frame, info, line, pubnames, ranges, str []byte) (*Data, error) {
	d := &Data{
		abbrev:      abbrev,
		aranges:     aranges,
		frame:       frame,
		info:        info,
		line:        line,
		pubnames:    pubnames,
		ranges:      ranges,
		str:         str,
		abbrevCache: make(map[uint64]abbrevTable),
		typeCache:   make(map[Offset]Type),
		typeSigs:    make(map[uint64]*typeUnit),
	}

	// 嗅探 .debug_info 以确定字节序。
	// 32 位 DWARF：4 字节长度，2 字节版本。
	// 64 位 DWARF：4 字节 0xff，8 字节长度，2 字节版本。
	if len(d.info) < 6 {
		return nil, DecodeError{"info", Offset(len(d.info)), "too short"}
	}
	offset := 4
	if d.info[0] == 0xff && d.info[1] == 0xff && d.info[2] == 0xff && d.info[3] == 0xff {
		if len(d.info) < 14 {
			return nil, DecodeError{"info", Offset(len(d.info)), "too short"}
		}
		offset = 12
	}
	// 获取版本号，一个小的 16 位数字（1、2、3、4、5）。
	x, y := d.info[offset], d.info[offset+1]
	switch {
	case x == 0 && y == 0:
		return nil, DecodeError{"info", 4, "unsupported version 0"}
	case x == 0:
		d.bigEndian = true
		d.order = binary.BigEndian
	case y == 0:
		d.bigEndian = false
		d.order = binary.LittleEndian
	default:
		return nil, DecodeError{"info", 4, "cannot determine byte order"}
	}

	u, err := d.parseUnits()
	if err != nil {
		return nil, err
	}
	d.unit = u
	return d, nil
}

// AddTypes 将一个 .debug_types 节添加到 DWARF 数据中。
// 具有 DWARF 版本 4 调试信息的典型目标文件将有多个 .debug_types 节。
// name 仅用于错误报告，用于区分不同的 .debug_types 节。
func (d *Data) AddTypes(name string, types []byte) error {
	return d.parseTypes(name, types)
}

// AddSection 按名称添加另一个 DWARF 节。名称应该是
// DWARF 节名称，如 ".debug_addr"、".debug_str_offsets" 等。
// 此方法用于 DWARF 5 及更高版本中新增的 DWARF 节。
func (d *Data) AddSection(name string, contents []byte) error {
	var err error
	switch name {
	case ".debug_addr":
		d.addr = contents
	case ".debug_line_str":
		d.lineStr = contents
	case ".debug_str_offsets":
		d.strOffsets = contents
	case ".debug_rnglists":
		d.rngLists = contents
	}
	// 只是忽略我们尚不支持的名称。
	return err
}
