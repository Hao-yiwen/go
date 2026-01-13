// 版权所有 2018 The Go Authors。保留所有权利。
// 本源代码的使用受 BSD 风格许可证约束，
// 该许可证可在 LICENSE 文件中找到。

//go:build js && wasm

// js 包在使用 js/wasm 架构时提供对 WebAssembly 主机环境的访问。
// 其 API 基于 JavaScript 语义。
//
// 此包是实验性的。其当前范围仅允许测试运行，但尚未提供
// 用户的全面 API。它不受 Go 兼容性承诺的约束。
package js

import (
	"runtime"
	"unsafe"
)

// ref 用于标识 JavaScript 值，因为该值本身无法传递到 WebAssembly。
//
// JavaScript 值 "undefined" 由值 0 表示。
// JavaScript 数字（64 位浮点数，除 0 和 NaN）由其 IEEE 754 二进制表示形式表示。
// 所有其他值都表示为 NaN 的 IEEE 754 二进制表示形式，其中位 0-31 用作
// ID，位 32-34 用于区分字符串、符号、函数和对象。
type ref uint64

// nanHead 是 ref 的上 32 位，如果值未编码为 IEEE 754 数字（见上文），则设置这些位。
const nanHead = 0x7FF80000

// Value 表示 JavaScript 值。零值是 JavaScript 值 "undefined"。
// 可以使用 Equal 方法检查值是否相等。
type Value struct {
	_     [0]func() // 不可比较；使 == 无法编译
	ref   ref       // 标识 JavaScript 值，参见 ref 类型
	gcPtr *ref      // 用于在不再引用 Value 时触发终结器
}

const (
	// 类型标志需要与 wasm_exec.js 同步
	typeFlagNone = iota
	typeFlagObject
	typeFlagString
	typeFlagSymbol
	typeFlagFunction
)

func makeValue(r ref) Value {
	var gcPtr *ref
	typeFlag := (r >> 32) & 7
	if (r>>32)&nanHead == nanHead && typeFlag != typeFlagNone {
		gcPtr = new(ref)
		*gcPtr = r
		runtime.SetFinalizer(gcPtr, func(p *ref) {
			finalizeRef(*p)
		})
	}

	return Value{ref: r, gcPtr: gcPtr}
}

//go:wasmimport gojs syscall/js.finalizeRef
func finalizeRef(r ref)

func predefValue(id uint32, typeFlag byte) Value {
	return Value{ref: (nanHead|ref(typeFlag))<<32 | ref(id)}
}

func floatValue(f float64) Value {
	if f == 0 {
		return valueZero
	}
	if f != f {
		return valueNaN
	}
	return Value{ref: *(*ref)(unsafe.Pointer(&f))}
}

// Error 包装 JavaScript 错误。
type Error struct {
	// Value 是底层 JavaScript 错误值。
	Value
}

// Error implements the error interface.
func (e Error) Error() string {
	return "JavaScript error: " + e.Get("message").String()
}

var (
	valueUndefined = Value{ref: 0}
	valueNaN       = predefValue(0, typeFlagNone)
	valueZero      = predefValue(1, typeFlagNone)
	valueNull      = predefValue(2, typeFlagNone)
	valueTrue      = predefValue(3, typeFlagNone)
	valueFalse     = predefValue(4, typeFlagNone)
	valueGlobal    = predefValue(5, typeFlagObject)
	jsGo           = predefValue(6, typeFlagObject) // JavaScript 中 Go 类的实例

	objectConstructor = valueGlobal.Get("Object")
	arrayConstructor  = valueGlobal.Get("Array")
)

// Equal 报告根据 JavaScript 的 === 运算符 v 和 w 是否相等。
func (v Value) Equal(w Value) bool {
	return v.ref == w.ref && v.ref != valueNaN.ref
}

// Undefined 返回 JavaScript 值 "undefined"。
func Undefined() Value {
	return valueUndefined
}

// IsUndefined 报告 v 是否是 JavaScript 值 "undefined"。
func (v Value) IsUndefined() bool {
	return v.ref == valueUndefined.ref
}

// Null 返回 JavaScript 值 "null"。
func Null() Value {
	return valueNull
}

// IsNull 报告 v 是否是 JavaScript 值 "null"。
func (v Value) IsNull() bool {
	return v.ref == valueNull.ref
}

// IsNaN 报告 v 是否是 JavaScript 值 "NaN"。
func (v Value) IsNaN() bool {
	return v.ref == valueNaN.ref
}

// Global 返回 JavaScript 全局对象，通常是 "window" 或 "global"。
func Global() Value {
	return valueGlobal
}

// ValueOf 返回 x 作为 JavaScript 值：
//
//	| Go                     | JavaScript             |
//	| ---------------------- | ---------------------- |
//	| js.Value               | [其值]                 |
//	| js.Func                | 函数                   |
//	| nil                    | null                   |
//	| bool                   | 布尔值                 |
//	| 整数和浮点数           | 数字                   |
//	| string                 | 字符串                 |
//	| []interface{}          | 新数组                 |
//	| map[string]interface{} | 新对象                 |
//
// 如果 x 不是预期的类型之一，则触发 panic。
func ValueOf(x any) Value {
	switch x := x.(type) {
	case Value:
		return x
	case Func:
		return x.Value
	case nil:
		return valueNull
	case bool:
		if x {
			return valueTrue
		} else {
			return valueFalse
		}
	case int:
		return floatValue(float64(x))
	case int8:
		return floatValue(float64(x))
	case int16:
		return floatValue(float64(x))
	case int32:
		return floatValue(float64(x))
	case int64:
		return floatValue(float64(x))
	case uint:
		return floatValue(float64(x))
	case uint8:
		return floatValue(float64(x))
	case uint16:
		return floatValue(float64(x))
	case uint32:
		return floatValue(float64(x))
	case uint64:
		return floatValue(float64(x))
	case uintptr:
		return floatValue(float64(x))
	case unsafe.Pointer:
		return floatValue(float64(uintptr(x)))
	case float32:
		return floatValue(float64(x))
	case float64:
		return floatValue(x)
	case string:
		return makeValue(stringVal(x))
	case []any:
		a := arrayConstructor.New(len(x))
		for i, s := range x {
			a.SetIndex(i, s)
		}
		return a
	case map[string]any:
		o := objectConstructor.New()
		for k, v := range x {
			o.Set(k, v)
		}
		return o
	default:
		panic("ValueOf: invalid value")
	}
}

// stringVal 将字符串 x 复制到 Javascript 并返回一个 ref。
//
// 使用 go:noescape 是安全的，因为在系统调用返回后，
// 不会维护对 Go 字符串 x 的引用。
//
//go:wasmimport gojs syscall/js.stringVal
//go:noescape
func stringVal(x string) ref

// Type 表示 Value 的 JavaScript 类型。
type Type int

const (
	TypeUndefined Type = iota
	TypeNull
	TypeBoolean
	TypeNumber
	TypeString
	TypeSymbol
	TypeObject
	TypeFunction
)

func (t Type) String() string {
	switch t {
	case TypeUndefined:
		return "undefined"
	case TypeNull:
		return "null"
	case TypeBoolean:
		return "boolean"
	case TypeNumber:
		return "number"
	case TypeString:
		return "string"
	case TypeSymbol:
		return "symbol"
	case TypeObject:
		return "object"
	case TypeFunction:
		return "function"
	default:
		panic("bad type")
	}
}

func (t Type) isObject() bool {
	return t == TypeObject || t == TypeFunction
}

// Type 返回值 v 的 JavaScript 类型。它类似于 JavaScript 的 typeof 运算符，
// 但对于 null，它返回 TypeNull 而不是 TypeObject。
func (v Value) Type() Type {
	switch v.ref {
	case valueUndefined.ref:
		return TypeUndefined
	case valueNull.ref:
		return TypeNull
	case valueTrue.ref, valueFalse.ref:
		return TypeBoolean
	}
	if v.isNumber() {
		return TypeNumber
	}
	typeFlag := (v.ref >> 32) & 7
	switch typeFlag {
	case typeFlagObject:
		return TypeObject
	case typeFlagString:
		return TypeString
	case typeFlagSymbol:
		return TypeSymbol
	case typeFlagFunction:
		return TypeFunction
	default:
		panic("bad type flag")
	}
}

// Get 返回值 v 的 JavaScript 属性 p。
// 如果 v 不是 JavaScript 对象，则触发 panic。
func (v Value) Get(p string) Value {
	if vType := v.Type(); !vType.isObject() {
		panic(&ValueError{"Value.Get", vType})
	}
	r := makeValue(valueGet(v.ref, p))
	runtime.KeepAlive(v)
	return r
}

// valueGet 返回对 ref v 的 JavaScript 属性 p 的引用。
//
// 使用 go:noescape 是安全的，因为在系统调用返回后，
// 不会维护对 Go 字符串 p 的引用。
//
//go:wasmimport gojs syscall/js.valueGet
//go:noescape
func valueGet(v ref, p string) ref

// Set 将值 v 的 JavaScript 属性 p 设置为 ValueOf(x)。
// 如果 v 不是 JavaScript 对象，则触发 panic。
func (v Value) Set(p string, x any) {
	if vType := v.Type(); !vType.isObject() {
		panic(&ValueError{"Value.Set", vType})
	}
	xv := ValueOf(x)
	valueSet(v.ref, p, xv.ref)
	runtime.KeepAlive(v)
	runtime.KeepAlive(xv)
}

// valueSet 将 ref v 的属性 p 设置为 ref x。
//
// 使用 go:noescape 是安全的，因为在系统调用返回后，
// 不会维护对 Go 字符串 p 的引用。
//
//go:wasmimport gojs syscall/js.valueSet
//go:noescape
func valueSet(v ref, p string, x ref)

// Delete 删除值 v 的 JavaScript 属性 p。
// 如果 v 不是 JavaScript 对象，则触发 panic。
func (v Value) Delete(p string) {
	if vType := v.Type(); !vType.isObject() {
		panic(&ValueError{"Value.Delete", vType})
	}
	valueDelete(v.ref, p)
	runtime.KeepAlive(v)
}

// valueDelete 删除 ref v 的 JavaScript 属性 p。
//
// 使用 go:noescape 是安全的，因为在系统调用返回后，
// 不会维护对 Go 字符串 p 的引用。
//
//go:wasmimport gojs syscall/js.valueDelete
//go:noescape
func valueDelete(v ref, p string)

// Index 返回值 v 的 JavaScript 索引 i。
// 如果 v 不是 JavaScript 对象，则触发 panic。
func (v Value) Index(i int) Value {
	if vType := v.Type(); !vType.isObject() {
		panic(&ValueError{"Value.Index", vType})
	}
	r := makeValue(valueIndex(v.ref, i))
	runtime.KeepAlive(v)
	return r
}

//go:wasmimport gojs syscall/js.valueIndex
func valueIndex(v ref, i int) ref

// SetIndex 将值 v 的 JavaScript 索引 i 设置为 ValueOf(x)。
// 如果 v 不是 JavaScript 对象，则触发 panic。
func (v Value) SetIndex(i int, x any) {
	if vType := v.Type(); !vType.isObject() {
		panic(&ValueError{"Value.SetIndex", vType})
	}
	xv := ValueOf(x)
	valueSetIndex(v.ref, i, xv.ref)
	runtime.KeepAlive(v)
	runtime.KeepAlive(xv)
}

//go:wasmimport gojs syscall/js.valueSetIndex
func valueSetIndex(v ref, i int, x ref)

// makeArgSlices 创建两个切片来保存 JavaScript arg 数据。
// 可以与 storeArgs 配对以制作和存储 JavaScript arg 切片。
// 但是，这两个函数被分开以确保 makeArgSlices 被内联，
// 这将防止为少量（<=16）个 arg 在堆上分配切片。
func makeArgSlices(size int) (argVals []Value, argRefs []ref) {
	// 选择的值是 2 的幂，足以处理所有 web API
	// 特别是，请注意 WebGL2 的 texImage2D 最多需要 10 个参数
	const maxStackArgs = 16
	if size <= maxStackArgs {
		// 只要 makeArgs 被内联，这些就会被栈分配
		argVals = make([]Value, size, maxStackArgs)
		argRefs = make([]ref, size, maxStackArgs)
	} else {
		// 在堆上分配，但超过 maxStackArgs 应该很少见
		argVals = make([]Value, size)
		argRefs = make([]ref, size)
	}
	return
}

// storeArgs 将输入 args 映射到各自的 Value 和 ref 切片。
// 可以与 makeArgSlices 配对以制作和存储 JavaScript arg 切片。
func storeArgs(args []any, argValsDst []Value, argRefsDst []ref) {
	// 如果组合后的函数足够简单，可以内联，则会在 makeArgs 中处理
	for i, arg := range args {
		v := ValueOf(arg)
		argValsDst[i] = v
		argRefsDst[i] = v.ref
	}
}

// Length 返回 v 的 JavaScript 属性 "length"。
// 如果 v 不是 JavaScript 对象，则触发 panic。
func (v Value) Length() int {
	if vType := v.Type(); !vType.isObject() {
		panic(&ValueError{"Value.SetIndex", vType})
	}
	r := valueLength(v.ref)
	runtime.KeepAlive(v)
	return r
}

//go:wasmimport gojs syscall/js.valueLength
func valueLength(v ref) int

// Call 使用给定的参数对值 v 的方法 m 进行 JavaScript 调用。
// 如果 v 没有方法 m，则触发 panic。
// 根据 ValueOf 函数将参数映射到 JavaScript 值。
func (v Value) Call(m string, args ...any) Value {
	argVals, argRefs := makeArgSlices(len(args))
	storeArgs(args, argVals, argRefs)
	res, ok := valueCall(v.ref, m, argRefs)
	runtime.KeepAlive(v)
	runtime.KeepAlive(argVals)
	if !ok {
		if vType := v.Type(); !vType.isObject() { // 在此检查以避免成功情况下的开销
			panic(&ValueError{"Value.Call", vType})
		}
		if propType := v.Get(m).Type(); propType != TypeFunction {
			panic("syscall/js: Value.Call: property " + m + " is not a function, got " + propType.String())
		}
		panic(Error{makeValue(res)})
	}
	return makeValue(res)
}

// valueCall 使用给定的参数对 ref v 的方法名 m 进行 JavaScript 调用。
//
// 使用 go:noescape 是安全的，因为在系统调用返回后，
// 不会维护对 Go 字符串 m 的引用。此外，args 切片
// 仅临时用于收集 JavaScript 对象以进行
// JavaScript 方法调用。
//
//go:wasmimport gojs syscall/js.valueCall
//go:nosplit
//go:noescape
func valueCall(v ref, m string, args []ref) (ref, bool)

// Invoke 使用给定的参数对值 v 进行 JavaScript 调用。
// 如果 v 不是 JavaScript 函数，则触发 panic。
// 根据 ValueOf 函数将参数映射到 JavaScript 值。
func (v Value) Invoke(args ...any) Value {
	argVals, argRefs := makeArgSlices(len(args))
	storeArgs(args, argVals, argRefs)
	res, ok := valueInvoke(v.ref, argRefs)
	runtime.KeepAlive(v)
	runtime.KeepAlive(argVals)
	if !ok {
		if vType := v.Type(); vType != TypeFunction { // 在此检查以避免成功情况下的开销
			panic(&ValueError{"Value.Invoke", vType})
		}
		panic(Error{makeValue(res)})
	}
	return makeValue(res)
}

// valueInvoke 使用给定的参数对值 v 进行 JavaScript 调用。
//
// 使用 go:noescape 是安全的，因为 args 切片仅临时用于
// 收集 JavaScript 对象以进行 JavaScript 方法
// 调用。
//
//go:wasmimport gojs syscall/js.valueInvoke
//go:noescape
func valueInvoke(v ref, args []ref) (ref, bool)

// New 使用 JavaScript 的 "new" 运算符，以值 v 作为构造函数和给定的参数。
// 如果 v 不是 JavaScript 函数，则触发 panic。
// 根据 ValueOf 函数将参数映射到 JavaScript 值。
func (v Value) New(args ...any) Value {
	argVals, argRefs := makeArgSlices(len(args))
	storeArgs(args, argVals, argRefs)
	res, ok := valueNew(v.ref, argRefs)
	runtime.KeepAlive(v)
	runtime.KeepAlive(argVals)
	if !ok {
		if vType := v.Type(); vType != TypeFunction { // 在此检查以避免成功情况下的开销
			panic(&ValueError{"Value.Invoke", vType})
		}
		panic(Error{makeValue(res)})
	}
	return makeValue(res)
}

// valueNew 使用 JavaScript 的 "new" 运算符，以值 v 作为构造函数和给定的参数。
//
// 使用 go:noescape 是安全的，因为 args 切片仅临时用于
// 收集 JavaScript 对象以进行构造函数执行。
//
//go:wasmimport gojs syscall/js.valueNew
//go:noescape
func valueNew(v ref, args []ref) (ref, bool)

func (v Value) isNumber() bool {
	return v.ref == valueZero.ref ||
		v.ref == valueNaN.ref ||
		(v.ref != valueUndefined.ref && (v.ref>>32)&nanHead != nanHead)
}

func (v Value) float(method string) float64 {
	if !v.isNumber() {
		panic(&ValueError{method, v.Type()})
	}
	if v.ref == valueZero.ref {
		return 0
	}
	return *(*float64)(unsafe.Pointer(&v.ref))
}

// Float 返回值 v 作为 float64。
// 如果 v 不是 JavaScript 数字，则触发 panic。
func (v Value) Float() float64 {
	return v.float("Value.Float")
}

// Int 返回值 v 截断到 int。
// 如果 v 不是 JavaScript 数字，则触发 panic。
func (v Value) Int() int {
	return int(v.float("Value.Int"))
}

// Bool 返回值 v 作为 bool。
// 如果 v 不是 JavaScript 布尔值，则触发 panic。
func (v Value) Bool() bool {
	switch v.ref {
	case valueTrue.ref:
		return true
	case valueFalse.ref:
		return false
	default:
		panic(&ValueError{"Value.Bool", v.Type()})
	}
}

// Truthy 返回值 v 的 JavaScript "truthiness"。在 JavaScript 中，
// false、0、""、null、undefined 和 NaN 是 "falsy"，其他一切都是
// "truthy"。参见 https://developer.mozilla.org/en-US/docs/Glossary/Truthy。
func (v Value) Truthy() bool {
	switch v.Type() {
	case TypeUndefined, TypeNull:
		return false
	case TypeBoolean:
		return v.Bool()
	case TypeNumber:
		return v.ref != valueNaN.ref && v.ref != valueZero.ref
	case TypeString:
		return v.String() != ""
	case TypeSymbol, TypeFunction, TypeObject:
		return true
	default:
		panic("bad type")
	}
}

// String 返回值 v 作为字符串。
// String 是特殊的，因为 Go 的 String 方法约定。与其他 getter 不同，
// 如果 v 的 Type 不是 TypeString，它不会触发 panic。反而，它返回形如 "<T>"
// 或 "<T: V>" 的字符串，其中 T 是 v 的类型，V 是 v 值的字符串表示。
func (v Value) String() string {
	switch v.Type() {
	case TypeString:
		return jsString(v)
	case TypeUndefined:
		return "<undefined>"
	case TypeNull:
		return "<null>"
	case TypeBoolean:
		return "<boolean: " + jsString(v) + ">"
	case TypeNumber:
		return "<number: " + jsString(v) + ">"
	case TypeSymbol:
		return "<symbol>"
	case TypeObject:
		return "<object>"
	case TypeFunction:
		return "<function>"
	default:
		panic("bad type")
	}
}

func jsString(v Value) string {
	str, length := valuePrepareString(v.ref)
	runtime.KeepAlive(v)
	b := make([]byte, length)
	valueLoadString(str, b)
	finalizeRef(str)
	return string(b)
}

//go:wasmimport gojs syscall/js.valuePrepareString
func valuePrepareString(v ref) (ref, int)

// valueLoadString 将位于 ref v 的字符串数据加载到字节切片 b 中。
//
// 使用 go:noescape 是安全的，因为字节切片仅用作存储
// 字符串数据的目标，并且不维护对其的引用。
//
//go:wasmimport gojs syscall/js.valueLoadString
//go:noescape
func valueLoadString(v ref, b []byte)

// InstanceOf 报告根据 JavaScript 的 instanceof 运算符 v 是否是类型 t 的实例。
func (v Value) InstanceOf(t Value) bool {
	r := valueInstanceOf(v.ref, t.ref)
	runtime.KeepAlive(v)
	runtime.KeepAlive(t)
	return r
}

//go:wasmimport gojs syscall/js.valueInstanceOf
func valueInstanceOf(v ref, t ref) bool

// ValueError 发生在对不支持它的 Value 调用 Value 方法时。
// 这些情况在每个方法的描述中有文档记载。
type ValueError struct {
	Method string
	Type   Type
}

func (e *ValueError) Error() string {
	return "syscall/js: call of " + e.Method + " on " + e.Type.String()
}

// CopyBytesToGo 将字节从 src 复制到 dst。
// 如果 src 不是 Uint8Array 或 Uint8ClampedArray，则触发 panic。
// 返回复制的字节数，这将是 src 和 dst 长度的最小值。
func CopyBytesToGo(dst []byte, src Value) int {
	n, ok := copyBytesToGo(dst, src.ref)
	runtime.KeepAlive(src)
	if !ok {
		panic("syscall/js: CopyBytesToGo: expected src to be a Uint8Array or Uint8ClampedArray")
	}
	return n
}

// copyBytesToGo 将字节从 src 复制到 dst。
//
// 使用 go:noescape 是安全的，因为 dst 字节切片仅用作 dst
// 复制缓冲区，并且不维护对其的引用。
//
//go:wasmimport gojs syscall/js.copyBytesToGo
//go:noescape
func copyBytesToGo(dst []byte, src ref) (int, bool)

// CopyBytesToJS 将字节从 src 复制到 dst。
// 如果 dst 不是 Uint8Array 或 Uint8ClampedArray，则触发 panic。
// 返回复制的字节数，这将是 src 和 dst 长度的最小值。
func CopyBytesToJS(dst Value, src []byte) int {
	n, ok := copyBytesToJS(dst.ref, src)
	runtime.KeepAlive(dst)
	if !ok {
		panic("syscall/js: CopyBytesToJS: expected dst to be a Uint8Array or Uint8ClampedArray")
	}
	return n
}

// copyBytesToJS 将字节从 src 复制到 dst。
//
// 使用 go:noescape 是安全的，因为 src 字节切片仅用作 src
// 复制缓冲区，并且不维护对其的引用。
//
//go:wasmimport gojs syscall/js.copyBytesToJS
//go:noescape
func copyBytesToJS(dst ref, src []byte) (int, bool)
