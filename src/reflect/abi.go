// 版权所有 2021 The Go Authors。保留所有权利。
// 本源代码的使用受 BSD 风格许可证约束，
// 该许可证可在 LICENSE 文件中找到。

package reflect

import (
	"internal/abi"
	"internal/goarch"
	"unsafe"
)

// 这些变量被本文件中的寄存器分配算法使用。
//
// 修改这些变量时需要谨慎（不能有其他 reflect 代码正在执行），
// 通常只在测试本包时才会修改。
//
// 这些值永远不应设置得比 internal/abi 中的常量对应值更高，
// 因为系统依赖于一个至少足够大以容纳系统支持的寄存器的结构。
//
// 目前它们被设置为零，因为使用实际的常量会破坏工具链中
// 使用 reflect 调用函数的每个部分（例如 go test，或任何
// 使用 text/template 的程序）。目前被注释掉的值应该是
// 我们准备在所有地方使用寄存器 ABI 后的实际值。
var (
	intArgRegs   = abi.IntArgRegs
	floatArgRegs = abi.FloatArgRegs
	floatRegSize = uintptr(abi.EffectiveFloatRegSize)
)

// abiStep 表示一个 ABI "指令"。每条指令描述了
// 如何在内存中的 Go 值和调用帧之间进行转换的一部分。
type abiStep struct {
	kind abiStepKind

	// offset 和 size 一起描述内存中 Go 值的一部分。
	offset uintptr
	size   uintptr // 该部分的字节大小

	// 这些字段描述转换的 ABI 端。
	stkOff uintptr // 栈偏移量，当 kind == abiStepStack 时使用
	ireg   int     // 整数寄存器索引，当 kind == abiStepIntReg 或 kind == abiStepPointer 时使用
	freg   int     // 浮点寄存器索引，当 kind == abiStepFloatReg 时使用
}

// abiStepKind 是 abiStep 指令的 "操作码"。
type abiStepKind int

const (
	abiStepBad      abiStepKind = iota
	abiStepStack                // 复制到栈/从栈复制
	abiStepIntReg               // 复制到整数寄存器/从整数寄存器复制
	abiStepPointer              // 复制指针到整数寄存器/从整数寄存器复制指针
	abiStepFloatReg             // 复制到浮点寄存器/从浮点寄存器复制
)

// abiSeq 表示一系列 ABI 指令，用于从一系列 reflect.Value
// 复制到调用帧（用于调用参数）或反向复制（用于调用结果）。
//
// abiSeq 应通过调用其 addArg 方法来填充。
type abiSeq struct {
	// steps 是指令集合。
	//
	// 指令按完整参数分组，第 i 个 Go 值的指令起始索引
	// 可在 valueStart 中获取。
	//
	// 例如，如果这个 abiSeq 表示传递给函数的 3 个参数，
	// 那么第 2 个参数的步骤从 steps[valueStart[1]] 开始。
	//
	// 由于 reflect 接受不同的 Value 作为 Go 参数，
	// 且每个 Value 都是单独存储的，所以每个开始新参数的
	// abiStep 的 offset 字段都为 0。
	steps      []abiStep
	valueStart []int

	stackBytes   uintptr // 使用的栈空间
	iregs, fregs int     // 使用的寄存器
}

func (a *abiSeq) dump() {
	for i, p := range a.steps {
		println("part", i, p.kind, p.offset, p.size, p.stkOff, p.ireg, p.freg)
	}
	print("values ")
	for _, i := range a.valueStart {
		print(i, " ")
	}
	println()
	println("stack", a.stackBytes)
	println("iregs", a.iregs)
	println("fregs", a.fregs)
}

// stepsForValue 返回用于将此 abiSeq 表示的第 i 个
// Go 参数或返回值转换为 Go ABI 的 ABI 指令。
func (a *abiSeq) stepsForValue(i int) []abiStep {
	s := a.valueStart[i]
	var e int
	if i == len(a.valueStart)-1 {
		e = len(a.steps)
	} else {
		e = a.valueStart[i+1]
	}
	return a.steps[s:e]
}

// addArg 用类型为 t 的新 Go 值扩展 abiSeq。
//
// 如果值被分配到栈上，则返回描述该转换的单个 abiStep，
// 否则返回 nil。
func (a *abiSeq) addArg(t *abi.Type) *abiStep {
	// 我们总是会添加一个新值，所以先做这个。
	pStart := len(a.steps)
	a.valueStart = append(a.valueStart, pStart)
	if t.Size() == 0 {
		// 如果参数类型的大小为零，那么为了优雅地降级到 ABI0，
		// 我们需要将此类型分配到栈上。原因是虽然零大小类型
		// 在栈上不占用空间，但它们确实会导致下一个参数被对齐。
		// 所以在这里这样做，但不必为其实际生成新的 ABI 步骤
		// （实际上没有任何东西需要复制）。
		//
		// 我们无法在 regAssign 的递归情况下处理这种情况，
		// 因为非零大小结构体的零大小*字段*不会导致其被分配到栈上。
		// 所以我们需要在顶层这里处理这个特殊情况。
		a.stackBytes = align(a.stackBytes, uintptr(t.Align()))
		return nil
	}
	// 保留 "a" 的副本，以便在寄存器分配失败时可以回滚。
	aOld := *a
	if !a.regAssign(t, 0) {
		// 寄存器分配失败。回滚所有更改并分配到栈上。
		*a = aOld
		a.stackAssign(t.Size(), uintptr(t.Align()))
		return &a.steps[len(a.steps)-1]
	}
	return nil
}

// addRcvr 根据接口调用约定用新的方法调用接收者扩展 abiSeq。
//
// 如果接收者被分配到栈上，则返回描述该转换的单个 abiStep，
// 否则返回 nil。如果接收者是指针则返回 true。
func (a *abiSeq) addRcvr(rcvr *abi.Type) (*abiStep, bool) {
	// 接收者始终是一个字。
	a.valueStart = append(a.valueStart, len(a.steps))
	var ok, ptr bool
	if !rcvr.IsDirectIface() || rcvr.Pointers() {
		ok = a.assignIntN(0, goarch.PtrSize, 1, 0b1)
		ptr = true
	} else {
		// TODO(mknyszek): 这种情况真的可能发生吗？
		// 接口数据工作区永远不会包含非指针值。
		// 这个情况是从 reflect 包中的旧代码复制过来的，
		// 该代码仅有条件地向 reflect.(Value).Call 栈帧的
		// GC 位图添加指针位。
		ok = a.assignIntN(0, goarch.PtrSize, 1, 0b0)
		ptr = false
	}
	if !ok {
		a.stackAssign(goarch.PtrSize, goarch.PtrSize)
		return &a.steps[len(a.steps)-1], ptr
	}
	return nil, ptr
}

// regAssign 尝试为存储在某个偏移量处的类型为 t 的值保留参数寄存器。
//
// 它返回分配是否成功，但会保留对 a.steps 所做的任何更改，
// 因此如果失败，调用者必须通过调整 a.steps 来撤销这些工作。
//
// 此方法与 assign* 方法一起代表了 Go ABI 的完整寄存器分配算法。
func (a *abiSeq) regAssign(t *abi.Type, offset uintptr) bool {
	switch Kind(t.Kind()) {
	case UnsafePointer, Pointer, Chan, Map, Func:
		return a.assignIntN(offset, t.Size(), 1, 0b1)
	case Bool, Int, Uint, Int8, Uint8, Int16, Uint16, Int32, Uint32, Uintptr:
		return a.assignIntN(offset, t.Size(), 1, 0b0)
	case Int64, Uint64:
		switch goarch.PtrSize {
		case 4:
			return a.assignIntN(offset, 4, 2, 0b0)
		case 8:
			return a.assignIntN(offset, 8, 1, 0b0)
		}
	case Float32, Float64:
		return a.assignFloatN(offset, t.Size(), 1)
	case Complex64:
		return a.assignFloatN(offset, 4, 2)
	case Complex128:
		return a.assignFloatN(offset, 8, 2)
	case String:
		return a.assignIntN(offset, goarch.PtrSize, 2, 0b01)
	case Interface:
		return a.assignIntN(offset, goarch.PtrSize, 2, 0b10)
	case Slice:
		return a.assignIntN(offset, goarch.PtrSize, 3, 0b001)
	case Array:
		tt := (*arrayType)(unsafe.Pointer(t))
		switch tt.Len {
		case 0:
			// 没有需要分配的内容，所以不修改 a.steps，
			// 但返回成功以便调用者不会尝试将此值分配到栈上。
			return true
		case 1:
			return a.regAssign(tt.Elem, offset)
		default:
			return false
		}
	case Struct:
		st := (*structType)(unsafe.Pointer(t))
		for i := range st.Fields {
			f := &st.Fields[i]
			if !a.regAssign(f.Typ, offset+f.Offset) {
				return false
			}
		}
		return true
	default:
		print("t.Kind == ", t.Kind(), "\n")
		panic("unknown type kind")
	}
	panic("unhandled register assignment path")
}

// assignIntN 将 n 个值分配到寄存器，每个值大小为 "size" 字节，
// 数据来自内存中的 [offset, offset+n*size)。对于 i < n，
// 每个位于 [offset+i*size, offset+(i+1)*size) 的值被分配到接下来的 n 个整数寄存器。
//
// ptrMap 中的第 i 位指示第 i 个值是否为指针。
// n 必须 <= 8。
//
// 返回分配是否成功。
func (a *abiSeq) assignIntN(offset, size uintptr, n int, ptrMap uint8) bool {
	if n > 8 || n < 0 {
		panic("invalid n")
	}
	if ptrMap != 0 && size != goarch.PtrSize {
		panic("non-empty pointer map passed for non-pointer-size values")
	}
	if a.iregs+n > intArgRegs {
		return false
	}
	for i := 0; i < n; i++ {
		kind := abiStepIntReg
		if ptrMap&(uint8(1)<<i) != 0 {
			kind = abiStepPointer
		}
		a.steps = append(a.steps, abiStep{
			kind:   kind,
			offset: offset + uintptr(i)*size,
			size:   size,
			ireg:   a.iregs,
		})
		a.iregs++
	}
	return true
}

// assignFloatN 将 n 个值分配到寄存器，每个值大小为 "size" 字节，
// 数据来自内存中的 [offset, offset+n*size)。对于 i < n，
// 每个位于 [offset+i*size, offset+(i+1)*size) 的值被分配到接下来的 n 个浮点寄存器。
//
// 返回分配是否成功。
func (a *abiSeq) assignFloatN(offset, size uintptr, n int) bool {
	if n < 0 {
		panic("invalid n")
	}
	if a.fregs+n > floatArgRegs || floatRegSize < size {
		return false
	}
	for i := 0; i < n; i++ {
		a.steps = append(a.steps, abiStep{
			kind:   abiStepFloatReg,
			offset: offset + uintptr(i)*size,
			size:   size,
			freg:   a.fregs,
		})
		a.fregs++
	}
	return true
}

// stackAssign 在栈上为一个大小为 "size" 字节、
// 对齐方式为 "alignment" 的值保留空间。
//
// 不应直接调用；请使用 addArg。
func (a *abiSeq) stackAssign(size, alignment uintptr) {
	a.stackBytes = align(a.stackBytes, alignment)
	a.steps = append(a.steps, abiStep{
		kind:   abiStepStack,
		offset: 0, // 仅用于完整参数，所以内存偏移量为 0。
		size:   size,
		stkOff: a.stackBytes,
	})
	a.stackBytes += size
}

// abiDesc 描述函数或方法的 ABI。
type abiDesc struct {
	// call 和 ret 表示 Go 函数的调用路径和返回路径的转换步骤。
	call, ret abiSeq

	// 这些字段描述为调用分配的栈空间。stackCallArgsSize 是
	// 为参数（但不包括返回值）保留的空间大小。retOffset 是
	// 返回值开始的偏移量，spill 是额外保留空间的字节大小，
	// 用于在 reflectcall 栈帧中发生抢占时溢出参数寄存器。
	stackCallArgsSize, retOffset, spill uintptr

	// stackPtrs 是一个位图，指示 ABI 栈空间（栈分配的参数 + 返回值）
	// 中的每个字是否为指针。用作传递给 reflectcall 的栈空间的堆指针位图。
	stackPtrs *bitVector

	// inRegPtrs 是一个位图，其第 i 位指示第 i 个整数参数寄存器
	// 是否包含指针。由 makeFuncStub 和 methodValueCall 使用，
	// 以使结果指针对 GC 可见。
	//
	// outRegPtrs 相同，但用于结果值。
	// 由 reflectcall 使用，以使结果指针对 GC 可见。
	inRegPtrs, outRegPtrs abi.IntArgRegBitmap
}

func (a *abiDesc) dump() {
	println("ABI")
	println("call")
	a.call.dump()
	println("ret")
	a.ret.dump()
	println("stackCallArgsSize", a.stackCallArgsSize)
	println("retOffset", a.retOffset)
	println("spill", a.spill)
	print("inRegPtrs:")
	dumpPtrBitMap(a.inRegPtrs)
	println()
	print("outRegPtrs:")
	dumpPtrBitMap(a.outRegPtrs)
	println()
}

func dumpPtrBitMap(b abi.IntArgRegBitmap) {
	for i := 0; i < intArgRegs; i++ {
		x := 0
		if b.Get(i) {
			x = 1
		}
		print(" ", x)
	}
}

func newAbiDesc(t *funcType, rcvr *abi.Type) abiDesc {
	// 我们需要为这个参数在帧中添加空间，以便可以将参数溢出到其中。
	//
	// 这个空间的大小就是每个寄存器分配类型的大小之和。
	//
	// TODO(mknyszek): 当我们不再有调用者保留的溢出空间时，删除此代码。
	spill := uintptr(0)

	// 计算栈参数的 gc 程序和栈位图
	stackPtrs := new(bitVector)

	// 计算参数的栈帧指针位图和寄存器指针位图。
	inRegPtrs := abi.IntArgRegBitmap{}

	// 计算输入参数的 abiSeq。
	var in abiSeq
	if rcvr != nil {
		stkStep, isPtr := in.addRcvr(rcvr)
		if stkStep != nil {
			if isPtr {
				stackPtrs.append(1)
			} else {
				stackPtrs.append(0)
			}
		} else {
			spill += goarch.PtrSize
		}
	}
	for i, arg := range t.InSlice() {
		stkStep := in.addArg(arg)
		if stkStep != nil {
			addTypeBits(stackPtrs, stkStep.stkOff, arg)
		} else {
			spill = align(spill, uintptr(arg.Align()))
			spill += arg.Size()
			for _, st := range in.stepsForValue(i) {
				if st.kind == abiStepPointer {
					inRegPtrs.Set(st.ireg)
				}
			}
		}
	}
	spill = align(spill, goarch.PtrSize)

	// 仅从输入参数，我们现在就知道 stackCallArgsSize 和 retOffset。
	stackCallArgsSize := in.stackBytes
	retOffset := align(in.stackBytes, goarch.PtrSize)

	// 计算返回值的栈帧指针位图和寄存器指针位图。
	outRegPtrs := abi.IntArgRegBitmap{}

	// 计算输出参数的 abiSeq。
	var out abiSeq
	// 栈分配的返回值不像寄存器那样与参数共享空间，
	// 所以我们需要在这里注入一个栈偏移量。
	// 通过人为地将 stackBytes 扩展返回偏移量来模拟。
	out.stackBytes = retOffset
	for i, res := range t.OutSlice() {
		stkStep := out.addArg(res)
		if stkStep != nil {
			addTypeBits(stackPtrs, stkStep.stkOff, res)
		} else {
			for _, st := range out.stepsForValue(i) {
				if st.kind == abiStepPointer {
					outRegPtrs.Set(st.ireg)
				}
			}
		}
	}
	// 撤销之前的模拟，使 stackBytes 准确。
	out.stackBytes -= retOffset
	return abiDesc{in, out, stackCallArgsSize, retOffset, spill, stackPtrs, inRegPtrs, outRegPtrs}
}

// intFromReg 从 reg 加载一个 argSize 大小的整数并将其放置到 to。
//
// argSize 必须非零、能放入寄存器且是 2 的幂。
func intFromReg(r *abi.RegArgs, reg int, argSize uintptr, to unsafe.Pointer) {
	memmove(to, r.IntRegArgAddr(reg, argSize), argSize)
}

// intToReg 加载一个 argSize 大小的整数并将其存储到 reg。
//
// argSize 必须非零、能放入寄存器且是 2 的幂。
func intToReg(r *abi.RegArgs, reg int, argSize uintptr, from unsafe.Pointer) {
	memmove(r.IntRegArgAddr(reg, argSize), from, argSize)
}

// floatFromReg 从 r 中的寄存器表示加载一个浮点值。
//
// argSize 必须是 4 或 8。
func floatFromReg(r *abi.RegArgs, reg int, argSize uintptr, to unsafe.Pointer) {
	switch argSize {
	case 4:
		*(*float32)(to) = archFloat32FromReg(r.Floats[reg])
	case 8:
		*(*float64)(to) = *(*float64)(unsafe.Pointer(&r.Floats[reg]))
	default:
		panic("bad argSize")
	}
}

// floatToReg 将浮点值存储到 r 中的寄存器表示。
//
// argSize 必须是 4 或 8。
func floatToReg(r *abi.RegArgs, reg int, argSize uintptr, from unsafe.Pointer) {
	switch argSize {
	case 4:
		r.Floats[reg] = archFloat32ToReg(*(*float32)(from))
	case 8:
		r.Floats[reg] = *(*uint64)(from)
	default:
		panic("bad argSize")
	}
}
