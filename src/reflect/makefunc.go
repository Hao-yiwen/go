// 版权所有 2012 The Go Authors。保留所有权利。
// 本源代码的使用受 BSD 风格许可证约束，
// 该许可证可在 LICENSE 文件中找到。

// MakeFunc 实现。

package reflect

import (
	"internal/abi"
	"internal/goarch"
	"unsafe"
)

// makeFuncImpl 是实现 MakeFunc 返回的函数的闭包值。
// 此类型的前三个字必须与 methodValue 和 runtime.reflectMethodValue 保持同步。
// 任何更改都应反映在这三者中。
type makeFuncImpl struct {
	makeFuncCtxt
	ftyp *funcType
	fn   func([]Value) []Value
}

// MakeFunc 返回一个给定 [Type] 的新函数，该函数包装了函数 fn。
// 当调用时，这个新函数执行以下操作：
//
//   - 将其参数转换为 Value 切片。
//   - 运行 results := fn(args)。
//   - 将结果作为 Value 切片返回，每个形式结果对应一个。
//
// 实现函数 fn 可以假设参数 [Value] 切片具有 typ 给出的参数数量和类型。
// 如果 typ 描述一个可变参数函数，最后一个 Value 本身就是一个
// 表示可变参数的切片，就像在可变参数函数体中一样。
// fn 返回的结果 Value 切片必须具有 typ 给出的结果数量和类型。
//
// [Value.Call] 方法允许调用者以 Value 的形式调用类型化函数；
// 相反，MakeFunc 允许调用者以 Value 的形式实现类型化函数。
//
// 文档的示例部分包含了如何使用 MakeFunc
// 为不同类型构建交换函数的说明。
func MakeFunc(typ Type, fn func(args []Value) (results []Value)) Value {
	if typ.Kind() != Func {
		panic("reflect: call of MakeFunc with non-Func type")
	}

	t := typ.common()
	ftyp := (*funcType)(unsafe.Pointer(t))

	code := abi.FuncPCABI0(makeFuncStub)

	// makeFuncImpl 包含一个供运行时使用的栈映射
	_, _, abid := funcLayout(ftyp, nil)

	impl := &makeFuncImpl{
		makeFuncCtxt: makeFuncCtxt{
			fn:      code,
			stack:   abid.stackPtrs,
			argLen:  abid.stackCallArgsSize,
			regPtrs: abid.inRegPtrs,
		},
		ftyp: ftyp,
		fn:   fn,
	}

	return Value{t, unsafe.Pointer(impl), flag(Func)}
}

// makeFuncStub 是一个汇编函数，是 MakeFunc 返回的函数的代码部分。
// 它期望一个 *callReflectFunc 作为其上下文寄存器，
// 它的工作是调用 callReflect(ctxt, frame)，
// 其中 ctxt 是上下文寄存器，frame 是指向传入参数帧中第一个字的指针。
func makeFuncStub()

// 此类型的前 3 个字必须与 makeFuncImpl 和 runtime.reflectMethodValue 保持同步。
// 任何更改都应反映在这三者中。
type methodValue struct {
	makeFuncCtxt
	method int
	rcvr   Value
}

// makeMethodValue 将 v 从方法值的 rcvr+method 索引表示
// （基本上是设置了特殊位的接收者值）转换为真正的 func 值
// （持有实际 func 的值）。就 reflect 包的用户所能看到的而言，
// 输出在语义上等同于输入，但真正的 func 表示可以被
// Convert、Interface 和 Assign 等代码处理。
func makeMethodValue(op string, v Value) Value {
	if v.flag&flagMethod == 0 {
		panic("reflect: internal error: invalid use of makeMethodValue")
	}

	// 忽略 flagMethod 位，v 描述的是接收者，而不是方法类型。
	fl := v.flag & (flagRO | flagAddr | flagIndir)
	fl |= flag(v.typ().Kind())
	rcvr := Value{v.typ(), v.ptr, fl}

	// v.Type 返回方法值的实际类型。
	ftyp := (*funcType)(unsafe.Pointer(v.Type().(*rtype)))

	code := methodValueCallCodePtr()

	// methodValue 包含一个供运行时使用的栈映射
	_, _, abid := funcLayout(ftyp, nil)
	fv := &methodValue{
		makeFuncCtxt: makeFuncCtxt{
			fn:      code,
			stack:   abid.stackPtrs,
			argLen:  abid.stackCallArgsSize,
			regPtrs: abid.inRegPtrs,
		},
		method: int(v.flag) >> flagMethodShift,
		rcvr:   rcvr,
	}

	// 如果方法不合适则引发 panic。
	// 如果我们省略这个，panic 仍然会在调用期间发生，
	// 但我们希望 Interface() 和其他操作尽早失败。
	methodReceiver(op, fv.rcvr, fv.method)

	return Value{ftyp.Common(), unsafe.Pointer(fv), v.flag&flagRO | flag(Func)}
}

func methodValueCallCodePtr() uintptr {
	return abi.FuncPCABI0(methodValueCall)
}

// methodValueCall 是一个汇编函数，是 makeMethodValue 返回的函数的代码部分。
// 它期望一个 *methodValue 作为其上下文寄存器，
// 它的工作是调用 callMethod(ctxt, frame)，
// 其中 ctxt 是上下文寄存器，frame 是指向传入参数帧中第一个字的指针。
func methodValueCall()

// 此结构必须与 runtime.reflectMethodValue 保持同步。
// 任何更改都应反映在两者中。
type makeFuncCtxt struct {
	fn      uintptr
	stack   *bitVector // 栈参数和结果的指针映射
	argLen  uintptr    // 仅参数
	regPtrs abi.IntArgRegBitmap
}

// moveMakeFuncArgPtrs 使用 ctxt.regPtrs 将 args.Ints 中的整数指针参数
// 复制到 args.Ptrs 中，GC 可以在那里看到它们。
//
// 这与运行时中 reflectcallmove 的功能类似，
// 只是那个发生在返回路径上，而这个发生在调用路径上。
//
// nosplit 是因为指针被保存在 args 的 uintptr 槽中，
// 所以现在扫描我们的栈可能会导致意外释放内存。
//
//go:nosplit
func moveMakeFuncArgPtrs(ctxt *makeFuncCtxt, args *abi.RegArgs) {
	for i, arg := range args.Ints {
		// 避免写屏障！因为我们的写屏障会将之前存在的内容入队，
		// 我们可能会将垃圾入队。
		// 同时避免边界检查，我们没有足够的栈空间。
		// （通常 prove pass 会移除它们，但对于 -N 构建我们使用太多栈。）
		// ptr := &args.Ptrs[i]（但从 *unsafe.Pointer 转换为 *uintptr）
		ptr := (*uintptr)(add(unsafe.Pointer(unsafe.SliceData(args.Ptrs[:])), uintptr(i)*goarch.PtrSize, "always in [0:IntArgRegs]"))
		if ctxt.regPtrs.Get(i) {
			*ptr = arg
		} else {
			// 我们*必须*自己将此空间清零，因为它是在汇编代码中定义的，
			// GC 会扫描这些指针。否则，这里会有垃圾。
			*ptr = 0
		}
	}
}
