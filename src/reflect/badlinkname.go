// 版权所有 2024 The Go Authors。保留所有权利。
// 本源代码的使用受 BSD 风格许可证约束，
// 该许可证可在 LICENSE 文件中找到。

package reflect

import (
	"internal/abi"
	"unsafe"
	_ "unsafe"
)

// 广泛使用的包通过 linkname 访问这些符号，
// 最著名的有：
//  - github.com/goccy/go-json
//  - github.com/goccy/go-reflect
//  - github.com/sohaha/zlsgo
//  - github.com/undefinedlabs/go-mpatch
//
// 不要删除或更改类型签名。
// 参见 go.dev/issue/67401
// 和 go.dev/issue/67279。

// ifaceIndir 报告 t 是否间接存储在接口值中。
// 本包不再使用它，这里完全是为了 linkname 的使用。
//
//go:linkname unusedIfaceIndir reflect.ifaceIndir
func unusedIfaceIndir(t *abi.Type) bool {
	return !t.IsDirectIface()
}

//go:linkname valueInterface

// 编译器有充分的理由不允许在方法上使用 linkname。
// 我们使用这个技巧来推送方法的 linkname。
// 不要在本包中调用它们。

//go:linkname badlinkname_rtype_Align reflect.(*rtype).Align
func badlinkname_rtype_Align(*rtype) int

//go:linkname badlinkname_rtype_AssignableTo reflect.(*rtype).AssignableTo
func badlinkname_rtype_AssignableTo(*rtype, Type) bool

//go:linkname badlinkname_rtype_Bits reflect.(*rtype).Bits
func badlinkname_rtype_Bits(*rtype) int

//go:linkname badlinkname_rtype_ChanDir reflect.(*rtype).ChanDir
func badlinkname_rtype_ChanDir(*rtype) ChanDir

//go:linkname badlinkname_rtype_Comparable reflect.(*rtype).Comparable
func badlinkname_rtype_Comparable(*rtype) bool

//go:linkname badlinkname_rtype_ConvertibleTo reflect.(*rtype).ConvertibleTo
func badlinkname_rtype_ConvertibleTo(*rtype, Type) bool

//go:linkname badlinkname_rtype_Elem reflect.(*rtype).Elem
func badlinkname_rtype_Elem(*rtype) Type

//go:linkname badlinkname_rtype_Field reflect.(*rtype).Field
func badlinkname_rtype_Field(*rtype, int) StructField

//go:linkname badlinkname_rtype_FieldAlign reflect.(*rtype).FieldAlign
func badlinkname_rtype_FieldAlign(*rtype) int

//go:linkname badlinkname_rtype_FieldByIndex reflect.(*rtype).FieldByIndex
func badlinkname_rtype_FieldByIndex(*rtype, []int) StructField

//go:linkname badlinkname_rtype_FieldByName reflect.(*rtype).FieldByName
func badlinkname_rtype_FieldByName(*rtype, string) (StructField, bool)

//go:linkname badlinkname_rtype_FieldByNameFunc reflect.(*rtype).FieldByNameFunc
func badlinkname_rtype_FieldByNameFunc(*rtype, func(string) bool) (StructField, bool)

//go:linkname badlinkname_rtype_Implements reflect.(*rtype).Implements
func badlinkname_rtype_Implements(*rtype, Type) bool

//go:linkname badlinkname_rtype_In reflect.(*rtype).In
func badlinkname_rtype_In(*rtype, int) Type

//go:linkname badlinkname_rtype_IsVariadic reflect.(*rtype).IsVariadic
func badlinkname_rtype_IsVariadic(*rtype) bool

//go:linkname badlinkname_rtype_Key reflect.(*rtype).Key
func badlinkname_rtype_Key(*rtype) Type

//go:linkname badlinkname_rtype_Kind reflect.(*rtype).Kind
func badlinkname_rtype_Kind(*rtype) Kind

//go:linkname badlinkname_rtype_Len reflect.(*rtype).Len
func badlinkname_rtype_Len(*rtype) int

//go:linkname badlinkname_rtype_Method reflect.(*rtype).Method
func badlinkname_rtype_Method(*rtype, int) Method

//go:linkname badlinkname_rtype_MethodByName reflect.(*rtype).MethodByName
func badlinkname_rtype_MethodByName(*rtype, string) (Method, bool)

//go:linkname badlinkname_rtype_Name reflect.(*rtype).Name
func badlinkname_rtype_Name(*rtype) string

//go:linkname badlinkname_rtype_NumField reflect.(*rtype).NumField
func badlinkname_rtype_NumField(*rtype) int

//go:linkname badlinkname_rtype_NumIn reflect.(*rtype).NumIn
func badlinkname_rtype_NumIn(*rtype) int

//go:linkname badlinkname_rtype_NumMethod reflect.(*rtype).NumMethod
func badlinkname_rtype_NumMethod(*rtype) int

//go:linkname badlinkname_rtype_NumOut reflect.(*rtype).NumOut
func badlinkname_rtype_NumOut(*rtype) int

//go:linkname badlinkname_rtype_Out reflect.(*rtype).Out
func badlinkname_rtype_Out(*rtype, int) Type

//go:linkname badlinkname_rtype_PkgPath reflect.(*rtype).PkgPath
func badlinkname_rtype_PkgPath(*rtype) string

//go:linkname badlinkname_rtype_Size reflect.(*rtype).Size
func badlinkname_rtype_Size(*rtype) uintptr

//go:linkname badlinkname_rtype_String reflect.(*rtype).String
func badlinkname_rtype_String(*rtype) string

//go:linkname badlinkname_rtype_ptrTo reflect.(*rtype).ptrTo
func badlinkname_rtype_ptrTo(*rtype) *abi.Type

//go:linkname badlinkname_Value_pointer reflect.(*Value).pointer
func badlinkname_Value_pointer(Value) unsafe.Pointer
