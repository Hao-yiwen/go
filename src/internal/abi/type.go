// 版权所有 2023 The Go Authors。保留所有权利。
// 本源代码的使用受 BSD 风格许可证约束，
// 该许可证可在 LICENSE 文件中找到。

package abi

import (
	"unsafe"
)

// Type 是 Go 类型的运行时表示。
//
// 在构建时访问此类型要小心，因为编译器/链接器中此类型的版本
// 可能与目标二进制文件中的版本布局不同，这是由于指针宽度差异
// 和任何实验性功能。请改用 cmd/compile/internal/rttype 或
// compiletype.go 中的函数来访问此类型。
// （TODO: 此警告适用于本包中的每个类型。应该放在某个共享位置？）
type Type struct {
	Size_       uintptr
	PtrBytes    uintptr // 类型中可以包含指针的（前缀）字节数
	Hash        uint32  // 类型的哈希值；避免在哈希表中计算
	TFlag       TFlag   // 额外的类型信息标志
	Align_      uint8   // 此类型变量的对齐方式
	FieldAlign_ uint8   // 此类型结构体字段的对齐方式
	Kind_       Kind    // 这是什么类型（string、int……）
	// 用于比较此类型对象的函数
	// (指向对象 A 的指针, 指向对象 B 的指针) -> ==?
	Equal func(unsafe.Pointer, unsafe.Pointer) bool
	// GCData 存储垃圾回收器的 GC 类型数据。
	// 通常，GCData 指向描述类型 ptr/nonptr 字段的位掩码。
	// 位掩码至少有 PtrBytes/ptrSize 位。
	// 如果设置了 TFlagGCMaskOnDemand 位，GCData 则是 **byte，
	// 指向位掩码的指针需要一次解引用。
	// 运行时将在需要时构建位掩码。
	// （参见 runtime/type.go:getGCMask。）
	// 注意：多个类型可能具有相同的 GCData 值，
	// 包括设置了 TFlagGCMaskOnDemand 时。当然，这些类型将具有
	// 相同的指针布局（但不一定具有相同的大小）。
	GCData    *byte
	Str       NameOff // 字符串形式
	PtrToThis TypeOff // 指向此类型的指针的类型，可能为零
}

// Kind 表示 Type 所代表的特定类型种类。
// 零值 Kind 不是有效的种类。
type Kind uint8

const (
	Invalid Kind = iota
	Bool
	Int
	Int8
	Int16
	Int32
	Int64
	Uint
	Uint8
	Uint16
	Uint32
	Uint64
	Uintptr
	Float32
	Float64
	Complex64
	Complex128
	Array
	Chan
	Func
	Interface
	Map
	Pointer
	Slice
	String
	Struct
	UnsafePointer
)

// TFlag 被 Type 用来标示在 Type 值之后的内存中有哪些额外的类型信息可用。
type TFlag uint8

const (
	// TFlagUncommon 表示在每个类型共享的公共数据之后，
	// 有一个 UncommonType 类型的数据。也就是说，struct 类型的数据
	// 会在一个偏移量处存储其 UncommonType，interface 类型的数据
	// 会在不同的偏移量处存储其 UncommonType。UncommonType 总是
	// 通过使用"相信我们是实现者"的指针算术计算出的指针来访问。
	//
	// 例如，如果 t.Kind() == Struct 且 t.tflag&TFlagUncommon != 0，
	// 那么 t 有 UncommonType 数据，可以这样访问：
	//
	//	type structTypeUncommon struct {
	//		structType
	//		u UncommonType
	//	}
	//	u := &(*structTypeUncommon)(unsafe.Pointer(t)).u
	TFlagUncommon TFlag = 1 << 0

	// TFlagExtraStar 表示 str 字段中的名称有一个多余的 '*' 前缀。
	// 这是因为对于程序中的大多数类型 T，类型 *T 也存在，
	// 重用 str 数据可以节省二进制文件大小。
	TFlagExtraStar TFlag = 1 << 1

	// TFlagNamed 表示类型有名称。
	TFlagNamed TFlag = 1 << 2

	// TFlagRegularMemory 表示 equal 和 hash 函数可以将此类型
	// 视为 t.size 字节的单个内存区域。
	TFlagRegularMemory TFlag = 1 << 3

	// TFlagGCMaskOnDemand 表示 GC 指针位掩码将在运行时按需计算，
	// 而不是在编译时预计算。如果设置了此标志，GCData 字段实际上
	// 具有 **byte 类型而不是 *byte。运行时将在 *GCData 中存储
	// 指向 GC 指针位掩码的指针。
	TFlagGCMaskOnDemand TFlag = 1 << 4

	// TFlagDirectIface 表示此类型的值直接存储在接口的 data 字段中，
	// 而不是间接存储。此标志只是 Size_ == PtrBytes == goarch.PtrSize
	// 的缓存计算结果。
	TFlagDirectIface TFlag = 1 << 5

	// 为 dlv 留下的面包屑。不应使用它，任何 Kind 都不应该大到设置此位。
	KindDirectIface Kind = 1 << 5
)

// NameOff 是从 moduledata.types 到名称的偏移量。参见 runtime 中的 resolveNameOff。
type NameOff int32

// TypeOff 是从 moduledata.types 到类型的偏移量。参见 runtime 中的 resolveTypeOff。
type TypeOff int32

// TextOff 是从 text 节顶部的偏移量。参见 runtime 中的 (rtype).textOff。
type TextOff int32

// String 返回 k 的名称。
func (k Kind) String() string {
	if int(k) < len(kindNames) {
		return kindNames[k]
	}
	return kindNames[0]
}

var kindNames = []string{
	Invalid:       "invalid",
	Bool:          "bool",
	Int:           "int",
	Int8:          "int8",
	Int16:         "int16",
	Int32:         "int32",
	Int64:         "int64",
	Uint:          "uint",
	Uint8:         "uint8",
	Uint16:        "uint16",
	Uint32:        "uint32",
	Uint64:        "uint64",
	Uintptr:       "uintptr",
	Float32:       "float32",
	Float64:       "float64",
	Complex64:     "complex64",
	Complex128:    "complex128",
	Array:         "array",
	Chan:          "chan",
	Func:          "func",
	Interface:     "interface",
	Map:           "map",
	Pointer:       "ptr",
	Slice:         "slice",
	String:        "string",
	Struct:        "struct",
	UnsafePointer: "unsafe.Pointer",
}

// TypeOf 返回某个值的 abi.Type。
func TypeOf(a any) *Type {
	eface := *(*EmptyInterface)(unsafe.Pointer(&a))
	// 类型要么是静态的（编译器创建的类型），要么是堆分配但始终可达的
	// （反射创建的类型，保存在中央映射中）。因此不需要让类型逃逸。
	// 这里的 noescape 帮助避免 v 不必要的逃逸。
	return (*Type)(NoEscape(unsafe.Pointer(eface.Type)))
}

// TypeFor 返回类型参数的 abi.Type。
func TypeFor[T any]() *Type {
	return (*PtrType)(unsafe.Pointer(TypeOf((*T)(nil)))).Elem
}

func (t *Type) Kind() Kind { return t.Kind_ }

func (t *Type) HasName() bool {
	return t.TFlag&TFlagNamed != 0
}

// Pointers 报告 t 是否包含指针。
func (t *Type) Pointers() bool { return t.PtrBytes != 0 }

// IsDirectIface 报告 t 是否直接存储在接口值中。
func (t *Type) IsDirectIface() bool {
	return t.TFlag&TFlagDirectIface != 0
}

func (t *Type) GcSlice(begin, end uintptr) []byte {
	if t.TFlag&TFlagGCMaskOnDemand != 0 {
		panic("GcSlice can't handle on-demand gcdata types")
	}
	return unsafe.Slice(t.GCData, int(end))[begin:]
}

// Method 非接口类型上的方法
type Method struct {
	Name NameOff // 方法名称
	Mtyp TypeOff // 方法类型（不含接收者）
	Ifn  TextOff // 接口调用中使用的函数（单字接收者）
	Tfn  TextOff // 普通方法调用使用的函数
}

// UncommonType 仅存在于已定义类型或具有方法的类型中
// （如果 T 是已定义类型，T 和 *T 的 uncommonTypes 都有方法）。
// 使用指向此结构的指针可以减少描述无方法的未定义类型所需的总体大小。
type UncommonType struct {
	PkgPath NameOff // 导入路径；对于 int、string 等内置类型为空
	Mcount  uint16  // 方法数量
	Xcount  uint16  // 导出方法数量
	Moff    uint32  // 从此 uncommontype 到 [mcount]Method 的偏移量
	_       uint32  // 未使用
}

func (t *UncommonType) Methods() []Method {
	if t.Mcount == 0 {
		return nil
	}
	return (*[1 << 16]Method)(addChecked(unsafe.Pointer(t), uintptr(t.Moff), "t.mcount > 0"))[:t.Mcount:t.Mcount]
}

func (t *UncommonType) ExportedMethods() []Method {
	if t.Xcount == 0 {
		return nil
	}
	return (*[1 << 16]Method)(addChecked(unsafe.Pointer(t), uintptr(t.Moff), "t.xcount > 0"))[:t.Xcount:t.Xcount]
}

// addChecked 返回 p+x。
//
// whySafe 字符串被忽略，以便函数仍然像 p+x 一样高效内联，
// 但所有调用点都应使用该字符串记录为什么这个加法是安全的，
// 也就是说为什么这个加法不会导致 x 前进到 p 分配的最末端
// 从而错误地指向内存中的下一个块。
func addChecked(p unsafe.Pointer, x uintptr, whySafe string) unsafe.Pointer {
	return unsafe.Pointer(uintptr(p) + x)
}

// Imethod 表示接口类型上的方法
type Imethod struct {
	Name NameOff // 方法名称
	Typ  TypeOff // 底层是 .(*FuncType)
}

// ArrayType 表示固定数组类型。
type ArrayType struct {
	Type
	Elem  *Type // 数组元素类型
	Slice *Type // 切片类型
	Len   uintptr
}

// Len 如果 t 是数组类型则返回 t 的长度，否则返回 0
func (t *Type) Len() int {
	if t.Kind() == Array {
		return int((*ArrayType)(unsafe.Pointer(t)).Len)
	}
	return 0
}

func (t *Type) Common() *Type {
	return t
}

type ChanDir int

const (
	RecvDir    ChanDir = 1 << iota         // <-chan
	SendDir                                // chan<-
	BothDir            = RecvDir | SendDir // chan
	InvalidDir ChanDir = 0
)

// ChanType 表示通道类型
type ChanType struct {
	Type
	Elem *Type
	Dir  ChanDir
}

type structTypeUncommon struct {
	StructType
	u UncommonType
}

// ChanDir 如果 t 是通道类型则返回 t 的方向，否则返回 InvalidDir (0)。
func (t *Type) ChanDir() ChanDir {
	if t.Kind() == Chan {
		ch := (*ChanType)(unsafe.Pointer(t))
		return ch.Dir
	}
	return InvalidDir
}

// Uncommon 如果存在 T 的"非公共"数据则返回指向它的指针，否则返回 nil
func (t *Type) Uncommon() *UncommonType {
	if t.TFlag&TFlagUncommon == 0 {
		return nil
	}
	switch t.Kind() {
	case Struct:
		return &(*structTypeUncommon)(unsafe.Pointer(t)).u
	case Pointer:
		type u struct {
			PtrType
			u UncommonType
		}
		return &(*u)(unsafe.Pointer(t)).u
	case Func:
		type u struct {
			FuncType
			u UncommonType
		}
		return &(*u)(unsafe.Pointer(t)).u
	case Slice:
		type u struct {
			SliceType
			u UncommonType
		}
		return &(*u)(unsafe.Pointer(t)).u
	case Array:
		type u struct {
			ArrayType
			u UncommonType
		}
		return &(*u)(unsafe.Pointer(t)).u
	case Chan:
		type u struct {
			ChanType
			u UncommonType
		}
		return &(*u)(unsafe.Pointer(t)).u
	case Map:
		type u struct {
			MapType
			u UncommonType
		}
		return &(*u)(unsafe.Pointer(t)).u
	case Interface:
		type u struct {
			InterfaceType
			u UncommonType
		}
		return &(*u)(unsafe.Pointer(t)).u
	default:
		type u struct {
			Type
			u UncommonType
		}
		return &(*u)(unsafe.Pointer(t)).u
	}
}

// Elem 如果 t 是数组、通道、map、指针或切片则返回元素类型，否则返回 nil。
func (t *Type) Elem() *Type {
	switch t.Kind() {
	case Array:
		tt := (*ArrayType)(unsafe.Pointer(t))
		return tt.Elem
	case Chan:
		tt := (*ChanType)(unsafe.Pointer(t))
		return tt.Elem
	case Map:
		tt := (*MapType)(unsafe.Pointer(t))
		return tt.Elem
	case Pointer:
		tt := (*PtrType)(unsafe.Pointer(t))
		return tt.Elem
	case Slice:
		tt := (*SliceType)(unsafe.Pointer(t))
		return tt.Elem
	}
	return nil
}

// StructType 将 t 转换为 *StructType，如果其标签不匹配则返回 nil。
func (t *Type) StructType() *StructType {
	if t.Kind() != Struct {
		return nil
	}
	return (*StructType)(unsafe.Pointer(t))
}

// MapType 将 t 转换为 *MapType，如果其标签不匹配则返回 nil。
func (t *Type) MapType() *MapType {
	if t.Kind() != Map {
		return nil
	}
	return (*MapType)(unsafe.Pointer(t))
}

// ArrayType 将 t 转换为 *ArrayType，如果其标签不匹配则返回 nil。
func (t *Type) ArrayType() *ArrayType {
	if t.Kind() != Array {
		return nil
	}
	return (*ArrayType)(unsafe.Pointer(t))
}

// FuncType 将 t 转换为 *FuncType，如果其标签不匹配则返回 nil。
func (t *Type) FuncType() *FuncType {
	if t.Kind() != Func {
		return nil
	}
	return (*FuncType)(unsafe.Pointer(t))
}

// InterfaceType 将 t 转换为 *InterfaceType，如果其标签不匹配则返回 nil。
func (t *Type) InterfaceType() *InterfaceType {
	if t.Kind() != Interface {
		return nil
	}
	return (*InterfaceType)(unsafe.Pointer(t))
}

// Size 返回类型 t 的数据大小。
func (t *Type) Size() uintptr { return t.Size_ }

// Align 返回类型 t 的数据对齐方式。
func (t *Type) Align() int { return int(t.Align_) }

func (t *Type) FieldAlign() int { return int(t.FieldAlign_) }

type InterfaceType struct {
	Type
	PkgPath Name      // 导入路径
	Methods []Imethod // 按哈希排序
}

func (t *Type) ExportedMethods() []Method {
	ut := t.Uncommon()
	if ut == nil {
		return nil
	}
	return ut.ExportedMethods()
}

func (t *Type) NumMethod() int {
	if t.Kind() == Interface {
		tt := (*InterfaceType)(unsafe.Pointer(t))
		return tt.NumMethod()
	}
	return len(t.ExportedMethods())
}

// NumMethod 返回类型方法集中接口方法的数量。
func (t *InterfaceType) NumMethod() int { return len(t.Methods) }

func (t *Type) Key() *Type {
	if t.Kind() == Map {
		return (*MapType)(unsafe.Pointer(t)).Key
	}
	return nil
}

type SliceType struct {
	Type
	Elem *Type // 切片元素类型
}

// FuncType 表示函数类型。
//
// 每个输入和输出参数的 *Type 存储在一个数组中，该数组直接跟在
// funcType（可能还有其 uncommonType）之后。因此，一个具有
// 一个方法、一个输入和一个输出的函数类型是：
//
//	struct {
//		funcType
//		uncommonType
//		[2]*rtype    // [0] 是输入，[1] 是输出
//	}
type FuncType struct {
	Type
	InCount  uint16
	OutCount uint16 // 如果最后一个输入参数是 ... 则设置高位
}

func (t *FuncType) In(i int) *Type {
	return t.InSlice()[i]
}

func (t *FuncType) NumIn() int {
	return int(t.InCount)
}

func (t *FuncType) NumOut() int {
	return int(t.OutCount & (1<<15 - 1))
}

func (t *FuncType) Out(i int) *Type {
	return (t.OutSlice()[i])
}

func (t *FuncType) InSlice() []*Type {
	uadd := unsafe.Sizeof(*t)
	if t.TFlag&TFlagUncommon != 0 {
		uadd += unsafe.Sizeof(UncommonType{})
	}
	if t.InCount == 0 {
		return nil
	}
	return (*[1 << 16]*Type)(addChecked(unsafe.Pointer(t), uadd, "t.inCount > 0"))[:t.InCount:t.InCount]
}
func (t *FuncType) OutSlice() []*Type {
	outCount := uint16(t.NumOut())
	if outCount == 0 {
		return nil
	}
	uadd := unsafe.Sizeof(*t)
	if t.TFlag&TFlagUncommon != 0 {
		uadd += unsafe.Sizeof(UncommonType{})
	}
	return (*[1 << 17]*Type)(addChecked(unsafe.Pointer(t), uadd, "outCount > 0"))[t.InCount : t.InCount+outCount : t.InCount+outCount]
}

func (t *FuncType) IsVariadic() bool {
	return t.OutCount&(1<<15) != 0
}

type PtrType struct {
	Type
	Elem *Type // 指针元素（被指向的）类型
}

type StructField struct {
	Name   Name    // 名称始终非空
	Typ    *Type   // 字段类型
	Offset uintptr // 字段的字节偏移量
}

func (f *StructField) Embedded() bool {
	return f.Name.IsEmbedded()
}

type StructType struct {
	Type
	PkgPath Name
	Fields  []StructField
}

// Name 是带有可选额外数据的编码类型名称。
//
// 第一个字节是位字段，包含：
//
//	1<<0 名称是导出的
//	1<<1 名称后面跟着标签数据
//	1<<2 名称和标签后面跟着 pkgPath nameOff
//	1<<3 名称是嵌入（又名匿名）字段的
//
// 之后是名称长度的 varint 编码，
// 然后是名称本身。
//
// 如果存在标签数据，它也有 varint 编码的长度，
// 后面跟着标签本身。
//
// 如果跟着导入路径，则数据末尾的 4 个字节形成一个 nameOff。
// 导入路径仅为定义在与其类型不同包中的具体方法设置。
//
// 如果名称以 "*" 开头，则导出位表示被指向的类型是否导出。
//
// 注意：此编码必须在此处与以下位置匹配：
//   cmd/compile/internal/reflectdata/reflect.go
//   cmd/link/internal/ld/decodesym.go

type Name struct {
	Bytes *byte
}

// DataChecked 对 n 的 Bytes 进行指针算术运算，该运算被断言为安全的，
// 原因在 whySafe 中（可以出现在回溯等中）。
func (n Name) DataChecked(off int, whySafe string) *byte {
	return (*byte)(addChecked(unsafe.Pointer(n.Bytes), uintptr(off), whySafe))
}

// Data 对 n 的 Bytes 进行指针算术运算，该运算被断言为安全的，
// 因为是运行时进行的调用（其他包使用 DataChecked）。
func (n Name) Data(off int) *byte {
	return (*byte)(addChecked(unsafe.Pointer(n.Bytes), uintptr(off), "the runtime doesn't need to give you a reason"))
}

// IsExported 返回"n 是否导出？"
func (n Name) IsExported() bool {
	return (*n.Bytes)&(1<<0) != 0
}

// HasTag 当且仅当此名称后面有标签数据时返回 true
func (n Name) HasTag() bool {
	return (*n.Bytes)&(1<<1) != 0
}

// IsEmbedded 当且仅当 n 是嵌入的（匿名字段）时返回 true。
func (n Name) IsEmbedded() bool {
	return (*n.Bytes)&(1<<3) != 0
}

// ReadVarint 解析由 encoding/binary 编码的 varint。
// 它返回编码的字节数和编码的值。
func (n Name) ReadVarint(off int) (int, int) {
	v := 0
	for i := 0; ; i++ {
		x := *n.DataChecked(off+i, "read varint")
		v += int(x&0x7f) << (7 * i)
		if x&0x80 == 0 {
			return i + 1, v
		}
	}
}

// IsBlank 指示 n 是否为 "_"。
func (n Name) IsBlank() bool {
	if n.Bytes == nil {
		return false
	}
	_, l := n.ReadVarint(1)
	return l == 1 && *n.Data(2) == '_'
}

// writeVarint 以 varint 形式将 n 写入 buf。返回写入的字节数。
// n 必须为非负数。最多写入 10 个字节。
func writeVarint(buf []byte, n int) int {
	for i := 0; ; i++ {
		b := byte(n & 0x7f)
		n >>= 7
		if n == 0 {
			buf[i] = b
			return i + 1
		}
		buf[i] = b | 0x80
	}
}

// Name 返回 n 的名称，如果它实际上没有名称则返回空。
func (n Name) Name() string {
	if n.Bytes == nil {
		return ""
	}
	i, l := n.ReadVarint(1)
	return unsafe.String(n.DataChecked(1+i, "non-empty string"), l)
}

// Tag 返回 n 的标签字符串，如果没有则返回空。
func (n Name) Tag() string {
	if !n.HasTag() {
		return ""
	}
	i, l := n.ReadVarint(1)
	i2, l2 := n.ReadVarint(1 + i + l)
	return unsafe.String(n.DataChecked(1+i+l+i2, "non-empty string"), l2)
}

func NewName(n, tag string, exported, embedded bool) Name {
	if len(n) >= 1<<29 {
		panic("abi.NewName: name too long: " + n[:1024] + "...")
	}
	if len(tag) >= 1<<29 {
		panic("abi.NewName: tag too long: " + tag[:1024] + "...")
	}
	var nameLen [10]byte
	var tagLen [10]byte
	nameLenLen := writeVarint(nameLen[:], len(n))
	tagLenLen := writeVarint(tagLen[:], len(tag))

	var bits byte
	l := 1 + nameLenLen + len(n)
	if exported {
		bits |= 1 << 0
	}
	if len(tag) > 0 {
		l += tagLenLen + len(tag)
		bits |= 1 << 1
	}
	if embedded {
		bits |= 1 << 3
	}

	b := make([]byte, l)
	b[0] = bits
	copy(b[1:], nameLen[:nameLenLen])
	copy(b[1+nameLenLen:], n)
	if len(tag) > 0 {
		tb := b[1+nameLenLen+len(n):]
		copy(tb, tagLen[:tagLenLen])
		copy(tb[tagLenLen:], tag)
	}

	return Name{Bytes: &b[0]}
}

const (
	TraceArgsLimit    = 10 // 打印不超过 10 个参数/组件
	TraceArgsMaxDepth = 5  // 嵌套层数不超过 5

	// maxLen 是字节流长度的（保守）上界。对于每个参数/组件，
	// 它最多有 2 字节的数据（size，offset），并且每层最多有一个 {、}、...
	// （除非是最后一个，否则不能同时有数据和 ...，只是保守估计）。
	// 加 1 用于 _endSeq。
	TraceArgsMaxLen = (TraceArgsMaxDepth*3+2)*TraceArgsLimit + 1
)

// 填充数据。
// 数据是字节流，包含非聚合参数或聚合类型参数的非聚合字段/元素的
// 偏移量和大小，以及特殊"操作符"。具体来说，
//   - 对于每个非聚合参数/字段/元素，其从 FP 的偏移量（1 字节）和
//     大小（1 字节）
//   - 特殊操作符：
//   - 0xff - 序列结束
//   - 0xfe - 打印 {（在聚合类型参数的开始）
//   - 0xfd - 打印 }（在聚合类型参数的结束）
//   - 0xfc - 打印 ...（更多参数/字段/元素）
//   - 0xfb - 打印 _（偏移量太大）
const (
	TraceArgsEndSeq         = 0xff
	TraceArgsStartAgg       = 0xfe
	TraceArgsEndAgg         = 0xfd
	TraceArgsDotdotdot      = 0xfc
	TraceArgsOffsetTooLarge = 0xfb
	TraceArgsSpecial        = 0xf0 // 此值以上是操作符，以下是普通偏移量
)

// MaxPtrmaskBytes 是 GC ptrmask 位图的最大长度，
// 该位图保存描述给定类型中指针位置的 1 位条目。
// 超过此长度，GC 信息将记录为 GC 程序，
// 该程序可以紧凑地表达重复。无论哪种形式，
// 运行时都使用该信息来初始化堆位图，
// 对于大类型（如 128 个或更多字），它们的速度大致相同。
// GC 程序从不会大很多，而且通常更紧凑。
// （如果涉及大数组，它们可以任意地更紧凑。）
//
// 截止值必须足够大，以便任何足够大到使用 GC 程序的分配
// 也足够大到不与任何其他对象共享堆位图字节，
// 从而允许 GC 程序执行假定对齐的开始而不使用原子操作。
// 在当前运行时中，这意味着所有大于截止值的 malloc 大小类
// 必须是四个字的倍数。在 32 位系统上是 16 字节，
// 所有 >= 16 字节的大小类都是 16 字节对齐的，所以没有实际约束。
// 在 64 位系统上是 32 字节，对于 >= 256 字节的大小类
// 保证 32 字节对齐。在 64 位系统上，256 字节分配
// 是 32 个指针，其位适合 4 字节。所以 MaxPtrmaskBytes
// 必须 >= 4。
//
// 我们过去使用 16，因为 GC 程序确实有一些常量开销才能启动，
// 处理 128 个指针似乎足以很好地摊销该开销。
//
// 为了确保运行时的 chansend 可以调用 typeBitsBulkBarrier，
// 我们将限制提高到 2048，这样即使 32 位系统也保证
// 对大小高达 64 kB 的对象使用位图。
const MaxPtrmaskBytes = 2048
