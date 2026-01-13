// 版权所有 2024 The Go Authors。保留所有权利。
// 本源代码的使用受 BSD 风格许可证约束，
// 该许可证可在 LICENSE 文件中找到。

package unique

import (
	"internal/abi"
	"internal/stringslite"
	"unsafe"
)

// clone 复制 value，并可能用这些字符串的克隆版本更新 value 中找到的字符串值。
// 显式克隆字符串的目的是避免意外地给大字符串一个长生命周期。
//
// 注意，这将克隆 value 中找到的结构体和数组中的字符串，
// 如果 value 本身是字符串，也会克隆 value。但是，如果 value 是接口或切片类型
// （即通过间接引用找到的），则不会克隆字符串。
func clone[T comparable](value T, seq *cloneSeq) T {
	for _, offset := range seq.stringOffsets {
		ps := (*string)(unsafe.Pointer(uintptr(unsafe.Pointer(&value)) + offset))
		*ps = stringslite.Clone(*ps)
	}
	return abi.EscapeToResultNonString(value)
}

// singleStringClone 描述如何克隆单个字符串。
var singleStringClone = cloneSeq{stringOffsets: []uintptr{0}}

// cloneSeq 描述如何克隆特定类型的值。
type cloneSeq struct {
	stringOffsets []uintptr
}

// makeCloneSeq 为类型创建 cloneSeq。
func makeCloneSeq(typ *abi.Type) cloneSeq {
	if typ == nil {
		return cloneSeq{}
	}
	if typ.Kind() == abi.String {
		return singleStringClone
	}
	var seq cloneSeq
	switch typ.Kind() {
	case abi.Struct:
		buildStructCloneSeq(typ, &seq, 0)
	case abi.Array:
		buildArrayCloneSeq(typ, &seq, 0)
	}
	return seq
}

// buildStructCloneSeq 为 Kind 为 abi.Struct 的 abi.Type 填充 cloneSeq。
func buildStructCloneSeq(typ *abi.Type, seq *cloneSeq, baseOffset uintptr) {
	styp := typ.StructType()
	for i := range styp.Fields {
		f := &styp.Fields[i]
		switch f.Typ.Kind() {
		case abi.String:
			seq.stringOffsets = append(seq.stringOffsets, baseOffset+f.Offset)
		case abi.Struct:
			buildStructCloneSeq(f.Typ, seq, baseOffset+f.Offset)
		case abi.Array:
			buildArrayCloneSeq(f.Typ, seq, baseOffset+f.Offset)
		}
	}
}

// buildArrayCloneSeq 为 Kind 为 abi.Array 的 abi.Type 填充 cloneSeq。
func buildArrayCloneSeq(typ *abi.Type, seq *cloneSeq, baseOffset uintptr) {
	atyp := typ.ArrayType()
	etyp := atyp.Elem
	offset := baseOffset
	for range atyp.Len {
		switch etyp.Kind() {
		case abi.String:
			seq.stringOffsets = append(seq.stringOffsets, offset)
		case abi.Struct:
			buildStructCloneSeq(etyp, seq, offset)
		case abi.Array:
			buildArrayCloneSeq(etyp, seq, offset)
		}
		offset += etyp.Size()
		align := uintptr(etyp.FieldAlign())
		offset = (offset + align - 1) &^ (align - 1)
	}
}
