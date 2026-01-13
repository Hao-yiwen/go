// 版权所有 2009 The Go Authors。保留所有权利。
// 本源代码的使用受 BSD 风格许可证约束，
// 该许可证可在 LICENSE 文件中找到。

// DWARF 调试信息条目解析器。
// 条目是给定格式的数据项序列。
// 条目中的第一个字是 DWARF 所谓的"缩写表"的索引。
// 缩写实际上只是一个类型描述符：它是属性标签/值格式对的数组。

package dwarf

import (
	"encoding/binary"
	"errors"
	"fmt"
	"strconv"
)

// 单个条目的描述：属性序列
type abbrev struct {
	tag      Tag
	children bool
	field    []afield
}

type afield struct {
	attr  Attr
	fmt   format
	class Class
	val   int64 // 用于 formImplicitConst
}

// 从条目格式 ID 到其描述的映射
type abbrevTable map[uint32]abbrev

// parseAbbrev 返回从 .debug_abbrev 节中字节偏移 off 处开始的缩写表。
func (d *Data) parseAbbrev(off uint64, vers int) (abbrevTable, error) {
	if m, ok := d.abbrevCache[off]; ok {
		return m, nil
	}

	data := d.abbrev
	if off > uint64(len(data)) {
		data = nil
	} else {
		data = data[off:]
	}
	b := makeBuf(d, unknownFormat{}, "abbrev", 0, data)

	// 错误处理通过 buf 的 getter 在错误后返回无尽的 0 流来简化。
	m := make(abbrevTable)
	for {
		// 表以 id == 0 结束。
		id := uint32(b.uint())
		if id == 0 {
			break
		}

		// 遍历属性，进行计数。
		n := 0
		b1 := b // 从 b 的副本读取。
		b1.uint()
		b1.uint8()
		for {
			tag := b1.uint()
			fmt := b1.uint()
			if tag == 0 && fmt == 0 {
				break
			}
			if format(fmt) == formImplicitConst {
				b1.int()
			}
			n++
		}
		if b1.err != nil {
			return nil, b1.err
		}

		// 再次遍历属性，这次记录它们。
		var a abbrev
		a.tag = Tag(b.uint())
		a.children = b.uint8() != 0
		a.field = make([]afield, n)
		for i := range a.field {
			a.field[i].attr = Attr(b.uint())
			a.field[i].fmt = format(b.uint())
			a.field[i].class = formToClass(a.field[i].fmt, a.field[i].attr, vers, &b)
			if a.field[i].fmt == formImplicitConst {
				a.field[i].val = b.int()
			}
		}
		b.uint()
		b.uint()

		m[id] = a
	}
	if b.err != nil {
		return nil, b.err
	}
	d.abbrevCache[off] = m
	return m, nil
}

// attrIsExprloc 指示在 DWARF 2 和 3 中编码为块值的允许 exprloc 值的属性。
// 参见 DWARF 4，图 20。
var attrIsExprloc = map[Attr]bool{
	AttrLocation:      true,
	AttrByteSize:      true,
	AttrBitOffset:     true,
	AttrBitSize:       true,
	AttrStringLength:  true,
	AttrLowerBound:    true,
	AttrReturnAddr:    true,
	AttrStrideSize:    true,
	AttrUpperBound:    true,
	AttrCount:         true,
	AttrDataMemberLoc: true,
	AttrFrameBase:     true,
	AttrSegment:       true,
	AttrStaticLink:    true,
	AttrUseLocation:   true,
	AttrVtableElemLoc: true,
	AttrAllocated:     true,
	AttrAssociated:    true,
	AttrDataLocation:  true,
	AttrStride:        true,
}

// attrPtrClass 指示在 DWARF 4 中具有 formSecOffset 编码或在 DWARF 2 和 3 中
// 具有 formData* 编码的属性的 *ptr 类。
var attrPtrClass = map[Attr]Class{
	AttrLocation:      ClassLocListPtr,
	AttrStmtList:      ClassLinePtr,
	AttrStringLength:  ClassLocListPtr,
	AttrReturnAddr:    ClassLocListPtr,
	AttrStartScope:    ClassRangeListPtr,
	AttrDataMemberLoc: ClassLocListPtr,
	AttrFrameBase:     ClassLocListPtr,
	AttrMacroInfo:     ClassMacPtr,
	AttrSegment:       ClassLocListPtr,
	AttrStaticLink:    ClassLocListPtr,
	AttrUseLocation:   ClassLocListPtr,
	AttrVtableElemLoc: ClassLocListPtr,
	AttrRanges:        ClassRangeListPtr,
	// 以下是 DWARF 5 中新增的。
	AttrStrOffsetsBase: ClassStrOffsetsPtr,
	AttrAddrBase:       ClassAddrPtr,
	AttrRnglistsBase:   ClassRngListsPtr,
	AttrLoclistsBase:   ClassLocListPtr,
}

// formToClass 返回给定格式的 DWARF 4 Class。如果 DWARF 版本低于 4，
// 它将根据属性消除某些格式的歧义。
func formToClass(form format, attr Attr, vers int, b *buf) Class {
	switch form {
	default:
		b.error("cannot determine class of unknown attribute form")
		return 0

	case formIndirect:
		return ClassUnknown

	case formAddr, formAddrx, formAddrx1, formAddrx2, formAddrx3, formAddrx4:
		return ClassAddress

	case formDwarfBlock1, formDwarfBlock2, formDwarfBlock4, formDwarfBlock:
		// 在 DWARF 2 和 3 中，ClassExprLoc 被编码为块。
		// DWARF 4 区分 ClassBlock 和 ClassExprLoc，但没有属性可以同时是两者，
		// 所以我们也会将 DWARF 4 中应该是 ClassExprLoc 的 ClassBlock 值提升，
		// 以防生产者搞错。
		if attrIsExprloc[attr] {
			return ClassExprLoc
		}
		return ClassBlock

	case formData1, formData2, formData4, formData8, formSdata, formUdata, formData16, formImplicitConst:
		// 在 DWARF 2 和 3 中，ClassPtr 被编码为常量。
		// 与 ClassExprLoc/ClassBlock 不同，某些 DWARF 4 属性需要区分
		// Class*Ptr 和 ClassConstant，所以我们只对版本 2 和 3 进行此提升。
		if class, ok := attrPtrClass[attr]; vers < 4 && ok {
			return class
		}
		return ClassConstant

	case formFlag, formFlagPresent:
		return ClassFlag

	case formRefAddr, formRef1, formRef2, formRef4, formRef8, formRefUdata, formRefSup4, formRefSup8:
		return ClassReference

	case formRefSig8:
		return ClassReferenceSig

	case formString, formStrp, formStrx, formStrpSup, formLineStrp, formStrx1, formStrx2, formStrx3, formStrx4:
		return ClassString

	case formSecOffset:
		// DWARF 4 定义了四个 *ptr 类，但在编码中不区分它们。
		// 使用属性来消除这些类的歧义。
		if class, ok := attrPtrClass[attr]; ok {
			return class
		}
		return ClassUnknown

	case formExprloc:
		return ClassExprLoc

	case formGnuRefAlt:
		return ClassReferenceAlt

	case formGnuStrpAlt:
		return ClassStringAlt

	case formLoclistx:
		return ClassLocList

	case formRnglistx:
		return ClassRngList
	}
}

// Entry 是属性/值对的序列。
type Entry struct {
	Offset   Offset // Entry 在 DWARF info 中的偏移量
	Tag      Tag    // 标签（Entry 的类型）
	Children bool   // Entry 后面是否跟有子项
	Field    []Field
}

// Field 是 [Entry] 中的单个属性/值对。
//
// 值可以是 DWARF 定义的几种"属性类"之一。
// 每个类对应的 Go 类型如下：
//
//	DWARF 类           Go 类型         Class
//	-----------       -------        -----
//	address           uint64         ClassAddress
//	block             []byte         ClassBlock
//	constant          int64          ClassConstant
//	flag              bool           ClassFlag
//	reference
//	  to info         dwarf.Offset   ClassReference
//	  to type unit    uint64         ClassReferenceSig
//	string            string         ClassString
//	exprloc           []byte         ClassExprLoc
//	lineptr           int64          ClassLinePtr
//	loclistptr        int64          ClassLocListPtr
//	macptr            int64          ClassMacPtr
//	rangelistptr      int64          ClassRangeListPtr
//
// 对于无法识别或供应商定义的属性，[Class] 可能是 [ClassUnknown]。
type Field struct {
	Attr  Attr
	Val   any
	Class Class
}

// Class 是属性值的 DWARF 4 类。
//
// 通常，给定属性的值可以采用 DWARF 定义的几种可能类之一，
// 每种类都会导致对属性的略微不同的解释。
//
// DWARF 版本 4 比以前的 DWARF 版本更精细地区分属性值类。
// 读取器将把早期 DWARF 版本中较粗糙的类消除歧义为适当的 DWARF 4 类。
// 例如，DWARF 2 对常量和所有类型的节偏移量都使用"constant"，
// 但读取器会将 DWARF 2 文件中引用节偏移量的属性规范化为 Class*Ptr 类之一，
// 即使这些类只在 DWARF 3 中定义。
type Class int

const (
	// ClassUnknown 表示未知 DWARF 类的值。
	ClassUnknown Class = iota

	// ClassAddress 表示 uint64 类型的值，这些值是目标机器上的地址。
	ClassAddress

	// ClassBlock 表示 []byte 类型的值，其解释取决于属性。
	ClassBlock

	// ClassConstant 表示 int64 类型的常量值。此常量的解释取决于属性。
	ClassConstant

	// ClassExprLoc 表示包含编码的 DWARF 表达式或位置描述的 []byte 类型值。
	ClassExprLoc

	// ClassFlag 表示 bool 类型的值。
	ClassFlag

	// ClassLinePtr 表示 "line" 节中 int64 偏移量的值。
	ClassLinePtr

	// ClassLocListPtr 表示 "loclist" 节中 int64 偏移量的值。
	ClassLocListPtr

	// ClassMacPtr 表示 "mac" 节中 int64 偏移量的值。
	ClassMacPtr

	// ClassRangeListPtr 表示 "rangelist" 节中 int64 偏移量的值。
	ClassRangeListPtr

	// ClassReference 表示 info 节中 Entry 的 Offset 偏移量值
	// （用于 Reader.Seek）。DWARF 规范将 ClassReference 和
	// ClassReferenceSig 合并为 "reference" 类。
	ClassReference

	// ClassReferenceSig 表示引用类型 Entry 的 uint64 类型签名值。
	ClassReferenceSig

	// ClassString 表示字符串值。如果编译单元指定了 AttrUseUTF8 标志
	// （强烈推荐），字符串值将以 UTF-8 编码。否则，编码未指定。
	ClassString

	// ClassReferenceAlt 表示 int64 类型的值，是备用目标文件
	// DWARF "info" 节中的偏移量。
	ClassReferenceAlt

	// ClassStringAlt 表示 int64 类型的值，是备用目标文件
	// DWARF 字符串节中的偏移量。
	ClassStringAlt

	// ClassAddrPtr 表示 "addr" 节中 int64 偏移量的值。
	ClassAddrPtr

	// ClassLocList 表示 "loclists" 节中 int64 偏移量的值。
	ClassLocList

	// ClassRngList 表示从 "rnglists" 节基址起的 uint64 偏移量值。
	ClassRngList

	// ClassRngListsPtr 表示 "rnglists" 节中 int64 偏移量的值。
	// 这些用作 ClassRngList 值的基址。
	ClassRngListsPtr

	// ClassStrOffsetsPtr 表示 "str_offsets" 节中 int64 偏移量的值。
	ClassStrOffsetsPtr
)

//go:generate stringer -type=Class

func (i Class) GoString() string {
	return "dwarf." + i.String()
}

// Val 返回 [Entry] 中与属性 [Attr] 关联的值，如果没有此属性则返回 nil。
//
// 常见的惯用法是将 nil 返回检查与值具有预期动态类型的检查合并，如：
//
//	v, ok := e.Val(AttrSibling).(int64)
func (e *Entry) Val(a Attr) any {
	if f := e.AttrField(a); f != nil {
		return f.Val
	}
	return nil
}

// AttrField 返回 [Entry] 中与属性 [Attr] 关联的 [Field]，如果没有此属性则返回 nil。
func (e *Entry) AttrField(a Attr) *Field {
	for i, f := range e.Field {
		if f.Attr == a {
			return &e.Field[i]
		}
	}
	return nil
}

// Offset 表示 [Entry] 在 DWARF info 中的位置。
// （参见 [Reader.Seek]。）
type Offset uint32

// entry 从 buf 读取单个条目，根据给定的缩写表进行解码。
func (b *buf) entry(cu *Entry, u *unit) *Entry {
	atab, ubase, vers := u.atable, u.base, u.vers
	off := b.off
	id := uint32(b.uint())
	if id == 0 {
		return &Entry{}
	}
	a, ok := atab[id]
	if !ok {
		b.error("unknown abbreviation table index")
		return nil
	}
	e := &Entry{
		Offset:   off,
		Tag:      a.tag,
		Children: a.children,
		Field:    make([]Field, len(a.field)),
	}

	resolveStrx := func(strBase, off uint64) string {
		off += strBase
		if uint64(int(off)) != off {
			b.error("DW_FORM_strx offset out of range")
		}

		b1 := makeBuf(b.dwarf, b.format, "str_offsets", 0, b.dwarf.strOffsets)
		b1.skip(int(off))
		is64, _ := b.format.dwarf64()
		if is64 {
			off = b1.uint64()
		} else {
			off = uint64(b1.uint32())
		}
		if b1.err != nil {
			b.err = b1.err
			return ""
		}
		if uint64(int(off)) != off {
			b.error("DW_FORM_strx indirect offset out of range")
		}
		b1 = makeBuf(b.dwarf, b.format, "str", 0, b.dwarf.str)
		b1.skip(int(off))
		val := b1.string()
		if b1.err != nil {
			b.err = b1.err
		}
		return val
	}

	resolveRnglistx := func(rnglistsBase, off uint64) uint64 {
		is64, _ := b.format.dwarf64()
		if is64 {
			off *= 8
		} else {
			off *= 4
		}
		off += rnglistsBase
		if uint64(int(off)) != off {
			b.error("DW_FORM_rnglistx offset out of range")
		}

		b1 := makeBuf(b.dwarf, b.format, "rnglists", 0, b.dwarf.rngLists)
		b1.skip(int(off))
		if is64 {
			off = b1.uint64()
		} else {
			off = uint64(b1.uint32())
		}
		if b1.err != nil {
			b.err = b1.err
			return 0
		}
		if uint64(int(off)) != off {
			b.error("DW_FORM_rnglistx indirect offset out of range")
		}
		return rnglistsBase + off
	}

	for i := range e.Field {
		e.Field[i].Attr = a.field[i].attr
		e.Field[i].Class = a.field[i].class
		fmt := a.field[i].fmt
		if fmt == formIndirect {
			fmt = format(b.uint())
			e.Field[i].Class = formToClass(fmt, a.field[i].attr, vers, b)
		}
		var val any
		switch fmt {
		default:
			b.error("unknown entry attr format 0x" + strconv.FormatInt(int64(fmt), 16))

		// 地址
		case formAddr:
			val = b.addr()
		case formAddrx, formAddrx1, formAddrx2, formAddrx3, formAddrx4:
			var off uint64
			switch fmt {
			case formAddrx:
				off = b.uint()
			case formAddrx1:
				off = uint64(b.uint8())
			case formAddrx2:
				off = uint64(b.uint16())
			case formAddrx3:
				off = uint64(b.uint24())
			case formAddrx4:
				off = uint64(b.uint32())
			}
			if b.dwarf.addr == nil {
				b.error("DW_FORM_addrx with no .debug_addr section")
			}
			if b.err != nil {
				return nil
			}

			addrBase := int64(u.addrBase())
			var err error
			val, err = b.dwarf.debugAddr(b.format, uint64(addrBase), off)
			if err != nil {
				if b.err == nil {
					b.err = err
				}
				return nil
			}

		// 块
		case formDwarfBlock1:
			val = b.bytes(int(b.uint8()))
		case formDwarfBlock2:
			val = b.bytes(int(b.uint16()))
		case formDwarfBlock4:
			val = b.bytes(int(b.uint32()))
		case formDwarfBlock:
			val = b.bytes(int(b.uint()))

		// 常量
		case formData1:
			val = int64(b.uint8())
		case formData2:
			val = int64(b.uint16())
		case formData4:
			val = int64(b.uint32())
		case formData8:
			val = int64(b.uint64())
		case formData16:
			val = b.bytes(16)
		case formSdata:
			val = b.int()
		case formUdata:
			val = int64(b.uint())
		case formImplicitConst:
			val = a.field[i].val

		// 标志
		case formFlag:
			val = b.uint8() == 1
		// DWARF 4 中新增。
		case formFlagPresent:
			// 属性被隐式指示为存在，且调试信息条目本身中没有编码值。
			val = true

		// 对其他条目的引用
		case formRefAddr:
			vers := b.format.version()
			if vers == 0 {
				b.error("unknown version for DW_FORM_ref_addr")
			} else if vers == 2 {
				val = Offset(b.addr())
			} else {
				is64, known := b.format.dwarf64()
				if !known {
					b.error("unknown size for DW_FORM_ref_addr")
				} else if is64 {
					val = Offset(b.uint64())
				} else {
					val = Offset(b.uint32())
				}
			}
		case formRef1:
			val = Offset(b.uint8()) + ubase
		case formRef2:
			val = Offset(b.uint16()) + ubase
		case formRef4:
			val = Offset(b.uint32()) + ubase
		case formRef8:
			val = Offset(b.uint64()) + ubase
		case formRefUdata:
			val = Offset(b.uint()) + ubase

		// 字符串
		case formString:
			val = b.string()
		case formStrp, formLineStrp:
			var off uint64 // .debug_str 中的偏移量
			is64, known := b.format.dwarf64()
			if !known {
				b.error("unknown size for DW_FORM_strp/line_strp")
			} else if is64 {
				off = b.uint64()
			} else {
				off = uint64(b.uint32())
			}
			if uint64(int(off)) != off {
				b.error("DW_FORM_strp/line_strp offset out of range")
			}
			if b.err != nil {
				return nil
			}
			var b1 buf
			if fmt == formStrp {
				b1 = makeBuf(b.dwarf, b.format, "str", 0, b.dwarf.str)
			} else {
				if len(b.dwarf.lineStr) == 0 {
					b.error("DW_FORM_line_strp with no .debug_line_str section")
					return nil
				}
				b1 = makeBuf(b.dwarf, b.format, "line_str", 0, b.dwarf.lineStr)
			}
			b1.skip(int(off))
			val = b1.string()
			if b1.err != nil {
				b.err = b1.err
				return nil
			}
		case formStrx, formStrx1, formStrx2, formStrx3, formStrx4:
			var off uint64
			switch fmt {
			case formStrx:
				off = b.uint()
			case formStrx1:
				off = uint64(b.uint8())
			case formStrx2:
				off = uint64(b.uint16())
			case formStrx3:
				off = uint64(b.uint24())
			case formStrx4:
				off = uint64(b.uint32())
			}
			if len(b.dwarf.strOffsets) == 0 {
				b.error("DW_FORM_strx with no .debug_str_offsets section")
			}
			is64, known := b.format.dwarf64()
			if !known {
				b.error("unknown offset size for DW_FORM_strx")
			}
			if b.err != nil {
				return nil
			}
			if is64 {
				off *= 8
			} else {
				off *= 4
			}

			strBase := int64(u.strOffsetsBase())
			val = resolveStrx(uint64(strBase), off)

		case formStrpSup:
			is64, known := b.format.dwarf64()
			if !known {
				b.error("unknown size for DW_FORM_strp_sup")
			} else if is64 {
				val = b.uint64()
			} else {
				val = b.uint32()
			}

		// lineptr、loclistptr、macptr、rangelistptr
		// DWARF 4 中新增，但 clang 可以用 -gdwarf-2 生成它们。
		// 节引用，替代 formData4 和 formData8 的使用。
		case formSecOffset, formGnuRefAlt, formGnuStrpAlt:
			is64, known := b.format.dwarf64()
			if !known {
				b.error("unknown size for form 0x" + strconv.FormatInt(int64(fmt), 16))
			} else if is64 {
				val = int64(b.uint64())
			} else {
				val = int64(b.uint32())
			}

		// exprloc
		// DWARF 4 中新增。
		case formExprloc:
			val = b.bytes(int(b.uint()))

		// 引用
		// DWARF 4 中新增。
		case formRefSig8:
			// 64 位类型签名。
			val = b.uint64()
		case formRefSup4:
			val = b.uint32()
		case formRefSup8:
			val = b.uint64()

		// 位置列表
		case formLoclistx:
			val = b.uint()

		// 范围列表
		case formRnglistx:
			off := b.uint()

			rnglistsBase := int64(u.rngListsBase())
			val = resolveRnglistx(uint64(rnglistsBase), off)
		}

		e.Field[i].Val = val
	}
	if b.err != nil {
		return nil
	}
	return e
}

// Reader 允许从 DWARF "info" 节读取 [Entry] 结构。
// [Entry] 结构被安排成树形结构。[Reader.Next] 函数从树的前序遍历中返回连续的条目。
// 如果条目有子项，其 Children 字段将为 true，子项随后跟随，
// 以 [Tag] 为 0 的 [Entry] 结束。
type Reader struct {
	b            buf
	d            *Data
	err          error
	unit         int
	lastUnit     bool   // 如果 Next 返回的最后一个条目是 TagCompileUnit/TagPartialUnit 则设置
	lastChildren bool   // Next 返回的最后一个条目的 .Children
	lastSibling  Offset // Next 返回的最后一个条目的 .Val(AttrSibling)
	cu           *Entry // 当前编译单元
}

// Reader 为 [Data] 返回一个新的 Reader。
// 读取器定位在 DWARF "info" 节的字节偏移 0 处。
func (d *Data) Reader() *Reader {
	r := &Reader{d: d}
	r.Seek(0)
	return r
}

// AddressSize 返回当前编译单元中地址的字节大小。
func (r *Reader) AddressSize() int {
	return r.d.unit[r.unit].asize
}

// ByteOrder 返回当前编译单元中的字节序。
func (r *Reader) ByteOrder() binary.ByteOrder {
	return r.b.order
}

// Seek 将 [Reader] 定位到编码条目流中的偏移 off 处。
// 偏移 0 可用于表示第一个条目。
func (r *Reader) Seek(off Offset) {
	d := r.d
	r.err = nil
	r.lastChildren = false
	if off == 0 {
		if len(d.unit) == 0 {
			return
		}
		u := &d.unit[0]
		r.unit = 0
		r.b = makeBuf(r.d, u, "info", u.off, u.data)
		r.collectDwarf5BaseOffsets(u)
		r.cu = nil
		return
	}

	i := d.offsetToUnit(off)
	if i == -1 {
		r.err = errors.New("offset out of range")
		return
	}
	if i != r.unit {
		r.cu = nil
	}
	u := &d.unit[i]
	r.unit = i
	r.b = makeBuf(r.d, u, "info", off, u.data[off-u.off:])
	r.collectDwarf5BaseOffsets(u)
}

// maybeNextUnit 如果当前单元已完成则前进到下一个单元。
func (r *Reader) maybeNextUnit() {
	for len(r.b.data) == 0 && r.unit+1 < len(r.d.unit) {
		r.nextUnit()
	}
}

// nextUnit 前进到下一个单元。
func (r *Reader) nextUnit() {
	r.unit++
	u := &r.d.unit[r.unit]
	r.b = makeBuf(r.d, u, "info", u.off, u.data)
	r.cu = nil
	r.collectDwarf5BaseOffsets(u)
}

func (r *Reader) collectDwarf5BaseOffsets(u *unit) {
	if u.vers < 5 || u.unit5 != nil {
		return
	}
	u.unit5 = new(unit5)
	if err := r.d.collectDwarf5BaseOffsets(u); err != nil {
		r.err = err
	}
}

// Next 从编码的条目流中读取下一个条目。
// 当到达节末尾时返回 nil, nil。
// 如果当前偏移无效或偏移处的数据无法解码为有效的 [Entry]，则返回错误。
func (r *Reader) Next() (*Entry, error) {
	if r.err != nil {
		return nil, r.err
	}
	r.maybeNextUnit()
	if len(r.b.data) == 0 {
		return nil, nil
	}
	u := &r.d.unit[r.unit]
	e := r.b.entry(r.cu, u)
	if r.b.err != nil {
		r.err = r.b.err
		return nil, r.err
	}
	r.lastUnit = false
	if e != nil {
		r.lastChildren = e.Children
		if r.lastChildren {
			r.lastSibling, _ = e.Val(AttrSibling).(Offset)
		}
		if e.Tag == TagCompileUnit || e.Tag == TagPartialUnit {
			r.lastUnit = true
			r.cu = e
		}
	} else {
		r.lastChildren = false
	}
	return e, nil
}

// SkipChildren 跳过与 [Reader.Next] 返回的最后一个 [Entry] 关联的子条目。
// 如果该 [Entry] 没有子项或尚未调用 [Reader.Next]，则 SkipChildren 是空操作。
func (r *Reader) SkipChildren() {
	if r.err != nil || !r.lastChildren {
		return
	}

	// 如果最后一个条目有兄弟属性，
	// 该属性给出下一个兄弟的偏移量，
	// 因此我们可以避免解码子树。
	if r.lastSibling >= r.b.off {
		r.Seek(r.lastSibling)
		return
	}

	if r.lastUnit && r.unit+1 < len(r.d.unit) {
		r.nextUnit()
		return
	}

	for {
		e, err := r.Next()
		if err != nil || e == nil || e.Tag == 0 {
			break
		}
		if e.Children {
			r.SkipChildren()
		}
	}
}

// clone 返回读取器的副本。这由 typeReader 接口使用。
func (r *Reader) clone() typeReader {
	return r.d.Reader()
}

// offset 返回当前缓冲区偏移量。这由 typeReader 接口使用。
func (r *Reader) offset() Offset {
	return r.b.off
}

// SeekPC 返回包含 pc 的编译单元的 [Entry]，并将读取器定位以读取该单元的子项。
// 如果 pc 不被任何单元覆盖，SeekPC 返回 [ErrUnknownPC] 且读取器位置未定义。
//
// 因为编译单元可以描述可执行文件的多个区域，最坏情况下 SeekPC 必须搜索
// 所有编译单元中的所有范围。每次调用 SeekPC 都从上次调用的编译单元开始搜索，
// 因此通常如果 PC 是排序的，查找一系列 PC 会更快。如果调用者希望重复快速查找 PC，
// 应使用 Ranges 方法构建适当的索引。
func (r *Reader) SeekPC(pc uint64) (*Entry, error) {
	unit := r.unit
	for i := 0; i < len(r.d.unit); i++ {
		if unit >= len(r.d.unit) {
			unit = 0
		}
		r.err = nil
		r.lastChildren = false
		r.unit = unit
		r.cu = nil
		u := &r.d.unit[unit]
		r.b = makeBuf(r.d, u, "info", u.off, u.data)
		r.collectDwarf5BaseOffsets(u)
		e, err := r.Next()
		if err != nil {
			return nil, err
		}
		if e == nil || e.Tag == 0 {
			return nil, ErrUnknownPC
		}
		ranges, err := r.d.Ranges(e)
		if err != nil {
			return nil, err
		}
		for _, pcs := range ranges {
			if pcs[0] <= pc && pc < pcs[1] {
				return e, nil
			}
		}
		unit++
	}
	return nil, ErrUnknownPC
}

// Ranges 返回 e 覆盖的 PC 范围，是 [low,high) 对的切片。
// 只有某些条目类型（如 [TagCompileUnit] 或 [TagSubprogram]）具有 PC 范围；
// 对于其他类型，这将返回 nil 且无错误。
func (d *Data) Ranges(e *Entry) ([][2]uint64, error) {
	var ret [][2]uint64

	low, lowOK := e.Val(AttrLowpc).(uint64)

	var high uint64
	var highOK bool
	highField := e.AttrField(AttrHighpc)
	if highField != nil {
		switch highField.Class {
		case ClassAddress:
			high, highOK = highField.Val.(uint64)
		case ClassConstant:
			off, ok := highField.Val.(int64)
			if ok {
				high = low + uint64(off)
				highOK = true
			}
		}
	}

	if lowOK && highOK {
		ret = append(ret, [2]uint64{low, high})
	}

	var u *unit
	if uidx := d.offsetToUnit(e.Offset); uidx >= 0 && uidx < len(d.unit) {
		u = &d.unit[uidx]
	}

	if u != nil && u.vers >= 5 && d.rngLists != nil {
		// DWARF 版本 5 及更高版本
		field := e.AttrField(AttrRanges)
		if field == nil {
			return ret, nil
		}
		switch field.Class {
		case ClassRangeListPtr:
			ranges, rangesOK := field.Val.(int64)
			if !rangesOK {
				return ret, nil
			}
			cu, base, err := d.baseAddressForEntry(e)
			if err != nil {
				return nil, err
			}
			return d.dwarf5Ranges(u, cu, base, ranges, ret)

		case ClassRngList:
			rnglist, ok := field.Val.(uint64)
			if !ok {
				return ret, nil
			}
			cu, base, err := d.baseAddressForEntry(e)
			if err != nil {
				return nil, err
			}
			return d.dwarf5Ranges(u, cu, base, int64(rnglist), ret)

		default:
			return ret, nil
		}
	}

	// DWARF 版本 2 到 4
	ranges, rangesOK := e.Val(AttrRanges).(int64)
	if rangesOK && d.ranges != nil {
		_, base, err := d.baseAddressForEntry(e)
		if err != nil {
			return nil, err
		}
		return d.dwarf2Ranges(u, base, ranges, ret)
	}

	return ret, nil
}

// baseAddressForEntry 返回查找条目 e 的范围列表时使用的初始基地址。
// DWARF 规定这应该是包含的编译单元的 lowpc 属性，
// 但是 gdb/dwarf2read.c 中的注释说某些版本的 GCC 使用 entrypc 属性，
// 所以我们也检查那个。
func (d *Data) baseAddressForEntry(e *Entry) (*Entry, uint64, error) {
	var cu *Entry
	if e.Tag == TagCompileUnit {
		cu = e
	} else {
		i := d.offsetToUnit(e.Offset)
		if i == -1 {
			return nil, 0, errors.New("no unit for entry")
		}
		u := &d.unit[i]
		b := makeBuf(d, u, "info", u.off, u.data)
		cu = b.entry(nil, u)
		if b.err != nil {
			return nil, 0, b.err
		}
	}

	if cuEntry, cuEntryOK := cu.Val(AttrEntrypc).(uint64); cuEntryOK {
		return cu, cuEntry, nil
	} else if cuLow, cuLowOK := cu.Val(AttrLowpc).(uint64); cuLowOK {
		return cu, cuLow, nil
	}

	return cu, 0, nil
}

func (d *Data) dwarf2Ranges(u *unit, base uint64, ranges int64, ret [][2]uint64) ([][2]uint64, error) {
	if ranges < 0 || ranges > int64(len(d.ranges)) {
		return nil, fmt.Errorf("invalid range offset %d (max %d)", ranges, len(d.ranges))
	}
	buf := makeBuf(d, u, "ranges", Offset(ranges), d.ranges[ranges:])
	for len(buf.data) > 0 {
		low := buf.addr()
		high := buf.addr()

		if low == 0 && high == 0 {
			break
		}

		if low == ^uint64(0)>>uint((8-u.addrsize())*8) {
			base = high
		} else {
			ret = append(ret, [2]uint64{base + low, base + high})
		}
	}

	return ret, nil
}

// dwarf5Ranges 解释 debug_rnglists 序列，参见 DWARFv5 第 2.17.3 节（第 53 页）。
func (d *Data) dwarf5Ranges(u *unit, cu *Entry, base uint64, ranges int64, ret [][2]uint64) ([][2]uint64, error) {
	if ranges < 0 || ranges > int64(len(d.rngLists)) {
		return nil, fmt.Errorf("invalid rnglist offset %d (max %d)", ranges, len(d.ranges))
	}
	var addrBase int64
	if cu != nil {
		addrBase, _ = cu.Val(AttrAddrBase).(int64)
	}

	buf := makeBuf(d, u, "rnglists", 0, d.rngLists)
	buf.skip(int(ranges))
	for {
		opcode := buf.uint8()
		switch opcode {
		case rleEndOfList:
			if buf.err != nil {
				return nil, buf.err
			}
			return ret, nil

		case rleBaseAddressx:
			baseIdx := buf.uint()
			var err error
			base, err = d.debugAddr(u, uint64(addrBase), baseIdx)
			if err != nil {
				return nil, err
			}

		case rleStartxEndx:
			startIdx := buf.uint()
			endIdx := buf.uint()

			start, err := d.debugAddr(u, uint64(addrBase), startIdx)
			if err != nil {
				return nil, err
			}
			end, err := d.debugAddr(u, uint64(addrBase), endIdx)
			if err != nil {
				return nil, err
			}
			ret = append(ret, [2]uint64{start, end})

		case rleStartxLength:
			startIdx := buf.uint()
			len := buf.uint()
			start, err := d.debugAddr(u, uint64(addrBase), startIdx)
			if err != nil {
				return nil, err
			}
			ret = append(ret, [2]uint64{start, start + len})

		case rleOffsetPair:
			off1 := buf.uint()
			off2 := buf.uint()
			ret = append(ret, [2]uint64{base + off1, base + off2})

		case rleBaseAddress:
			base = buf.addr()

		case rleStartEnd:
			start := buf.addr()
			end := buf.addr()
			ret = append(ret, [2]uint64{start, end})

		case rleStartLength:
			start := buf.addr()
			len := buf.uint()
			ret = append(ret, [2]uint64{start, start + len})
		}
	}
}

// debugAddr 返回 debug_addr 中索引 idx 处的地址
func (d *Data) debugAddr(format dataFormat, addrBase, idx uint64) (uint64, error) {
	off := idx*uint64(format.addrsize()) + addrBase

	if uint64(int(off)) != off {
		return 0, errors.New("offset out of range")
	}

	b := makeBuf(d, format, "addr", 0, d.addr)
	b.skip(int(off))
	val := b.addr()
	if b.err != nil {
		return 0, b.err
	}
	return val, nil
}
