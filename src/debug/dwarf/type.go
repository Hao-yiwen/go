// 版权所有 2009 The Go Authors。保留所有权利。
// 本源代码的使用受 BSD 风格许可证约束，
// 该许可证可在 LICENSE 文件中找到。

// DWARF 类型信息结构。
// 格式严重偏向 C，但为简单起见，String 方法使用伪 Go 语法。

package dwarf

import "strconv"

// Type 按惯例表示指向任何特定 Type 结构（[CharType]、[StructType] 等）的指针。
type Type interface {
	Common() *CommonType
	String() string
	Size() int64
}

// CommonType 保存多种类型共有的字段。
// 如果某个字段未知或不适用于给定类型，则使用零值。
type CommonType struct {
	ByteSize int64  // 此类型值的大小，以字节为单位
	Name     string // 可用于引用类型的名称
}

func (c *CommonType) Common() *CommonType { return c }

func (c *CommonType) Size() int64 { return c.ByteSize }

// 基本类型

// BasicType 保存所有基本类型共有的字段。
//
// 有关 BitSize/BitOffset/DataBitOffset 字段解释的更多信息，
// 请参阅 [StructField] 的文档。
type BasicType struct {
	CommonType
	BitSize       int64
	BitOffset     int64
	DataBitOffset int64
}

func (b *BasicType) Basic() *BasicType { return b }

func (t *BasicType) String() string {
	if t.Name != "" {
		return t.Name
	}
	return "?"
}

// CharType 表示有符号字符类型。
type CharType struct {
	BasicType
}

// UcharType 表示无符号字符类型。
type UcharType struct {
	BasicType
}

// IntType 表示有符号整数类型。
type IntType struct {
	BasicType
}

// UintType 表示无符号整数类型。
type UintType struct {
	BasicType
}

// FloatType 表示浮点类型。
type FloatType struct {
	BasicType
}

// ComplexType 表示复数浮点类型。
type ComplexType struct {
	BasicType
}

// BoolType 表示布尔类型。
type BoolType struct {
	BasicType
}

// AddrType 表示机器地址类型。
type AddrType struct {
	BasicType
}

// UnspecifiedType 表示隐式、未知、模糊或不存在的类型。
type UnspecifiedType struct {
	BasicType
}

// 类型限定符

// QualType 表示具有 C/C++ "const"、"restrict" 或 "volatile" 限定符的类型。
type QualType struct {
	CommonType
	Qual string
	Type Type
}

func (t *QualType) String() string { return t.Qual + " " + t.Type.String() }

func (t *QualType) Size() int64 { return t.Type.Size() }

// ArrayType 表示固定大小的数组类型。
type ArrayType struct {
	CommonType
	Type          Type
	StrideBitSize int64 // 如果 > 0，表示容纳每个元素所需的位数
	Count         int64 // 如果 == -1，表示不完整数组，如 char x[]。
}

func (t *ArrayType) String() string {
	return "[" + strconv.FormatInt(t.Count, 10) + "]" + t.Type.String()
}

func (t *ArrayType) Size() int64 {
	if t.Count == -1 {
		return 0
	}
	return t.Count * t.Type.Size()
}

// VoidType 表示 C 语言的 void 类型。
type VoidType struct {
	CommonType
}

func (t *VoidType) String() string { return "void" }

// PtrType 表示指针类型。
type PtrType struct {
	CommonType
	Type Type
}

func (t *PtrType) String() string { return "*" + t.Type.String() }

// StructType 表示结构体、联合体或 C++ 类类型。
type StructType struct {
	CommonType
	StructName string
	Kind       string // "struct"、"union" 或 "class"。
	Field      []*StructField
	Incomplete bool // 如果为 true，表示结构体、联合体或类已声明但未定义
}

// StructField 表示结构体、联合体或 C++ 类类型中的字段。
//
// # 位字段
//
// BitSize、BitOffset 和 DataBitOffset 字段描述了在 C/C++ 结构体/联合体/类类型中
// 声明为位字段的数据成员的位大小和偏移量。
//
// BitSize 是位字段中的位数。
//
// DataBitOffset（如果非零）是从包含实体（例如包含的结构体/类/联合体）的起始位置
// 到位字段起始位置的位数。这对应于 DWARF 4 中引入的 DW_AT_data_bit_offset DWARF 属性。
//
// BitOffset（如果非零）是从存储位字段的存储单元的最高有效位到位字段的最高有效位之间的位数。
// 这里的"存储单元"是位字段之前的类型名称（对于字段 "unsigned x:17"，
// 存储单元是 "unsigned"）。BitOffset 值可能因系统的字节序而异。
// BitOffset 对应于 DW_AT_bit_offset DWARF 属性，该属性在 DWARF 4 中已被弃用，
// 并在 DWARF 5 中被移除。
//
// DataBitOffset 和 BitOffset 中最多有一个非零；
// 只有当 BitSize 非零时，DataBitOffset/BitOffset 才会非零。
// C 编译器使用哪一个取决于编译器版本和命令行选项。
//
// 以下是 C/C++ 位字段使用的示例，以及预期的 DWARF 位偏移信息。考虑这段代码：
//
//	struct S {
//		int q;
//		int j:5;
//		int k:6;
//		int m:5;
//		int n:8;
//	} s;
//
// 对于上述代码，使用 GCC 8 时，DW_AT_bit_offset 值预期如下：
//
//	       小端     |     大端
//	       模式     |     模式
//	                |
//	"j":     27     |     0
//	"k":     21     |     5
//	"m":     16     |     11
//	"n":     8      |     16
//
// 请注意，上述偏移量纯粹是相对于 j/k/m/n 的包含存储单元——
// 这些值不会因包含结构体中先前数据成员的大小而变化。
//
// 如果编译器输出 DW_AT_data_bit_offset，预期值将是：
//
//	"j":     32
//	"k":     37
//	"m":     43
//	"n":     48
//
// 这里 "j" 的值 32 反映了位字段之前有其他数据成员的事实
// （请记住 DW_AT_data_bit_offset 值是相对于包含结构体的起始位置的）。
// 因此，对于有很多字段的结构体，DW_AT_data_bit_offset 值可能会很大。
//
// DWARF 还允许基本类型具有非零的位大小和位偏移，因此这些信息也会为基本类型捕获，
// 但值得注意的是，使用主流语言无法触发此行为。
type StructField struct {
	Name          string
	Type          Type
	ByteOffset    int64
	ByteSize      int64 // 通常为零；对于普通字段使用 Type.Size()
	BitOffset     int64
	DataBitOffset int64
	BitSize       int64 // 如果不是位字段则为零
}

func (t *StructType) String() string {
	if t.StructName != "" {
		return t.Kind + " " + t.StructName
	}
	return t.Defn()
}

func (f *StructField) bitOffset() int64 {
	if f.BitOffset != 0 {
		return f.BitOffset
	}
	return f.DataBitOffset
}

func (t *StructType) Defn() string {
	s := t.Kind
	if t.StructName != "" {
		s += " " + t.StructName
	}
	if t.Incomplete {
		s += " /*incomplete*/"
		return s
	}
	s += " {"
	for i, f := range t.Field {
		if i > 0 {
			s += "; "
		}
		s += f.Name + " " + f.Type.String()
		s += "@" + strconv.FormatInt(f.ByteOffset, 10)
		if f.BitSize > 0 {
			s += " : " + strconv.FormatInt(f.BitSize, 10)
			s += "@" + strconv.FormatInt(f.bitOffset(), 10)
		}
	}
	s += "}"
	return s
}

// EnumType 表示枚举类型。
// 其原生整数类型的唯一指示是其 ByteSize（在 [CommonType] 内）。
type EnumType struct {
	CommonType
	EnumName string
	Val      []*EnumValue
}

// EnumValue 表示单个枚举值。
type EnumValue struct {
	Name string
	Val  int64
}

func (t *EnumType) String() string {
	s := "enum"
	if t.EnumName != "" {
		s += " " + t.EnumName
	}
	s += " {"
	for i, v := range t.Val {
		if i > 0 {
			s += "; "
		}
		s += v.Name + "=" + strconv.FormatInt(v.Val, 10)
	}
	s += "}"
	return s
}

// FuncType 表示函数类型。
type FuncType struct {
	CommonType
	ReturnType Type
	ParamType  []Type
}

func (t *FuncType) String() string {
	s := "func("
	for i, t := range t.ParamType {
		if i > 0 {
			s += ", "
		}
		s += t.String()
	}
	s += ")"
	if t.ReturnType != nil {
		s += " " + t.ReturnType.String()
	}
	return s
}

// DotDotDotType 表示可变参数 ... 函数参数。
type DotDotDotType struct {
	CommonType
}

func (t *DotDotDotType) String() string { return "..." }

// TypedefType 表示命名类型。
type TypedefType struct {
	CommonType
	Type Type
}

func (t *TypedefType) String() string { return t.Name }

func (t *TypedefType) Size() int64 { return t.Type.Size() }

// UnsupportedType 是在遇到不支持的类型时返回的占位符。
type UnsupportedType struct {
	CommonType
	Tag Tag
}

func (t *UnsupportedType) String() string {
	if t.Name != "" {
		return t.Name
	}
	return t.Name + "(unsupported type " + t.Tag.String() + ")"
}

// typeReader 用于从 info 节或 types 节读取。
type typeReader interface {
	Seek(Offset)
	Next() (*Entry, error)
	clone() typeReader
	offset() Offset
	// AddressSize 返回当前编译单元中地址的字节大小。
	AddressSize() int
}

// Type 读取 DWARF "info" 节中偏移量 off 处的类型。
func (d *Data) Type(off Offset) (Type, error) {
	return d.readType("info", d.Reader(), off, d.typeCache, nil)
}

type typeFixer struct {
	typedefs   []*TypedefType
	arraytypes []*Type
}

func (tf *typeFixer) recordArrayType(t *Type) {
	if t == nil {
		return
	}
	_, ok := (*t).(*ArrayType)
	if ok {
		tf.arraytypes = append(tf.arraytypes, t)
	}
}

func (tf *typeFixer) apply() {
	for _, t := range tf.typedefs {
		t.Common().ByteSize = t.Type.Size()
	}
	for _, t := range tf.arraytypes {
		zeroArray(t)
	}
}

// readType 从 r 的 off 位置读取名为 name 的类型。它将类型添加到类型缓存中，
// 将新的 typedef 类型追加到 typedefs，并计算类型的大小。
// 调用者应该为 typedefs 传递 nil；这用于内部递归。
func (d *Data) readType(name string, r typeReader, off Offset, typeCache map[Offset]Type, fixups *typeFixer) (Type, error) {
	if t, ok := typeCache[off]; ok {
		return t, nil
	}
	r.Seek(off)
	e, err := r.Next()
	if err != nil {
		return nil, err
	}
	addressSize := r.AddressSize()
	if e == nil || e.Offset != off {
		return nil, DecodeError{name, off, "no type at offset"}
	}

	// 如果这是递归的根，准备在递归完成后解析 typedef 大小并执行其他修复。
	// 这必须在类型图构建完成后进行，因为它可能需要以与 readType 遇到循环时
	// 不同的顺序来解析循环。
	if fixups == nil {
		var fixer typeFixer
		defer func() {
			fixer.apply()
		}()
		fixups = &fixer
	}

	// 从 Entry 解析类型。
	// 必须在递归调用 d.readType 之前设置 typeCache[off]，
	// 以正确处理循环类型。
	var typ Type

	nextDepth := 0

	// 获取下一个子条目；如果发生错误则设置 err。
	next := func() *Entry {
		if !e.Children {
			return nil
		}
		// 只返回直接子条目。
		// 跳过恰好嵌套在此条目内的复合条目。
		// 大多数 DWARF 生成器不会生成这样的东西，但 clang 会。
		// 参见 golang.org/issue/6472。
		for {
			kid, err1 := r.Next()
			if err1 != nil {
				err = err1
				return nil
			}
			if kid == nil {
				err = DecodeError{name, r.offset(), "unexpected end of DWARF entries"}
				return nil
			}
			if kid.Tag == 0 {
				if nextDepth > 0 {
					nextDepth--
					continue
				}
				return nil
			}
			if kid.Children {
				nextDepth++
			}
			if nextDepth > 0 {
				continue
			}
			return kid
		}
	}

	// 获取 Entry 的 AttrType 字段引用的类型。
	// 如果发生错误则设置 err。没有类型是一个错误。
	typeOf := func(e *Entry) Type {
		tval := e.Val(AttrType)
		var t Type
		switch toff := tval.(type) {
		case Offset:
			if t, err = d.readType(name, r.clone(), toff, typeCache, fixups); err != nil {
				return nil
			}
		case uint64:
			if t, err = d.sigToType(toff); err != nil {
				return nil
			}
		default:
			// 看起来没有 Type 意味着 "void"。
			return new(VoidType)
		}
		return t
	}

	switch e.Tag {
	case TagArrayType:
		// 多维数组。（DWARF v2 §5.4）
		// 属性：
		//	AttrType: 子类型 [必需]
		//	AttrStrideSize: 数组每个元素的位大小
		//	AttrByteSize: 整个数组的大小
		// 子条目：
		//	TagSubrangeType 或 TagEnumerationType 给出一个维度。
		//	维度按从左到右的顺序排列。
		t := new(ArrayType)
		typ = t
		typeCache[off] = t
		if t.Type = typeOf(e); err != nil {
			goto Error
		}
		t.StrideBitSize, _ = e.Val(AttrStrideSize).(int64)

		// 累积维度，
		var dims []int64
		for kid := next(); kid != nil; kid = next() {
			// TODO(rsc): 也可以是 TagEnumerationType
			// 但在实际中还没见过。
			switch kid.Tag {
			case TagSubrangeType:
				count, ok := kid.Val(AttrCount).(int64)
				if !ok {
					// 旧的二进制文件可能有上界代替。
					count, ok = kid.Val(AttrUpperBound).(int64)
					if ok {
						count++ // 长度比上界多一。
					} else if len(dims) == 0 {
						count = -1 // 如 x[]。
					}
				}
				dims = append(dims, count)
			case TagEnumerationType:
				err = DecodeError{name, kid.Offset, "cannot handle enumeration type as array bound"}
				goto Error
			}
		}
		if len(dims) == 0 {
			// LLVM 为 x[] 生成这样的代码。
			dims = []int64{-1}
		}

		t.Count = dims[0]
		for i := len(dims) - 1; i >= 1; i-- {
			t.Type = &ArrayType{Type: t.Type, Count: dims[i]}
		}

	case TagBaseType:
		// 基本类型。（DWARF v2 §5.1）
		// 属性：
		//	AttrName: 编译单元编程语言中基本类型的名称 [必需]
		//	AttrEncoding: 类型的编码值（encFloat 等）[必需]
		//	AttrByteSize: 类型的字节大小 [必需]
		//	AttrBitOffset: 值在包含存储单元内的位偏移
		//	AttrDataBitOffset: 值在包含存储单元内的位偏移
		//	AttrBitSize: 位大小
		//
		// 对于大多数语言，基本类型不会有 BitOffset/DataBitOffset/BitSize。
		name, _ := e.Val(AttrName).(string)
		enc, ok := e.Val(AttrEncoding).(int64)
		if !ok {
			err = DecodeError{name, e.Offset, "missing encoding attribute for " + name}
			goto Error
		}
		switch enc {
		default:
			err = DecodeError{name, e.Offset, "unrecognized encoding attribute value"}
			goto Error

		case encAddress:
			typ = new(AddrType)
		case encBoolean:
			typ = new(BoolType)
		case encComplexFloat:
			typ = new(ComplexType)
			if name == "complex" {
				// clang 输出 'complex' 而不是 'complex float' 或 'complex double'。
				// clang 还输出一个字节大小，我们可以用来区分。
				// 参见 issue 8694。
				switch byteSize, _ := e.Val(AttrByteSize).(int64); byteSize {
				case 8:
					name = "complex float"
				case 16:
					name = "complex double"
				}
			}
		case encFloat:
			typ = new(FloatType)
		case encSigned:
			typ = new(IntType)
		case encUnsigned:
			typ = new(UintType)
		case encSignedChar:
			typ = new(CharType)
		case encUnsignedChar:
			typ = new(UcharType)
		}
		typeCache[off] = typ
		t := typ.(interface {
			Basic() *BasicType
		}).Basic()
		t.Name = name
		t.BitSize, _ = e.Val(AttrBitSize).(int64)
		haveBitOffset := false
		haveDataBitOffset := false
		t.BitOffset, haveBitOffset = e.Val(AttrBitOffset).(int64)
		t.DataBitOffset, haveDataBitOffset = e.Val(AttrDataBitOffset).(int64)
		if haveBitOffset && haveDataBitOffset {
			err = DecodeError{name, e.Offset, "duplicate bit offset attributes"}
			goto Error
		}

	case TagClassType, TagStructType, TagUnionType:
		// 结构体、联合体或类类型。（DWARF v2 §5.5）
		// 属性：
		//	AttrName: 结构体、联合体或类的名称
		//	AttrByteSize: 字节大小 [必需]
		//	AttrDeclaration: 如果为 true，则结构体/联合体/类不完整
		// 子条目：
		//	TagMember 描述一个成员。
		//		AttrName: 成员名称 [必需]
		//		AttrType: 成员类型 [必需]
		//		AttrByteSize: 字节大小
		//		AttrBitOffset: 位字段在字节内的位偏移
		//		AttrDataBitOffset: 字段相对于结构体起始的位偏移
		//		AttrBitSize: 位字段的位大小
		//		AttrDataMemberLoc: 在结构体内的位置 [结构体、类必需]
		// 处理 C++ 还有很多内容，目前全部忽略。
		t := new(StructType)
		typ = t
		typeCache[off] = t
		switch e.Tag {
		case TagClassType:
			t.Kind = "class"
		case TagStructType:
			t.Kind = "struct"
		case TagUnionType:
			t.Kind = "union"
		}
		t.StructName, _ = e.Val(AttrName).(string)
		t.Incomplete = e.Val(AttrDeclaration) != nil
		t.Field = make([]*StructField, 0, 8)
		var lastFieldType *Type
		var lastFieldBitSize int64
		var lastFieldByteOffset int64
		for kid := next(); kid != nil; kid = next() {
			if kid.Tag != TagMember {
				continue
			}
			f := new(StructField)
			if f.Type = typeOf(kid); err != nil {
				goto Error
			}
			switch loc := kid.Val(AttrDataMemberLoc).(type) {
			case []byte:
				// TODO: 这里应该有原始编译单元，
				// 而不是 unknownFormat。
				b := makeBuf(d, unknownFormat{}, "location", 0, loc)
				if b.uint8() != opPlusUconst {
					err = DecodeError{name, kid.Offset, "unexpected opcode"}
					goto Error
				}
				f.ByteOffset = int64(b.uint())
				if b.err != nil {
					err = b.err
					goto Error
				}
			case int64:
				f.ByteOffset = loc
			}

			f.Name, _ = kid.Val(AttrName).(string)
			f.ByteSize, _ = kid.Val(AttrByteSize).(int64)
			haveBitOffset := false
			haveDataBitOffset := false
			f.BitOffset, haveBitOffset = kid.Val(AttrBitOffset).(int64)
			f.DataBitOffset, haveDataBitOffset = kid.Val(AttrDataBitOffset).(int64)
			if haveBitOffset && haveDataBitOffset {
				err = DecodeError{name, e.Offset, "duplicate bit offset attributes"}
				goto Error
			}
			f.BitSize, _ = kid.Val(AttrBitSize).(int64)
			t.Field = append(t.Field, f)

			if lastFieldBitSize == 0 && lastFieldByteOffset == f.ByteOffset && t.Kind != "union" {
				// 上一个字段宽度为零。修复数组长度。
				// （DWARF 将 0 长度数组写成好像是 1 长度数组。）
				fixups.recordArrayType(lastFieldType)
			}
			lastFieldType = &f.Type
			lastFieldByteOffset = f.ByteOffset
			lastFieldBitSize = f.BitSize
		}
		if t.Kind != "union" {
			b, ok := e.Val(AttrByteSize).(int64)
			if ok && b == lastFieldByteOffset {
				// 最后一个字段必须是零宽度。修复数组长度。
				fixups.recordArrayType(lastFieldType)
			}
		}

	case TagConstType, TagVolatileType, TagRestrictType:
		// 类型修饰符（DWARF v2 §5.2）
		// 属性：
		//	AttrType: 子类型
		t := new(QualType)
		typ = t
		typeCache[off] = t
		if t.Type = typeOf(e); err != nil {
			goto Error
		}
		switch e.Tag {
		case TagConstType:
			t.Qual = "const"
		case TagRestrictType:
			t.Qual = "restrict"
		case TagVolatileType:
			t.Qual = "volatile"
		}

	case TagEnumerationType:
		// 枚举类型（DWARF v2 §5.6）
		// 属性：
		//	AttrName: 枚举名称（如果有）
		//	AttrByteSize: 表示最大值所需的字节数
		// 子条目：
		//	TagEnumerator:
		//		AttrName: 常量名称
		//		AttrConstValue: 常量值
		t := new(EnumType)
		typ = t
		typeCache[off] = t
		t.EnumName, _ = e.Val(AttrName).(string)
		t.Val = make([]*EnumValue, 0, 8)
		for kid := next(); kid != nil; kid = next() {
			if kid.Tag == TagEnumerator {
				f := new(EnumValue)
				f.Name, _ = kid.Val(AttrName).(string)
				f.Val, _ = kid.Val(AttrConstValue).(int64)
				n := len(t.Val)
				if n >= cap(t.Val) {
					val := make([]*EnumValue, n, n*2)
					copy(val, t.Val)
					t.Val = val
				}
				t.Val = t.Val[0 : n+1]
				t.Val[n] = f
			}
		}

	case TagPointerType:
		// 类型修饰符（DWARF v2 §5.2）
		// 属性：
		//	AttrType: 子类型 [非必需！void* 没有 AttrType]
		//	AttrAddrClass: 地址类 [忽略]
		t := new(PtrType)
		typ = t
		typeCache[off] = t
		if e.Val(AttrType) == nil {
			t.Type = &VoidType{}
			break
		}
		t.Type = typeOf(e)

	case TagSubroutineType:
		// 子程序类型。（DWARF v2 §5.7）
		// 属性：
		//	AttrType: 返回值的类型（如果有）
		//	AttrName: 类型的可能名称 [忽略]
		//	AttrPrototyped: 是否使用 ANSI C 原型 [忽略]
		// 子条目：
		//	TagFormalParameter: 带类型的参数
		//		AttrType: 参数类型
		//	TagUnspecifiedParameter: 最后的 ...
		t := new(FuncType)
		typ = t
		typeCache[off] = t
		if t.ReturnType = typeOf(e); err != nil {
			goto Error
		}
		t.ParamType = make([]Type, 0, 8)
		for kid := next(); kid != nil; kid = next() {
			var tkid Type
			switch kid.Tag {
			default:
				continue
			case TagFormalParameter:
				if tkid = typeOf(kid); err != nil {
					goto Error
				}
			case TagUnspecifiedParameters:
				tkid = &DotDotDotType{}
			}
			t.ParamType = append(t.ParamType, tkid)
		}

	case TagTypedef:
		// 类型定义（DWARF v2 §5.3）
		// 属性：
		//	AttrName: 名称 [必需]
		//	AttrType: 类型定义 [必需]
		t := new(TypedefType)
		typ = t
		typeCache[off] = t
		t.Name, _ = e.Val(AttrName).(string)
		t.Type = typeOf(e)

	case TagUnspecifiedType:
		// 未指定类型（DWARF v3 §5.2）
		// 属性：
		//	AttrName: 名称
		t := new(UnspecifiedType)
		typ = t
		typeCache[off] = t
		t.Name, _ = e.Val(AttrName).(string)

	default:
		// 这是我们目前无法处理的其他类型 DIE。
		// 在这种情况下返回一个抽象的"不支持的类型"对象。
		t := new(UnsupportedType)
		typ = t
		typeCache[off] = t
		t.Tag = e.Tag
		t.Name, _ = e.Val(AttrName).(string)
	}

	if err != nil {
		goto Error
	}

	{
		b, ok := e.Val(AttrByteSize).(int64)
		if !ok {
			b = -1
			switch t := typ.(type) {
			case *TypedefType:
				// 记录我们需要在类型图构建完成后
				// 解析此类型的大小。
				fixups.typedefs = append(fixups.typedefs, t)
			case *PtrType:
				b = int64(addressSize)
			}
		}
		typ.Common().ByteSize = b
	}
	return typ, nil

Error:
	// 如果解析失败，将类型从缓存中移除，
	// 这样下次使用此偏移量调用时不会命中
	// 缓存并返回成功。
	delete(typeCache, off)
	return nil, err
}

func zeroArray(t *Type) {
	at := (*t).(*ArrayType)
	if at.Type.Size() == 0 {
		return
	}
	// 创建副本以避免使 typeCache 失效。
	tt := *at
	tt.Count = 0
	*t = &tt
}
