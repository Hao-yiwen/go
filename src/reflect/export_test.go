// 版权所有 2012 The Go Authors。保留所有权利。
// 本源代码的使用受 BSD 风格许可证约束，
// 该许可证可在 LICENSE 文件中找到。

package reflect

import (
	"internal/abi"
	"internal/goarch"
	"sync"
	"unsafe"
)

// MakeRO 返回 v 的副本，并设置只读标志。
func MakeRO(v Value) Value {
	v.flag |= flagStickyRO
	return v
}

// IsRO 报告 v 的只读标志是否已设置。
func IsRO(v Value) bool {
	return v.flag&flagStickyRO != 0
}

var CallGC = &callGC

// FuncLayout 调用 funcLayout 并返回用于测试的结果子集。
//
// stack、gc、inReg 和 outReg 等位图被扩展，使得每个位占用一个字节，
// 这样编写测试用例会更清晰一些。
// 如果 ptrs 为 false，gc 将为 nil。
func FuncLayout(t Type, rcvr Type) (frametype Type, argSize, retOffset uintptr, stack, gc, inReg, outReg []byte, ptrs bool) {
	var ft *abi.Type
	var abid abiDesc
	if rcvr != nil {
		ft, _, abid = funcLayout((*funcType)(unsafe.Pointer(t.common())), rcvr.common())
	} else {
		ft, _, abid = funcLayout((*funcType)(unsafe.Pointer(t.(*rtype))), nil)
	}
	// 提取大小信息。
	argSize = abid.stackCallArgsSize
	retOffset = abid.retOffset
	frametype = toType(ft)

	// 将栈指针位图扩展为字节映射。
	for i := uint32(0); i < abid.stackPtrs.n; i++ {
		stack = append(stack, abid.stackPtrs.data[i/8]>>(i%8)&1)
	}

	// 将寄存器指针位图扩展为字节映射。
	bool2byte := func(b bool) byte {
		if b {
			return 1
		}
		return 0
	}
	for i := 0; i < intArgRegs; i++ {
		inReg = append(inReg, bool2byte(abid.inRegPtrs.Get(i)))
		outReg = append(outReg, bool2byte(abid.outRegPtrs.Get(i)))
	}

	// 将帧类型的 GC 位图扩展为字节映射。
	ptrs = ft.Pointers()
	if ptrs {
		nptrs := ft.PtrBytes / goarch.PtrSize
		gcdata := ft.GcSlice(0, (nptrs+7)/8)
		for i := uintptr(0); i < nptrs; i++ {
			gc = append(gc, gcdata[i/8]>>(i%8)&1)
		}
	}
	return
}

func TypeLinks() []string {
	var r []string
	sections, offset := typelinks()
	for i, offs := range offset {
		rodata := sections[i]
		for _, off := range offs {
			typ := (*rtype)(resolveTypeOff(rodata, off))
			r = append(r, typ.String())
		}
	}
	return r
}

var GCBits = gcbits

func gcbits(any) []byte // 由 runtime 提供

type EmbedWithUnexpMeth struct{}

func (EmbedWithUnexpMeth) f() {}

type pinUnexpMeth interface {
	f()
}

var pinUnexpMethI = pinUnexpMeth(EmbedWithUnexpMeth{})

func FirstMethodNameBytes(t Type) *byte {
	_ = pinUnexpMethI

	ut := t.uncommon()
	if ut == nil {
		panic("type has no methods")
	}
	m := ut.Methods()[0]
	mname := t.(*rtype).nameOff(m.Name)
	if *mname.DataChecked(0, "name flag field")&(1<<2) == 0 {
		panic("method name does not have pkgPath *string")
	}
	return mname.Bytes
}

type OtherPkgFields struct {
	OtherExported   int
	otherUnexported int
}

func IsExported(t Type) bool {
	typ := t.(*rtype)
	n := typ.nameOff(typ.t.Str)
	return n.IsExported()
}

func ResolveReflectName(s string) {
	resolveReflectName(newName(s, "", false, false))
}

type Buffer struct {
	buf []byte
}

func clearLayoutCache() {
	layoutCache = sync.Map{}
}

func SetArgRegs(ints, floats int, floatSize uintptr) (oldInts, oldFloats int, oldFloatSize uintptr) {
	oldInts = intArgRegs
	oldFloats = floatArgRegs
	oldFloatSize = floatRegSize
	intArgRegs = ints
	floatArgRegs = floats
	floatRegSize = floatSize
	clearLayoutCache()
	return
}

var MethodValueCallCodePtr = methodValueCallCodePtr

var InternalIsZero = isZero

var IsRegularMemory = isRegularMemory

func MapGroupOf(x, y Type) Type {
	grp, _ := groupAndSlotOf(x, y)
	return grp
}
